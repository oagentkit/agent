package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── 组合结论：结局 × 额度 × 无进展 的全组合对照 ──

// TestMutationMatrixDecideExhaustive 钉住组合结论 Decide 的**整个组合面**。
//
// 穷举：8 种结局 × 5 份额度声明 × 2 份用量 × 3 种回合历史 = 240 种组合，逐条
// 与**独立写成的规格 oracle**（按 spec §4.2 的次序写死，含「先回合数、后工具
// 调用总数」这一条）对照。
//
// 为什么必须穷举：Decide 的结论由**三段判据的组合**决定，而每段各自都对时
// 组合仍可能错（「额度与无进展同时成立时该报哪一个」「终局结局是否被额度改写」
// 这两类只有组合才显形）。
//
// 反向验证：把任一段判定提前 / 把额度两维次序对调 / 把处置口径分流并成一条 ⇒ 红。
func TestMutationMatrixDecideExhaustive(t *testing.T) {
	outcomes := []roundguard.Outcome{
		{},                     // 零值形态（保守收敛）
		{ToolCalls: 1},         // 要工具（可继续）
		{Completed: true},      // 终答
		{Paused: true},         // 原样再问（可继续）
		{Failed: errSynthetic}, // 报错
		{Aborted: true},        // 中止
		{Handoff: true},        // 交接
		{ForcedFinal: true},    // 强制收尾步
	}
	budgets := []roundguard.Budget{
		{},                              // 不限额
		{MaxRounds: 1},                  // 只限回合数
		{MaxToolCalls: 1},               // 只限工具调用总数
		{MaxRounds: 1, MaxToolCalls: 1}, // 两维都限（缺省口径）
		{MaxRounds: 1, MaxToolCalls: 1, Policy: roundguard.PolicyFailFast}, // 两维都限（失败口径）
	}
	useds := []roundguard.Used{
		{},
		{Rounds: 5, ToolCalls: 5},
	}
	histories := [][]roundguard.Observation{
		nil,                                     // 无进展
		mxSeq(3, mxOpts{fps: mxSame, tools: 1}), // 首档
		mxSeq(4, mxOpts{fps: mxSame, tools: 1}), // 停止档
	}
	for oi, out := range outcomes {
		for bi, b := range budgets {
			for ui, used := range useds {
				for hi, hist := range histories {
					snap := roundguard.Snapshot{Outcome: out, Used: used, Obs: hist}
					got := roundguard.Decide(snap, b, roundguard.Config{})
					want := oracleDecide(snap, b, roundguard.Config{})
					if !sameDecision(got, want) {
						t.Fatalf("组合 %d/%d/%d/%d：实现判 %+v，规格 oracle 判 %+v",
							oi, bi, ui, hi, got, want)
					}
				}
			}
		}
	}
}

// TestMutationMatrixDecideBudgetDimensionOrder 钉住额度两维的次序：两者同时
// 用尽 ⇒ 报**回合数**那一维。
//
// 为什么单列：这是 Task 4 新增的语义（spec §4.2 / §6.3），而「报哪一维」只在
// **两维同时撞上**时才显形 ⇒ 只钉单维的用例看不出次序。
//
// 反向验证：把两维次序对调 ⇒ 本用例红（Limit 变成工具调用总数那一维）。
func TestMutationMatrixDecideBudgetDimensionOrder(t *testing.T) {
	snap := roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{Rounds: 5, ToolCalls: 5},
	}
	d := roundguard.Decide(snap, roundguard.Budget{MaxRounds: 1, MaxToolCalls: 1}, roundguard.Config{})
	if d.Limit != roundguard.LimitRounds {
		t.Fatalf("两维同时用尽应报 %q，实得 %q", roundguard.LimitRounds, d.Limit)
	}
	// 反过来只撞工具调用总数那一维时，报的必须是那一维（别被回合数那维盖住）。
	only := roundguard.Snapshot{
		Outcome: roundguard.Outcome{ToolCalls: 1},
		Used:    roundguard.Used{ToolCalls: 5},
	}
	e := roundguard.Decide(only, roundguard.Budget{MaxToolCalls: 1}, roundguard.Config{})
	if e.Limit != roundguard.LimitToolCalls {
		t.Fatalf("只撞工具调用总额度应报 %q，实得 %q", roundguard.LimitToolCalls, e.Limit)
	}
}
