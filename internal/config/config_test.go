package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fastype/internal/engine"
	"fastype/internal/keys"
)

func TestConfigValidation(t *testing.T) {
	bad := []string{
		`{"layers": [{"keys": {"d": "nosuchkey"}}]}`,                                                       // 未知键名
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":["d"],"hold":{"type":"layer","layer":5}}}}]}`, // 层越界
		`{"layers": []}`, // 空层
		`{"layers": [{"keys": {"d": {"type":"wat"}}}]}`,                                                   // 未知类型
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":["d"]}}}]}`,                                  // tap_hold 缺 hold
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":[],"hold":{"type":"layer","layer":0}}}}]}`,   // tap 为空
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":["d"],"hold":{"type":"wat"}}}}]}`,            // hold.type 非法
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":["d"],"hold":{"type":"mods","mods":[]}}}}]}`, // hold.mods 为空
		`{"layers": [{"keys": {"d": {"type":"key","keys":[]}}}]}`,                                         // keys 为空
		`{"layers": [{"keys": {"d": ""}}]}`,                                                               // 简写映射为空
		`{"layers": [{"keys": {"d": "a+nosuchkey"}}]}`,                                                    // 组合里混入未知键
		`{"layers": [{"keys": {"nosuchkey": "a"}}]}`,                                                      // 按键名本身未知
		`{"layers": [{"keys": {"x": {"type":"layer_mods","layer":9}}}]}`,                                  // layer_mods 层越界
		`{"layers": [{"keys": {"x": {"type":"layer_mods","layer":-1}}}]}`,                                 // 负层号
		`{"tap_timeout_ms": 10001, "layers": [{"keys": {"d": "a"}}]}`,                                     // 超时上限
		`{}`,                         // 没有层
		`not json`,                   // 非 JSON
		`{"layers": [{"keys": []}]}`, // keys 应为对象
	}
	for _, s := range bad {
		cfg, err := ParseBytes([]byte(s))
		if err == nil {
			_, err = cfg.Compile()
		}
		if err == nil {
			t.Fatalf("配置应当校验失败: %s", s)
		}
	}
}

func TestDefaultJSON(t *testing.T) {
	cfg, err := ParseBytes(DefaultJSON())
	if err != nil {
		t.Fatalf("内置默认配置解析失败: %v", err)
	}
	if _, err := cfg.Compile(); err != nil {
		t.Fatalf("内置默认配置编译失败: %v", err)
	}
	if len(cfg.Layers) != 2 {
		t.Fatalf("默认配置应有 2 层，实际 %d", len(cfg.Layers))
	}
}

