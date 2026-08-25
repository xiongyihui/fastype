// Package keylog 异步记录两类按键事件——
//   - 真实按键：平台钩子看到的物理键盘事件；
//   - 模拟按键：注入的映射输出（Windows SendInput / macOS CGEventPost）。
//
// 钩子线程对延迟最敏感，只做一次非阻塞 channel 发送（缓冲满则丢弃并计数），
// 由后台 goroutine 统一落库、维护"当前按住"状态并唤醒 Web UI 的 SSE 推送。
package keylog

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"fastype/internal/keys"
)

const (
	keep    = 512  // 内存中保留的最近事件条数（超出裁掉最旧的）
	chanCap = 1024 // 钩子线程 → 日志 goroutine 的缓冲深度
)

// Entry 是一条按键事件记录。
type Entry struct {
	Seq   uint64  `json:"seq"`
	T     int64   `json:"t"`   // Unix 毫秒
	Sim   bool    `json:"sim"` // true = 模拟注入；false = 真实按键
	VK    keys.VK `json:"vk"`
	Name  string  `json:"name"`
	Down  bool    `json:"down"`
	Layer int     `json:"layer"` // 真实事件记录时刻的活动层；暂停期间/模拟事件为 -1
}

// Snap 是 /api/keylog 与 SSE 快照消息的载荷。
type Snap struct {
	Entries  []Entry  `json:"entries"`   // 旧 → 新
	RealDown []string `json:"real_down"` // 当前真实按住（按按下先后排序）
	SimDown  []string `json:"sim_down"`  // 当前模拟按住
	Layer    int      `json:"layer"`
	LastSeq  uint64   `json:"last_seq"`
	Dropped  uint64   `json:"dropped"` // 缓冲满被丢弃的事件数
}

type rawEvent struct {
	sim   bool
	vk    keys.VK
	down  bool
	layer int
	t     time.Time
}

type logger struct {
	mu      sync.Mutex
	seq     uint64
	entries []Entry
	real    map[keys.VK]uint64 // 真实按住中：vk → 按下时的 seq
	sim     map[keys.VK]uint64 // 模拟按住中
	layer   int                // 最近一次真实事件时的活动层
	dropped atomic.Uint64

	ch     chan rawEvent
	notify chan struct{} // 容量 1：唤醒各 SSE 流；信号合并没关系，流自身有定时器兜底
}

var std logger

func init() {
	std.real = map[keys.VK]uint64{}
	std.sim = map[keys.VK]uint64{}
	std.ch = make(chan rawEvent, chanCap)
	std.notify = make(chan struct{}, 1)
	go std.loop()
}

// Record 从钩子/注入路径调用：非阻塞投递，绝不阻塞按键处理。
func Record(sim bool, vk keys.VK, down bool, layer int, t time.Time) {
	select {
	case std.ch <- rawEvent{sim: sim, vk: vk, down: down, layer: layer, t: t}:
	default:
		std.dropped.Add(1)
	}
}

// Snapshot 返回最近的事件与当前真实/模拟按住的键。
func Snapshot() Snap { return std.snapshot() }

// EntriesAfter 返回 seq 之后的新事件；调用方用首条 Seq 是否恰为 seq+1 检测环形缓冲已越过水位。
func EntriesAfter(seq uint64) []Entry { return std.entriesAfter(seq) }

// CurrentLayer 返回最近一次真实事件时的活动层。
func CurrentLayer() int { return std.currentLayer() }

// Notify 返回 SSE 流等待的唤醒信号（容量 1，信号合并）。
func Notify() <-chan struct{} { return std.notify }

func (l *logger) loop() {
	for ev := range l.ch {
		l.apply(ev)
		l.wake()
	}
}

func (l *logger) apply(ev rawEvent) {
	l.mu.Lock()
	l.seq++
	e := Entry{
		Seq: l.seq, T: ev.t.UnixMilli(), Sim: ev.sim,
		VK: ev.vk, Name: keys.Name(ev.vk), Down: ev.down, Layer: ev.layer,
	}
	l.entries = append(l.entries, e)
	if excess := len(l.entries) - keep; excess > 0 {
		l.entries = append(l.entries[:0], l.entries[excess:]...)
	}
	held := &l.real
	if ev.sim {
		held = &l.sim
	}
	if ev.down {
		if _, ok := (*held)[ev.vk]; !ok { // 自动重复不改变按住顺序
			(*held)[ev.vk] = e.Seq
		}
	} else {
		delete(*held, ev.vk)
	}
	if !ev.sim && ev.layer >= 0 {
		l.layer = ev.layer
	}
	l.mu.Unlock()
}

func (l *logger) wake() {
	select {
	case l.notify <- struct{}{}:
	default:
	}
}

func (l *logger) snapshot() Snap {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]Entry, len(l.entries))
	copy(entries, l.entries)
	return Snap{
		Entries:  entries,
		RealDown: heldNames(l.real),
		SimDown:  heldNames(l.sim),
		Layer:    l.layer,
		LastSeq:  l.seq,
		Dropped:  l.dropped.Load(),
	}
}

func (l *logger) currentLayer() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.layer
}

func (l *logger) entriesAfter(seq uint64) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	lo := sort.Search(len(l.entries), func(i int) bool { return l.entries[i].Seq > seq })
	if lo == len(l.entries) {
		return nil
	}
	return append([]Entry(nil), l.entries[lo:]...)
}

func heldNames(m map[keys.VK]uint64) []string {
	type kv struct {
		vk  keys.VK
		seq uint64
	}
	ps := make([]kv, 0, len(m))
	for vk, seq := range m {
		ps = append(ps, kv{vk, seq})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].seq < ps[j].seq })
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = keys.Name(p.vk)
	}
	return names
}
