package ui

import (
	"bufio"
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
