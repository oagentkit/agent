package roundguard_test

import (
	"strconv"
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// 合成数据：指纹用中性字母串，不带任何领域味。

func obs(fingerprint string, o roundguard.Observation) roundguard.Observation {
	o.Fingerprint = fingerprint
	return o
}

// repeatSeq 造 n 个**同指纹**回合。
func repeatSeq(n int) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, obs("a", roundguard.Observation{}))
	}
	return out
}

// failSeq 造 n 个**同指纹且全部失败**的回合。
func failSeq(n int) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, obs("a", roundguard.Observation{Failed: true}))
	}
	return out
}

// altSeq 造 n 个 A/B **交替**回合。
func altSeq(n int) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		fp := "a"
		if i%2 == 1 {
			fp = "b"
		}
		out = append(out, obs(fp, roundguard.Observation{}))
	}
	return out
}

// driftSeq 造 n 个**归一指纹相同、原指纹不同**的回合。
func driftSeq(n int) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, roundguard.Observation{Fingerprint: "p" + strconv.Itoa(i), Normalized: "pN"})
	}
	return out
}

// idleSeq 造 n 个**零工具、未收敛**的回合（指纹各异，免得先命中重复）。
func idleSeq(n int) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, obs("i"+strconv.Itoa(i), roundguard.Observation{}))
	}
	return out
}

// TestDetectRepeat 钉住精确重复：同一指纹连着来。
func TestDetectRepeat(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeRepeat {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeRepeat, f.Shape)
	}
	if !f.Warn || f.Stalled {
		t.Fatalf("刚凑够阈值应为首档（Warn）：%+v", f)
	}
}

// TestDetectRepeatEscalates 钉住定档：再犯一次 ⇒ 停止档。
func TestDetectRepeatEscalates(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if !f.Stalled || f.Warn {
		t.Fatalf("再犯一次应判停止档：%+v", f)
	}
}

// TestDetectResultChangedBreaksStreak 钉住：相同动作但**结果变了**是进展，
// 不计入重复。
func TestDetectResultChangedBreaksStreak(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{ResultChanged: true}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeRepeat {
		t.Fatalf("结果变了不该判重复：%+v", f)
	}
}

// TestDetectOscillation 钉住震荡：A B A B …
func TestDetectOscillation(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("b", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("b", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("b", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeOscillation {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeOscillation, f.Shape)
	}
}

// TestDetectDrift 钉住缓慢漂移：归一后同、原指纹不同。
func TestDetectDrift(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "page=1", Normalized: "page=N"},
		{Fingerprint: "page=2", Normalized: "page=N"},
		{Fingerprint: "page=3", Normalized: "page=N"},
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeDrift {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeDrift, f.Shape)
	}
}

// TestDetectErrorSpiralBeatsRepeat 钉住判定次序：错误螺旋优先于重复（病因更具体）。
func TestDetectErrorSpiralBeatsRepeat(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{Failed: true}),
		obs("a", roundguard.Observation{Failed: true}),
		obs("a", roundguard.Observation{Failed: true}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeErrorSpiral {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeErrorSpiral, f.Shape)
	}
}

// TestDetectIdle 钉住只规划不动手。
func TestDetectIdle(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("b", roundguard.Observation{}),
		obs("c", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeIdle {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeIdle, f.Shape)
	}
}

// TestDetectIdleNotTriggeredBySettled 钉住：已收敛的回合不算空转。
func TestDetectIdleNotTriggeredBySettled(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{Settled: true}),
		obs("b", roundguard.Observation{Settled: true}),
		obs("c", roundguard.Observation{Settled: true}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeIdle {
		t.Fatalf("已收敛不该判空转：%+v", f)
	}
}

// TestDetectExemptIsSkipped 钉住豁免面：Exempt 的回合不参与计数
// （轮询类动作 / 刻意的规划步——smolagents 的 planning_interval 就是后者）。
func TestDetectExemptIsSkipped(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{Exempt: true}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeRepeat {
		t.Fatalf("豁免回合不该计入重复：%+v", f)
	}
}

// TestDetectEmptyIsNone 钉住：没有可判的回合 ⇒ 不判任何形态。
func TestDetectEmptyIsNone(t *testing.T) {
	f := roundguard.Detect(nil, roundguard.Config{})
	if f.Shape != roundguard.ShapeNone || f.Warn || f.Stalled {
		t.Fatalf("空输入应判无形态：%+v", f)
	}
}

// TestShapeTableOutletIsSortedAndComplete 钉住：枚举出口与词表逐字一致。
func TestShapeTableOutletIsSortedAndComplete(t *testing.T) {
	got := roundguard.AllShapes()
	if len(got) != 6 {
		t.Fatalf("无进展形态词表应有 6 值，实得 %d（%v）", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("枚举出口应排序，实得 %v", got)
		}
	}
	for _, s := range got {
		if !roundguard.Shape(s).Valid() {
			t.Fatalf("枚举出口给出的 %q 不在词表内", s)
		}
	}
}

// TestShapeZeroValueIsNotAMember 钉住零值语义：空串不是词表成员（未设 ≠ 合法形态）。
func TestShapeZeroValueIsNotAMember(t *testing.T) {
	var s roundguard.Shape
	if s.Valid() {
		t.Fatal("零值不该被判为词表成员")
	}
}
