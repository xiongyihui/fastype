//go:build darwin

// Package mactray 在 macOS 菜单栏显示状态图标（对应 Windows 版的托盘），
// 直接驱动 NSStatusItem，不引第三方依赖。ObjC 实现见 tray.m。
//
// 必须在锁定为主线程的 goroutine 里调用 Run（AppKit 要求）。
package mactray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#include <stdlib.h>

// 实现全部位于 tray.m（含 //export 的包，前导里不能出现定义）。
// tray.m 开启 ARC：否则 statusItemWithLength: 返回的 autorelease 对象
// 存进静态变量后无引用计数，随池释放导致菜单栏图标静默消失。
extern void traySetup(const char* tip, const char* open,
                      const char* pause, const char* autostart, const char* quit);
extern void trayRun(void);
extern void trayStopAsync(void);
extern void traySetPauseTitle(const char* s);
extern void traySetAutoState(int on);
extern void traySetTip(const char* s);
extern int trayDiag(void);
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

// 菜单命令，与 Windows 版托盘命令语义一致。
const (
	CmdOpen        = 1
	CmdTogglePause = 2
	CmdQuit        = 3
	CmdAutoStart   = 5
)

// OnCmd 由使用方设置：菜单命令回调（在主线程执行）。
var OnCmd func(cmd int)

// DiagLogf 由使用方设置：诊断日志回调（缺省丢弃）。
var DiagLogf func(format string, args ...any)

var running = false

func diagLogf(format string, args ...any) {
	if DiagLogf != nil {
		DiagLogf(format, args...)
	}
}

//export goTrayCmd
func goTrayCmd(cmd C.int) {
	if OnCmd != nil {
		OnCmd(int(cmd))
	}
}

// Run 创建菜单栏图标（模板图：圆角方块 + 镂空闪电）并进入 AppKit 事件循环（阻塞，必须在主线程）。
func Run(tip, open, pause, autostart, quit string) error {
	if running {
		return errors.New("mactray: 已在运行")
	}
	running = true
	runtime.LockOSThread()

	tp := C.CString(tip)
	op, pa := C.CString(open), C.CString(pause)
	au, qt := C.CString(autostart), C.CString(quit)
	C.traySetup(tp, op, pa, au, qt)
	diag := int(C.trayDiag())
	for _, p := range []*C.char{tp, op, pa, au, qt} {
		C.free(unsafe.Pointer(p))
	}
	// 静默失败排查：item/button 任一为空说明状态图标没有真正建立
	diagLogf("菜单栏图标诊断: item=%v button=%v image=%v title=%v",
		diag&1 != 0, diag&2 != 0, diag&4 != 0, diag&8 != 0)

	C.trayRun()
	return nil
}

// Stop 摘除图标并结束事件循环（Run 随之返回）。可从任意线程调用。
func Stop() {
	if !running {
		return
	}
	C.trayStopAsync()
}

// SetPauseTitle 更新「暂停/恢复」菜单项标题。可从任意线程调用。
func SetPauseTitle(s string) {
	if !running {
		return
	}
	p := C.CString(s)
	C.traySetPauseTitle(p)
	C.free(unsafe.Pointer(p))
}

// SetAutoState 设置「开机自启」菜单项的勾选状态（主线程事件循环内调用）。
func SetAutoState(on bool) {
	if !running {
		return
	}
	if on {
		C.traySetAutoState(1)
	} else {
		C.traySetAutoState(0)
	}
}

// SetTip 更新悬停提示。可从任意线程调用。
func SetTip(s string) {
	if !running {
		return
	}
	p := C.CString(s)
	C.traySetTip(p)
	C.free(unsafe.Pointer(p))
}
