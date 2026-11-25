package roundguard

import (
	"maps"
	"slices"

	"github.com/oekit/pkg/errors"
)

// StopPolicy 额度用尽后的处置口径（判定词表；主人表 stopPolicyAll）。
//
// 五家主流实现三种口径（spec §1.4 有实测对照）：pydantic-ai 与 Claude 是硬失败；
// agentloop 把工具预算做成强制收尾；OpenAI 装了 max-turn handler 就产出合成输出；
// smolagents 触顶后单独再调一次模型产出终答。⇒ 处置是**策略**，不是事实。
type StopPolicy string

const (
	// PolicyForceFinal 强制收尾：不再给工具（模型面），要求模型用已收集的证据
	// 给出终答；产物仍然可用（不是失败）。**缺省**。
	PolicyForceFinal StopPolicy = "force_final"
	// PolicyFailFast 直接失败：本次运行以失败收场。
	PolicyFailFast StopPolicy = "fail_fast"
)

// Valid 报告该处置口径是否在词表内。
//
// ⚠️ **零值（空串）不是有效口径**——它不是「第三种口径」，而是「未设」。
//
// 但这与 `Budget.Policy` 的零值**不矛盾**：零值放在 `Budget.Policy` 上是
// **合法缺省**（= 强制收尾，见 `Budget.EffectivePolicy`）。两件事问的是不同的
// 问题：本方法答「这个**取值**在不在词表内」，缺省解析答「没设时按哪个口径办」。
//
// ⇒ **宿主不要用本方法校验 `Budget.Policy`**：那会把合法缺省判成非法。
// 校验预算声明用 `Budget.Validate`；要拿生效口径用 `Budget.EffectivePolicy`。
func (p StopPolicy) Valid() bool { return stopPolicyAll[p] }

// Limit 额度类别（判定词表；主人表 roundLimitAll，枚举出口 AllLimits）。
type Limit string

const (
	// LimitNone 未因额度叫停。
	LimitNone Limit = "none"
	// LimitRounds 回合数额度用尽（处置按 Budget.Policy）。
	LimitRounds Limit = "rounds"
	// LimitToolCalls 工具调用总额度用尽（处置按 Budget.Policy）。
	LimitToolCalls Limit = "tool_calls"
	// LimitPerTool 单工具额度用尽 ⇒ 该次调用被拒。
	LimitPerTool Limit = "per_tool"
	// LimitStalled 无进展（由 Detect 产出，见 detect.go）。
	LimitStalled Limit = "stalled"
)

// Valid 报告该额度类别是否在词表内。
func (l Limit) Valid() bool { return roundLimitAll[l] }

// Budget 回合预算（宿主给；内核不解释工具标识的含义）。
//
// **额度维度（事实）与处置口径（策略）分开**——前者各家一致，后者没有共识
// （详见 spec §6.1.1 与 §1.4 的五家实测对照）。
type Budget struct {
	// MaxRounds 回合数上限（0 = 不限）。
	MaxRounds int
	// MaxToolCalls 工具调用总数上限（0 = 不限）。
	MaxToolCalls int
	// MaxPerTool 单工具调用上限（开放键；值为 0 = 该工具不限）。
	// 工具标识是宿主词表——内核只做键存在性与计数判定，不解释它是哪个工具。
	MaxPerTool map[string]int
	// Policy 额度用尽后的处置口径（零值 = PolicyForceFinal）。
	Policy StopPolicy
}

// Validate 校验预算声明（装配期用）。非法值一律报错，不静默降级为「不限」。
//
// 零值 = **显式关闭**对应额度（合法的「不限」）；**负值一律非法**——把一个写错的
// 负值默默当成「不限」跑一整轮，是典型的 fail-open；宁可装配期报错。
// 另：MaxPerTool 的键不得为空串（空标识无法计数，等于声明了一个无法判的上限）。
//
// 键按字典序遍历（不是 map 随机序）：多个键同时非法时，报出的那一个是**确定的**
// ——同一份声明在两次装配里报出不同的键，调用方就没法把它当成稳定事实。
func (b Budget) Validate() error {
	if b.MaxRounds < 0 {
		return errors.Wrap(ErrInvalidBudget, "roundguard: 回合数上限不能为负（负值不是「不限」）")
	}
	if b.MaxToolCalls < 0 {
		return errors.Wrap(ErrInvalidBudget, "roundguard: 工具调用上限不能为负（负值不是「不限」）")
	}
	for _, id := range slices.Sorted(maps.Keys(b.MaxPerTool)) {
		if id == "" {
			return errors.Wrap(ErrInvalidBudget, "roundguard: 单工具上限的键不得为空串（空标识无法计数）")
		}
		if b.MaxPerTool[id] < 0 {
			return errors.Wrap(ErrInvalidBudget, "roundguard: 单工具上限不能为负："+id)
		}
	}
	if b.Policy != "" && !b.Policy.Valid() {
		return errors.Wrap(ErrInvalidBudget, "roundguard: 未知处置口径："+string(b.Policy))
	}
	return nil
}

