package roundguard_test

import (
	"testing"

	"github.com/oekit/pkg/errors"

	"github.com/oagentkit/agent/roundguard"
)

// ── 以下为反向验证用例（优先级逐对钉住 / 档位分流 / 无生产者债的闭合）──

// TestDecidePriorityOutcomeBeatsBudget 钉住优先级**第一对**：结局终局 vs 额度。
//
// 模型给了终答（可用的产物已经拿到）而额度刚好也撞了 ⇒ 以结局为准：
// 额度用尽不该把一个**已经完成**的回合改判成「预算耗尽」。
//
// 反向验证：把额度判定提到结局判定之前 ⇒ 本用例红（同时
// TestDecideTerminalOutcomeBeatsBudget 也红）。
func TestDecidePriorityOutcomeBeatsBudget(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{Completed: true},
		Used:    roundguard.Used{Rounds: 99},
	}, roundguard.Budget{MaxRounds: 1}, roundguard.Config{})
	if d.Reason != roundguard.ReasonCompleted {
		t.Fatalf("结局终局应压过额度，实得 %q", d.Reason)
	}
	if d.Limit != roundguard.LimitNone {
		t.Fatalf("终局不该报命中额度，实得 %q", d.Limit)
	}
	if d.Continue {
		t.Fatal("终局不该判为可继续")
	}
}

// TestDecidePriorityOutcomeBeatsStall 钉住优先级**第二对**：结局终局 vs 无进展。
//
// 回合历史里有重复形态，但本回合**已经终答** ⇒ 以结局为准。
//
// 为什么单独钉：终局优先这条分支挡的是「额度」与「无进展」**两处**改写，
// 只钉额度那处 ⇒ 把无进展判定提到结局之前仍全绿（而一个已终答的回合
// 被改判成「停滞」= 宿主把可用产物当失败上报）。
//
// 反向验证：把无进展判定提到结局判定之前 ⇒ 本用例红。
func TestDecidePriorityOutcomeBeatsStall(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{Completed: true},
		Obs:     repeatSeq(4),
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Reason != roundguard.ReasonCompleted {
		t.Fatalf("结局终局应压过无进展，实得 %q", d.Reason)
	}
	if d.Shape != roundguard.ShapeNone {
		t.Fatalf("终局不该报命中形态，实得 %q", d.Shape)
	}
	if d.Limit != roundguard.LimitNone {
		t.Fatalf("终局不该报命中额度，实得 %q", d.Limit)
	}
}

// TestDecidePriorityBudgetBeatsStall 钉住优先级**第三对**：额度 vs 无进展。
//
// 额度用尽与无进展命中同时成立 ⇒ 以**额度**为准（更硬的那条：续跑必然再撞
// 同一个上限，而无进展只是「这一段在打转」）。
//
// 反向验证：把无进展判定提到额度判定之前 ⇒ 本用例红。
func TestDecidePriorityBudgetBeatsStall(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 9},
		Obs:     repeatSeq(4),
	}, roundguard.Budget{MaxRounds: 2}, roundguard.Config{})
	if d.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("额度应压过无进展，实得 %q", d.Reason)
	}
	if d.Limit != roundguard.LimitRounds {
		t.Fatalf("应命中 %q，实得 %q", roundguard.LimitRounds, d.Limit)
	}
	if d.Shape != roundguard.ShapeNone {
		t.Fatalf("额度先判时不该报命中形态，实得 %q", d.Shape)
	}
}

