//go:build darwin

// 进程内全链路测试：离线构造 CGEvent 喂给事件回调（真实 tap 的安装与
// 拦截/注入需要辅助功能权限，无法在 CI 中验证，这里覆盖其余全部逻辑：
// 键码规范化、flagsChanged 边沿判定、caps 锁型键路径、与 engine 的完整联动）。

package machook

import (
	"testing"
	"time"

	"fastype/internal/config"
	"fastype/internal/engine"
	"fastype/internal/keys"
)

var (
	eng   *engine.Engine
	allFX []engine.Effect // 每次物理事件期间引擎产生的全部注入
	allSu []bool          // 每次引擎调用的抑制结果
)

func engineSim(vk keys.VK, down bool) (bool, []engine.Effect) {
	sup, fx := eng.OnEvent(engine.Event{VK: vk, Down: down, T: time.Now()})
	allFX = append(allFX, fx...)
	allSu = append(allSu, sup)
	return sup, fx
}

func setupEngine(t *testing.T, cfgJSON string) {
	t.Helper()
	cfg, err := config.ParseBytes([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatal(err)
	}
	eng = engine.NewEngine(compiled)
	handler = engineSim
	allFX, allSu = nil, nil
}

// sendFeed 发送一个物理事件并返回期间产生的注入/抑制记录。
func sendFeed(t *testing.T, send func() bool) ([]engine.Effect, []bool) {
	t.Helper()
	allFX, allSu = nil, nil
	swallowed := send()
	if DryRun && swallowed {
		t.Fatal("dry-run 模式不应吞事件")
	}
	return allFX, allSu
}

const macCfg = `{
  "tap_timeout_ms": 500,
  "layers": [
    {"keys": {
      "d": {"type":"tap_hold","tap":["d"],"hold":{"type":"layer","layer":1}},
      ";": {"type":"tap_hold","tap":[";"],"hold":{"type":"mods","mods":["command"]}},
      "caps lock": "esc"
    }},
    {"keys": {
      "h": "left", "j": "down", "k": "up", "l": "right",
      ";": "backspace", "'": "esc"
    }}
  ]
}`

func vk(name string) keys.VK {
	v, ok := keys.VkOf(name)
	if !ok {
		panic("unknown " + name)
	}
	return v
}

func fxIs(fx []engine.Effect, want ...engine.Effect) bool {
	if len(fx) != len(want) {
		return false
	}
	for i := range fx {
		if fx[i] != want[i] {
			return false
		}
	}
	return true
}

func TestPipelineTapHoldLayer(t *testing.T) {
	setupEngine(t, macCfg)
	DryRun = true
	defer func() { DryRun = false; eng = nil }()

	// 按住 d → 按 h → 抬 h：h 的抬起触发"长按 d"判定，h 映射为 left
	sendFeed(t, func() bool { return testSendKey(0x02, true) }) // d down → 等待判定
	if len(allFX) != 0 {
		t.Fatalf("d 按下后应等待判定，实际 fx=%v", allFX)
	}
	sendFeed(t, func() bool { return testSendKey(0x04, true) }) // h down → 仍需再等一个事件
	if eng.Layer != 0 {
		t.Fatalf("判定完成前不应切层，实际层 %d", eng.Layer)
	}
	fx, su := sendFeed(t, func() bool { return testSendKey(0x04, false) }) // h up → 触发判定
	if eng.Layer != 1 {
		t.Fatalf("应切到层 1，实际层 %d", eng.Layer)
	}
	if !fxIs(fx, engine.Effect{VK: vk("left"), Down: true}, engine.Effect{VK: vk("left"), Down: false}) {
		t.Fatalf("层 1 的 h 应注入 left 按下+抬起，实际 %v", fx)
	}
	if !su[0] {
		t.Fatalf("层 1 的 h 应被抑制，实际 %v", su)
	}
	sendFeed(t, func() bool { return testSendKey(0x02, false) }) // d up → 回层 0
	if eng.Layer != 0 {
		t.Fatalf("d 抬起后应回到层 0，实际层 %d", eng.Layer)
	}
}

func TestPipelineTapHoldMods(t *testing.T) {
	setupEngine(t, macCfg)
	DryRun = true
	defer func() { DryRun = false; eng = nil }()

	// 快速点按 ; → 输出 ;
	sendFeed(t, func() bool { return testSendKey(0x29, true) })
	fx, _ := sendFeed(t, func() bool { return testSendKey(0x29, false) })
	if !fxIs(fx, engine.Effect{VK: vk(";"), Down: true}, engine.Effect{VK: vk(";"), Down: false}) {
		t.Fatalf("点按 ; 应输出 ; 按下+抬起，实际 %v", fx)
	}

	// 长按 ; 期间按 c（层 0 无映射）→ ; 判定为 mods：⌘ 按下，c 透传
	sendFeed(t, func() bool { return testSendKey(0x29, true) })          // ; down
	sendFeed(t, func() bool { return testSendKey(0x08, true) })          // c down → 积压
	fx, _ = sendFeed(t, func() bool { return testSendKey(0x08, false) }) // c up → 触发判定
	if !fxIs(fx,
		engine.Effect{VK: vk("windows"), Down: true},
		engine.Effect{VK: vk("c"), Down: true},
		engine.Effect{VK: vk("c"), Down: false}) {
		t.Fatalf("长按 ; 应注入 ⌘↓ 并透传 c，实际 %v", fx)
	}
	fx, _ = sendFeed(t, func() bool { return testSendKey(0x29, false) }) // ; up
	if !fxIs(fx, engine.Effect{VK: vk("windows"), Down: false}) {
		t.Fatalf("抬 ; 应释放 ⌘，实际 %v", fx)
	}
}

func TestCapsLockAsTapKey(t *testing.T) {
	setupEngine(t, macCfg)
	DryRun = false // 验证拦截模式下锁型键的一次性点按路径
	defer func() { DryRun = true; eng = nil }()

	var swallowed bool
	fx, _ := sendFeed(t, func() bool {
		swallowed = testSendFlags(0x39, capsMask)
		return swallowed
	})
	if !swallowed {
		t.Fatal("caps 有映射时应吞掉物理事件")
	}
	if !fxIs(fx, engine.Effect{VK: vk("esc"), Down: true}, engine.Effect{VK: vk("esc"), Down: false}) {
		t.Fatalf("caps 应立即注入 esc 按下+抬起，实际 %v", fx)
	}
}

func TestModifierPassthroughAndNormalize(t *testing.T) {
	setupEngine(t, macCfg)
	DryRun = true
	defer func() { DryRun = false; eng = nil }()

	// 右 ⌘ (0x36) 物理按下：层 0 无映射 → 放行，缓存记 command 位
	sendFeed(t, func() bool { return testSendFlags(0x36, cmdMask) })
	if currentFlags()&cmdMask == 0 {
		t.Fatal("右 ⌘ 放行后缓存应包含 command 位")
	}
	// 左 ⌘ 抬起（左右键码合并为同一 VK）
	sendFeed(t, func() bool { return testSendFlags(0x37, 0) })
	if currentFlags()&cmdMask != 0 {
		t.Fatal("⌘ 抬起后缓存不应包含 command 位")
	}
}

func TestNormalizeKeycodeTable(t *testing.T) {
	cases := []struct {
		kc   uint16
		name string
	}{
		{0x00, "a"}, {0x02, "d"}, {0x04, "h"}, {0x10, "y"}, {0x11, "t"},
		{0x15, "4"}, {0x17, "5"}, {0x16, "6"}, {0x1D, "0"},
		{0x29, ";"}, {0x27, "'"}, {0x32, "`"}, {0x2A, "\\"},
		{0x33, "backspace"}, {0x35, "esc"}, {0x39, "caps lock"},
		{0x37, "windows"}, {0x36, "windows"}, {0x3A, "alt"}, {0x3D, "alt"},
		{0x3B, "ctrl"}, {0x3E, "ctrl"},
		{0x73, "home"}, {0x79, "page down"}, {0x7E, "up"}, {0x7B, "left"},
		{0x72, "insert"}, {0x75, "delete"},
		{0x7A, "f1"}, {0x6F, "f12"},
		{0x52, "keypad 0"}, {0x4B, "keypad /"},
	}
	for _, c := range cases {
		if got := keys.Name(normalizeKeycode(c.kc)); got != c.name {
			t.Errorf("keycode 0x%02X 应为 %q，实际 %q", c.kc, c.name, got)
		}
	}
	// fn / 音量键等无法映射
	if normalizeKeycode(0x3F) != 0 || normalizeKeycode(0x48) != 0 {
		t.Error("fn / 音量键不应产生映射")
	}
	// 反查注入表：每个规范名都应有 Mac 键码
	for _, name := range []string{"a", "left", "enter", "windows", "caps lock", "insert", "keypad 9"} {
		if _, ok := keycodeOfVK(vk(name)); !ok {
			t.Errorf("键 %q 缺少注入键码", name)
		}
	}
}
