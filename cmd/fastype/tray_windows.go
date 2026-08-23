//go:build windows && amd64

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// 托盘图标：不引第三方库，直接用 Shell_NotifyIconW + 消息专用窗口实现，
// 与 kb.py 的 pystray 菜单等价（打开配置 / 暂停恢复 / 退出）。

const (
	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nimfMessage = 0x01
	nimfIcon    = 0x02
	nimfTip     = 0x04
	nimfInfo    = 0x10

	wmAppTrayCmd    = 0x8001 // 主循环里的自定义消息：托盘菜单命令
	trayCallback    = 0x8002 // 图标通知回调消息
	wmNull          = 0x0000
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203

	hwndMessage = 0xFFFFFFFFFFFFFFFD // -3

	idiApplication = 32512
	swShowNormal   = 1

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	mfSeparator    = 0x0800
	mfChecked      = 0x0008

	cmdOpen        = 1
	cmdTogglePause = 2
	cmdQuit        = 3
	cmdRefreshTip  = 4
	cmdAutoStart   = 5
)

var (
	pRegisterClassExW    = user32.NewProc("RegisterClassExW")
	pCreateWindowExW     = user32.NewProc("CreateWindowExW")
	pDestroyWindow       = user32.NewProc("DestroyWindow")
	pDefWindowProcW      = user32.NewProc("DefWindowProcW")
	pShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	pLoadIconW           = user32.NewProc("LoadIconW")
	pCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	pDestroyMenu         = user32.NewProc("DestroyMenu")
	pAppendMenuW         = user32.NewProc("AppendMenuW")
	pTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pPostMessageW        = user32.NewProc("PostMessageW")
	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	pShellExecuteW       = shell32.NewProc("ShellExecuteW")
)

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	_       uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

// NOTIFYICONDATAW（x64 布局，976 字节）
type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

var (
	trayWnd uintptr
	trayNID notifyIconData
)

// 生命周期与进程相同的 UTF-16 缓冲（作为 uintptr 存进结构体，必须保证不被回收）
var trayClassName = mustUTF16("FastypeTrayWnd")

func mustUTF16(s string) []uint16 {
	w, err := syscall.UTF16FromString(s)
	if err != nil {
		panic(err)
	}
	return w
}

func trayWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if message == trayCallback {
		switch lParam {
		case wmRButtonUp:
			showTrayMenu(hwnd)
			return 0
		case wmLButtonDblClk:
			openConfigPage()
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func createTray() {
	// 托盘失败不应导致整个程序退出（钩子/配置界面仍可用）
	defer func() {
		if r := recover(); r != nil {
			logf("托盘初始化失败（忽略，继续运行）: %v", r)
			trayWnd = 0
		}
	}()
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cls := trayClassName
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(trayWndProc),
		hInstance:     hInst,
		lpszClassName: &cls[0],
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	title := mustUTF16("Fastype")
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(&cls[0])),
		uintptr(unsafe.Pointer(&title[0])),
		0, 0, 0, 0, 0,
		hwndMessage, // 消息专用窗口，不可见
		0, hInst, 0)
	if hwnd == 0 {
		logf("创建托盘窗口失败（忽略，继续运行）")
		return
	}
	trayWnd = hwnd

	hIcon := buildKeycapIcon()
	if hIcon == 0 {
		hIcon, _, _ = pLoadIconW.Call(0, idiApplication)
	}
	trayNID = notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nimfMessage | nimfIcon | nimfTip,
		uCallbackMessage: trayCallback,
		hIcon:            hIcon,
	}
	setTrayTip()
	r, _, err := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&trayNID)))
	if r == 0 {
		logf("添加托盘图标失败: %v", err)
	}
}

func setTrayTip() {
	state := "运行中"
	if pausedFlag.Load() {
		state = "已暂停"
	}
	tip := fmt.Sprintf("Fastype - %s", state)
	runes := []rune(tip)
	if len(runes) > 127 {
		runes = runes[:127]
	}
	for i, r := range runes {
		trayNID.szTip[i] = uint16(r)
	}
	trayNID.szTip[len(runes)] = 0
}

