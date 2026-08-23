//go:build windows && amd64

package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
)

// mainTID 是跑钩子消息循环的主线程 ID，供其它线程投递消息（托盘/退出）。
var mainTID atomic.Uintptr

// osFallbackConfigDir 返回 Windows 下配置文件的兜底目录（%APPDATA%\fastype）。
func osFallbackConfigDir() (string, bool) {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "fastype"), true
	}
	return "", false
}

// trayStateRefresh 通知主线程刷新托盘提示（Web UI 切换暂停时调用）。
func trayStateRefresh() {
	if t := mainTID.Load(); t != 0 {
		pPostThreadMessageW.Call(t, wmAppTrayCmd, cmdRefreshTip, 0)
	}
}

// hookActive Windows 版钩子随主循环运行，进程存活即生效。
func hookActive() bool { return true }

// hookTrusted Windows 无需辅助功能授权。
func hookTrusted() bool { return true }

// hookError Windows 版无等待/重试路径。
func hookError() string { return "" }
