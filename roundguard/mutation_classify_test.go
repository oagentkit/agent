package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── 结局分类：输入空间的全穷举对照 ──

// TestMutationMatrixClassifyExhaustive 钉住结局分类的**整个输入空间**。
//
// 为什么是穷举而不是逐对：结局分类有 7 个输入面（中止 / 报错 / 交接 / 要工具 /
// 强制收尾 / 终答 / 原样再问）⇒ 128 种组合。逐对钉只能覆盖「两两同成立」的
// 情形，而「某臂被删掉」「某个条件被写成 >」这类变异往往只在**三臂同成立**时
// 才显形。穷举 128 种组合 ⇒ 任何一处改动都会与**独立写成的臂表 oracle** 分歧。
//
// 反向验证：把任意两个臂对调 / 删掉一个臂 / 把 Continue 恒为 true ⇒ 红。
func TestMutationMatrixClassifyExhaustive(t *testing.T) {
	const arms = 7
	for mask := 0; mask < 1<<arms; mask++ {
		o := roundguard.Outcome{
			Aborted:     mask&0x01 != 0,
			Handoff:     mask&0x02 != 0,
			ForcedFinal: mask&0x04 != 0,
			Completed:   mask&0x08 != 0,
			Paused:      mask&0x10 != 0,
		}
		if mask&0x20 != 0 {
			o.ToolCalls = 1
		}
		if mask&0x40 != 0 {
			o.Failed = errSynthetic
		}
		got := roundguard.Classify(o)
		want := classifyOracle(o)
		if got != want {
			t.Fatalf("输入组合 %07b：实现判 %q/%v，规格 oracle 判 %q/%v",
				mask, got.Reason, got.Continue, want.Reason, want.Continue)
		}
	}
}

// TestMutationMatrixClassifyContinueOnlyOnContinuable 钉住 Continue 的口径：
// 只有「要工具」与「原样再问」两个结局允许继续；其余（含零值形态）一律收敛。
//
// 为什么单独钉：Continue 恒为 true 的错实现在「每个臂各自可达」的用例上**全绿**
// （每个臂的 Reason 都对）——只有把 Continue 与 Reason 摆在一起逐个断言才红。
func TestMutationMatrixClassifyContinueOnlyOnContinuable(t *testing.T) {
	cases := []struct {
		name string
		out  roundguard.Outcome
		want roundguard.Reason
		cont bool
	}{
		{"要工具", roundguard.Outcome{ToolCalls: 1}, roundguard.ReasonToolCalls, true},
		{"原样再问", roundguard.Outcome{Paused: true}, roundguard.ReasonPaused, true},
		{"终答", roundguard.Outcome{Completed: true}, roundguard.ReasonCompleted, false},
		{"交接", roundguard.Outcome{Handoff: true}, roundguard.ReasonHandedOff, false},
		{"强制收尾", roundguard.Outcome{ForcedFinal: true}, roundguard.ReasonForcedFinal, false},
		{"报错", roundguard.Outcome{Failed: errSynthetic}, roundguard.ReasonFailed, false},
		{"中止", roundguard.Outcome{Aborted: true}, roundguard.ReasonAborted, false},
		{"零值", roundguard.Outcome{}, roundguard.ReasonNoOutcome, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := roundguard.Classify(c.out)
			if r.Reason != c.want {
				t.Fatalf("应判 %q，实得 %q", c.want, r.Reason)
			}
			if r.Continue != c.cont {
				t.Fatalf("%q 的 Continue 应为 %v，实得 %v", c.want, c.cont, r.Continue)
			}
		})
	}
}