// EffectivePolicy 缺省处置口径（零值 = 强制收尾）。
//
// 为什么导出：**缺省解析本身就是判定语义**，而它原先只有一个未导出的入口
// （`effectivePolicy`）⇒ 宿主拿不到「本预算实际按哪个口径处置」这个事实。
// 拿不到就只有两条路：要么在宿主侧**手抄**一条缺省规则（每个宿主各抄一遍 ⇒
// 同一份声明在不同宿主得到不同处置，正是本包要消灭的病），要么干脆不判。
//
// ⚠️ 这条方法的存在也消掉了「零值是不是合法口径」的歧义：`StopPolicy.Valid`
// 答的是「这个**取值**在不在词表内」（零值不在），本方法答的是「没设时按哪个
// 口径办」（零值 = 强制收尾）。**宿主校验 `Budget.Policy` 不要用 `Valid`**——
// 那会把合法缺省判成非法；校验预算声明用 `Budget.Validate`。
func (b Budget) EffectivePolicy() StopPolicy { return b.effectivePolicy() }

// effectivePolicy 缺省处置口径（零值 = 强制收尾）。
//
// ⚠️ 本方法与导出的 `EffectivePolicy` 是**同一份判据**（导出面转调它，不是两份
// 实现）：包内与包外走同一条解析路径 ⇒ 缺省口径不会在两侧漂。
func (b Budget) effectivePolicy() StopPolicy {
	if b.Policy == "" {
		return PolicyForceFinal
	}
	return b.Policy
}

// Used 当前用量（回合数 + 工具调用总数 + 按工具的调用次数）。
type Used struct {
	// Rounds 回合数（**不是** provider 请求数——fallback/重试/续跑请求可能更多）。
	Rounds int
	// ToolCalls 工具调用总数（MaxToolCalls 额度用）。
	ToolCalls int
	// Tools 按工具的调用次数（MaxPerTool 额度用）。
	Tools map[string]int
}

// Verdict 准入判定结论。
type Verdict struct {
	// Allowed 是否放行。
	Allowed bool
	// Limit 命中的额度类别（Allowed=true 时恒为 LimitNone）。
	Limit Limit
	// Detail 机械留痕（命中的工具标识等）；**展示文案归宿主**。
	Detail string
}

// CheckRounds 判定下一回合能否开始（**模型调用前**检查；回合数额度）。
//
// 调用后检查意味着**最后一次昂贵调用已经发生** ⇒ 额度判定一律在调用前。
//
// 前置条件：**Budget 须先经 Validate**。未校验的负值上限在这里是 fail-open
// （上限 <= 0 一律判成「不限」）——「负值 = 不限」在语义上是错的，故非法声明
// 必须在装配期被 Validate 拒掉，而不是在这里被默默当成不限。
func CheckRounds(b Budget, used Used) Verdict {
	if b.MaxRounds <= 0 || used.Rounds < b.MaxRounds {
		return Verdict{Allowed: true, Limit: LimitNone}
	}
	return Verdict{Allowed: false, Limit: LimitRounds}
}

// CheckToolCalls 判定本回合还能不能再调工具（**模型调用前**检查；工具调用总额度）。
//
// 前置条件：**Budget 须先经 Validate**。未校验的负值上限在这里是 fail-open
// （上限 <= 0 一律判成「不限」）——「负值 = 不限」在语义上是错的，故非法声明
// 必须在装配期被 Validate 拒掉，而不是在这里被默默当成不限。
func CheckToolCalls(b Budget, used Used) Verdict {
	if b.MaxToolCalls <= 0 || used.ToolCalls < b.MaxToolCalls {
		return Verdict{Allowed: true, Limit: LimitNone}
	}
	return Verdict{Allowed: false, Limit: LimitToolCalls}
}

// CheckTool 判定某工具的下一次调用能否进行（**工具执行前**检查；单工具额度）。
//
// 被拒时宿主仍须回灌一条失败结果给模型——否则 tool_call 没有配对的 tool_result，
// 下一轮请求会被 provider 拒（配对纪律见 watchdog INV1）。
//
// 前置条件：**Budget 须先经 Validate**。未校验的负值上限在这里是 fail-open
// （上限 <= 0 一律判成「不限」）——「负值 = 不限」在语义上是错的，故非法声明
// 必须在装配期被 Validate 拒掉，而不是在这里被默默当成不限。
func CheckTool(b Budget, used Used, toolID string) Verdict {
	limit, ok := b.MaxPerTool[toolID]
	if !ok || limit <= 0 || used.Tools[toolID] < limit {
		return Verdict{Allowed: true, Limit: LimitNone}
	}
	return Verdict{Allowed: false, Limit: LimitPerTool, Detail: toolID}
}
