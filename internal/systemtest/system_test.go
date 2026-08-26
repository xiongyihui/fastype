// Package systemtest 做跨平台的全链路系统性测试：
// 配置 JSON → 编译 → 引擎 → 按键事件流 → keylog → Web UI HTTP API/SSE。
//
// 不依赖平台钩子（Windows 钩子 / macOS CGEventTap），用与 cmd/fastype 中
// apiHandler、hook 回调等价的装配方式把各 internal 包串起来，
// 在 Windows 与 macOS 的 CI 上都能跑完整的业务逻辑验证。
package systemtest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fastype/internal/config"
	"fastype/internal/engine"
	"fastype/internal/keylog"
	"fastype/internal/keys"
	"fastype/internal/ui"
)

// baseCfg 与默认配置同构，但超时更短、映射可断言：
// 层0: caps lock=点按 esc / 长按切层1；x=按住即切层1(layer_mods)
// 层1: h/j=方向键；p=shift+insert；q 无映射（透传）
const baseCfg = `{
  "port": 8765,
  "tap_timeout_ms": 200,
  "layers": [
    {"keys": {
      "caps lock": {"type":"tap_hold","tap":["esc"],"hold":{"type":"layer","layer":1}},
      "x": {"type":"layer_mods","layer":1}
    }},
    {"keys": {
      "h": "left", "j": "down",
      "p": "shift+insert"
    }}
  ]
}`

// 修改版配置：层1 的 h 改为 up，用于热更新断言。
const reloadedCfg = `{
  "port": 8765,
  "tap_timeout_ms": 200,
  "layers": [
    {"keys": {
      "caps lock": {"type":"tap_hold","tap":["esc"],"hold":{"type":"layer","layer":1}},
      "x": {"type":"layer_mods","layer":1}
    }},
    {"keys": {
      "h": "up", "j": "down",
      "p": "shift+insert"
    }}
  ]
}`

// testApp 复刻 cmd/fastype 的全局装配（apiHandler + 钩子回调），
// 但注入效果只记录到内存，供测试断言。
type testApp struct {
	mu     sync.Mutex
	eng    *engine.Engine
	cfg    *config.Config
	cfgPt  string
	paused atomic.Bool

	injected   []string // 每条注入效果 "d↓"/"d↑"
	suppressed []bool   // 每次物理事件的抑制结果
}

func newTestApp(t *testing.T, cfgJSON, cfgPath string) *testApp {
	t.Helper()
	cfg, err := config.ParseBytes([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatal(err)
	}
	return &testApp{eng: engine.NewEngine(compiled), cfg: cfg, cfgPt: cfgPath}
}

var feedBase = time.Now()

// onKey 等价于 hook_darwin/hook_windows 里装进平台钩子的回调。
func (a *testApp) onKey(vk keys.VK, down bool, ms int) {
	t := feedBase.Add(time.Duration(ms) * time.Millisecond)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.paused.Load() {
		keylog.Record(false, vk, down, -1, t)
		a.suppressed = append(a.suppressed, false)
		return
	}
	sup, fx := a.eng.OnEvent(engine.Event{VK: vk, Down: down, T: t})
	keylog.Record(false, vk, down, a.eng.Layer, t)
	a.suppressed = append(a.suppressed, sup)
	a.applyEffects(fx, t)
}

// applyEffects 等价于平台侧的注入入口（SendInput / CGEventPost）：
// 注入同时记为模拟事件，供按键监控展示。
func (a *testApp) applyEffects(fx []engine.Effect, t time.Time) {
	for _, f := range fx {
		a.injected = append(a.injected, keys.Name(f.VK)+arrow(f.Down))
		keylog.Record(true, f.VK, f.Down, -1, t)
	}
}

// ---- ui.Handler：与 cmd/fastype 的 apiHandler 行为保持一致 ----

func (a *testApp) Config() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *testApp) SaveConfigJSON(data []byte) error {
	newCfg, err := config.ParseBytes(data)
	if err != nil {
		return fmt.Errorf("配置格式错误: %w", err)
	}
	compiled, err := newCfg.Compile()
	if err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := config.SaveFile(a.cfgPt, newCfg); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	fx := a.eng.Reload(compiled)
	a.applyEffects(fx, time.Now())
	a.cfg = newCfg
	return nil
}

