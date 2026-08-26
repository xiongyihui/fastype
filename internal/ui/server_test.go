package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fastype/internal/keylog"
	"fastype/internal/keys"
)

func vkOf(t *testing.T, name string) keys.VK {
	t.Helper()
	vk, ok := keys.VkOf(name)
	if !ok {
		t.Fatalf("未知键名 %q", name)
	}
	return vk
}

func seqNow() uint64 { return keylog.Snapshot().LastSeq }

// waitLog 轮询等待后台日志 goroutine 处理到 seq ≥ want。
func waitLog(t *testing.T, want uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for seqNow() < want {
		if time.Now().After(deadline) {
			t.Fatalf("等待日志 seq≥%d 超时（当前 %d）", want, seqNow())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestKeyLogSSEStream 走真实 HTTP 连接验证 SSE 推流：
// 连接先收全量快照，随后的新事件应作为增量消息实时到达。
// 每条事件都在读到上一条的消息之后才记录，避免服务端合批导致的不确定顺序。
func TestKeyLogSSEStream(t *testing.T) {
	s0 := seqNow()
	keylog.Record(false, vkOf(t, "a"), true, 0, time.Now())
	waitLog(t, s0+1)

	srv := httptest.NewServer(http.HandlerFunc(handleKeyLogStream))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type 应为 text/event-stream: %s", ct)
	}
	r := bufio.NewReader(resp.Body)
	readData := func() string {
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("读 SSE 消息失败: %v", err)
			}
			if s := strings.TrimRight(line, "\r\n"); strings.HasPrefix(s, "data: ") {
				return strings.TrimPrefix(s, "data: ")
			}
		}
	}

	msg := readData()
	if !strings.Contains(msg, `"snapshot"`) || !strings.Contains(msg, `"name":"a"`) {
		t.Fatalf("首条应为含历史事件的快照: %s", msg)
	}

	keylog.Record(false, vkOf(t, "b"), true, 3, time.Now())
	waitLog(t, s0+2)
	msg = readData()
	if !strings.Contains(msg, `"type":"events"`) || !strings.Contains(msg, `"name":"b"`) {
		t.Fatalf("快照后的真实事件应以增量消息推送: %s", msg)
	}

	keylog.Record(true, vkOf(t, "left"), true, -1, time.Now())
	waitLog(t, s0+3)
	msg = readData()
	if !strings.Contains(msg, `"name":"left"`) || !strings.Contains(msg, `"sim":true`) {
		t.Fatalf("模拟事件未推送: %s", msg)
	}

	// 收尾释放，不影响其它测试
	keylog.Record(false, vkOf(t, "a"), false, 0, time.Now())
	keylog.Record(false, vkOf(t, "b"), false, 0, time.Now())
	keylog.Record(true, vkOf(t, "left"), false, -1, time.Now())
	waitLog(t, s0+6)
}

// fakeApp 实现 Handler，记录调用并返回可断言的结果。
type fakeApp struct {
	cfg        map[string]any
	saveErr    error
	savedJSON  []byte
	pausedFrom []bool
	pauseCalls int
}

func (f *fakeApp) Config() any { return f.cfg }

func (f *fakeApp) SaveConfigJSON(data []byte) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedJSON = append([]byte(nil), data...)
	return nil
}

func (f *fakeApp) Status() map[string]any {
	return map[string]any{"running": true, "version": "test"}
}

func (f *fakeApp) SetPaused(paused bool) map[string]any {
	f.pauseCalls++
	f.pausedFrom = append(f.pausedFrom, paused)
	return map[string]any{"paused": paused}
}

