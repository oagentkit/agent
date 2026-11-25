package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── 次数预算：额度判定的全网格对照 ──

// TestMutationMatrixBudgetCheckGrid 钉住三个 Check* 的**整个边界网格**。
//
// 为什么是网格而不是三个单点：额度判定只有一条判据（上限 <= 0 ⇒ 不限；否则
// 已用 < 上限 ⇒ 放行），但**边界位置**多（上限本身 / 已用本身 / 零值 / 未登记
// 工具 / 别的工具）。逐点钉必漏一格，而漏掉的那格恰好是「把 >= 写成 >」的
// 落点之一。
//
// 反向验证：把 < 写成 <= / 把 <= 0 写成 < 0 / 去掉工具维度过滤 ⇒ 红。
func TestMutationMatrixBudgetCheckGrid(t *testing.T) {
	t.Run("回合数额度", func(t *testing.T) {
		for max := 0; max <= 3; max++ {
			for used := 0; used <= 4; used++ {
				b := roundguard.Budget{MaxRounds: max}
				v := roundguard.CheckRounds(b, roundguard.Used{Rounds: used})
				wantAllowed := max <= 0 || used < max
				wantLimit := roundguard.LimitNone
				if !wantAllowed {
					wantLimit = roundguard.LimitRounds
				}
				if v.Allowed != wantAllowed || v.Limit != wantLimit {
					t.Fatalf("上限 %d / 已用 %d：应 %v/%q，实得 %v/%q",
						max, used, wantAllowed, wantLimit, v.Allowed, v.Limit)
				}
			}
		}
	})
	t.Run("工具调用总额度", func(t *testing.T) {
		for max := 0; max <= 3; max++ {
			for used := 0; used <= 4; used++ {
				b := roundguard.Budget{MaxToolCalls: max}
				v := roundguard.CheckToolCalls(b, roundguard.Used{ToolCalls: used})
				wantAllowed := max <= 0 || used < max
				wantLimit := roundguard.LimitNone
				if !wantAllowed {
					wantLimit = roundguard.LimitToolCalls
				}
				if v.Allowed != wantAllowed || v.Limit != wantLimit {
					t.Fatalf("上限 %d / 已用 %d：应 %v/%q，实得 %v/%q",
						max, used, wantAllowed, wantLimit, v.Allowed, v.Limit)
				}
			}
		}
	})
	t.Run("单工具额度", func(t *testing.T) {
		for _, tool := range []string{"read", "write"} {
			for limit := 0; limit <= 2; limit++ {
				for used := 0; used <= 3; used++ {
					// 额度表是**开放键**：只登记 read，write 根本没上限。
					b := roundguard.Budget{MaxPerTool: map[string]int{"read": limit}}
					v := roundguard.CheckTool(b, roundguard.Used{Tools: map[string]int{"read": used}}, tool)
					wantAllowed := tool != "read" || limit <= 0 || used < limit
					wantLimit := roundguard.LimitNone
					wantDetail := ""
					if !wantAllowed {
						wantLimit = roundguard.LimitPerTool
						wantDetail = tool
					}
					if v.Allowed != wantAllowed || v.Limit != wantLimit || v.Detail != wantDetail {
						t.Fatalf("工具 %q / 上限 %d / 已用 %d：应 %v/%q/%q，实得 %v/%q/%q",
							tool, limit, used, wantAllowed, wantLimit, wantDetail,
							v.Allowed, v.Limit, v.Detail)
					}
				}
			}
		}
	})
}

// TestMutationMatrixBudgetZeroMeansUnlimited 钉住「零值 = 显式关闭该额度」
// 这一口径在**三个额度维度**上的一致性。
//
// 为什么单列：零值在三处的落点不同（回合数 / 工具总数 / 单工具），实现里分属
// 三处判据 ⇒ 任何一处把零值当成「上限 0 ⇒ 立刻用尽」，其它两处的用例都照绿。
func TestMutationMatrixBudgetZeroMeansUnlimited(t *testing.T) {
	used := roundguard.Used{Rounds: 999, ToolCalls: 999, Tools: map[string]int{"read": 999}}
	if v := roundguard.CheckRounds(roundguard.Budget{}, used); !v.Allowed {
		t.Fatal("回合数上限为零应表示不限")
	}
	if v := roundguard.CheckToolCalls(roundguard.Budget{}, used); !v.Allowed {
		t.Fatal("工具调用上限为零应表示不限")
	}
	b := roundguard.Budget{MaxPerTool: map[string]int{"read": 0}}
	if v := roundguard.CheckTool(b, used, "read"); !v.Allowed {
		t.Fatal("单工具上限为零应表示该工具不限")
	}
}
