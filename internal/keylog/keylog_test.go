package keylog

import (
	"fmt"
	"testing"
	"time"

	"fastype/internal/keys"
)

func mustVK(t *testing.T, name string) keys.VK {
	t.Helper()
	vk, ok := keys.VkOf(name)
	if !ok {
		t.Fatalf("未知键名 %q", name)
	}
	return vk
}

// seqNow 读取当前已落库的 seq（异步 goroutine 处理）。
func seqNow() uint64 {
	std.mu.Lock()
	defer std.mu.Unlock()
	return std.seq
}

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

func TestHeldStateAndOrder(t *testing.T) {
	s0 := seqNow()
	t0 := time.Now()

	// 真实按下 a s d（层 2），顺序应保留
	Record(false, mustVK(t, "a"), true, 2, t0)
	Record(false, mustVK(t, "s"), true, 2, t0.Add(time.Millisecond))
	Record(false, mustVK(t, "d"), true, 2, t0.Add(2*time.Millisecond))
	// 模拟按下 left
	Record(true, mustVK(t, "left"), true, -1, t0.Add(3*time.Millisecond))
	waitLog(t, s0+4)

	snap := Snapshot()
	if got := fmt.Sprint(snap.RealDown); got != "[a s d]" {
		t.Fatalf("真实按住顺序错误: %v", got)
	}
	if got := fmt.Sprint(snap.SimDown); got != "[left]" {
		t.Fatalf("模拟按住错误: %v", got)
	}
	if snap.Layer != 2 {
		t.Fatalf("层应为 2: %d", snap.Layer)
	}
	ents := snap.Entries
	if len(ents) < 4 || ents[len(ents)-4].Name != "a" || ents[len(ents)-1].Name != "left" {
		t.Fatalf("事件顺序错误: %v", ents)
	}
	if last := ents[len(ents)-1]; !last.Sim || last.Layer != -1 {
		t.Fatalf("模拟事件字段错误: %+v", last)
	}
	for i := len(ents) - 3; i < len(ents); i++ {
		if ents[i].Seq <= ents[i-1].Seq {
			t.Fatalf("seq 应单调递增: %v", ents)
		}
	}

	// 抬起 s → 剩 a d；全部抬起 → 两组均空
	Record(false, mustVK(t, "s"), false, 2, t0)
	waitLog(t, s0+5)
	if got := fmt.Sprint(Snapshot().RealDown); got != "[a d]" {
		t.Fatalf("抬起 s 后应为 [a d]: %v", got)
	}

	Record(false, mustVK(t, "a"), false, 0, t0)
	Record(false, mustVK(t, "d"), false, 0, t0)
	Record(true, mustVK(t, "left"), false, -1, t0)
	waitLog(t, s0+8)
	snap = Snapshot()
	if len(snap.RealDown) != 0 || len(snap.SimDown) != 0 {
		t.Fatalf("全部抬起后不应有按住的键: %v %v", snap.RealDown, snap.SimDown)
	}
	if snap.Layer != 0 {
		t.Fatalf("层应回到 0: %d", snap.Layer)
	}
}

func TestAutoRepeatKeepsOrder(t *testing.T) {
	s0 := seqNow()
	Record(false, mustVK(t, "f"), true, 0, time.Now())
	Record(false, mustVK(t, "g"), true, 0, time.Now())
	Record(false, mustVK(t, "f"), true, 0, time.Now()) // 自动重复
	waitLog(t, s0+3)
	if got := fmt.Sprint(Snapshot().RealDown); got != "[f g]" {
		t.Fatalf("自动重复不应改变按住顺序: %v", got)
	}
	Record(false, mustVK(t, "f"), false, 0, time.Now())
	Record(false, mustVK(t, "g"), false, 0, time.Now())
	waitLog(t, s0+5)
}

func TestEntriesAfter(t *testing.T) {
	s0 := seqNow()
	if ents := EntriesAfter(s0); len(ents) != 0 {
		t.Fatalf("无新事件时应返回空: %v", ents)
	}
	Record(false, mustVK(t, "q"), true, 0, time.Now())
	Record(false, mustVK(t, "q"), false, 0, time.Now())
	waitLog(t, s0+2)

	ents := EntriesAfter(s0)
	if len(ents) != 2 || ents[0].Seq != s0+1 || ents[1].Seq != s0+2 {
		t.Fatalf("增量事件错误: %v", ents)
	}
	if ents[0].Name != "q" || !ents[0].Down || ents[1].Down {
		t.Fatalf("事件内容错误: %v", ents)
	}
}
