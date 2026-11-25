package roundguard

// Outcome 一回合结束后的机械事实（宿主注入；内核不调模型、不认识工具语义）。
type Outcome struct {
	// ToolCalls 本回合请求的工具调用次数（>0 ⇒ 模型还要工具）。
	ToolCalls int
	// Completed 模型给出了终答。
	Completed bool
	// Handoff 模型请求交接。
	Handoff bool
	// Paused 服务端要求原样再问（pause_turn）。
	Paused bool
	// Failed 模型调用或工具执行的错误（nil = 无）。
	Failed error
	// Aborted ctx 被取消。
	Aborted bool
	// ForcedFinal 本回合是「额度耗尽后的强制收尾步」：宿主已禁用工具，要求模型
	// 用已收集的证据给出终答。它是**产出仍然可用**的收束，不是失败。
	ForcedFinal bool
}

// Route 结局分类结果。
type Route struct {
	// Reason 回合结局分类。
	Reason Reason
	// Continue 允许进入下一回合。
	Continue bool
}

// Classify 回合结局分类（纯函数）。
//
// 判定顺序（**先具体后笼统**，取证据最硬的那条）：
//
//  1. o.Aborted       → ReasonAborted
//  2. o.Failed != nil → ReasonFailed
//  3. o.Handoff       → ReasonHandedOff
//  4. o.ToolCalls > 0 → ReasonToolCalls（要工具优先于终答）
//  5. o.ForcedFinal   → ReasonForcedFinal（强制收尾优先于普通终答）
//  6. o.Completed     → ReasonCompleted
//  7. o.Paused        → ReasonPaused
//  8. 都不成立         → ReasonNoOutcome（零值形态，保守收敛）
//
// 为什么「要工具」排在「给终答」前面：模型可以同时给文本与工具调用；此时「还要
// 工具」才是决定下一步的事实（文本是给用户看的，工具调用才是控制流）。把终答排
// 前面会让「模型一边收尾一边补一个工具调用」被误判成已完成——而工具**还没执行**，
// 判完成就是谎报已完成。
func Classify(o Outcome) Route {
	switch {
	case o.Aborted:
		return Route{Reason: ReasonAborted}
	case o.Failed != nil:
		return Route{Reason: ReasonFailed}
	case o.Handoff:
		return Route{Reason: ReasonHandedOff}
	case o.ToolCalls > 0:
		return Route{Reason: ReasonToolCalls, Continue: true}
	case o.ForcedFinal:
		return Route{Reason: ReasonForcedFinal}
	case o.Completed:
		return Route{Reason: ReasonCompleted}
	case o.Paused:
		return Route{Reason: ReasonPaused, Continue: true}
	default:
		return Route{Reason: ReasonNoOutcome}
	}
}
