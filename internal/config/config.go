package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fastype/internal/engine"
	"fastype/internal/keys"
)

// defaultJSON 由 defaults_*.go 按平台注入（Windows 与 macOS 的默认键位不同）。

// DefaultJSON 返回内置的默认配置（首次启动时生成 config.json 用）。
func DefaultJSON() []byte { return defaultJSON }

// ---------- 配置文件 JSON 模型 ----------
//
// {
//   "port": 8765,
//   "tap_timeout_ms": 500,
//   "layers": [
//     { "keys": {
//         "d":  {"type":"tap_hold","tap":["d"],"hold":{"type":"layer","layer":1}},
//         ";":  {"type":"tap_hold","tap":[";"],"hold":{"type":"mods","mods":["ctrl"]}},
//         "h":  "left",                 // 简写等价于 {"type":"key","keys":["left"]}
//         "p":  "shift+insert"          // 组合键可以用 "+" 连写的简写
//     }}
//   ]
// }

type Config struct {
	Port         uint16      `json:"port"`
	TapTimeoutMS int64       `json:"tap_timeout_ms"`
	Layers       []*LayerCfg `json:"layers"`
}

// LayerCfg.keys 在文件里是对象；这里保存为有序切片，回写时保持顺序。
type LayerCfg struct {
	Keys []KeyCfg
}

type KeyCfg struct {
	Name string
	Bind *BindingJSON
}

type BindingJSON struct {
	Type  string     `json:"type"` // key | tap_hold | layer_mods
	Keys  stringList `json:"keys,omitempty"`
	Tap   stringList `json:"tap,omitempty"`
	Hold  *HoldJSON  `json:"hold,omitempty"`
	Mods  stringList `json:"mods,omitempty"`
	Layer int        `json:"layer,omitempty"`
}

type HoldJSON struct {
	Type  string     `json:"type"` // layer | mods
	Layer int        `json:"layer,omitempty"`
	Mods  stringList `json:"mods,omitempty"`
}

// stringList 接受 ["ctrl","shift"] 或 "ctrl+shift" 两种写法，
// 两种写法同样做去空格与小写归一化。
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = splitCombo(str)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return fmt.Errorf("应为按键名数组或 \"a+b\" 形式的字符串")
	}
	out := make([]string, 0, len(arr))
	for _, p := range arr {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	*s = out
	return nil
}

func (s stringList) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(s))
}

func splitCombo(s string) []string {
	parts := strings.Split(s, "+")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- LayerCfg 保序解析/回写 ----------

func (l *LayerCfg) UnmarshalJSON(b []byte) error {
	var probe struct {
		Keys json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	if len(probe.Keys) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(probe.Keys))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("keys 应为对象")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := tok.(string)
		if !ok {
			return fmt.Errorf("按键名应为字符串")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("按键 %q: %w", name, err)
		}
		bind, err := parseBindingJSON(raw)
		if err != nil {
			return fmt.Errorf("按键 %q: %w", name, err)
		}
		l.Keys = append(l.Keys, KeyCfg{Name: name, Bind: bind})
	}
	return nil
}

