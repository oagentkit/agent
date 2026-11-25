package roundguard_test

import (
	"strings"
	"testing"

	"github.com/oekit/pkg/errors"

	"github.com/oagentkit/agent/roundguard"
)

// 合成数据：用中性词面构造事实，不带任何领域味。

// TestCheckRoundsBoundary 钉住边界：已用 2、上限 3 ⇒ 放行；已用 3 ⇒ 拒绝。
// 这条是「把 >= 写成 > 会被抓住」的那道反向验证。
func TestCheckRoundsBoundary(t *testing.T) {
	b := roundguard.Budget{MaxRounds: 3}
	if v := roundguard.CheckRounds(b, roundguard.Used{Rounds: 2}); !v.Allowed {
		t.Fatalf("已用 2 / 上限 3 应放行，实得 %+v", v)
	}
	v := roundguard.CheckRounds(b, roundguard.Used{Rounds: 3})
	if v.Allowed {
		t.Fatal("已用 3 / 上限 3 应拒绝")
	}
	if v.Limit != roundguard.LimitRounds {
		t.Fatalf("应命中 %q，实得 %q", roundguard.LimitRounds, v.Limit)
	}
}

// TestCheckRoundsZeroMeansUnlimited 钉住：零值 = 显式关闭该额度（合法）。
func TestCheckRoundsZeroMeansUnlimited(t *testing.T) {
	if v := roundguard.CheckRounds(roundguard.Budget{}, roundguard.Used{Rounds: 999}); !v.Allowed {
		t.Fatal("上限为 0 应表示不限")
	}
}

// TestCheckToolCallsBoundary 钉住工具总额度的边界。
func TestCheckToolCallsBoundary(t *testing.T) {
	b := roundguard.Budget{MaxToolCalls: 2}
	if v := roundguard.CheckToolCalls(b, roundguard.Used{ToolCalls: 1}); !v.Allowed {
		t.Fatal("已用 1 / 上限 2 应放行")
	}
	v := roundguard.CheckToolCalls(b, roundguard.Used{ToolCalls: 2})
	if v.Allowed {
		t.Fatal("已用 2 / 上限 2 应拒绝")
	}
	if v.Limit != roundguard.LimitToolCalls {
		t.Fatalf("应命中 %q，实得 %q", roundguard.LimitToolCalls, v.Limit)
	}
}

// TestCheckToolIsPerToolScoped 钉住：单工具上限只作用于该工具，且 Detail 带工具标识。
func TestCheckToolIsPerToolScoped(t *testing.T) {
	b := roundguard.Budget{MaxPerTool: map[string]int{"read": 2}}
	if v := roundguard.CheckTool(b, roundguard.Used{Tools: map[string]int{"read": 2}}, "read"); v.Allowed {
		t.Fatal("read 已用满应拒绝")
	} else if v.Limit != roundguard.LimitPerTool || v.Detail != "read" {
		t.Fatalf("应命中 %q 且带工具标识，实得 %+v", roundguard.LimitPerTool, v)
	}
	if v := roundguard.CheckTool(b, roundguard.Used{Tools: map[string]int{"read": 2}}, "write"); !v.Allowed {
		t.Fatal("别的工具不该受 read 的上限影响")
	}
}

// TestCheckToolZeroLimitMeansUnlimited 钉住：单工具上限为 0 = 该工具不限
// （与「负值非法」是两种不同语义，别混）。
func TestCheckToolZeroLimitMeansUnlimited(t *testing.T) {
	b := roundguard.Budget{MaxPerTool: map[string]int{"read": 0}}
	if v := roundguard.CheckTool(b, roundguard.Used{Tools: map[string]int{"read": 99}}, "read"); !v.Allowed {
		t.Fatal("单工具上限为 0 应表示不限")
	}
}

// TestCheckToolUnregisteredToolIsUnlimited 钉住：没登记上限的工具不受任何额度约束
// （额度表是**开放键**，不是白名单）。
func TestCheckToolUnregisteredToolIsUnlimited(t *testing.T) {
	b := roundguard.Budget{MaxPerTool: map[string]int{"read": 1}}
	if v := roundguard.CheckTool(b, roundguard.Used{Tools: map[string]int{"write": 99}}, "write"); !v.Allowed {
		t.Fatal("未登记的工具不该被判超限")
	}
}

