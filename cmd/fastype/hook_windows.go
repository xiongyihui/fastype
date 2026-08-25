//go:build windows && amd64

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"fastype/internal/engine"
	"fastype/internal/keylog"
	"fastype/internal/keys"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	pSetWindowsHookEx      = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx        = user32.NewProc("CallNextHookEx")
	pUnhookWindowsHookEx   = user32.NewProc("UnhookWindowsHookEx")
	pGetMessageW           = user32.NewProc("GetMessageW")
	pPostThreadMessageW    = user32.NewProc("PostThreadMessageW")
	pGetCurrentThreadId    = kernel32.NewProc("GetCurrentThreadId")
	pSendInput             = user32.NewProc("SendInput")
	pSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

const (
	whKeyboardLL = 13

	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	wmQuit       = 0x0012

	llkhfInjected = 0x10

	inputKeyboard        = 1
	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
)

type llHookStruct struct {
	vkCode    uint32
	scanCode  uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// SendInput 的 INPUT 结构在 x64 上必须正好 40 字节：
// type(4) + 填充(4) + union(32，按 MOUSEINPUT 取最大)。
type inputEvent struct {
	typ uint32
	_   uint32
	ki  keybdInput
	_2  [8]byte
}

func init() {
	if unsafe.Sizeof(inputEvent{}) != 40 {
		panic(fmt.Sprintf("INPUT 结构大小不符: %d", unsafe.Sizeof(inputEvent{})))
	}
}

func sendEffects(fx []engine.Effect) {
	if len(fx) == 0 {
		return
	}
	now := time.Now()
	inputs := make([]inputEvent, len(fx))
	for i, f := range fx {
		var fl uint32
		if !f.Down {
			fl |= keyeventfKeyUp
		}
		if keys.IsExtendedVK(f.VK) {
			fl |= keyeventfExtendedKey
		}
		inputs[i] = inputEvent{typ: inputKeyboard, ki: keybdInput{wVk: uint16(f.VK), dwFlags: fl}}
		keylog.Record(true, f.VK, f.Down, -1, now)
	}
	pSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

func keyboardHookProc(nCode int32, wParam uintptr, lParamPtr unsafe.Pointer) uintptr {
	lParam := uintptr(lParamPtr)
	next := func() uintptr {
		r, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
		return r
	}
	if nCode < 0 {
		return next()
	}
	if wParam != wmKeyDown && wParam != wmKeyUp && wParam != wmSysKeyDown && wParam != wmSysKeyUp {
		return next()
	}
	info := (*llHookStruct)(lParamPtr)
	if info.flags&llkhfInjected != 0 {
		// 自己（或其它程序）注入的事件直接放行，避免递归
		return next()
	}
	vk := keys.NormalizeVK(uint16(info.vkCode))
	if vk == 0 {
		return next()
	}
	ev := engine.Event{VK: vk, Down: wParam == wmKeyDown || wParam == wmSysKeyDown, T: time.Now()}
	if pausedFlag.Load() {
		keylog.Record(false, vk, ev.Down, -1, ev.T) // 暂停期间仍记录真实按键（无层信息）
		return next()
	}

	engineMu.Lock()
	suppressed, fx := eng.OnEvent(ev)
	layer := eng.Layer
	engineMu.Unlock()
	keylog.Record(false, vk, ev.Down, layer, ev.T)

	if dryRun {
		return next()
	}
	sendEffects(fx)
	if suppressed {
		return 1
	}
	return next()
}

// runHookLoop 占用主线程：安装低级键盘钩子并跑消息循环（两者必须在同一线程）。
// 返回即表示收到退出消息。
func runHookLoop() {
	runtime.LockOSThread()

	tid, _, _ := pGetCurrentThreadId.Call()
	mainTID.Store(tid)

	cb := syscall.NewCallback(keyboardHookProc)
	hook, _, err := pSetWindowsHookEx.Call(whKeyboardLL, cb, 0, 0)
	if hook == 0 {
		fatalf("安装键盘钩子失败（试试以管理员运行）: %v", err)
	}
	logf("键盘钩子已安装 (线程 %d)", tid)

	createTray()

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 = WM_QUIT, -1 = 错误
			break
		}
		if m.message == wmAppTrayCmd {
			handleTrayCommand(m.wParam)
		}
	}

	pUnhookWindowsHookEx.Call(hook)
	destroyTray()
}

func consoleCtrlHandler(ctrlType uint32) uintptr {
	postQuitToMain()
	return 1
}

func postQuitToMain() {
	if t := mainTID.Load(); t != 0 {
		pPostThreadMessageW.Call(t, wmQuit, 0, 0)
	}
}
