package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── Meter 的零值可用性（终审 I3）──
//
// # 这个用例在钉什么
//
// 「Meter」是**导出类型**，而 Go 的默认契约是「零值可用」。本包原先只保证
// 「NewMeter()」造出来的实例可用 ⇒ 宿主按惯例写
//
//	var meter roundguard.Meter
//	meter.Tool("read")
//
// 就会在**运行期**炸掉（assignment to entry in nil map）——**编译期完全
// 看不出来**，而宿主循环恰恰是这种写法（把账本当 struct 字段零值起步）。
//
// ⇒ 判据：**零值 Meter 直接调 Tool / Round / Used 不得 panic**。
//
// # 反向验证（坏实现会红）
//
// 把「Tool」里的惰性初始化（if m.perTool == nil { … }）删掉 ⇒ 零值
// 「Tool」立刻 panic ⇒ 本用例红 ⇒ 修复的承重夹具就在这里，而不是靠注释。
//
// 合成数据：工具标识用 read / write（中性词面）。

// TestMeterZeroValueIsUsable 钉住零值 Meter 可直接记账（不得 panic）。
func TestMeterZeroValueIsUsable(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("零值 Meter 记账 panic（宿主按 Go 惯例零值起步就会踩到）：%v", r)
		}
	}()

	var m roundguard.Meter // 零值：不调 NewMeter

	if got := m.Round(); got != 1 {
		t.Fatalf("零值 Meter 首次 Round 应得 1，实得 %d", got)
	}
	if got := m.Tool("read"); got != 1 {
		t.Fatalf("零值 Meter 首次 Tool(read) 应得 1，实得 %d", got)
	}
	if got := m.Tool("read"); got != 2 {
		t.Fatalf("零值 Meter 二次 Tool(read) 应得 2，实得 %d", got)
	}
	if got := m.Tool("write"); got != 1 {
		t.Fatalf("零值 Meter 首次 Tool(write) 应得 1，实得 %d", got)
	}

	u := m.Used()
	if u.Rounds != 1 || u.ToolCalls != 3 {
		t.Fatalf("零值 Meter 用量快照不对：rounds=%d calls=%d（实得 %+v）", u.Rounds, u.ToolCalls, u)
	}
	if u.Tools["read"] != 2 || u.Tools["write"] != 1 {
		t.Fatalf("零值 Meter 按工具计数不对：%+v", u.Tools)
	}
}

// TestMeterZeroValueMatchesNewMeter 钉住「零值起步」与「构造函数起步」等效。
//
// 为什么单独一条：惰性初始化只保证**不 panic**；若它初始化出的状态与
// 「NewMeter()」不一致（例如多算一次调用、或把 tools 表建在锁外导致丢计数），
// 上一条用例是判不出来的 ⇒ 两条宿主的账本会从「同一段循环」得出不同用量。
func TestMeterZeroValueMatchesNewMeter(t *testing.T) {
	var zero roundguard.Meter
	built := roundguard.NewMeter()

	for _, m := range []*roundguard.Meter{&zero, built} {
		m.Round()
		m.Tool("read")
		m.Tool("read")
		m.Tool("write")
	}

	zu, bu := zero.Used(), built.Used()
	if zu.Rounds != bu.Rounds || zu.ToolCalls != bu.ToolCalls {
		t.Fatalf("零值与构造值的前置用量不一致：zero=%+v built=%+v", zu, bu)
	}
	if len(zu.Tools) != len(bu.Tools) {
		t.Fatalf("零值与构造值的按工具表大小不一致：zero=%+v built=%+v", zu.Tools, bu.Tools)
	}
	for id, n := range bu.Tools {
		if zu.Tools[id] != n {
			t.Fatalf("零值与构造值的按工具计数不一致（tool=%s）：zero=%d built=%d", id, zu.Tools[id], n)
		}
	}
}

// TestMeterUsedOnFreshZeroValue 钉住零值 Meter 的 Used 快照本身也可读。
//
// 为什么单独一条：Used 是**读**零值那侧（perTool 为 nil map）——nil map 的
// len / range 是合法的，但若实现改成往里写或解引用，就会在这里炸。
func TestMeterUsedOnFreshZeroValue(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("零值 Meter 的 Used 快照 panic：%v", r)
		}
	}()

	var m roundguard.Meter
	u := m.Used()
	if u.Rounds != 0 || u.ToolCalls != 0 {
		t.Fatalf("零值 Meter 的初始用量应为全 0，实得 %+v", u)
	}
	if len(u.Tools) != 0 {
		t.Fatalf("零值 Meter 的初始按工具表应为空，实得 %+v", u.Tools)
	}
}
