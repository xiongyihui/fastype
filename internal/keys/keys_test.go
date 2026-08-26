package keys

import "testing"

// 规范键名应与 VK 双向可逆；别名（command/cmd/option/opt）指向同一 VK，
// Name() 返回首见的规范名。
func TestNameVkRoundTrip(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range keyPairs {
		if seen[p.name] {
			t.Fatalf("键名 %q 在表中重复定义", p.name)
		}
		seen[p.name] = true
		vk, ok := VkOf(p.name)
		if !ok {
			t.Fatalf("VkOf(%q) 应能解析", p.name)
		}
		if vk != p.vk {
			t.Fatalf("VkOf(%q) = %#x，期望 %#x", p.name, vk, p.vk)
		}
		if !isAlias[p.name] && Name(vk) != p.name {
			t.Fatalf("Name(VkOf(%q)) = %q，应返回规范名 %q", p.name, Name(vk), p.name)
		}
	}
}

// isAlias 标出追加在表尾的 macOS 别名：它们的 VK 与前面的规范键相同。
var isAlias = map[string]bool{"command": true, "cmd": true, "option": true, "opt": true}

func TestAliasesResolveToCanonical(t *testing.T) {
	cases := []struct{ alias, canonical string }{
		{"command", "windows"}, {"cmd", "windows"},
		{"option", "alt"}, {"opt", "alt"},
	}
	for _, c := range cases {
		aliasVK, ok := VkOf(c.alias)
		if !ok {
			t.Fatalf("别名 %q 应可解析", c.alias)
		}
		if Name(aliasVK) != c.canonical {
			t.Fatalf("Name(VkOf(%q)) = %q，期望规范名 %q", c.alias, Name(aliasVK), c.canonical)
		}
	}
	if vk, ok := VkOf("no such key"); ok || vk != 0 {
		t.Fatal("未知键名应返回 (0, false)")
	}
}

func TestNormalizeVK(t *testing.T) {
	cases := []struct {
		raw  uint16
		want VK
	}{
		{0x41, 0x41},         // 字母原样
		{0xA0, vkLShift},     // 左修饰键
		{0xA1, vkLShift},     // 右 shift → 合并
		{0xA2, vkLCtrl},      // 左
		{0xA3, vkLCtrl},      // 右 ctrl → 合并
		{0xA4, vkLAlt},       // 左
		{0xA5, vkLAlt},       // 右 alt → 合并
		{0x5B, vkLWin},       // 左
		{0x5C, vkLWin},       // 右 win → 合并
		{0x07, 0},            // 未定义键码
		{0xFF, 0},            // 未定义键码
		{0x1D, 0},            // Windows 保留
		{0xE0 /* 前缀字节 */, 0}, // 钩子不会单独投递，防御性返回 0
	}
	for _, c := range cases {
		if got := NormalizeVK(c.raw); got != c.want {
			t.Fatalf("NormalizeVK(%#x) = %#x，期望 %#x", c.raw, got, c.want)
		}
	}
}

func TestIsExtendedVK(t *testing.T) {
	extended := []string{
		"left", "right", "up", "down", "insert", "delete", "home", "end",
		"page up", "page down", "print screen", "windows", "menu", "keypad /",
	}
	for _, name := range extended {
		vk, _ := VkOf(name)
		if !IsExtendedVK(vk) {
			t.Fatalf("%s 注入时应使用扩展键标志", name)
		}
	}
	plain := []string{"a", "enter", "space", "backspace", "keypad 0", "keypad +", "f5", "shift"}
	for _, name := range plain {
		vk, _ := VkOf(name)
		if IsExtendedVK(vk) {
			t.Fatalf("%s 不应使用扩展键标志", name)
		}
	}
}

// 修饰键合并后，左右键码不应在表外残留独立条目。
func TestNoOrphanRightModifiers(t *testing.T) {
	for _, raw := range []uint16{0xA1, 0xA3, 0xA5, 0x5C} {
		if vk := VK(raw); vkToName[vk] != "" {
			t.Fatalf("右侧修饰键 %#x 不应出现在键名表中", raw)
		}
	}
}
