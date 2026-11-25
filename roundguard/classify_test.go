package roundguard_test

import (
	"testing"

	"github.com/oekit/pkg/errors"

	"github.com/oagentkit/agent/roundguard"
)

// 合成数据：用中性词面构造事实，不带任何领域味。

// TestClassifyToolCallsContinues 钉住：模型请求工具 ⇒ 允许进入下一回合。
func TestClassifyToolCallsContinues(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{ToolCalls: 2})
	if r.Reason != roundguard.ReasonToolCalls {
		t.Fatalf("要工具应判 %q，实得 %q", roundguard.ReasonToolCalls, r.Reason)
	}
	if !r.Continue {
		t.Fatal("要工具应允许继续")
	}
}

// TestClassifyToolCallsBeatsCompleted 钉住**判定次序**：模型同时给终答与工具调用时，
// 「还要工具」才是控制流——终答排前面会把未执行的工具判成已完成。
func TestClassifyToolCallsBeatsCompleted(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{ToolCalls: 1, Completed: true})
	if r.Reason != roundguard.ReasonToolCalls {
		t.Fatalf("同时给终答与工具调用应判 %q，实得 %q", roundguard.ReasonToolCalls, r.Reason)
	}
}

// TestClassifyForcedFinalBeatsCompleted 钉住：强制收尾步同时满足 Completed ⇒
// 判强制收尾（宿主据此知道「这是被上限逼出来的收尾」）。
func TestClassifyForcedFinalBeatsCompleted(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Completed: true, ForcedFinal: true})
	if r.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("强制收尾步应判 %q，实得 %q", roundguard.ReasonForcedFinal, r.Reason)
	}
	if r.Continue {
		t.Fatal("强制收尾步不该判为可继续（要转收束）")
	}
}

// TestClassifyZeroOutcomeIsConservative 钉住零值形态：宿主忘了填 Outcome 时判收敛，
// 而不是「无事发生所以继续」——那会让空循环空转到超时。
func TestClassifyZeroOutcomeIsConservative(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{})
	if r.Reason != roundguard.ReasonNoOutcome {
		t.Fatalf("零值形态应判 %q，实得 %q", roundguard.ReasonNoOutcome, r.Reason)
	}
	if r.Continue {
		t.Fatal("零值形态不该判为可继续")
	}
}

// TestIsFailureSeparatesGracefulFromFailure 钉住失败型与温和型可区分。
func TestIsFailureSeparatesGracefulFromFailure(t *testing.T) {
	failures := []roundguard.Reason{
		roundguard.ReasonFailed,
		roundguard.ReasonNoOutcome,
		roundguard.ReasonAborted,
		roundguard.ReasonBudgetExhausted,
	}
	for _, r := range failures {
		if !r.IsFailure() {
			t.Fatalf("%q 应判为失败型", r)
		}
	}
	graceful := []roundguard.Reason{
		roundguard.ReasonForcedFinal,
		roundguard.ReasonStalled,
		roundguard.ReasonCompleted,
		roundguard.ReasonHandedOff,
		roundguard.ReasonToolCalls,
		roundguard.ReasonPaused,
	}
	for _, r := range graceful {
		if r.IsFailure() {
			t.Fatalf("%q 不该判为失败型", r)
		}
	}
}

// TestReasonTableOutletIsSortedAndComplete 钉住：枚举出口与词表逐字一致。
func TestReasonTableOutletIsSortedAndComplete(t *testing.T) {
	got := roundguard.AllRoundReasons()
	if len(got) != 10 {
		t.Fatalf("回合结局词表应有 10 值，实得 %d（%v）", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("枚举出口应排序，实得 %v", got)
		}
	}
	for _, s := range got {
		if !roundguard.Reason(s).Valid() {
			t.Fatalf("枚举出口给出的 %q 不在词表内", s)
		}
	}
}

// TestReasonZeroValueIsNotAMember 钉住零值语义：空串不是词表成员（未设 ≠ 合法结局）。
func TestReasonZeroValueIsNotAMember(t *testing.T) {
	var r roundguard.Reason
	if r.Valid() {
		t.Fatal("零值不该被判为词表成员")
	}
}

