//go:build darwin

package main

// 开机自启：写/删当前用户的 LaunchAgent plist（无需管理员），
// 登录时由 launchd 拉起，与 Windows 的 HKCU Run 注册表键等效。

import (
	"fmt"
	"os"
	"path/filepath"
)

const autoStartLabel = "com.xiongyihui.fastype"

func autoStartPlist() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", autoStartLabel+".plist"), nil
}

// autoStartEnabled 查询开机自启是否已开启。
func autoStartEnabled() bool {
	p, err := autoStartPlist()
	return err == nil && fileExists(p)
}

// autoStartSet 开启/关闭开机自启，注册当前正在运行的这个可执行文件。
// 写入的 LaunchAgent 从下一次登录开始生效。
func autoStartSet(enable bool) error {
	p, err := autoStartPlist()
	if err != nil {
		return err
	}
	if !enable {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 LaunchAgent 失败: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real // Homebrew bin 目录里通常是符号链接
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, autoStartLabel, exe)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}
	if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("写入 LaunchAgent 失败: %w", err)
	}
	return nil
}
