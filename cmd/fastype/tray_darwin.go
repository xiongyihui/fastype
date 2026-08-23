//go:build darwin

package main

// 菜单栏图标（对应 Windows 版托盘）：打开配置 / 暂停恢复 / 开机自启 / 退出。

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"fastype/internal/machook"
	"fastype/internal/mactray"
)

type trayStrings struct {
	open, pause, resume, autoStart, quit string
	running, paused, waiting             string
	autoOnTitle, autoOnText              string
	autoOffTitle, autoOffText            string
}

// loadTrayStrings 跟随 macOS 系统语言（中文系统显示中文，其余显示英文）。
func loadTrayStrings() trayStrings {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), `"zh`) {
		return trayStrings{
			open: "打开配置页面...", pause: "暂停按键映射", resume: "恢复按键映射",
			autoStart: "登录时自动启动", quit: "退出",
			running: "运行中", paused: "已暂停", waiting: "等待授权",
			autoOnTitle: "开机自启已开启", autoOnText: "Fastype 将在登录 macOS 时自动启动",
			autoOffTitle: "开机自启已关闭", autoOffText: "Fastype 不再随系统启动",
		}
	}
	return trayStrings{
		open: "Open Config Page...", pause: "Pause Mapping", resume: "Resume Mapping",
		autoStart: "Start at Login", quit: "Exit",
		running: "Running", paused: "Paused", waiting: "Waiting for permission",
		autoOnTitle: "Auto start enabled", autoOnText: "Fastype will start automatically when you log in",
		autoOffTitle: "Auto start disabled", autoOffText: "Fastype will no longer start with macOS",
	}
}

var tr = loadTrayStrings()

var (
	hookDone   = make(chan struct{})
	quitOnce   sync.Once
	noTrayQuit = make(chan struct{})
)

func trayStateText() string {
	if !dryRun && !machook.Running() {
		return tr.waiting
	}
	if pausedFlag.Load() {
		return tr.paused
	}
	return tr.running
}

func pauseLabel() string {
	if pausedFlag.Load() {
		return tr.resume
	}
	return tr.pause
}

func setupTray() {
	mactray.OnCmd = handleTrayCommand
	mactray.DiagLogf = logf
}

func runTray() {
	err := mactray.Run("Fastype - "+trayStateText(),
		tr.open, pauseLabel(), tr.autoStart, tr.quit)
	if err != nil {
		// 菜单栏失败不应拖垮程序：钩子/配置界面继续可用
		logf("菜单栏图标初始化失败（忽略，继续运行）: %v", err)
		<-noTrayQuit
	}
}

// requestQuit 停止钩子并请求退出（可从主线程菜单回调或信号线程调用，幂等）。
func requestQuit() {
	quitOnce.Do(func() {
		close(hookDone) // 结束授权等待循环
		mactray.Stop()
		close(noTrayQuit)
	})
}

// handleTrayCommand 在 AppKit 主线程执行。
func handleTrayCommand(cmd int) {
	switch cmd {
	case mactray.CmdOpen:
		openConfigPage()
	case mactray.CmdTogglePause:
		pausedFlag.Store(!pausedFlag.Load())
		if pausedFlag.Load() {
			logf("按键映射已暂停")
		} else {
			logf("按键映射已恢复")
		}
		refreshTrayUI()
	case mactray.CmdQuit:
		requestQuit()
	case mactray.CmdAutoStart:
		enable := !autoStartEnabled()
		if err := autoStartSet(enable); err != nil {
			logf("设置开机自启失败: %v", err)
			showBalloon("开机自启", "设置失败: "+err.Error())
			return
		}
		mactray.SetAutoState(enable)
		if enable {
			logf("已开启开机自启（下次登录生效）")
			showBalloon(tr.autoOnTitle, tr.autoOnText)
		} else {
			logf("已关闭开机自启")
			showBalloon(tr.autoOffTitle, tr.autoOffText)
		}
	}
}

func refreshTrayUI() {
	mactray.SetPauseTitle(pauseLabel())
	mactray.SetTip("Fastype - " + trayStateText())
}

func openConfigPage() {
	exec.Command("open", configURL()).Start()
}

// showBalloon 用系统通知代替 Windows 托盘气泡。
func showBalloon(title, text string) {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`,
		strings.ReplaceAll(text, `"`, `\"`), strings.ReplaceAll(title, `"`, `\"`))
	exec.Command("osascript", "-e", script).Run()
}
