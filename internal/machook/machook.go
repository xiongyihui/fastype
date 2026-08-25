//go:build darwin

// Package machook 通过 macOS CGEventTap 实现全局按键拦截与合成注入，
// 对应 Windows 版的 WH_KEYBOARD_LL 钩子 + SendInput。
//
// 与 Windows 的差异：
//   - 需要用户授予「辅助功能」权限（相当于 Windows 钩子无需提权即可用）；
//   - 修饰键不走 keyDown/keyUp，而是 flagsChanged 事件：按下=置位、抬起=清位，
//     需要自行维护两套状态——物理状态（驱动按键判定）与下游状态（合成事件携带的
//     修饰键标志，被抑制的键不计入）；
//   - caps lock 是锁型键，只有"按下"没有"抬起"：物理事件一律视为一次点按，
//     不支持长按判定（注入时表现为翻转大写锁定标志）。
package machook

/*
#include <ApplicationServices/ApplicationServices.h>

extern CGEventRef goTapTrampoline(CGEventTapProxy proxy, CGEventType type,
                                  CGEventRef event, void *userInfo);
*/
import "C"

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"fastype/internal/engine"
	"fastype/internal/keylog"
	"fastype/internal/keys"
)

// injectMarker 写进注入事件的 kCGEventSourceUserData 字段，回调据此放行自己。
const injectMarker = int64(0x46415354595045) // "FASTYPE"

// Handler 处理一个（已规范化的）物理按键事件。
// suppress 为 true 时吞掉原始事件；fx 是需要合成注入的事件序列。
type Handler func(vk keys.VK, down bool) (suppress bool, fx []engine.Effect)

var (
	handler Handler

	// DryRun 只记录判定，不真正拦截/注入。
	DryRun bool
	// Logf 可选的调试日志回调。
	Logf func(format string, args ...any)

	started atomic.Bool

	errMu   sync.Mutex
	lastErr string // 最近一次 Start 失败原因（诊断用）

	srcMu   sync.Mutex
	src     C.CGEventSourceRef
	tapMu   sync.Mutex
	tap     C.CFMachPortRef
	rlMu    sync.Mutex
	runloop C.CFRunLoopRef
	// done 初始为已关闭的通道：从未启动时 Stopped() 不阻塞调用方。
	done chan struct{} = func() chan struct{} {
		c := make(chan struct{})
		close(c)
		return c
	}()

	flagsMu sync.Mutex
	physBit uint64 // 各修饰键的物理按下状态（驱动 down/up 判定）
	curFlag uint64 // 下游应用感知的修饰键标志（合成事件携带；被抑制的键不计入）
)

// Running 报告事件监听是否已成功安装（供状态接口展示「等待授权」等）。
func Running() bool { return started.Load() }

// LastError 返回最近一次监听安装失败的原因（成功后为空）。
func LastError() string {
	errMu.Lock()
	defer errMu.Unlock()
	return lastErr
}

func setLastErr(s string) {
	errMu.Lock()
	lastErr = s
	errMu.Unlock()
}

// Start 安装事件监听并启动事件循环（独立锁定线程）。
// DryRun 为 true 时使用只读监听——不需要辅助功能权限，也不拦截/注入任何事件。
// 拦截模式因缺少辅助功能权限失败时，可在授权后再次调用（内部幂等）。
func Start(h Handler) error {
	if !started.CompareAndSwap(false, true) {
		return nil
	}
	reset := func(err string) {
		started.Store(false)
		setLastErr(err)
	}
	handler = h
	srcMu.Lock()
	src = cSourceCreate()
	srcMu.Unlock()
	if cSrcIsNull(src) {
		reset("创建 CGEventSource 失败")
		return errors.New("创建 CGEventSource 失败")
	}
	t := cTapCreate(DryRun)
	if cTapIsNull(t) {
		reset("创建 CGEventTap 失败（辅助功能授权未生效）")
		return errors.New("创建 CGEventTap 失败")
	}
	tapMu.Lock()
	tap = t
	tapMu.Unlock()
	setLastErr("")

	flagsMu.Lock()
	curFlag = cSourceFlagsHID()
	flagsMu.Unlock()

	ready := make(chan struct{})
	done = make(chan struct{})
	go func() {
		runtime.LockOSThread()
		rl := cRunLoopCurrent()
		cRunLoopAddTap(t, rl)
		rlMu.Lock()
		runloop = rl
		rlMu.Unlock()
		close(ready)
		cRunLoopRun()
		close(done)
	}()
	<-ready
	return nil
}

