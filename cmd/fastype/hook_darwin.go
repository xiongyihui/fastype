//go:build darwin

package main

import (
	"time"

	"fastype/internal/engine"
	"fastype/internal/keys"
	"fastype/internal/machook"
)

// startHook 安装 CGEventTap 事件监听（对应 Windows 的 WH_KEYBOARD_LL 钩子 + 消息循环）。
func startHook() error {
	machook.DryRun = dryRun
	machook.Logf = logf
	return machook.Start(func(vk keys.VK, down bool) (bool, []engine.Effect) {
		if pausedFlag.Load() {
			return false, nil
		}
		engineMu.Lock()
		defer engineMu.Unlock()
		return eng.OnEvent(engine.Event{VK: vk, Down: down, T: time.Now()})
	})
}

// sendEffects 注入合成按键（与 Windows 版同名的注入入口，热更新/收尾路径使用）。
func sendEffects(fx []engine.Effect) { machook.PostEffects(fx) }
