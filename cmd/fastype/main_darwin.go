//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"fastype/internal/engine"
	"fastype/internal/machook"
	"fastype/internal/mactray"
	"fastype/internal/ui"
)

const permissionGuide = `fastype 需要「辅助功能」权限才能拦截与改写键盘事件：

    系统设置 → 隐私与安全性 → 辅助功能
    → 若列表里已有 Fastype：先选中按「−」删除，再点「＋」重新添加并打开开关
      （更换新版本后旧授权记录会失效，仅切换开关无效）
    → 若列表里没有：点「＋」添加 Fastype 并打开开关

已保持运行并等待授权——重新添加并打开开关后几秒内自动生效，无需重启。
也可以先用 --dry-run 免授权体验（只读模式）。`

func main() {
	// AppKit 的 NSStatusItem 必须在主 OS 线程上创建：
	// 立即锁定主协程，避免它在任何 cgo 调用/阻塞点被调度到其它线程。
	runtime.LockOSThread()

	cfg, compiled := loadOrCreateConfig(parseCLI())

	eng = engine.NewEngine(compiled)
	eng.Logf = logf
	appConfig = cfg

	if alreadyRunning(cfg.Port) {
		fatalf("fastype 已在运行（端口 %d）。请先从菜单栏图标退出旧实例，或访问 http://127.0.0.1:%d/", cfg.Port, cfg.Port)
	}

	actualPort, err := ui.Start(cfg.Port, apiHandler{})
	if err != nil {
		fatalf("%v", err)
	}
	cfg.Port = actualPort // 端口被占用时自动 +1 递增后的实际端口

	fmt.Printf("fastype %s 已启动  配置: %s  界面: http://127.0.0.1:%d/\n", version, cfgPath, actualPort)
	if dryRun {
		fmt.Println("【dry-run 模式】只读监听，不拦截/注入按键（无需辅助功能权限）")
	}
	fmt.Println("屏幕顶部菜单栏的 ⌨ 图标: 打开配置 / 暂停 / 退出；Ctrl+C 退出")

	// 键盘监听独立启动：缺少辅助功能权限时不退出，
	// 菜单栏图标照常出现，授权后自动生效。
	go hookLifecycle()

	setupTray()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logf("收到退出信号")
		requestQuit()
	}()

	runTray() // 阻塞在主线程 AppKit 事件循环，直到退出

	// 收尾：先停监听，再释放可能按住的合成键，避免退出后修饰键卡住
	machook.Stop()
	<-machook.Stopped()
	engineMu.Lock()
	fx := eng.ReleaseAll()
	engineMu.Unlock()
	if !dryRun {
		machook.PostEffects(fx)
	}
	ui.Stop()
	logf("fastype 已退出")
}

// hookLifecycle 反复尝试安装键盘监听：未授权时保持等待，授权后自动生效。
func hookLifecycle() {
	var guideOnce sync.Once
	for {
		if err := startHook(); err == nil {
			logf("CGEventTap 键盘监听已安装")
			mactray.SetTip("Fastype - " + trayStateText())
			return
		} else if !machook.Trusted(false) {
			guideOnce.Do(showPermissionGuide)
			logf("等待辅助功能授权…（开启开关后自动生效）")
		} else {
			logf("安装键盘监听失败: %v，2 秒后重试", err)
		}
		select {
		case <-hookDone:
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// showPermissionGuide 首次发现未授权时：终端提示 + 系统通知 + 打开授权设置页。
func showPermissionGuide() {
	fmt.Fprintln(os.Stderr, permissionGuide)
	if os.Getenv("FASTYPE_NO_PROMPT") != "1" {
		showBalloon("fastype 需要辅助功能权限",
			"系统设置 → 隐私与安全性 → 辅助功能：删除旧的 Fastype 条目后重新添加并打开开关")
		exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Start()
	}
}