// TestValidateRejectsIllegalBudgetsWithSentinel 钉住：**每一条**非法预算路径都
// 带同一个哨兵（errors.Is 可判定），且错误文案点名具体的字段 / 键 / 口径。
//
// 为什么必须是表驱动而不是「每条路径各写一条」：docs/error-semantics.md 把
// 「负值上限 / 空工具标识 / 未知处置口径」**三条**都登记在同一个哨兵名下
// （ErrInvalidBudget）。若其中任何一条路径把 errors.Wrap(ErrInvalidBudget, …)
// 写成 errors.New(…)，只断言「err != nil」的用例照样全绿，而宿主的
// errors.Is(err, roundguard.ErrInvalidBudget) 对**那一条路径**静默失效——
// 宿主拿不到哨兵就只能靠文案匹配，判定链断在那一处，且没有任何信号。
//
// 这张表逐条钉住哨兵可判定 + 文案点名，两件事一起断言：文案点名是为了让
// 宿主与排查者知道**哪一维**声明非法，而不是只拿到一个笼统的「预算非法」。
func TestValidateRejectsIllegalBudgetsWithSentinel(t *testing.T) {
	cases := []struct {
		name        string
		budget      roundguard.Budget
		wantMention string
	}{
		{"回合数上限为负", roundguard.Budget{MaxRounds: -1}, "回合数上限"},
		{"工具调用上限为负", roundguard.Budget{MaxToolCalls: -1}, "工具调用上限"},
		{"单工具上限为负", roundguard.Budget{MaxPerTool: map[string]int{"read": -1}}, "read"},
		{"单工具上限键为空", roundguard.Budget{MaxPerTool: map[string]int{"": 1}}, "键不得为空串"},
		{"未知处置口径", roundguard.Budget{Policy: "nonsense"}, "nonsense"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.budget.Validate()
			if err == nil {
				t.Fatalf("非法预算应报错：%+v", c.budget)
			}
			if !errors.Is(err, roundguard.ErrInvalidBudget) {
				t.Fatalf("应能 errors.Is 判定 %v，实得 %v", roundguard.ErrInvalidBudget, err)
			}
			if !strings.Contains(err.Error(), c.wantMention) {
				t.Fatalf("错误文案应点名 %q，实得 %q", c.wantMention, err.Error())
			}
		})
	}
}

// TestValidateReportsDeterministicKey 钉住：多个非法键同时存在时，报出的是
// **字典序第一个**键——不是 map 随机序。
//
// 为什么必须钉：同一份声明在两次装配里报出不同的键，调用方就没法把它当成稳定
// 事实（报出的键进日志与排查记录，随机序 ⇒ 同一份配置两次排查看到两个键）。
//
// 为什么跑 20 遍：map 遍历顺序是**随机**的，单次跑有 1/5 的概率恰好撞上期望
// 值 ⇒ 单次断言是概率性的；跑 20 遍并要求每次都一样，map 随机序的实现几乎
// 必然被抓住（(1/5)^20）。
func TestValidateReportsDeterministicKey(t *testing.T) {
	b := roundguard.Budget{MaxPerTool: map[string]int{
		"beta": -1, "alpha": -1, "gamma": -1, "delta": -1, "epsilon": -1,
	}}
	for i := 0; i < 20; i++ {
		err := b.Validate()
		if err == nil {
			t.Fatal("负值单工具上限应报错")
		}
		if !strings.Contains(err.Error(), "alpha") {
			t.Fatalf("第 %d 次应报字典序第一个键 alpha，实得 %v", i, err)
		}
		for _, other := range []string{"beta", "gamma", "delta", "epsilon"} {
			if strings.Contains(err.Error(), other) {
				t.Fatalf("第 %d 次只该报第一个键，不该出现 %q，实得 %v", i, other, err)
			}
		}
	}
}

// TestValidateAcceptsZeroPolicy 钉住：处置口径零值合法（缺省 = 强制收尾）。
func TestValidateAcceptsZeroPolicy(t *testing.T) {
	if err := (roundguard.Budget{}).Validate(); err != nil {
		t.Fatalf("零值处置口径应合法：%v", err)
	}
}

