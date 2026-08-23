//go:build windows && amd64

package main

import "testing"

// 往返测试：开启 → 查询已开启 → 关闭 → 查询已关闭（净效果为零）。
func TestAutoStartRoundtrip(t *testing.T) {
	if err := autoStartSet(true); err != nil {
		t.Fatalf("开启失败: %v", err)
	}
	if !autoStartEnabled() {
		t.Fatal("开启后查询应为已开启")
	}
	if err := autoStartSet(false); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if autoStartEnabled() {
		t.Fatal("关闭后查询应为未开启")
	}
}
