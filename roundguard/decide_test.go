package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// 合成数据：用中性词面构造事实，不带任何领域味。

// TestDecideTerminalOutcomeBeatsBudget 钉住**结局终局优先**：模型给了终答，哪怕
// 刚好撞上额度上限，也以结局为准（模型说完了就是完了）。
func TestDecideTerminalOutcomeBeatsBudget(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{Completed: true},
		Used:    roundguard.Used{Rounds: 99},
	}, roundguard.Budget{MaxRounds: 1}, roundguard.Config{})
	if d.Reason != roundguard.ReasonCompleted {
		t.Fatalf("终局优先应判 %q，实得 %q", roundguard.ReasonCompleted, d.Reason)
	}
	if d.Continue {
		t.Fatal("终局不该判为可继续")
	}
}

// TestDecidePolicyForceFinal 钉住处置口径分流：强制收尾 ⇒ 不是失败。
func TestDecidePolicyForceFinal(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 5},
	}, roundguard.Budget{MaxRounds: 2, Policy: roundguard.PolicyForceFinal}, roundguard.Config{})
	if d.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonForcedFinal, d.Reason)
	}
	if d.Reason.IsFailure() {
		t.Fatal("强制收尾不该判为失败型")
	}
	if d.Limit != roundguard.LimitRounds {
		t.Fatalf("应命中 %q，实得 %q", roundguard.LimitRounds, d.Limit)
	}
}

// TestDecidePolicyFailFast 钉住：同一额度、另一种口径 ⇒ 失败型。
func TestDecidePolicyFailFast(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 5},
	}, roundguard.Budget{MaxRounds: 2, Policy: roundguard.PolicyFailFast}, roundguard.Config{})
	if d.Reason != roundguard.ReasonBudgetExhausted {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonBudgetExhausted, d.Reason)
	}
	if !d.Reason.IsFailure() {
		t.Fatal("失败即止口径应判为失败型")
	}
}

// TestDecideStalledWinsOverContinue 钉住：停滞 ⇒ 温和叫停（不是失败）。
//
// ⚠️ **停止档的表达方式**：Decision 上没有「停止档」布尔字段——档位由
// (Reason=ReasonStalled, Warn=false, Continue=false) 三项共同表达。
// 「停止档」= 首档（Warn=true，继续观察）之外的那一档，故此处断言
// Warn=false（不是首档）且 Continue=false（已叫停）。
//
// 反向验证：把停止档判成首档（Warn=true / Continue=true）⇒ 本用例红。
func TestDecideStalledWinsOverContinue(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Obs:     repeatSeq(4),
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Reason != roundguard.ReasonStalled {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonStalled, d.Reason)
	}
	if d.Reason.IsFailure() {
		t.Fatal("温和叫停不该判为失败型")
	}
	if d.Warn {
		t.Fatalf("再犯一次应判停止档（不是首档）：%+v", d)
	}
	if d.Continue {
		t.Fatalf("应判停止档：%+v", d)
	}
}

// TestDecideContinuesNormally 钉住正常路径。
func TestDecideContinuesNormally(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
	}, roundguard.Budget{}, roundguard.Config{})
	if !d.Continue {
		t.Fatalf("应判为可继续：%+v", d)
	}
	if d.Reason != roundguard.ReasonToolCalls {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonToolCalls, d.Reason)
	}
}

// TestDecideZeroPolicyDefaultsToForceFinal 钉住**缺省处置口径**落在组合结论上：
// Budget.Policy 零值 = 合法缺省 = 强制收尾（不是失败即止）。
//
// 这条同时钉住「零值不是词表成员」与「零值在 Budget 上是合法缺省」两件事
// **不矛盾**：词表判定问的是「这个取值在不在词表内」，缺省解析问的是
// 「没设时按哪个口径办」——后者由 Budget.EffectivePolicy() 统一回答。
func TestDecideZeroPolicyDefaultsToForceFinal(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 5},
	}, roundguard.Budget{MaxRounds: 2}, roundguard.Config{})
	if d.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("零值口径应缺省为强制收尾（%q），实得 %q", roundguard.ReasonForcedFinal, d.Reason)
	}
	if d.Reason.IsFailure() {
		t.Fatal("缺省口径（强制收尾）不该判为失败型")
	}
}