// TestDecidePriorityRoundsBeforeToolCalls 钉住额度两维之间的次序：
// 回合数额度与工具调用总额度**同时**用尽 ⇒ 报回合数那一维。
//
// 为什么有次序：两维都撞上时，续跑必然再撞**回合数**那个上限（回合数是
// 外层边界，工具调用额是内层）⇒ 报外层那一维，宿主的处置（少转一圈 /
// 转强制收尾）与留痕才对得上。
//
// 反向验证：把两维次序对调 ⇒ 本用例红（Limit 变成 LimitToolCalls）。
func TestDecidePriorityRoundsBeforeToolCalls(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 9, ToolCalls: 9},
	}, roundguard.Budget{MaxRounds: 2, MaxToolCalls: 2}, roundguard.Config{})
	if d.Limit != roundguard.LimitRounds {
		t.Fatalf("两维同时用尽应报 %q，实得 %q", roundguard.LimitRounds, d.Limit)
	}
}

// TestDecidePolicySplitOnSameBudget 钉住**处置口径分流**：同一份额度、两种口径
// ⇒ 两条不同结论，且失败型标记随之翻转。
//
// 反向验证：让两种 policy 走同一条分支 ⇒ 本用例红。
func TestDecidePolicySplitOnSameBudget(t *testing.T) {
	snap := roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 9},
	}
	soft := roundguard.Decide(snap, roundguard.Budget{MaxRounds: 2, Policy: roundguard.PolicyForceFinal}, roundguard.Config{})
	hard := roundguard.Decide(snap, roundguard.Budget{MaxRounds: 2, Policy: roundguard.PolicyFailFast}, roundguard.Config{})
	if soft.Reason != roundguard.ReasonForcedFinal {
		t.Fatalf("强制收尾口径应判 %q，实得 %q", roundguard.ReasonForcedFinal, soft.Reason)
	}
	if hard.Reason != roundguard.ReasonBudgetExhausted {
		t.Fatalf("失败即止口径应判 %q，实得 %q", roundguard.ReasonBudgetExhausted, hard.Reason)
	}
	if soft.Reason.IsFailure() {
		t.Fatal("强制收尾不该判为失败型")
	}
	if !hard.Reason.IsFailure() {
		t.Fatal("失败即止该判为失败型")
	}
	if soft.Limit != hard.Limit {
		t.Fatalf("两种口径命中同一维额度，实得 %q vs %q", soft.Limit, hard.Limit)
	}
}

// TestDecideFirstTierWarnsAndContinues 钉住**处置分档**在组合结论上的落点：
// 刚凑够阈值（首档）⇒ 只警告回灌、**不停止**（Continue 仍为 true）。
//
// 反向验证：把首档也判成停止（Continue=false）⇒ 本用例红；
// 把首档判成停止档（Warn=false）⇒ 本用例也红。
func TestDecideFirstTierWarnsAndContinues(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Obs:     repeatSeq(3),
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Reason != roundguard.ReasonStalled {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonStalled, d.Reason)
	}
	if !d.Warn {
		t.Fatalf("刚凑够阈值应为首档（Warn）：%+v", d)
	}
	if !d.Continue {
		t.Fatalf("首档只该警告回灌、不停止：%+v", d)
	}
	if d.Limit != roundguard.LimitStalled {
		t.Fatalf("首档同样命中 %q，实得 %q", roundguard.LimitStalled, d.Limit)
	}
	if d.Shape != roundguard.ShapeRepeat {
		t.Fatalf("应报形态 %q，实得 %q", roundguard.ShapeRepeat, d.Shape)
	}
}

// TestDecideStopTierStops 钉住停止档：再犯一次 ⇒ 温和叫停（不是失败）。
//
// 反向验证：把停止档判成首档（Warn=true）⇒ 本用例红；
// 把停止档判成可继续（Continue=true）⇒ 本用例也红。
func TestDecideStopTierStops(t *testing.T) {
	d := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Obs:     repeatSeq(4),
	}, roundguard.Budget{}, roundguard.Config{})
	if d.Reason != roundguard.ReasonStalled {
		t.Fatalf("应判 %q，实得 %q", roundguard.ReasonStalled, d.Reason)
	}
	if d.Warn {
		t.Fatalf("再犯一次不该判首档：%+v", d)
	}
	if d.Continue {
		t.Fatalf("停止档该温和叫停：%+v", d)
	}
	if d.Reason.IsFailure() {
		t.Fatal("温和叫停不该判为失败型")
	}
}