func updateTrayTip() {
	setTrayTip()
	if trayWnd != 0 {
		pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&trayNID)))
	}
}

func destroyTray() {
	if trayWnd != 0 {
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&trayNID)))
		pDestroyWindow.Call(trayWnd)
		trayWnd = 0
	}
}

func showTrayMenu(hwnd uintptr) {
	// 任何绘制/菜单失败都不应拖垮整个进程
	defer func() {
		if r := recover(); r != nil {
			logf("托盘菜单异常（忽略）: %v", r)
		}
	}()
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)

	open := mustUTF16("打开配置页面...")
	pauseLabel := "暂停按键映射"
	if pausedFlag.Load() {
		pauseLabel = "恢复按键映射"
	}
	pause := mustUTF16(pauseLabel)
	auto := mustUTF16("开机自启")
	autoFlags := uintptr(0)
	if autoStartEnabled() {
		autoFlags = mfChecked
	}
	quit := mustUTF16("退出")

	pAppendMenuW.Call(menu, 0, cmdOpen, uintptr(unsafe.Pointer(&open[0])))
	pAppendMenuW.Call(menu, 0, cmdTogglePause, uintptr(unsafe.Pointer(&pause[0])))
	pAppendMenuW.Call(menu, autoFlags, cmdAutoStart, uintptr(unsafe.Pointer(&auto[0])))
	pAppendMenuW.Call(menu, mfSeparator, 0, 0)
	pAppendMenuW.Call(menu, 0, cmdQuit, uintptr(unsafe.Pointer(&quit[0])))

	pSetForegroundWindow.Call(hwnd)
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton,
		uintptr(int32(pt.x)), uintptr(int32(pt.y)), 0, hwnd, 0)
	pPostMessageW.Call(hwnd, wmNull, 0, 0) // 让点击菜单外区域时能正确关闭

	if cmd != 0 {
		// wndProc 在 GetMessage 内执行，直接处理即可（不重入消息循环）
		handleTrayCommand(cmd)
	}
}

// handleTrayCommand 在主线程消息循环中执行。
func handleTrayCommand(cmd uintptr) {
	switch cmd {
	case cmdOpen:
		openConfigPage()
	case cmdTogglePause:
		pausedFlag.Store(!pausedFlag.Load())
		if pausedFlag.Load() {
			logf("按键映射已暂停")
		} else {
			logf("按键映射已恢复")
		}
		updateTrayTip()
	case cmdQuit:
		postQuitToMain()
	case cmdRefreshTip:
		updateTrayTip()
	case cmdAutoStart:
		enable := !autoStartEnabled()
		if err := autoStartSet(enable); err != nil {
			logf("设置开机自启失败: %v", err)
			showBalloon("开机自启", "设置失败: "+err.Error())
			return
		}
		if enable {
			logf("已开启开机自启")
			showBalloon("开机自启已开启", "Fastype 将在登录 Windows 时自动启动")
		} else {
			logf("已关闭开机自启")
			showBalloon("开机自启已关闭", "Fastype 不再随系统启动")
		}
	}
}

func openConfigPage() {
	verb := mustUTF16("open")
	url := mustUTF16(configURL())
	pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&url[0])),
		0, 0, swShowNormal)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(url)
}

// showBalloon 弹托盘气泡通知；发送后恢复结构体快照，不影响后续 NIM_MODIFY。
func showBalloon(title, text string) {
	if trayWnd == 0 {
		return
	}
	saved := trayNID
	setRunes(trayNID.szInfoTitle[:], title)
	setRunes(trayNID.szInfo[:], text)
	trayNID.uFlags = nimfInfo
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&trayNID)))
	trayNID = saved
}

func setRunes(dst []uint16, s string) {
	runes := []rune(s)
	if len(runes) > len(dst)-1 {
		runes = runes[:len(dst)-1]
	}
	for i, r := range runes {
		dst[i] = uint16(r)
	}
	dst[len(runes)] = 0
}
