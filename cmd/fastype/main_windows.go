//go:build windows && amd64

package main

import (
	"fmt"
	"syscall"

	"fastype/internal/engine"
	"fastype/internal/ui"
)

func main() {
	cfg, compiled := loadOrCreateConfig(parseCLI())

	eng = engine.NewEngine(compiled)
	eng.Logf = logf
	appConfig = cfg

	if alreadyRunning(cfg.Port) {
		fatalf("fastype 已在运行（端口 %d）。请先从托盘菜单退出旧实例，或访问 http://127.0.0.1:%d/", cfg.Port, cfg.Port)
	}

	actualPort, err := ui.Start(cfg.Port, apiHandler{})
	if err != nil {
		fatalf("%v", err)
	}
	cfg.Port = actualPort // 端口被占用时自动 +1 递增后的实际端口

	fmt.Printf("fastype %s 已启动  配置: %s  界面: http://127.0.0.1:%d/\n", version, cfgPath, actualPort)
	if dryRun {
		fmt.Println("【dry-run 模式】只记录判定，不拦截按键")
	}
	fmt.Println("托盘图标右键: 打开配置 / 暂停 / 退出；Ctrl+C 退出")

	handler := syscall.NewCallback(consoleCtrlHandler)
	pSetConsoleCtrlHandler.Call(handler, 1)

	runHookLoop() // 阻塞直到退出

	// 收尾：释放可能按住的合成键，避免退出后修饰键卡住
	engineMu.Lock()
	fx := eng.ReleaseAll()
	engineMu.Unlock()
	if !dryRun {
		sendEffects(fx)
	}
	ui.Stop()
	logf("fastype 已退出")
}
