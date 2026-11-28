package iface

import "context"

// Input 一次执行的输入。
type Input struct {
	// Query 本轮输入（用户消息 / 任务指令）。空串是否合法由实现方裁
	// （有会话历史的入口可空，一次性入口通常要求非空）。
	Query string
	// SessionID 会话锚点。空 = 一次性执行——没有会话概念的入口（agentrun）
	// 天然为空；会话制入口（agentloop）要求非空并由适配器透传。
	SessionID string
	// OnEvent 过程事件回调（可 nil）。
	//
	// 纪律与各入口既有约定一致：回调 panic **不由 iface 代为 recover**
	// （与 agentrun.WithOnEvent 同约——panic 击穿调用方 goroutine 是既有
	// 契约，收编 recover 会改变既有消费方的故障形态）。
	// 不支持事件翻译的实现必须**显式拒绝**（返回错误），不得静默丢弃：
	// 静默丢事件等于对消费方撒谎「没有事件」。
	OnEvent func(Event)
}

// Result 一次执行的读面（窄面：身份与进度的丰富面留在各入口原生类型上，
// 需要时经适配器的原生句柄取，如 agentloop.Result 的 RunID/Rounds/Uncertain）。
type Result struct {
	// Status 状态词表取值，遵循 kernel contract 词表
	// （succeeded / failed / timed_out / cancelled …）。
	// 本包不定义状态常量（见包文档「状态词表不另立」）。
	Status string
	// Final 终答文本（Status=succeeded 时有值）；失败信息由 error 承载。
	Final string
}

// Event 过程事件（观测面）。
//
// Type 沿用**入口原生词表**不做二次抽象：agentrun 侧是 reasoning/text/
// tool_call/…（含 delta），agentloop 侧经 eventbus 是 agent.round_start/…
// ——对照见 agent#174 ADR 对照表。Payload 承载入口特有载荷原样透传。
type Event struct {
	Type    string
	Text    string
	Tool    string
	CallID  string
	Payload map[string]any
}

// Agent 窄执行接口：只定语义不定装配。
//
// 实现方（适配器）负责把 Input 映射到原生入口、把原生结果映射回 Result；
// 装配（Config / Store / ApprovalGate …）不在本接口的语义内——那是宿主的事。
type Agent interface {
	// Run 执行一次。ctx 取消/超时语义由实现方按原入口纪律透传
	// （取消 → 在途工作收场 → 返回 ctx 错；词表状态由适配器映射）。
	Run(ctx context.Context, in Input) (Result, error)
}
