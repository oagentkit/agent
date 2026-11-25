package roundguard

import "slices"

// roundReasonAll 回合结局词表 → 成员（单一事实源）。
//
// 判定（Reason.Valid）与枚举出口（AllRoundReasons）都读这一张表，新增取值
// 只需改一处。
//
// 键是 **Reason**（不是 string）：键就是常量本身，成员判定与枚举出口都不必
// 转字符串——键型与取值型不一致时，多一次转换就多一处能写错的地方。
var roundReasonAll = map[Reason]bool{
	ReasonCompleted:       true,
	ReasonToolCalls:       true,
	ReasonHandedOff:       true,
	ReasonPaused:          true,
	ReasonFailed:          true,
	ReasonNoOutcome:       true,
	ReasonAborted:         true,
	ReasonBudgetExhausted: true,
	ReasonForcedFinal:     true,
	ReasonStalled:         true,
}

// AllRoundReasons 返回回合结局词表的全集（排序后的**新切片**）。
//
// 宿主据此渲染结局选择器 / 校验落库值，不必手抄常量清单——手抄清单必然漂移：
// 内核新增一个取值，宿主那份不会跟着变，且没有任何判据会红（issue #356）。
// 返回的是副本：调用方改写它不影响判定面。
func AllRoundReasons() []string {
	out := make([]string, 0, len(roundReasonAll))
	for s := range roundReasonAll {
		out = append(out, string(s))
	}
	slices.Sort(out)
	return out
}

// stopPolicyAll 处置口径词表 → 成员（单一事实源）。
//
// 键是 **StopPolicy**（与 roundReasonAll 同款：键就是常量本身，成员判定与枚举
// 出口都不必转字符串——键型与取值型不一致时，多一次转换就多一处能写错的地方）。
var stopPolicyAll = map[StopPolicy]bool{
	PolicyForceFinal: true,
	PolicyFailFast:   true,
}

// AllStopPolicies 返回处置口径词表的全集（排序后的**新切片**）。
//
// 返回的是副本：调用方改写它不影响判定面。
func AllStopPolicies() []string {
	out := make([]string, 0, len(stopPolicyAll))
	for s := range stopPolicyAll {
		out = append(out, string(s))
	}
	slices.Sort(out)
	return out
}

// roundLimitAll 额度类别词表 → 成员（单一事实源）。
//
// 键是 **Limit**（同款理由）。
var roundLimitAll = map[Limit]bool{
	LimitNone:      true,
	LimitRounds:    true,
	LimitToolCalls: true,
	LimitPerTool:   true,
	LimitStalled:   true,
}

// AllLimits 返回额度类别词表的全集（排序后的**新切片**）。
//
// 返回的是副本：调用方改写它不影响判定面。
func AllLimits() []string {
	out := make([]string, 0, len(roundLimitAll))
	for s := range roundLimitAll {
		out = append(out, string(s))
	}
	slices.Sort(out)
	return out
}

// roundShapeAll 无进展形态词表 → 成员（单一事实源）。
//
// 键是 **Shape**（与 roundReasonAll 同款理由：键就是常量本身，成员判定与
// 枚举出口都不必转字符串）。
//
// ⚠️ 键型不能写 map[string]bool：门禁的「取值派生」走 AST——键得是**常量名**
// （*ast.Ident）才能解析出取值；写成 map[string]bool{string(X): true} 的话键是
// 一个转换表达式，派生不出取值 ⇒ 当天即红（登记与事实已偏离）。
var roundShapeAll = map[Shape]bool{
	ShapeNone:        true,
	ShapeRepeat:      true,
	ShapeOscillation: true,
	ShapeDrift:       true,
	ShapeErrorSpiral: true,
	ShapeIdle:        true,
}

// AllShapes 返回无进展形态词表的全集（排序后的**新切片**）。
//
// 返回的是副本：调用方改写它不影响判定面。
func AllShapes() []string {
	out := make([]string, 0, len(roundShapeAll))
	for s := range roundShapeAll {
		out = append(out, string(s))
	}
	slices.Sort(out)
	return out
}
