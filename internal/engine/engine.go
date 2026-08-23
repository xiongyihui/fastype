package engine

import (
	"time"

	"fastype/internal/keys"
)

// 引擎是 kb.py 处理逻辑的忠实移植：
//
//   - 层（layer）切换：hold 某键时切换到指定层，抬起回 0 层；
//   - 点按/长按（tap_hold）判定采用原版 is_tapping_key 的事件驱动算法：
//     不用定时器，而是根据后续按键事件的顺序在"可以确定时"才做出判定，
//     判定前所有事件进入队列并整体被抑制，保证输出顺序正确；
//   - 未被映射的键直接透传（比 Python 版全局抑制+重发的延迟更低）。
type BindKind uint8

const (
	BindCombo BindKind = iota // 映射为组合键，如 "shift+insert"
	BindTapHold               // 点按/长按
	BindLayerMods             // 按下立即生效：修饰键 + 切层（原 LayerMods）
)

type HoldKind uint8

const (
	HoldLayer HoldKind = iota
	HoldMods
)

type Hold struct {
	Kind  HoldKind
	Layer int
	Mods  []keys.VK
}

type Binding struct {
	Kind  BindKind
	Combo []keys.VK // BindCombo

	Tap  []keys.VK // BindTapHold
	Hold Hold

	Mods  []keys.VK // BindLayerMods
	Layer int  // BindLayerMods
}

// Effect 是需要通过 SendInput 合成注入的按键事件。
type Effect struct {
	VK  keys.VK
	Down bool
}

// Event 是一次（已规范化的）物理按键事件。
type Event struct {
	VK   keys.VK
	Down bool
	T    time.Time
}

type heldKind uint8

const (
	heldNative heldKind = iota // 原样透传的键，抬起时也透传
	heldCombo                  // 已合成按下的组合键
	heldLayer                  // 长按切层中
	heldMods                   // 长按修饰键中
	heldLayerMods              // LayerMods 按住中
)

type held struct {
	kind heldKind
	keys []keys.VK
}

type pendingDec struct {
	vk keys.VK
	t0 time.Time
	b  *Binding
}

type Engine struct {
	layers  []map[keys.VK]*Binding
	timeout time.Duration

	Layer   int
	held    map[keys.VK]held
	pending *pendingDec // 正在等待 tap/hold 判定的键
	queue   []Event     // 判定期间积压的事件（先进先出）

	Logf func(format string, args ...any)
}

func NewEngine(c *Compiled) *Engine {
	return &Engine{
		layers:  c.Layers,
		timeout: c.Timeout,
		held:    make(map[keys.VK]held),
	}
}

// Reload 热更新配置，返回需要立即补发的释放事件（避免残留按住的合成键）。
func (e *Engine) Reload(c *Compiled) []Effect {
	fx := e.ReleaseAll()
	e.layers = c.Layers
	e.timeout = c.Timeout
	return fx
}

// ReleaseAll 释放所有合成按下的键并复位状态。
func (e *Engine) ReleaseAll() []Effect {
	var fx []Effect
	for vk, h := range e.held {
		switch h.kind {
		case heldCombo, heldMods, heldLayerMods:
			fx = append(fx, releases(h.keys)...)
		}
		delete(e.held, vk)
	}
	e.Layer = 0
	e.pending = nil
	e.queue = nil
	return fx
}

// OnEvent 处理一个物理按键事件。
// suppressed 为 true 时钩子必须吞掉原始事件；fx 是需要合成注入的事件序列。
func (e *Engine) OnEvent(ev Event) (suppressed bool, fx []Effect) {
	// 判定期间的自动重复按下直接吞掉（对应原版 on_key 开头的过滤）
	if p := e.pending; p != nil && ev.Down && ev.VK == p.vk {
		e.logf("忽略 %s 的自动重复", keys.Name(ev.VK))
		return true, nil
	}
	e.queue = append(e.queue, ev)
	if len(e.queue) > 1024 {
		e.queue = e.queue[1:]
	}

	if e.pending != nil {
		if ok, tap := e.decide(); ok {
			out := e.applyPending(tap)
			out = append(out, e.drain()...)
			return true, out
		}
		return true, nil // 继续等待可判定的事件
	}

	// 实时路径：队列为空，刚压入的就是当前事件
	e.queue = e.queue[:len(e.queue)-1]
	fx, suppressed = e.step(ev, false)
	return suppressed, fx
}

// decide 是 is_tapping_key 的移植：根据队列头部事件判定 tap/hold。
func (e *Engine) decide() (ok, tap bool) {
	q, p := e.queue, e.pending
	if len(q) == 0 {
		return false, false
	}
	e0 := q[0]
	if e0.VK == p.vk {
		// 队列里出现本键只可能是抬起：间隔小于超时 → 点按
		return true, e0.T.Sub(p.t0) < e.timeout
	}
	if !e0.Down {
		return true, true // 别的键先抬起了 → 本键只是快速点按
	}
	if len(q) == 1 {
		return false, false // 再等一个事件
	}
	return true, q[1].VK == p.vk // 本键紧跟着抬起 → 点按，否则长按
}