// TestValidateAcceptsKnownPolicies 钉住：词表内的两种处置口径都合法。
func TestValidateAcceptsKnownPolicies(t *testing.T) {
	for _, p := range []roundguard.StopPolicy{roundguard.PolicyForceFinal, roundguard.PolicyFailFast} {
		if err := (roundguard.Budget{Policy: p}).Validate(); err != nil {
			t.Fatalf("处置口径 %q 应合法：%v", p, err)
		}
	}
}

// TestMeterCountsRoundsAndTools 钉住计量。
func TestMeterCountsRoundsAndTools(t *testing.T) {
	m := roundguard.NewMeter()
	if got := m.Round(); got != 1 {
		t.Fatalf("第一次记回合应得 1，实得 %d", got)
	}
	m.Tool("read")
	if got := m.Tool("read"); got != 2 {
		t.Fatalf("read 累计应得 2，实得 %d", got)
	}
	u := m.Used()
	if u.Rounds != 1 || u.ToolCalls != 2 || u.Tools["read"] != 2 {
		t.Fatalf("用量快照不对：%+v", u)
	}
	u.Tools["read"] = 99
	if m.Used().Tools["read"] != 2 {
		t.Fatal("用量快照必须是副本")
	}
}

// TestMeterUsedIsIndependentSnapshot 钉住：两次取快照互不影响（副本语义）。
func TestMeterUsedIsIndependentSnapshot(t *testing.T) {
	m := roundguard.NewMeter()
	m.Round()
	first := m.Used()
	first.Rounds = 99
	first.Tools["read"] = 99
	second := m.Used()
	if second.Rounds != 1 {
		t.Fatalf("改写上一次的快照不该影响账本，实得 %d", second.Rounds)
	}
	if _, ok := second.Tools["read"]; ok {
		t.Fatal("快照里的工具表不该被调用方凭空造出条目")
	}
}

// TestErrInvalidBudgetIsIdentifiable 钉住哨兵可判定。
func TestErrInvalidBudgetIsIdentifiable(t *testing.T) {
	err := roundguard.Budget{MaxRounds: -1}.Validate()
	if !errors.Is(err, roundguard.ErrInvalidBudget) {
		t.Fatalf("应能 errors.Is 判定 %v", roundguard.ErrInvalidBudget)
	}
}

// TestLimitTableOutletIsSortedAndComplete 钉住：额度类别枚举出口与词表逐字一致。
func TestLimitTableOutletIsSortedAndComplete(t *testing.T) {
	got := roundguard.AllLimits()
	if len(got) != 5 {
		t.Fatalf("额度类别词表应有 5 值，实得 %d（%v）", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("枚举出口应排序，实得 %v", got)
		}
	}
	for _, s := range got {
		if !roundguard.Limit(s).Valid() {
			t.Fatalf("枚举出口给出的 %q 不在词表内", s)
		}
	}
}

// TestStopPolicyTableOutletIsSortedAndComplete 钉住：处置口径枚举出口与词表逐字一致。
func TestStopPolicyTableOutletIsSortedAndComplete(t *testing.T) {
	got := roundguard.AllStopPolicies()
	if len(got) != 2 {
		t.Fatalf("处置口径词表应有 2 值，实得 %d（%v）", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("枚举出口应排序，实得 %v", got)
		}
	}
	for _, s := range got {
		if !roundguard.StopPolicy(s).Valid() {
			t.Fatalf("枚举出口给出的 %q 不在词表内", s)
		}
	}
}

// TestLimitZeroValueIsNotAMember 钉住零值语义：空串不是额度类别（未设 ≠ 合法类别）。
func TestLimitZeroValueIsNotAMember(t *testing.T) {
	var l roundguard.Limit
	if l.Valid() {
		t.Fatal("零值不该被判为词表成员")
	}
}

// TestStopPolicyZeroValueIsNotAMember 钉住零值语义：空串不是处置口径成员
// （缺省由 Budget 语义承担，不是词表里的一个取值）。
func TestStopPolicyZeroValueIsNotAMember(t *testing.T) {
	var p roundguard.StopPolicy
	if p.Valid() {
		t.Fatal("零值不该被判为词表成员")
	}
}