// Stop 结束事件循环。
func Stop() {
	rlMu.Lock()
	rl := runloop
	rlMu.Unlock()
	if !cRlIsNull(rl) {
		cRunLoopStop(rl)
	}
}

// Stopped 在事件循环退出后关闭。
func Stopped() <-chan struct{} { return done }

// Trusted 报告辅助功能权限状态；prompt 为 true 时弹出系统授权引导对话框。
func Trusted(prompt bool) bool { return cAxTrusted(prompt) }

// PostEffects 合成注入一组按键事件（退出时释放残留修饰键等场景）。
func PostEffects(fx []engine.Effect) { postEffects(fx) }

//export goTapTrampoline
func goTapTrampoline(proxy C.CGEventTapProxy, etype C.CGEventType, ev C.CGEventRef, userInfo unsafe.Pointer) C.CGEventRef {
	if etype == C.kCGEventTapDisabledByTimeout || etype == C.kCGEventTapDisabledByUserInput {
		tapMu.Lock()
		t := tap
		tapMu.Unlock()
		if !cTapIsNull(t) {
			cTapEnable(t)
		}
		if Logf != nil {
			Logf("事件监听被系统停用，已重新启用")
		}
		return ev
	}
	if cEvIsNull(ev) {
		return cEvNull()
	}
	if cEventField(ev, uint32(C.kCGEventSourceUserData)) == injectMarker {
		// 自己注入的事件：放行，并同步修饰键标志缓存
		if etype == C.kCGEventFlagsChanged {
			setFlags(cEventFlags(ev))
		}
		return ev
	}
	if handler == nil {
		return ev
	}
	switch etype {
	case C.kCGEventKeyDown, C.kCGEventKeyUp:
		return handleKey(ev, etype == C.kCGEventKeyDown)
	case C.kCGEventFlagsChanged:
		return handleFlagsChanged(ev)
	}
	return ev
}

func handleKey(ev C.CGEventRef, down bool) C.CGEventRef {
	vk := normalizeKeycode(uint16(cEventField(ev, uint32(C.kCGKeyboardEventKeycode))))
	if vk == 0 {
		return ev
	}
	suppress, fx := handler(vk, down)
	postEffects(fx)
	if suppress && !DryRun {
		return cEvNull()
	}
	return ev
}

func handleFlagsChanged(ev C.CGEventRef) C.CGEventRef {
	vk := normalizeKeycode(uint16(cEventField(ev, uint32(C.kCGKeyboardEventKeycode))))
	evFlags := cEventFlags(ev)
	if vk == 0 {
		// fn 等无法映射的修饰键：放行，仅刷新非托管标志位
		setForeignFlags(evFlags)
		return ev
	}
	mask := modFlagMask(vk)
	if mask == 0 { // 表内非修饰键 keycode（不应出现在 flagsChanged），放行
		setForeignFlags(evFlags)
		return ev
	}

	newSet := evFlags&mask != 0
	flagsMu.Lock()
	down := newSet && physBit&mask == 0
	up := !newSet && physBit&mask != 0
	if newSet {
		physBit |= mask
	} else {
		physBit &^= mask
	}
	flagsMu.Unlock()

	if vk == capsVK {
		// 锁型键：每次物理事件都是一次"按下"，系统不存在抬起事件
		suppress, fx := handler(vk, true)
		postEffects(fx)
		if suppress && !DryRun {
			// 吞掉物理事件后立刻补一次合成抬起，让引擎完成点按判定
			_, fx2 := handler(vk, false)
			postEffects(fx2)
			return cEvNull()
		}
		setFlagBit(mask, newSet) // 放行时下游状态与事件一致
		return ev
	}

	if !down && !up {
		return ev // 状态未变化（重复的 flagsChanged）
	}
	suppress, fx := handler(vk, down)
	postEffects(fx)
	if suppress && !DryRun {
		return cEvNull() // 下游没看到这次变化，curFlag 保持旧值
	}
	setFlagBit(mask, newSet)
	return ev
}