func (e *Engine) applyPending(tap bool) []Effect {
	p := e.pending
	e.pending = nil
	b := p.b
	if tap {
		e.held[p.vk] = held{heldCombo, b.Tap}
		e.logf("%s 判定为点按 → %s", keys.Name(p.vk), comboString(b.Tap))
		return presses(b.Tap)
	}
	switch b.Hold.Kind {
	case HoldLayer:
		e.Layer = b.Hold.Layer
		e.held[p.vk] = held{kind: heldLayer}
		e.logf("%s 判定为长按 → 切换到层 %d", keys.Name(p.vk), b.Hold.Layer)
		return nil
	default:
		e.held[p.vk] = held{heldMods, b.Hold.Mods}
		e.logf("%s 判定为长按 → 按住 %s", keys.Name(p.vk), comboString(b.Hold.Mods))
		return presses(b.Hold.Mods)
	}
}

// drain 处理判定期间积压的队列；遇到新的 tap/hold 键则停下（保持剩余事件）。
func (e *Engine) drain() []Effect {
	var fx []Effect
	for len(e.queue) > 0 && e.pending == nil {
		ev := e.queue[0]
		e.queue = e.queue[1:]
		out, _ := e.step(ev, true)
		fx = append(fx, out...)
	}
	return fx
}

// step 处理单个事件。queued 表示该事件此前已被钩子抑制（需要合成补发透传键），
// false 表示实时事件（可以直接选择放行）。
func (e *Engine) step(ev Event, queued bool) (fx []Effect, suppressed bool) {
	if !ev.Down {
		h, ok := e.held[ev.VK]
		if !ok {
			if queued {
				// 与原版一致：未知来源的抬起也补发 release
				return []Effect{{VK: ev.VK, Down: false}}, true
			}
			return nil, false
		}
		delete(e.held, ev.VK)
		switch h.kind {
		case heldNative:
			if queued { // 当初的按下是透传的，但这次抬起被抑制了，需要补发
				return []Effect{{VK: ev.VK, Down: false}}, true
			}
			return nil, false
		case heldLayer:
			e.Layer = 0
			e.logf("层键 %s 抬起 → 回到层 0", keys.Name(ev.VK))
			return nil, true
		default: // heldCombo / heldMods / heldLayerMods
			out := releases(h.keys)
			if h.kind == heldLayerMods {
				e.Layer = 0
			}
			return out, true
		}
	}

	// 按下：先看是否是重复按下
	if h, ok := e.held[ev.VK]; ok {
		switch h.kind {
		case heldCombo:
			e.logf("%s 自动重复 → %s", keys.Name(ev.VK), comboString(h.keys))
			return presses(h.keys), true
		case heldNative:
			return nil, false
		default: // 长按中的层/修饰键：忽略重复
			return nil, true
		}
	}

	b := e.bindingAt(ev.VK)
	switch {
	case b == nil: // 当前层没有映射
		if queued { // 已被抑制，需要合成补发
			e.held[ev.VK] = held{heldCombo, []keys.VK{ev.VK}}
			return presses([]keys.VK{ev.VK}), true
		}
		e.held[ev.VK] = held{kind: heldNative}
		return nil, false
	case b.Kind == BindCombo:
		e.held[ev.VK] = held{heldCombo, b.Combo}
		e.logf("%s → %s", keys.Name(ev.VK), comboString(b.Combo))
		return presses(b.Combo), true
	case b.Kind == BindLayerMods:
		e.held[ev.VK] = held{heldLayerMods, b.Mods}
		e.Layer = b.Layer
		e.logf("%s → 按住 %s 并切换到层 %d", keys.Name(ev.VK), comboString(b.Mods), b.Layer)
		return presses(b.Mods), true
	default: // BindTapHold：进入判定
		e.pending = &pendingDec{vk: ev.VK, t0: ev.T, b: b}
		e.logf("%s 等待点按/长按判定…", keys.Name(ev.VK))
		return nil, true
	}
}

func (e *Engine) bindingAt(vk keys.VK) *Binding {
	if e.Layer < 0 || e.Layer >= len(e.layers) {
		return nil
	}
	return e.layers[e.Layer][vk]
}

func (e *Engine) logf(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

func presses(vks []keys.VK) []Effect {
	fx := make([]Effect, len(vks))
	for i, vk := range vks {
		fx[i] = Effect{VK: vk, Down: true}
	}
	return fx
}

func releases(vks []keys.VK) []Effect {
	fx := make([]Effect, len(vks))
	for i, vk := range vks {
		fx[len(vks)-1-i] = Effect{VK: vk, Down: false} // 反序释放，修饰键最后抬起
	}
	return fx
}

func comboString(vks []keys.VK) string {
	s := ""
	for i, vk := range vks {
		if i > 0 {
			s += "+"
		}
		s += keys.Name(vk)
	}
	return s
}

// Compiled 是配置编译产物，也是引擎的输入。
type Compiled struct {
	Layers  []map[keys.VK]*Binding
	Timeout time.Duration
}