// TestDecideFailureTypeOutcomesAreTerminal 钉住**失败型结局**在组合结论上
// 仍是终局（不被额度 / 无进展改写），且失败型标记为真。
//
// 反向验证：把失败型结局当成可继续 ⇒ 本用例红。
func TestDecideFailureTypeOutcomesAreTerminal(t *testing.T) {
	cases := []struct {
		name string
		out  roundguard.Outcome
		want roundguard.Reason
	}{
		{"报错", roundguard.Outcome{Failed: errors.New("合成失败")}, roundguard.ReasonFailed},
		{"中止", roundguard.Outcome{Aborted: true}, roundguard.ReasonAborted},
		{"零产出", roundguard.Outcome{}, roundguard.ReasonNoOutcome},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := roundguard.Decide(roundguard.Snapshot{
				Outcome: c.out,
				Used:    roundguard.Used{Rounds: 99},
				Obs:     repeatSeq(4),
			}, roundguard.Budget{MaxRounds: 1}, roundguard.Config{})
			if d.Reason != c.want {
				t.Fatalf("应判 %q，实得 %q", c.want, d.Reason)
			}
			if d.Continue {
				t.Fatal("失败型结局不该判为可继续")
			}
			if !d.Reason.IsFailure() {
				t.Fatalf("%q 该判为失败型", c.want)
			}
		})
	}
}

// TestDecideGracefulTerminalOutcomesAreTerminal 钉住**温和型终局**同样不被
// 额度 / 无进展改写，且失败型标记为假。
//
// 反向验证：把温和型终局当成可继续 ⇒ 本用例红。
func TestDecideGracefulTerminalOutcomesAreTerminal(t *testing.T) {
	cases := []struct {
		name string
		out  roundguard.Outcome
		want roundguard.Reason
	}{
		{"交接", roundguard.Outcome{Handoff: true}, roundguard.ReasonHandedOff},
		{"终答", roundguard.Outcome{Completed: true}, roundguard.ReasonCompleted},
		{"强制收尾步", roundguard.Outcome{ForcedFinal: true}, roundguard.ReasonForcedFinal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := roundguard.Decide(roundguard.Snapshot{
				Outcome: c.out,
				Used:    roundguard.Used{Rounds: 99},
				Obs:     repeatSeq(4),
			}, roundguard.Budget{MaxRounds: 1}, roundguard.Config{})
			if d.Reason != c.want {
				t.Fatalf("应判 %q，实得 %q", c.want, d.Reason)
			}
			if d.Continue {
				t.Fatal("终局不该判为可继续")
			}
			if d.Reason.IsFailure() {
				t.Fatalf("%q 不该判为失败型", c.want)
			}
		})
	}
}

// TestDecideContinuableOutcomesGoThroughBudgetAndStall 钉住「结局要求继续」
// 的两种结局（要工具 / 原样再问）**确实会**继续走额度与无进展两道判定——
// 不是直接返回「继续」。
//
// 反向验证：把「要工具」当终局 ⇒ 本用例红。
func TestDecideContinuableOutcomesGoThroughBudgetAndStall(t *testing.T) {
	t.Run("要工具撞额度", func(t *testing.T) {
		d := roundguard.Decide(roundguard.Snapshot{
			Outcome: roundguard.Outcome{ToolCalls: 1},
			Used:    roundguard.Used{Rounds: 9},
		}, roundguard.Budget{MaxRounds: 2}, roundguard.Config{})
		if d.Reason != roundguard.ReasonForcedFinal {
			t.Fatalf("应判 %q，实得 %q", roundguard.ReasonForcedFinal, d.Reason)
		}
	})
	t.Run("原样再问撞停滞", func(t *testing.T) {
		d := roundguard.Decide(roundguard.Snapshot{
			Outcome: roundguard.Outcome{Paused: true},
			Obs:     repeatSeq(4),
		}, roundguard.Budget{}, roundguard.Config{})
		if d.Reason != roundguard.ReasonStalled {
			t.Fatalf("应判 %q，实得 %q", roundguard.ReasonStalled, d.Reason)
		}
	})
	t.Run("原样再问无异常", func(t *testing.T) {
		d := roundguard.Decide(roundguard.Snapshot{
			Outcome: roundguard.Outcome{Paused: true},
		}, roundguard.Budget{}, roundguard.Config{})
		if !d.Continue {
			t.Fatalf("原样再问应允许继续：%+v", d)
		}
		if d.Reason != roundguard.ReasonPaused {
			t.Fatalf("应判 %q，实得 %q", roundguard.ReasonPaused, d.Reason)
		}
	})
}