func postEffects(fx []engine.Effect) {
	if DryRun || len(fx) == 0 {
		return
	}
	srcMu.Lock()
	s := src
	srcMu.Unlock()
	if cSrcIsNull(s) {
		return
	}
	now := time.Now()
	for _, f := range fx {
		keylog.Record(true, f.VK, f.Down, -1, now)
		kc, ok := keycodeOfVK(f.VK)
		if !ok {
			if Logf != nil {
				Logf("警告: %s 在 macOS 上无对应物理键，已跳过注入", keys.Name(f.VK))
			}
			continue
		}
		var fl uint64
		if mask := modFlagMask(f.VK); mask != 0 {
			if f.VK == capsVK {
				if !f.Down {
					continue // 锁型键没有抬起语义
				}
				fl = toggleFlagBit(mask) // 注入 caps = 翻转大写锁定
			} else {
				fl = applyFlagBit(mask, f.Down)
			}
			ev := cEventKeyboard(s, kc, f.Down || f.VK == capsVK)
			cEventSetFlags(ev, fl)
			cEventSetField(ev, uint32(C.kCGEventSourceUserData), injectMarker)
			cEventPost(ev)
			cEventRelease(ev)
			continue
		}
		fl = currentFlags()
		if strings.HasPrefix(keys.Name(f.VK), "keypad") {
			fl |= uint64(C.kCGEventFlagMaskNumericPad)
		}
		ev := cEventKeyboard(s, kc, f.Down)
		cEventSetFlags(ev, fl)
		cEventSetField(ev, uint32(C.kCGEventSourceUserData), injectMarker)
		cEventPost(ev)
		cEventRelease(ev)
	}
}

// ---------- 修饰键标志缓存 ----------

func setFlags(f uint64) {
	flagsMu.Lock()
	curFlag = f
	flagsMu.Unlock()
}

// setForeignFlags 用事件标志刷新非托管位（fn、numlock 等），保留被抑制的托管位。
func setForeignFlags(evFlags uint64) {
	flagsMu.Lock()
	curFlag = curFlag&managedMasks() | evFlags&^managedMasks()
	flagsMu.Unlock()
}

func setFlagBit(mask uint64, on bool) {
	flagsMu.Lock()
	if on {
		curFlag |= mask
	} else {
		curFlag &^= mask
	}
	flagsMu.Unlock()
}

func applyFlagBit(mask uint64, on bool) uint64 {
	flagsMu.Lock()
	if on {
		curFlag |= mask
	} else {
		curFlag &^= mask
	}
	f := curFlag
	flagsMu.Unlock()
	return f
}

func toggleFlagBit(mask uint64) uint64 {
	flagsMu.Lock()
	curFlag ^= mask
	f := curFlag
	flagsMu.Unlock()
	return f
}

func currentFlags() uint64 {
	flagsMu.Lock()
	f := curFlag
	flagsMu.Unlock()
	return f
}

func managedMasks() uint64 {
	return shiftMask | ctrlMask | altMask | cmdMask | capsMask
}

// ---------- Mac 虚拟键码 ↔ 规范键名（内部沿用 Windows VK 作为共同表示） ----------
// 键码值来自 HIToolbox/Events.h，按 ANSI 布局取位。

