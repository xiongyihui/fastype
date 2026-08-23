package engine_test

import (
	"fmt"
	"testing"
	"time"

	"fastype/internal/engine"
	"fastype/internal/keys"
)

func vk(name string) keys.VK {
	v, ok := keys.VkOf(name)
	if !ok {
		panic("未知键名: " + name)
	}
	return v
}

func combo(names ...string) []keys.VK {
	out := make([]keys.VK, len(names))
	for i, n := range names {
		out[i] = vk(n)
	}
	return out
}

func tapHold(tap string, hold engine.Hold) *engine.Binding {
	return &engine.Binding{Kind: engine.BindTapHold, Tap: combo(tap), Hold: hold}
}

func keyBind(names ...string) *engine.Binding {
	return &engine.Binding{Kind: engine.BindCombo, Combo: combo(names...)}
}

// testEngine 构造与默认配置等价的引擎（不经 JSON，直接给编译后的结构）：
// 层0: d=tap_hold(切层1), ;=tap_hold(ctrl), '=tap_hold(alt), caps lock=tap_hold(ctrl)
// 层1: hjkl=方向键, u/n=翻页, y/m=home/end, p=shift+insert, ;=backspace, '=esc
func testEngine(t *testing.T) *engine.Engine {
	t.Helper()
	l0 := map[keys.VK]*engine.Binding{
		vk("d"):         tapHold("d", engine.Hold{Kind: engine.HoldLayer, Layer: 1}),
		vk(";"):         tapHold(";", engine.Hold{Kind: engine.HoldMods, Mods: combo("ctrl")}),
		vk("'"):         tapHold("'", engine.Hold{Kind: engine.HoldMods, Mods: combo("alt")}),
		vk("caps lock"): tapHold("caps lock", engine.Hold{Kind: engine.HoldMods, Mods: combo("ctrl")}),
	}
	l1 := map[keys.VK]*engine.Binding{
		vk("h"): keyBind("left"), vk("j"): keyBind("down"), vk("k"): keyBind("up"), vk("l"): keyBind("right"),
		vk("u"): keyBind("page up"), vk("n"): keyBind("page down"), vk("y"): keyBind("home"), vk("m"): keyBind("end"),
		vk("p"): keyBind("shift", "insert"), vk(";"): keyBind("backspace"), vk("'"): keyBind("esc"),
	}
	return engine.NewEngine(&engine.Compiled{
		Layers:  []map[keys.VK]*engine.Binding{l0, l1},
		Timeout: 500 * time.Millisecond,
	})
}

type ev struct {
	name string
	down bool
	ms   int // 相对基准时间的毫秒偏移
}

var base = time.Now()

// feed 依次注入事件，收集全部合成效果，返回 "d↓"、"ctrl↑" 形式的切片。
func feed(e *engine.Engine, events ...ev) []string {
	var out []string
	for _, x := range events {
		_, fx := e.OnEvent(engine.Event{VK: vk(x.name), Down: x.down, T: base.Add(time.Duration(x.ms) * time.Millisecond)})
		for _, f := range fx {
			out = append(out, keys.Name(f.VK)+arrow(f.Down))
		}
	}
	return out
}

func arrow(down bool) string {
	if down {
		return "↓"
	}
	return "↑"
}

func assertEq(t *testing.T, got, want []string, msg string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s\n  实际: %v\n  期望: %v", msg, got, want)
	}
}

func TestPlainTap(t *testing.T) {
	e := testEngine(t)
	// 快速点按 d（120ms）→ 输出 d
	assertEq(t, feed(e, ev{"d", true, 0}, ev{"d", false, 120}),
		[]string{"d↓", "d↑"}, "快速点按 d 应原样输出")
}

func TestPlainTapModsKey(t *testing.T) {
	e := testEngine(t)
	// 快速点按 ; → 输出 ;
	assertEq(t, feed(e, ev{";", true, 0}, ev{";", false, 90}),
		[]string{";↓", ";↑"}, "快速点按 ; 应原样输出")
}

func TestHoldLayerRemap(t *testing.T) {
	e := testEngine(t)
	// 按住 d → 按 h → 抬 h → 抬 d：h 应映射为 left
	assertEq(t, feed(e,
		ev{"d", true, 0},
		ev{"h", true, 100},
		ev{"h", false, 200},
		ev{"d", false, 300}),
		[]string{"left↓", "left↑"}, "长按 d 期间 h 应映射为 left")
	if e.Layer != 0 {
		t.Fatalf("层未复位: %d", e.Layer)
	}
}

func TestHoldModsRemap(t *testing.T) {
	e := testEngine(t)
	// 按住 ; → 按 h（层0未映射）→ 长按判定成立，ctrl 按下，h 补发
	got := feed(e,
		ev{";", true, 0},
		ev{"h", true, 60},
		ev{"h", false, 200},
		ev{";", false, 250})
	assertEq(t, got, []string{"ctrl↓", "h↓", "h↑", "ctrl↑"}, "长按 ; 应按住 ctrl")
}

func TestQuickRollIsTap(t *testing.T) {
	e := testEngine(t)
	// d↓ h↓ d↑（快速轮转）：d 判定为点按，h 不受层影响
	got := feed(e,
		ev{"d", true, 0},
		ev{"h", true, 30},
		ev{"d", false, 60},
		ev{"h", false, 120})
	assertEq(t, got, []string{"d↓", "h↓", "d↑", "h↑"}, "快速轮转时 d 应判定为点按")
}

