package keys

// VK 是规范化后的 Windows 虚拟键码。
// 左右修饰键统一合并为左侧键码（shift/ctrl/alt/windows），与 kb.py 的命名保持一致。
type VK uint16

const (
	vkBackspace  VK = 0x08
	vkTab        VK = 0x09
	vkEnter      VK = 0x0D
	vkPause      VK = 0x13
	vkCapsLock   VK = 0x14
	vkEsc        VK = 0x1B
	vkSpace      VK = 0x20
	vkPageUp     VK = 0x21
	vkPageDown   VK = 0x22
	vkEnd        VK = 0x23
	vkHome       VK = 0x24
	vkLeft       VK = 0x25
	vkUp         VK = 0x26
	vkRight      VK = 0x27
	vkDown       VK = 0x28
	vkPrintScr   VK = 0x2C
	vkInsert     VK = 0x2D
	vkDelete     VK = 0x2E
	vkLWin       VK = 0x5B
	vkApps       VK = 0x5D
	vkNumLock    VK = 0x90
	vkScrollLock VK = 0x91
	vkLShift     VK = 0xA0
	vkLCtrl      VK = 0xA2
	vkLAlt       VK = 0xA4
)

var keyPairs = []struct {
	vk   VK
	name string
}{
	{vkBackspace, "backspace"}, {vkTab, "tab"}, {vkEnter, "enter"},
	{vkPause, "pause"}, {vkCapsLock, "caps lock"}, {vkEsc, "esc"},
	{vkSpace, "space"},
	{vkPageUp, "page up"}, {vkPageDown, "page down"}, {vkEnd, "end"}, {vkHome, "home"},
	{vkLeft, "left"}, {vkUp, "up"}, {vkRight, "right"}, {vkDown, "down"},
	{vkPrintScr, "print screen"}, {vkInsert, "insert"}, {vkDelete, "delete"},
	{0x30, "0"}, {0x31, "1"}, {0x32, "2"}, {0x33, "3"}, {0x34, "4"},
	{0x35, "5"}, {0x36, "6"}, {0x37, "7"}, {0x38, "8"}, {0x39, "9"},
	{0x41, "a"}, {0x42, "b"}, {0x43, "c"}, {0x44, "d"}, {0x45, "e"},
	{0x46, "f"}, {0x47, "g"}, {0x48, "h"}, {0x49, "i"}, {0x4A, "j"},
	{0x4B, "k"}, {0x4C, "l"}, {0x4D, "m"}, {0x4E, "n"}, {0x4F, "o"},
	{0x50, "p"}, {0x51, "q"}, {0x52, "r"}, {0x53, "s"}, {0x54, "t"},
	{0x55, "u"}, {0x56, "v"}, {0x57, "w"}, {0x58, "x"}, {0x59, "y"},
	{0x5A, "z"},
	{vkLWin, "windows"}, {vkApps, "menu"},
	{0x60, "keypad 0"}, {0x61, "keypad 1"}, {0x62, "keypad 2"},
	{0x63, "keypad 3"}, {0x64, "keypad 4"}, {0x65, "keypad 5"},
	{0x66, "keypad 6"}, {0x67, "keypad 7"}, {0x68, "keypad 8"}, {0x69, "keypad 9"},
	{0x6A, "keypad *"}, {0x6B, "keypad +"}, {0x6D, "keypad -"},
	{0x6E, "keypad ."}, {0x6F, "keypad /"},
	{0x70, "f1"}, {0x71, "f2"}, {0x72, "f3"}, {0x73, "f4"},
	{0x74, "f5"}, {0x75, "f6"}, {0x76, "f7"}, {0x77, "f8"},
	{0x78, "f9"}, {0x79, "f10"}, {0x7A, "f11"}, {0x7B, "f12"},
	{vkNumLock, "num lock"}, {vkScrollLock, "scroll lock"},
	{vkLShift, "shift"}, {vkLCtrl, "ctrl"}, {vkLAlt, "alt"},
	{0xBA, ";"}, {0xBB, "="}, {0xBC, ","}, {0xBD, "-"}, {0xBE, "."},
	{0xBF, "/"}, {0xC0, "`"}, {0xDB, "["}, {0xDC, "\\"}, {0xDD, "]"}, {0xDE, "'"},
}

var (
	nameToVK map[string]VK
	vkToName map[VK]string
)

func init() {
	nameToVK = make(map[string]VK, len(keyPairs))
	vkToName = make(map[VK]string, len(keyPairs))
	for _, p := range keyPairs {
		if _, dup := nameToVK[p.name]; !dup {
			nameToVK[p.name] = p.vk
		}
		if _, dup := vkToName[p.vk]; !dup {
			vkToName[p.vk] = p.name
		}
	}
}

// NormalizeVK 把钩子看到的原始 VK 规范化；无法识别返回 0（调用方应放行）。
func NormalizeVK(raw uint16) VK {
	switch raw {
	case 0xA1: // right shift
		return vkLShift
	case 0xA3: // right ctrl
		return vkLCtrl
	case 0xA5: // right alt
		return vkLAlt
	case 0x5C: // right win
		return vkLWin
	}
	vk := VK(raw)
	if _, ok := vkToName[vk]; !ok {
		return 0
	}
	return vk
}

func VkOf(name string) (VK, bool) {
	vk, ok := nameToVK[name]
	return vk, ok
}

func Name(vk VK) string { return vkToName[vk] }


// IsExtendedVK 返回注入该键时是否需要 KEYEVENTF_EXTENDEDKEY，
// 否则方向键/Insert 等会被部分应用识别为小键盘键。
func IsExtendedVK(vk VK) bool {
	switch vk {
	case vkPageUp, vkPageDown, vkEnd, vkHome, vkLeft, vkUp, vkRight, vkDown,
		vkInsert, vkDelete, vkPrintScr, vkLWin, vkApps, 0x6F /* keypad / */:
		return true
	}
	return false
}