var keyPairs = []struct {
	kc   uint16
	name string
}{
	{0x00, "a"}, {0x01, "s"}, {0x02, "d"}, {0x03, "f"}, {0x04, "h"}, {0x05, "g"},
	{0x06, "z"}, {0x07, "x"}, {0x08, "c"}, {0x09, "v"}, {0x0B, "b"},
	{0x0C, "q"}, {0x0D, "w"}, {0x0E, "e"}, {0x0F, "r"}, {0x10, "y"}, {0x11, "t"},
	{0x12, "1"}, {0x13, "2"}, {0x14, "3"}, {0x15, "4"}, {0x17, "5"}, {0x16, "6"},
	{0x1A, "7"}, {0x1C, "8"}, {0x19, "9"}, {0x1D, "0"},
	{0x1B, "-"}, {0x18, "="}, {0x1E, "]"}, {0x21, "["}, {0x2A, "\\"},
	{0x29, ";"}, {0x27, "'"}, {0x32, "`"}, {0x2B, ","}, {0x2F, "."}, {0x2C, "/"},
	{0x2D, "n"}, {0x2E, "m"}, {0x20, "u"}, {0x22, "i"}, {0x23, "p"},
	{0x25, "l"}, {0x26, "j"}, {0x28, "k"},
	{0x24, "enter"}, {0x30, "tab"}, {0x31, "space"}, {0x33, "backspace"}, {0x35, "esc"},
	{0x39, "caps lock"},
	{0x37, "windows"}, {0x36, "windows"}, // ⌘ 左 / 右
	{0x38, "shift"}, {0x3C, "shift"},
	{0x3A, "alt"}, {0x3D, "alt"}, // ⌥ option 左 / 右
	{0x3B, "ctrl"}, {0x3E, "ctrl"},
	{0x72, "insert"}, // Mac 此键码是 help，物理位置同 PC Insert
	{0x73, "home"}, {0x74, "page up"}, {0x75, "delete"}, {0x77, "end"}, {0x79, "page down"},
	{0x7B, "left"}, {0x7C, "right"}, {0x7D, "down"}, {0x7E, "up"},
	{0x7A, "f1"}, {0x78, "f2"}, {0x63, "f3"}, {0x76, "f4"}, {0x60, "f5"}, {0x61, "f6"},
	{0x62, "f7"}, {0x64, "f8"}, {0x65, "f9"}, {0x6D, "f10"}, {0x67, "f11"}, {0x6F, "f12"},
	{0x4C, "enter"}, // 小键盘回车
	{0x41, "keypad ."}, {0x43, "keypad *"}, {0x45, "keypad +"},
	{0x4B, "keypad /"}, {0x4E, "keypad -"},
	{0x52, "keypad 0"}, {0x53, "keypad 1"}, {0x54, "keypad 2"}, {0x55, "keypad 3"},
	{0x56, "keypad 4"}, {0x57, "keypad 5"}, {0x58, "keypad 6"}, {0x59, "keypad 7"},
	{0x5B, "keypad 8"}, {0x5C, "keypad 9"},
}

var (
	kc2vk map[uint16]keys.VK
	vk2kc map[keys.VK]uint16 // 注入用反查；左修饰键码排前，先见先得
)

func mustVK(name string) keys.VK {
	vk, ok := keys.VkOf(name)
	if !ok {
		panic("machook: 未知键名 " + name)
	}
	return vk
}

var (
	shiftVK = mustVK("shift")
	ctrlVK  = mustVK("ctrl")
	altVK   = mustVK("alt")
	cmdVK   = mustVK("windows") // macOS ⌘
	capsVK  = mustVK("caps lock")

	shiftMask = uint64(C.kCGEventFlagMaskShift)
	ctrlMask  = uint64(C.kCGEventFlagMaskControl)
	altMask   = uint64(C.kCGEventFlagMaskAlternate)
	cmdMask   = uint64(C.kCGEventFlagMaskCommand)
	capsMask  = uint64(C.kCGEventFlagMaskAlphaShift)
)

func modFlagMask(vk keys.VK) uint64 {
	switch vk {
	case shiftVK:
		return shiftMask
	case ctrlVK:
		return ctrlMask
	case altVK:
		return altMask
	case cmdVK:
		return cmdMask
	case capsVK:
		return capsMask
	}
	return 0
}

func normalizeKeycode(kc uint16) keys.VK { return kc2vk[kc] }

func keycodeOfVK(vk keys.VK) (uint16, bool) {
	kc, ok := vk2kc[vk]
	return kc, ok
}

func init() {
	kc2vk = make(map[uint16]keys.VK, len(keyPairs))
	vk2kc = make(map[keys.VK]uint16, len(keyPairs))
	for _, p := range keyPairs {
		vk := mustVK(p.name)
		kc2vk[p.kc] = vk
		if _, exists := vk2kc[vk]; !exists {
			vk2kc[vk] = p.kc
		}
	}
}