// TestClassifyAborted 钉住：被中止 ⇒ 失败型收敛，不允许再转一圈。
func TestClassifyAborted(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Aborted: true})
	if r.Reason != roundguard.ReasonAborted {
		t.Fatalf("中止应判 %q，实得 %q", roundguard.ReasonAborted, r.Reason)
	}
	if r.Continue {
		t.Fatal("中止不该判为可继续")
	}
}

// TestClassifyFailed 钉住：报错 ⇒ 失败型收敛。
func TestClassifyFailed(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Failed: errors.New("合成失败")})
	if r.Reason != roundguard.ReasonFailed {
		t.Fatalf("报错应判 %q，实得 %q", roundguard.ReasonFailed, r.Reason)
	}
	if r.Continue {
		t.Fatal("报错不该判为可继续")
	}
}

// TestClassifyHandoff 钉住：请求交接 ⇒ 收敛并移交（不再要工具）。
func TestClassifyHandoff(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Handoff: true})
	if r.Reason != roundguard.ReasonHandedOff {
		t.Fatalf("请求交接应判 %q，实得 %q", roundguard.ReasonHandedOff, r.Reason)
	}
	if r.Continue {
		t.Fatal("请求交接不该判为可继续")
	}
}

// TestClassifyCompleted 钉住：给出终答 ⇒ 收敛。
func TestClassifyCompleted(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Completed: true})
	if r.Reason != roundguard.ReasonCompleted {
		t.Fatalf("给出终答应判 %q，实得 %q", roundguard.ReasonCompleted, r.Reason)
	}
	if r.Continue {
		t.Fatal("给出终答不该判为可继续")
	}
}

// TestClassifyPaused 钉住：服务端要求原样再问 ⇒ 允许进入下一回合。
func TestClassifyPaused(t *testing.T) {
	r := roundguard.Classify(roundguard.Outcome{Paused: true})
	if r.Reason != roundguard.ReasonPaused {
		t.Fatalf("要求原样再问应判 %q，实得 %q", roundguard.ReasonPaused, r.Reason)
	}
	if !r.Continue {
		t.Fatal("要求原样再问应允许继续")
	}
}

// TestClassifyPriorityChain 钉住**判定次序**整体：优先级链上每一对相邻臂各给
// 一条成对断言——每次只留相邻两条事实，断言先者胜。
//
// 为什么必须**逐对**钉：只断言「全部事实同时置真时胜者是中止」只钉住链头，
// 中间两条换位照样全绿（实测：把 Handoff 移到 Failed 之前、Completed 移到
// Paused 之前，14 条用例无一变红，而「报错回合被静默重分类成温和交接」已经
// 发生——IsFailure() 由 true 翻 false，宿主据此不上报故障）。
//
// 逐对钉住后，任何一条臂换位都至少撞上一对断言：链上相邻关系全对 ⇒ 全序不变。
func TestClassifyPriorityChain(t *testing.T) {
	failed := errors.New("合成失败")
	cases := []struct {
		name string
		o    roundguard.Outcome
		want roundguard.Reason
	}{
		{"中止压报错", roundguard.Outcome{Aborted: true, Failed: failed}, roundguard.ReasonAborted},
		{"报错压交接", roundguard.Outcome{Failed: failed, Handoff: true}, roundguard.ReasonFailed},
		{"交接压要工具", roundguard.Outcome{Handoff: true, ToolCalls: 1}, roundguard.ReasonHandedOff},
		{"要工具压强制收尾", roundguard.Outcome{ToolCalls: 1, ForcedFinal: true}, roundguard.ReasonToolCalls},
		{"强制收尾压终答", roundguard.Outcome{ForcedFinal: true, Completed: true}, roundguard.ReasonForcedFinal},
		{"终答压原样再问", roundguard.Outcome{Completed: true, Paused: true}, roundguard.ReasonCompleted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := roundguard.Classify(c.o)
			if got.Reason != c.want {
				t.Fatalf("应判 %q，实得 %q", c.want, got.Reason)
			}
		})
	}
}

// TestReasonString 钉住：短码就是取值本身（展示文案归宿主，内核只给机器可读码）。
func TestReasonString(t *testing.T) {
	if got := roundguard.ReasonToolCalls.String(); got != "tool_calls" {
		t.Fatalf("短码应为 %q，实得 %q", "tool_calls", got)
	}
	if got := roundguard.Reason("").String(); got != "" {
		t.Fatalf("零值短码应为空串，实得 %q", got)
	}
}
