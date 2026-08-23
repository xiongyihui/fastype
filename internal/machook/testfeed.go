//go:build darwin

package machook

/*
#include <ApplicationServices/ApplicationServices.h>
*/
import "C"

// 进程内测试入口：离线构造 CGEvent 喂给回调逻辑。
// （cgo 不允许出现在 _test.go 文件里，故放在常规文件中，仅供测试使用。）

// testSendKey 模拟一次物理按键事件（keydown/keyup），返回事件是否被吞掉。
func testSendKey(kc uint16, down bool) bool {
	var etype C.CGEventType = C.kCGEventKeyUp
	if down {
		etype = C.kCGEventKeyDown
	}
	ev := cEventKeyboard(cSrcNull(), kc, down)
	return cEvIsNull(goTapTrampoline(nil, etype, ev, nil))
}

// testSendFlags 模拟一次 flagsChanged 事件，返回事件是否被吞掉。
func testSendFlags(kc uint16, flags uint64) bool {
	ev := cEventKeyboard(cSrcNull(), kc, true)
	cEventSetFlags(ev, flags)
	return cEvIsNull(goTapTrampoline(nil, C.kCGEventFlagsChanged, ev, nil))
}

// testTrusted 查询辅助功能权限（无弹窗）。
func testTrusted() bool { return Trusted(false) }