func TestRoutes(t *testing.T) {
	app := &fakeApp{cfg: map[string]any{"port": 8765.0, "layers": []any{}}}
	srv := httptest.NewServer(newMux(app))
	defer srv.Close()

	get := func(path string) *http.Response {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	t.Run("index", func(t *testing.T) {
		resp := get("/")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("/ 应为 200，实际 %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("/ Content-Type 应为 text/html: %s", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "<title>Fastype</title>") {
			t.Fatal("/ 应返回内嵌的 Web UI 页面")
		}
	})

	t.Run("index 子路径 404", func(t *testing.T) {
		resp := get("/no/such/path")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("未知路径应为 404，实际 %d", resp.StatusCode)
		}
	})

	t.Run("status", func(t *testing.T) {
		resp := get("/api/status")
		defer resp.Body.Close()
		var st map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		if st["version"] != "test" || st["running"] != true {
			t.Fatalf("status 应来自 Handler: %v", st)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("API 响应应禁止缓存: %q", cc)
		}
	})

	t.Run("config GET", func(t *testing.T) {
		resp := get("/api/config")
		defer resp.Body.Close()
		var cfg map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
			t.Fatal(err)
		}
		if cfg["port"] != 8765.0 {
			t.Fatalf("config 应来自 Handler: %v", cfg)
		}
	})

	t.Run("config POST", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/config", "application/json",
			strings.NewReader(`{"port":9000}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("合法配置应 200，实际 %d", resp.StatusCode)
		}
		if string(app.savedJSON) != `{"port":9000}` {
			t.Fatalf("Handler 应收到原始请求体: %q", app.savedJSON)
		}
		var ok map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&ok); err != nil || ok["ok"] != true {
			t.Fatalf("成功响应应为 {\"ok\":true}: %v %v", ok, err)
		}
	})

	t.Run("config POST 校验失败", func(t *testing.T) {
		app.saveErr = errors.New("配置校验失败: 层越界")
		defer func() { app.saveErr = nil }()
		resp, err := http.Post(srv.URL+"/api/config", "application/json",
			strings.NewReader(`{"bad":1}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("校验失败应为 400，实际 %d", resp.StatusCode)
		}
		var e map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&e); err != nil || e["error"] == nil {
			t.Fatalf("失败响应应带 error 字段: %v", e)
		}
	})

	t.Run("config 不支持的方法", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/config", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("DELETE /api/config 应为 405，实际 %d", resp.StatusCode)
		}
	})

	t.Run("pause", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/pause", "application/json",
			strings.NewReader(`{"paused":true}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("pause 应为 200，实际 %d", resp.StatusCode)
		}
		var st map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		if st["paused"] != true || app.pauseCalls != 1 || !app.pausedFrom[0] {
			t.Fatalf("pause 应转发给 Handler: %v %v", st, app.pausedFrom)
		}
	})

	t.Run("pause 请求体非法", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/pause", "application/json",
			strings.NewReader(`不是json`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法请求体应为 400，实际 %d", resp.StatusCode)
		}
	})

	t.Run("pause 只接受 POST", func(t *testing.T) {
		resp := get("/api/pause")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("GET /api/pause 应为 405，实际 %d", resp.StatusCode)
		}
	})

	t.Run("keylog 快照", func(t *testing.T) {
		before := keylog.Snapshot().LastSeq
		keylog.Record(false, vkOf(t, "q"), true, 2, time.Now())
		waitLog(t, before+1)
		defer func() { // 收尾释放
			keylog.Record(false, vkOf(t, "q"), false, 0, time.Now())
			waitLog(t, before+2)
		}()

		resp := get("/api/keylog")
		defer resp.Body.Close()
		var snap struct {
			Entries []struct {
				Name  string `json:"name"`
				Layer int    `json:"layer"`
			} `json:"entries"`
			RealDown []string `json:"real_down"`
			LastSeq  uint64   `json:"last_seq"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
			t.Fatal(err)
		}
		if snap.LastSeq < before+1 {
			t.Fatalf("快照应包含新事件: last_seq=%d < %d", snap.LastSeq, before+1)
		}
		if len(snap.Entries) == 0 || snap.Entries[len(snap.Entries)-1].Name != "q" || snap.Entries[len(snap.Entries)-1].Layer != 2 {
			t.Fatalf("最新事件应为 q@层2: %+v", snap.Entries)
		}
		if len(snap.RealDown) != 1 || snap.RealDown[0] != "q" {
			t.Fatalf("按住列表应含 q: %v", snap.RealDown)
		}
	})
}

// Start(0) 应由系统分配空闲端口，Stop 后端口释放。
func TestStartEphemeralPort(t *testing.T) {
	port, err := Start(0, &fakeApp{cfg: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if port == 0 {
		t.Fatal("应返回实际监听端口")
	}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status", port))
	if err != nil {
		t.Fatalf("临时端口上的服务不可用: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/status 应为 200，实际 %d", resp.StatusCode)
	}
}