// TestDecideProducesRegisteredReasons 钉住**词表成员的生产者闭合**：
// ReasonBudgetExhausted / ReasonStalled / LimitStalled 三个此前「已登记但
// 无生产者」的取值，在组合结论上都有生产路径。
//
// 为什么这条重要：一个登记了却**永远产不出来**的取值，等于一张写着
// 「本表有此值」的死条目——宿主按它写分支，那段分支永远走不到。
//
// 反向验证：把预算耗尽判成强制收尾（或反之）⇒ 本用例红；
// 把停滞判成别的结局 ⇒ 本用例红。
func TestDecideProducesRegisteredReasons(t *testing.T) {
	exhausted := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 9},
	}, roundguard.Budget{MaxRounds: 2, Policy: roundguard.PolicyFailFast}, roundguard.Config{})
	if exhausted.Reason != roundguard.ReasonBudgetExhausted {
		t.Fatalf("应产出 %q，实得 %q", roundguard.ReasonBudgetExhausted, exhausted.Reason)
	}
	if !exhausted.Reason.Valid() {
		t.Fatal("产出的结局不在词表内")
	}
	stalled := roundguard.Decide(roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Obs:     repeatSeq(4),
	}, roundguard.Budget{}, roundguard.Config{})
	if stalled.Reason != roundguard.ReasonStalled {
		t.Fatalf("应产出 %q，实得 %q", roundguard.ReasonStalled, stalled.Reason)
	}
	if stalled.Limit != roundguard.LimitStalled {
		t.Fatalf("应产出 %q，实得 %q", roundguard.LimitStalled, stalled.Limit)
	}
	if !stalled.Limit.Valid() {
		t.Fatal("产出的额度类别不在词表内")
	}
}

// TestDecideIsDeterministic 钉住**纯函数**性质：同一份输入两次判定结论
// 逐字相同（组合判定面不做任何隐式记账——记账是 Meter 的活）。
//
// ⚠️ 为什么逐字段比而不是直接比结构体：`Decision.Used` 内含 `Tools` 映射 ⇒
// 含映射的结构体**不可比较**（Go 的类型规则，`==` 编译不过）。逐字段比既是
// 唯一能写的写法，也顺带把「结论的哪几个字段参与同一性」写明了。
//
// 反向验证：让结论依赖调用顺序 / 随机 ⇒ 本用例红。
func TestDecideIsDeterministic(t *testing.T) {
	snap := roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 1},
		Obs:     repeatSeq(4),
	}
	b := roundguard.Budget{MaxRounds: 5, Policy: roundguard.PolicyFailFast}
	a := roundguard.Decide(snap, b, roundguard.Config{})
	c := roundguard.Decide(snap, b, roundguard.Config{})
	if a.Reason != c.Reason || a.Continue != c.Continue || a.Limit != c.Limit ||
		a.Shape != c.Shape || a.Warn != c.Warn || a.Used.Rounds != c.Used.Rounds {
		t.Fatalf("同一输入两次判定应逐字相同：%+v vs %+v", a, c)
	}
}
