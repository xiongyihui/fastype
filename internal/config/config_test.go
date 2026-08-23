package config

import "testing"

func TestConfigValidation(t *testing.T) {
	bad := []string{
		`{"layers": [{"keys": {"d": "nosuchkey"}}]}`,                       // 未知键名
		`{"layers": [{"keys": {"d": {"type":"tap_hold","tap":["d"],"hold":{"type":"layer","layer":5}}}}]}`, // 层越界
		`{"layers": []}`,                                                    // 空层
		`{"layers": [{"keys": {"d": {"type":"wat"}}}]}`,                     // 未知类型
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