func TestLongHoldAloneIsNoop(t *testing.T) {
	e := testEngine(t)
	// 单独长按 d 600ms 后抬起：无输出（超时后判定为长按，切层又立即切回）
	assertEq(t, feed(e, ev{"d", true, 0}, ev{"d", false, 600}),
		nil, "单独长按超时不应有输出")
}

func TestAutoRepeatIgnoredWhilePending(t *testing.T) {
	e := testEngine(t)
	// d 按住等待判定，自动重复的 d↓ 应被吞掉
	got := feed(e,
		ev{"d", true, 0},
		ev{"d", true, 520}, // 自动重复（超过 500ms）
		ev{"d", false, 600})
	assertEq(t, got, nil, "判定期间的自动重复应被忽略")
}

func TestLayerRemapRepeat(t *testing.T) {
	e := testEngine(t)
	// 长按 d 后 h 自动重复 → 重复输出 left
	got := feed(e,
		ev{"d", true, 0},
		ev{"h", true, 100},
		ev{"h", true, 130}, // h 的自动重复
		ev{"h", false, 400},
		ev{"d", false, 450})
	assertEq(t, got, []string{"left↓", "left↓", "left↑"}, "层内重复按下应重复输出")
}

func TestHoldLayerRepeatIgnored(t *testing.T) {
	e := testEngine(t)
	// 长按 d 期间 d 自身重复：不应产生输出也不应破坏层状态
	got := feed(e,
		ev{"d", true, 0},
		ev{"h", true, 100}, ev{"h", false, 150}, // 触发 hold 判定
		ev{"d", true, 300},                      // d 的自动重复
		ev{"j", true, 320}, ev{"j", false, 360}, // 层仍应生效
		ev{"d", false, 400})
	assertEq(t, got, []string{"left↓", "left↑", "down↓", "down↑"}, "层键重复不应破坏切层状态")
}

func TestPassthroughUnmapped(t *testing.T) {
	e := testEngine(t)
	sup, fx := e.OnEvent(engine.Event{VK: vk("a"), Down: true, T: base})
	if sup || len(fx) != 0 {
		t.Fatalf("未映射的键应透传: sup=%v fx=%v", sup, fx)
	}
	sup, fx = e.OnEvent(engine.Event{VK: vk("a"), Down: false, T: base})
	if sup || len(fx) != 0 {
		t.Fatalf("未映射键的抬起应透传: sup=%v fx=%v", sup, fx)
	}
}

func TestComboOutput(t *testing.T) {
	e := testEngine(t)
	// 层1 的 p → shift+insert
	got := feed(e,
		ev{"d", true, 0},
		ev{"p", true, 100}, ev{"p", false, 150},
		ev{"d", false, 200})
	assertEq(t, got, []string{"shift↓", "insert↓", "insert↑", "shift↑"}, "组合键按下/反序释放")
}

func TestOtherKeyReleaseFirstMeansTap(t *testing.T) {
	e := testEngine(t)
	// d↓ 期间，之前按下的 f 抬起 → 判定 d 为点按
	got := feed(e,
		ev{"f", true, -200}, // f 先按下（透传）
		ev{"d", true, 0},
		ev{"f", false, 80}, // f 抬起 → d 判定点按；f 的抬起在队列里，被抑制后补发
		ev{"d", false, 120})
	assertEq(t, got, []string{"d↓", "f↑", "d↑"}, "其它键先抬起时应判定为点按")
}

func TestQueuedPassthroughSynthesized(t *testing.T) {
	e := testEngine(t)
	// d 判定期间进入的 f：抑制后需补发（与原版 keyboard 库的全局抑制+重发一致）
	got := feed(e,
		ev{"d", true, 0},
		ev{"f", true, 50},  // 进入队列
		ev{"f", false, 80}, // 触发判定：f↓ f↑ → hold? q[1]=f↑≠d → hold！
		ev{"d", false, 400})
	// hold 判定后：f 在层1 无映射 → 合成补发 f↓ f↑
	assertEq(t, got, []string{"f↓", "f↑"}, "队列里的透传键需要合成补发")
}

func TestLayerModsImmediate(t *testing.T) {
	l0 := map[keys.VK]*engine.Binding{
		vk("x"): {Kind: engine.BindLayerMods, Mods: combo("ctrl"), Layer: 1},
	}
	l1 := map[keys.VK]*engine.Binding{vk("h"): keyBind("left")}
	e := engine.NewEngine(&engine.Compiled{
		Layers:  []map[keys.VK]*engine.Binding{l0, l1},
		Timeout: 500 * time.Millisecond,
	})
	got := feed(e,
		ev{"x", true, 0},
		ev{"h", true, 50}, ev{"h", false, 80},
		ev{"x", false, 100})
	assertEq(t, got, []string{"ctrl↓", "left↓", "left↑", "ctrl↑"}, "layer_mods 应立即生效")
	if e.Layer != 0 {
		t.Fatalf("层未复位: %d", e.Layer)
	}
}

func TestCapsLockTapHold(t *testing.T) {
	e := testEngine(t)
	got := feed(e, ev{"caps lock", true, 0}, ev{"caps lock", false, 100})
	assertEq(t, got, []string{"caps lock↓", "caps lock↑"}, "caps lock 快速点按应原样输出")
}
