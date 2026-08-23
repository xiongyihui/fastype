//go:build darwin

package main

import (
	"os"
	"path/filepath"

	"fastype/internal/machook"
)

// osFallbackConfigDir 返回 macOS 下配置文件的兜底目录
// （~/Library/Application Support/fastype）。
func osFallbackConfigDir() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, "Library", "Application Support", "fastype"), true
}

// trayStateRefresh 由 Web UI 暂停切换时调用（可能位于 HTTP 线程）。
func trayStateRefresh() {
	refreshTrayUI()
}

// hookActive 报告键盘监听是否已生效（未授权等待期为 false）。
func hookActive() bool { return machook.Running() }

// hookTrusted 报告辅助功能授权当前是否对本进程生效（区分「未授权」与「已授权但安装失败」）。
func hookTrusted() bool { return machook.Trusted(false) }

// hookError 返回最近一次监听安装失败的原因。
func hookError() string { return machook.LastError() }
