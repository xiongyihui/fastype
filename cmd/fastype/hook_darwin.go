//go:build darwin

package main

import (
	"time"

	"fastype/internal/engine"
	"fastype/internal/keylog"
	"fastype/internal/keys"
	"fastype/internal/machook"
)

// startHook 安装 CGEventTap 事件监听（对应 Windows 的 WH_KEYBOARD_LL 钩子 + 消息循环）。
func startHook() error {
	machook.DryRun = dryRun
	machook.Logf = logf
	return machook.Start(func(vk keys.VK, down bool) (bool, []engine.Effect) {
		t := time.Now()
		if pausedFlag.Load() {
			keylog.Record(false, vk, down, -1, t) // 暂停期间仍记录真实按键（无层信息）
			return false, nil
		}
		engineMu.Lock()
		suppressed, fx := eng.OnEvent(engine.Event{VK: vk, Down: down, T: t})
		layer := eng.Layer
		engineMu.Unlock()
		keylog.Record(false, vk, down, layer, t)
		return suppressed, fx
	})
}

// sendEffects 注入合成按键（与 Windows 版同名的注入入口，热更新/收尾路径使用）。
func sendEffects(fx []engine.Effect) { machook.PostEffects(fx) }