func (a *testApp) Status() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{
		"running": !a.paused.Load(),
		"paused":  a.paused.Load(),
		"layers":  len(a.cfg.Layers),
		"version": "systemtest",
	}
}

func (a *testApp) SetPaused(paused bool) map[string]any {
	a.paused.Store(paused)
	return a.Status()
}

// ---- 测试辅助 ----

func arrow(down bool) string {
	if down {
		return "↓"
	}
	return "↑"
}

func vkOf(t *testing.T, name string) keys.VK {
	t.Helper()
	vk, ok := keys.VkOf(name)
	if !ok {
		t.Fatalf("未知键名 %q", name)
	}
	return vk
}

func (a *testApp) injectedList() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.injected...)
}

func (a *testApp) lastSuppressed(t *testing.T) bool {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.suppressed) == 0 {
		t.Fatal("没有物理事件记录")
	}
	return a.suppressed[len(a.suppressed)-1]
}

func assertInjected(t *testing.T, got, want []string, msg string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s\n  实际: %v\n  期望: %v", msg, got, want)
	}
}

func waitLog(t *testing.T, want uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for keylog.Snapshot().LastSeq < want {
		if time.Now().After(deadline) {
			t.Fatalf("等待日志 seq≥%d 超时（当前 %d）", want, keylog.Snapshot().LastSeq)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func seqNow() uint64 { return keylog.Snapshot().LastSeq }

// TestTypingSession 端到端驱动一次"打字"过程：
// 点按/长按/切层/透传/暂停，断言注入序列与按键监控记录。
func TestTypingSession(t *testing.T) {
	a := newTestApp(t, baseCfg, "")

	// --- 点按 caps lock → 注入 esc ---
	s0 := seqNow()
	a.onKey(vkOf(t, "caps lock"), true, 0)
	a.onKey(vkOf(t, "caps lock"), false, 50)
	assertInjected(t, a.injectedList(), []string{"esc↓", "esc↑"}, "点按 caps 应输出 esc")

	// --- 按住 x（layer_mods 立即切层1）→ h 映射 left，j 映射 down ---
	a.onKey(vkOf(t, "x"), true, 100)
	if a.eng.Layer != 1 {
		t.Fatalf("x 按下后应在层 1，实际 %d", a.eng.Layer)
	}
	a.onKey(vkOf(t, "h"), true, 120)
	a.onKey(vkOf(t, "h"), false, 160)
	a.onKey(vkOf(t, "j"), true, 180)
	a.onKey(vkOf(t, "j"), false, 200)
	a.onKey(vkOf(t, "x"), false, 220)
	assertInjected(t, a.injectedList()[2:],
		[]string{"left↓", "left↑", "down↓", "down↑"}, "层 1 的 h/j 应映射为方向键")
	if a.eng.Layer != 0 {
		t.Fatalf("x 抬起后应回层 0，实际 %d", a.eng.Layer)
	}

	// --- 未映射键透传：无注入、不抑制 ---
	n := len(a.injectedList())
	a.onKey(vkOf(t, "q"), true, 300)
	a.onKey(vkOf(t, "q"), false, 340)
	assertInjected(t, a.injectedList()[n:], nil, "未映射键不应产生注入")
	if a.lastSuppressed(t) {
		t.Fatal("未映射键不应被抑制")
	}

	// --- 层 1 组合键 p → shift+insert 反序释放 ---
	a.onKey(vkOf(t, "x"), true, 400)
	a.onKey(vkOf(t, "p"), true, 420)
	a.onKey(vkOf(t, "p"), false, 460)
	a.onKey(vkOf(t, "x"), false, 480)
	assertInjected(t, a.injectedList()[n:], []string{"shift↓", "insert↓", "insert↑", "shift↑"},
		"层 1 的 p 应输出 shift+insert 并反序释放")

	// --- 暂停：物理事件照常记录（层 -1），但不映射不注入 ---
	a.SetPaused(true)
	n = len(a.injectedList())
	a.onKey(vkOf(t, "h"), true, 600)
	a.onKey(vkOf(t, "h"), false, 640)
	assertInjected(t, a.injectedList()[n:], nil, "暂停期间不应注入")
	if a.lastSuppressed(t) {
		t.Fatal("暂停期间不应抑制")
	}
	a.SetPaused(false)

	// --- keylog 汇总断言：等后台落库追平 ---
	// 上面共投递 16 次真实事件 + 10 条注入（模拟事件）
	waitLog(t, s0+26)

	snap := keylog.Snapshot()
	if snap.Dropped != 0 {
		t.Fatalf("事件不应被丢弃: %d", snap.Dropped)
	}
	// 最后两条应是暂停期间的 h 真实按下/抬起，层为 -1
	last := snap.Entries[len(snap.Entries)-2:]
	if last[0].Name != "h" || last[0].Sim || last[0].Layer != -1 ||
		last[1].Name != "h" || last[1].Down {
		t.Fatalf("暂停期间的事件应记为真实、层 -1: %+v", last)
	}
	if len(snap.RealDown) != 0 || len(snap.SimDown) != 0 {
		t.Fatalf("会话结束后按住列表应为空: real=%v sim=%v", snap.RealDown, snap.SimDown)
	}
	// 模拟事件确实入册：esc / left / shift …（按 seq 过滤，只看本测试的事件）
	var simNames []string
	for _, e := range snap.Entries {
		if e.Sim && e.Seq > s0 {
			simNames = append(simNames, e.Name)
		}
	}
	assertInjected(t, simNames, []string{
		"esc", "esc",
		"left", "left", "down", "down",
		"shift", "insert", "insert", "shift",
	}, "注入应以模拟事件入册")
}

// TestWebUIHotReload 走真实 HTTP：改配置 → 落盘 → 引擎热更新；
// 热更新时按住的组合键要被立即释放。
func TestWebUIHotReload(t *testing.T) {
	cfgPath := t.TempDir() + "/config.json"
	a := newTestApp(t, baseCfg, cfgPath)

	port, err := ui.Start(0, a)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	postJSON := func(path string, body string) *http.Response {
		resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// 首页可访问
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(html), "Fastype") {
		t.Fatalf("Web UI 首页异常: %d", resp.StatusCode)
	}

	// 层 1 按住 p（shift+insert 保持按下），随后热更新
	a.onKey(vkOf(t, "x"), true, 0)
	a.onKey(vkOf(t, "p"), true, 20)

	resp = postJSON("/api/config", reloadedCfg)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法配置应 200，实际 %d", resp.StatusCode)
	}

	// 热更新必须立刻释放按住的组合键
	assertInjected(t, a.injectedList(), []string{"shift↓", "insert↓", "insert↑", "shift↑"},
		"热更新应释放按住的组合键")
	// 释放事件在后台异步落库，等按住列表清空
	deadline := time.Now().Add(2 * time.Second)
	for len(keylog.Snapshot().SimDown) > 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if snap := keylog.Snapshot(); len(snap.SimDown) > 0 {
		t.Fatalf("热更新后模拟按住应为空: %v", snap.SimDown)
	}
	if a.eng.Layer != 0 {
		t.Fatalf("热更新后层应复位，实际 %d", a.eng.Layer)
	}

	// 配置确实落盘
	onDisk, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(onDisk), `"up"`) {
		t.Fatalf("新配置应写入磁盘:\n%s", onDisk)
	}

	// 引擎行为已切换到新映射：h 现在输出 up
	n := len(a.injectedList())
	a.onKey(vkOf(t, "x"), true, 100)
	a.onKey(vkOf(t, "h"), true, 120)
	a.onKey(vkOf(t, "h"), false, 140)
	a.onKey(vkOf(t, "x"), false, 160)
	assertInjected(t, a.injectedList()[n:], []string{"up↓", "up↑"}, "热更新后 h 应输出 up")

	// 非法配置被拒：引擎与磁盘都不受影响
	resp = postJSON("/api/config", `{"layers":[{"keys":{"no-such-key":"a"}}]}`)
	var e map[string]any
	json.NewDecoder(resp.Body).Decode(&e)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || e["error"] == nil {
		t.Fatalf("非法配置应 400 + error: %d %v", resp.StatusCode, e)
	}
	onDisk2, _ := os.ReadFile(cfgPath)
	if string(onDisk2) != string(onDisk) {
		t.Fatal("非法配置不应改动磁盘文件")
	}
}

// TestWebUIPauseFlow 走真实 HTTP 验证暂停/恢复与状态接口的联动。
func TestWebUIPauseFlow(t *testing.T) {
	a := newTestApp(t, baseCfg, "")
	port, err := ui.Start(0, a)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	getStatus := func() map[string]any {
		resp, err := http.Get(base + "/api/status")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var st map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}

	if st := getStatus(); st["paused"] != false || st["running"] != true || st["layers"] != float64(2) {
		t.Fatalf("初始状态异常: %v", st)
	}

	resp, err := http.Post(base+"/api/pause", "application/json", strings.NewReader(`{"paused":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if st := getStatus(); st["paused"] != true || st["running"] != false {
		t.Fatalf("暂停后状态异常: %v", st)
	}

	// 暂停期间按键：透传（不注入），HTTP 侧查 keylog 层为 -1
	s0 := seqNow()
	a.onKey(vkOf(t, "h"), true, 0)
	a.onKey(vkOf(t, "h"), false, 40)
	waitLog(t, s0+2)
	resp2, err := http.Get(base + "/api/keylog")
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Entries []struct {
			Name  string `json:"name"`
			Sim   bool   `json:"sim"`
			Layer int    `json:"layer"`
		} `json:"entries"`
	}
	json.NewDecoder(resp2.Body).Decode(&snap)
	resp2.Body.Close()
	last := snap.Entries[len(snap.Entries)-2:]
	if last[0].Name != "h" || last[0].Sim || last[0].Layer != -1 {
		t.Fatalf("暂停期间事件应为真实事件且层 -1: %+v", last)
	}
}

// TestWebUIKeyLogStream 端到端验证 SSE：真实浏览器式连接收到快照，
// 随后驱动一次完整映射，注入的模拟事件实时推送回来。
func TestWebUIKeyLogStream(t *testing.T) {
	a := newTestApp(t, baseCfg, "")
	port, err := ui.Start(0, a)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/keylog/stream", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	readMsg := func() string {
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("读 SSE 失败: %v", err)
			}
			if s := strings.TrimRight(line, "\r\n"); strings.HasPrefix(s, "data: ") {
				return strings.TrimPrefix(s, "data: ")
			}
		}
	}

	if msg := readMsg(); !strings.Contains(msg, `"type":"snapshot"`) {
		t.Fatalf("首条消息应为快照: %s", msg)
	}

	// 点按 caps → esc；注入的 esc(模拟) 应实时推到流上
	s0 := seqNow()
	a.onKey(vkOf(t, "caps lock"), true, 0)
	a.onKey(vkOf(t, "caps lock"), false, 50)
	waitLog(t, s0+4) // 2 真实 + 2 模拟

	sawSim := false
	deadline := time.Now().Add(2 * time.Second)
	for !sawSim && time.Now().Before(deadline) {
		msg := readMsg()
		if strings.Contains(msg, `"type":"events"`) &&
			strings.Contains(msg, `"name":"esc"`) && strings.Contains(msg, `"sim":true`) {
			sawSim = true
		}
	}
	if !sawSim {
		t.Fatal("SSE 应实时推送注入的模拟事件")
	}
}