// 缺省字段应补默认值：端口 8765、点按超时 500ms。
func TestParseDefaults(t *testing.T) {
	cfg, err := ParseBytes([]byte(`{"layers":[{"keys":{"d":"a"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8765 {
		t.Fatalf("缺省端口应为 8765，实际 %d", cfg.Port)
	}
	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Timeout != 500*time.Millisecond {
		t.Fatalf("缺省超时应为 500ms，实际 %v", compiled.Timeout)
	}
	// 显式值不覆盖
	cfg, err = ParseBytes([]byte(`{"port":9000,"tap_timeout_ms":80,"layers":[{"keys":{}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 || cfg.TapTimeoutMS != 80 {
		t.Fatalf("显式端口/超时不应被覆盖: %d/%d", cfg.Port, cfg.TapTimeoutMS)
	}
}

const sampleCfg = `{
  "port": 9000,
  "tap_timeout_ms": 250,
  "layers": [
    {"keys": {
      "d": {"type":"tap_hold","tap":["d"],"hold":{"type":"layer","layer":1}},
      ";": {"type":"tap_hold","tap":[";"],"hold":{"type":"mods","mods":["Shift","ctrl"]}},
      "h": "left",
      "p": "shift + INSERT",
      "x": {"type":"layer_mods","mods":["alt"],"layer":1},
      "m": {"type":"layer_mods","layer":1}
    }},
    {"keys": {";": "backspace"}}
  ]
}`

// 简写与结构化写法应解析出等价结构；组合键写法大小写/空格不敏感。
func TestParseFormsAndCompile(t *testing.T) {
	cfg, err := ParseBytes([]byte(sampleCfg))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Layers) != 2 || compiled.Timeout != 250*time.Millisecond {
		t.Fatalf("编译结构不符合预期: %v", compiled)
	}

	vkD, _ := keys.VkOf("d")
	vkSemi, _ := keys.VkOf(";")
	vkH, _ := keys.VkOf("h")
	vkP, _ := keys.VkOf("p")
	vkX, _ := keys.VkOf("x")
	vkM, _ := keys.VkOf("m")
	vkShift, _ := keys.VkOf("shift")
	vkCtrl, _ := keys.VkOf("ctrl")
	vkAlt, _ := keys.VkOf("alt")
	vkLeft, _ := keys.VkOf("left")
	vkInsert, _ := keys.VkOf("insert")
	vkBackspace, _ := keys.VkOf("backspace")

	want := map[keys.VK]*engine.Binding{
		vkD:    {Kind: engine.BindTapHold, Tap: []keys.VK{vkD}, Hold: engine.Hold{Kind: engine.HoldLayer, Layer: 1}},
		vkSemi: {Kind: engine.BindTapHold, Tap: []keys.VK{vkSemi}, Hold: engine.Hold{Kind: engine.HoldMods, Mods: []keys.VK{vkShift, vkCtrl}}},
		vkH:    {Kind: engine.BindCombo, Combo: []keys.VK{vkLeft}},
		vkP:    {Kind: engine.BindCombo, Combo: []keys.VK{vkShift, vkInsert}},
		vkX:    {Kind: engine.BindLayerMods, Mods: []keys.VK{vkAlt}, Layer: 1},
		vkM:    {Kind: engine.BindLayerMods, Layer: 1}, // 空 mods = QMK 的 MO(layer)
	}
	for vk, w := range want {
		got := compiled.Layers[0][vk]
		if got == nil {
			t.Fatalf("层 0 缺少 %s 的映射", keys.Name(vk))
		}
		if !reflect.DeepEqual(got, w) {
			t.Fatalf("%s 编译结果不符:\n  实际 %+v\n  期望 %+v", keys.Name(vk), got, w)
		}
	}
	if b := compiled.Layers[1][vkSemi]; b == nil || b.Kind != engine.BindCombo || b.Combo[0] != vkBackspace {
		t.Fatalf("层 1 的 ; 应映射为 backspace，实际 %+v", b)
	}
}

// 解析 → 回写 → 再解析应保持一致：层内键序不乱、key 简写保持简写。
func TestMarshalRoundTrip(t *testing.T) {
	cfg, err := ParseBytes([]byte(sampleCfg))
	if err != nil {
		t.Fatal(err)
	}
	b1, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := ParseBytes(b1)
	if err != nil {
		t.Fatalf("回写结果无法再次解析: %v\n%s", err, b1)
	}
	b2, err := json.MarshalIndent(cfg2, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("两次回写不一致:\n%s\n%s", b1, b2)
	}

	// 层内键顺序保持解析顺序
	wantOrder := []string{"d", ";", "h", "p", "x", "m"}
	for i, k := range cfg2.Layers[0].Keys {
		if k.Name != wantOrder[i] {
			t.Fatalf("层 0 键序应为 %v，实际第 %d 个是 %q", wantOrder, i, k.Name)
		}
	}
	// 简写回写仍为简写字符串
	out := string(b1)
	for _, frag := range []string{`"h": "left"`, `"p": "shift+insert"`, `"type": "tap_hold"`} {
		if !strings.Contains(out, frag) {
			t.Fatalf("回写应包含 %q:\n%s", frag, out)
		}
	}
}

// 保存到磁盘再加载应得到等价配置；文件不存在时返回 (nil, nil)。
func TestSaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")

	if cfg, err := LoadFile(path); err != nil || cfg != nil {
		t.Fatalf("不存在的配置应返回 (nil,nil)，实际 (%v,%v)", cfg, err)
	}

	orig, err := ParseBytes([]byte(sampleCfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(path, orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(orig)
	b2, _ := json.Marshal(loaded)
	if string(b1) != string(b2) {
		t.Fatalf("磁盘往返后配置不一致:\n%s\n%s", b1, b2)
	}

	// 坏 JSON 报错且带文件名
	if err := os.WriteFile(path, []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}