func (l *LayerCfg) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"keys":{`)
	for i, k := range l.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		name, err := json.Marshal(k.Name)
		if err != nil {
			return nil, err
		}
		buf.Write(name)
		buf.WriteByte(':')
		b, err := json.Marshal(k.Bind)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteString("}}")
	return buf.Bytes(), nil
}

// 简写 "left" / "shift+insert" 回写时保持简写形式，便于人工编辑。
func (b *BindingJSON) MarshalJSON() ([]byte, error) {
	if b.Type == "key" && len(b.Keys) > 0 {
		return json.Marshal(strings.Join(b.Keys, "+"))
	}
	type alias BindingJSON
	return json.Marshal((*alias)(b))
}

func parseBindingJSON(raw json.RawMessage) (*BindingJSON, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parts := splitCombo(s)
		if len(parts) == 0 {
			return nil, fmt.Errorf("映射为空")
		}
		return &BindingJSON{Type: "key", Keys: parts}, nil
	}
	var b BindingJSON
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	switch b.Type {
	case "key":
		if len(b.Keys) == 0 {
			return nil, fmt.Errorf("keys 不能为空")
		}
	case "tap_hold":
		if len(b.Tap) == 0 {
			return nil, fmt.Errorf("tap 不能为空")
		}
		if b.Hold == nil {
			return nil, fmt.Errorf("缺少 hold")
		}
		switch b.Hold.Type {
		case "layer":
		case "mods":
			if len(b.Hold.Mods) == 0 {
				return nil, fmt.Errorf("hold.mods 不能为空")
			}
		default:
			return nil, fmt.Errorf("hold.type 应为 layer 或 mods")
		}
	case "layer_mods":
		// mods 允许为空：等价 QMK 的 MO(layer)，仅按住时切层
	default:
		return nil, fmt.Errorf("未知绑定类型 %q（应为 key / tap_hold / layer_mods）", b.Type)
	}
	return &b, nil
}

// ---------- 加载 / 保存 ----------

func ParseBytes(b []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.TapTimeoutMS <= 0 {
		cfg.TapTimeoutMS = 500
	}
	if cfg.Port == 0 {
		cfg.Port = 8765
	}
	return &cfg, nil
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return &cfg, nil
}

func SaveFile(path string, cfg *Config) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------- 编译为引擎可用的结构 ----------

func parseVKCombo(parts []string, ctx string) ([]keys.VK, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s为空", ctx)
	}
	vks := make([]keys.VK, 0, len(parts))
	for _, p := range parts {
		vk, ok := keys.VkOf(p)
		if !ok {
			return nil, fmt.Errorf("%s包含无法识别的键名 %q", ctx, p)
		}
		vks = append(vks, vk)
	}
	return vks, nil
}

func (c *Config) Compile() (*engine.Compiled, error) {
	if len(c.Layers) == 0 {
		return nil, fmt.Errorf("至少需要定义一个层")
	}
	if c.TapTimeoutMS < 0 || c.TapTimeoutMS > 10000 {
		return nil, fmt.Errorf("tap_timeout_ms 取值应在 0~10000 之间")
	}
	compiled := &engine.Compiled{
		Layers:  make([]map[keys.VK]*engine.Binding, len(c.Layers)),
		Timeout: time.Duration(c.TapTimeoutMS) * time.Millisecond,
	}
	for li, layer := range c.Layers {
		m := make(map[keys.VK]*engine.Binding)
		if layer != nil {
			for _, k := range layer.Keys {
				b, err := compileBinding(k.Bind, len(c.Layers))
				if err != nil {
					return nil, fmt.Errorf("层 %d 的 %q: %w", li, k.Name, err)
				}
				vk, ok := keys.VkOf(k.Name)
				if !ok {
					return nil, fmt.Errorf("层 %d 的按键名 %q 无法识别", li, k.Name)
				}
				m[vk] = b
			}
		}
		compiled.Layers[li] = m
	}
	return compiled, nil
}

func compileBinding(bj *BindingJSON, numLayers int) (*engine.Binding, error) {
	if bj == nil {
		return nil, fmt.Errorf("映射为空")
	}
	switch bj.Type {
	case "key":
		combo, err := parseVKCombo(bj.Keys, "映射")
		if err != nil {
			return nil, err
		}
		return &engine.Binding{Kind: engine.BindCombo, Combo: combo}, nil
	case "tap_hold":
		tap, err := parseVKCombo(bj.Tap, "tap")
		if err != nil {
			return nil, err
		}
		b := &engine.Binding{Kind: engine.BindTapHold, Tap: tap}
		if bj.Hold == nil {
			return nil, fmt.Errorf("缺少 hold")
		}
		switch bj.Hold.Type {
		case "layer":
			if bj.Hold.Layer < 0 || bj.Hold.Layer >= numLayers {
				return nil, fmt.Errorf("长按目标层 %d 不存在（当前共 %d 层）", bj.Hold.Layer, numLayers)
			}
			b.Hold = engine.Hold{Kind: engine.HoldLayer, Layer: bj.Hold.Layer}
		case "mods":
			mods, err := parseVKCombo(bj.Hold.Mods, "hold.mods")
			if err != nil {
				return nil, err
			}
			b.Hold = engine.Hold{Kind: engine.HoldMods, Mods: mods}
		default:
			return nil, fmt.Errorf("hold.type 应为 layer 或 mods")
		}
		return b, nil
	case "layer_mods":
		// mods 允许为空：等价 QMK 的 MO(layer)，按住时仅切层
		var mods []keys.VK
		if len(bj.Mods) > 0 {
			var err error
			mods, err = parseVKCombo(bj.Mods, "mods")
			if err != nil {
				return nil, err
			}
		}
		if bj.Layer < 0 || bj.Layer >= numLayers {
			return nil, fmt.Errorf("目标层 %d 不存在（当前共 %d 层）", bj.Layer, numLayers)
		}
		return &engine.Binding{Kind: engine.BindLayerMods, Mods: mods, Layer: bj.Layer}, nil
	}
	return nil, fmt.Errorf("未知绑定类型 %q", bj.Type)
}