// TestDecideEffectivePolicyMatchesDecide 钉住**导出面与判定面同源**：
// Budget.EffectivePolicy()（缺省解析口）与 Decide 产出的结论必须一致——
// 宿主拿 EffectivePolicy() 渲染「本次按哪个口径处置」，若它与 Decide 的
// 实际分流不同步，宿主展示的就是假的。
func TestDecideEffectivePolicyMatchesDecide(t *testing.T) {
	cases := []struct {
		policy roundguard.StopPolicy
		want   roundguard.Reason
	}{
		{roundguard.PolicyForceFinal, roundguard.ReasonForcedFinal},
		{roundguard.PolicyFailFast, roundguard.ReasonBudgetExhausted},
		{"", roundguard.ReasonForcedFinal},
	}
	for _, c := range cases {
		t.Run(string(c.policy), func(t *testing.T) {
			b := roundguard.Budget{MaxRounds: 2, Policy: c.policy}
			d := roundguard.Decide(roundguard.Snapshot{
				Outcome: roundguard.Outcome{ToolCalls: 1},
				Used:    roundguard.Used{Rounds: 5},
			}, b, roundguard.Config{})
			if d.Reason != c.want {
				t.Fatalf("口径 %q 下应判 %q，实得 %q", c.policy, c.want, d.Reason)
			}
			eff := b.EffectivePolicy()
			if c.policy == "" {
				if eff != roundguard.PolicyForceFinal {
					t.Fatalf("零值口径应解析为 %q，实得 %q", roundguard.PolicyForceFinal, eff)
				}
				return
			}
			if eff != c.policy {
				t.Fatalf("显式口径应原样保留 %q，实得 %q", c.policy, eff)
			}
		})
	}
}

// TestDecideToolCallsLimitWins 钉住：工具调用总额度用尽 ⇒ 命中该维度。
func TestDecideToolCallsLimitWins(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{ToolCalls: 9},
	}, roundguard.Budget{MaxToolCalls: 3}, roundguard.Config{})
	if d.Limit != roundguard.LimitToolCalls {
		t.Fatalf("应命中 %q，实得 %q", roundguard.LimitToolCalls, d.Limit)
	}
	if d.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("零值口径下应判 %q，实得 %q", roundguard.ReasonForcedFinal, d.Reason)
	}
}

// TestDecideCarriesUsageSnapshot 钉住留痕面：结论带判定时的用量快照。
func TestDecideCarriesUsageSnapshot(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 4, ToolCalls: 2},
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Used.Rounds != 4 || d.Used.ToolCalls != 2 {
		t.Fatalf("结论应带用量快照：%+v", d.Used)
	}
}

// TestDecideNoLimitNoShapeOnNormalPath 钉住「未命中」的**显式取值**：
// Limit=LimitNone、Shape=ShapeNone——不是空串。
//
// 为什么要在意：空串在词表判定里是「未设 / 读不懂」，而这里是「确实没命中」
// ——两者混起来，宿主读到空串时无法区分「没命中」与「宿主忘了填」。
func TestDecideNoLimitNoShapeOnNormalPath(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Limit != roundguard.LimitNone {
		t.Fatalf("未命中额度应判 %q，实得 %q", roundguard.LimitNone, d.Limit)
	}
	if d.Shape != roundguard.ShapeNone {
		t.Fatalf("未命中形态应判 %q，实得 %q", roundguard.ShapeNone, d.Shape)
	}
	if d.Warn {
		t.Fatalf("未命中不该判首档：%+v", d)
	}
}
