//go:build darwin

package machook

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation

#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

extern CGEventRef goTapTrampoline(CGEventTapProxy proxy, CGEventType type,
                                  CGEventRef event, void *userInfo);

// C 辅助函数集中放在本文件（含 //export 的文件前导里不允许出现函数定义）。

static CGEventMask tapMask(void) {
	return CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp) |
	       CGEventMaskBit(kCGEventFlagsChanged);
}

static CFMachPortRef tapCreate(bool listenOnly) {
	// 只读模式（dry-run）无需辅助功能权限；拦截模式需要。
	CGEventTapOptions opt = listenOnly ? kCGEventTapOptionListenOnly
	                                   : kCGEventTapOptionDefault;
	return CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
	                        opt, tapMask(), goTapTrampoline, NULL);
}

static void tapEnable(CFMachPortRef tap) { CGEventTapEnable(tap, true); }

static void runloopAddTap(CFMachPortRef tap, CFRunLoopRef rl) {
	CFRunLoopAddSource(rl, CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0),
	                   kCFRunLoopCommonModes);
}
static void runloopRun(void)          { CFRunLoopRun(); }
static void runloopStop(CFRunLoopRef rl) { CFRunLoopStop(rl); }
static CFRunLoopRef runloopCurrent(void) { return CFRunLoopGetCurrent(); }

static CGEventSourceRef sourceCreate(void) {
	return CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
}
static uint64_t sourceFlagsHID(void) {
	return (uint64_t)CGEventSourceFlagsState(kCGEventSourceStateHIDSystemState);
}

static CGEventRef eventKeyboard(CGEventSourceRef src, uint16_t kc, bool down) {
	return CGEventCreateKeyboardEvent(src, (CGKeyCode)kc, down);
}
static void eventPost(CGEventRef ev)     { CGEventPost(kCGSessionEventTap, ev); }
static void eventRelease(CGEventRef ev)  { CFRelease(ev); }
static uint64_t eventFlags(CGEventRef ev) { return (uint64_t)CGEventGetFlags(ev); }
static void eventSetFlags(CGEventRef ev, uint64_t f) { CGEventSetFlags(ev, (CGEventFlags)f); }
static int64_t eventField(CGEventRef ev, uint32_t field) {
	return CGEventGetIntegerValueField(ev, (CGEventField)field);
}
static void eventSetField(CGEventRef ev, uint32_t field, int64_t v) {
	CGEventSetIntegerValueField(ev, (CGEventField)field, v);
}

// axTrusted 查询「辅助功能」信任状态；prompt 为 true 时弹出系统授权引导。
static bool axTrusted(bool prompt) {
	CFTypeRef keys[] = {(CFTypeRef)kAXTrustedCheckOptionPrompt};
	CFTypeRef vals[] = {(CFTypeRef)(prompt ? kCFBooleanTrue : kCFBooleanFalse)};
	CFDictionaryRef opts = CFDictionaryCreate(kCFAllocatorDefault, keys, vals, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	bool ok = AXIsProcessTrustedWithOptions(opts);
	if (opts) CFRelease(opts);
	return ok;
}

// cgo 的不透明指针类型无法直接与 nil 比较，判空/取空都在 C 侧完成。
static bool evIsNull(CGEventRef ev)             { return ev == NULL; }
static bool tapIsNull(CFMachPortRef t)          { return t == NULL; }
static bool srcIsNull(CGEventSourceRef s)       { return s == NULL; }
static bool rlIsNull(CFRunLoopRef rl)           { return rl == NULL; }
static CGEventRef evNull(void)                  { return NULL; }
static CGEventSourceRef srcNull(void)           { return NULL; }
*/
import "C"

// 供 machook.go 调用的薄封装（cgo 的 C 名字只在本文件可见）。

func cEvIsNull(ev C.CGEventRef) bool       { return bool(C.evIsNull(ev)) }
func cTapIsNull(t C.CFMachPortRef) bool    { return bool(C.tapIsNull(t)) }
func cSrcIsNull(s C.CGEventSourceRef) bool { return bool(C.srcIsNull(s)) }
func cRlIsNull(rl C.CFRunLoopRef) bool     { return bool(C.rlIsNull(rl)) }
func cEvNull() C.CGEventRef                { return C.evNull() }
func cSrcNull() C.CGEventSourceRef         { return C.srcNull() }
func cTapCreate(listenOnly bool) C.CFMachPortRef {
	return C.tapCreate(C._Bool(listenOnly))
}
func cTapEnable(t C.CFMachPortRef)                        { C.tapEnable(t) }
func cSourceCreate() C.CGEventSourceRef                   { return C.sourceCreate() }
func cSourceFlagsHID() uint64                             { return uint64(C.sourceFlagsHID()) }
func cRunLoopCurrent() C.CFRunLoopRef                     { return C.runloopCurrent() }
func cRunLoopRun()                                        { C.runloopRun() }
func cRunLoopStop(rl C.CFRunLoopRef)                      { C.runloopStop(rl) }
func cRunLoopAddTap(t C.CFMachPortRef, rl C.CFRunLoopRef) { C.runloopAddTap(t, rl) }

func cEventKeyboard(src C.CGEventSourceRef, kc uint16, down bool) C.CGEventRef {
	return C.eventKeyboard(src, C.uint16_t(kc), C._Bool(down))
}
func cEventPost(ev C.CGEventRef)               { C.eventPost(ev) }
func cEventRelease(ev C.CGEventRef)            { C.eventRelease(ev) }
func cEventFlags(ev C.CGEventRef) uint64       { return uint64(C.eventFlags(ev)) }
func cEventSetFlags(ev C.CGEventRef, f uint64) { C.eventSetFlags(ev, C.uint64_t(f)) }
func cEventField(ev C.CGEventRef, field uint32) int64 {
	return int64(C.eventField(ev, C.uint32_t(field)))
}
func cEventSetField(ev C.CGEventRef, field uint32, v int64) {
	C.eventSetField(ev, C.uint32_t(field), C.int64_t(v))
}
func cAxTrusted(prompt bool) bool { return bool(C.axTrusted(C._Bool(prompt))) }
