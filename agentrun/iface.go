package agentrun

import (
	"context"
	"errors"

	"github.com/oagentkit/agent/iface"
)

// Agent 把函数式入口 Run 适配为 iface.Agent（agent#174 窄接口）。
//
// 三入口适配之一（另两个在 goagent-runtime/bridge/agentiface：
// agentloop 与 contract 边界）。不改 Run 的既有语义——本类型只做参数/结果/
// 事件三处映射，装配仍在 Config，重试仍在 WithRetry。
type Agent struct {
	// Cfg 装配（与 Run 的 cfg 同义）。
	Cfg Config
	// Opts 既有选项（WithRetry 等）。Input.OnEvent 若非 nil 会在其后追加
	// 事件回调——WithOnEvent 末个生效，故 **Input.OnEvent 优先于 Opts 里
	// 预设的回调**（窄接口的输入面是本次执行的权威）。
	Opts []RunOption
}

// 编译期断言：接口放在 Agent 层的收益就是这行——实现关系可查证、可锚定 godoc。
// （若接口在装配层，agentrun 只能靠结构化类型隐式满足，这行写不出来。）
var _ iface.Agent = (*Agent)(nil)

// 状态字面量与 kernel contract 常量逐字对齐（contract Stable 只增不改）。
// 本仓是 tier 3、不 import kernel，故不能引用常量——跨仓对齐由
// goagent-runtime/bridge/agentiface 的适配器测试钉住（那边两仓都能 import）。
const (
	statusSucceeded = "succeeded"
	statusFailed    = "failed"
	statusCancelled = "cancelled"
	statusTimedOut  = "timed_out"
)

// Run 实现 iface.Agent。
//
// 映射规则：
//   - 参数：in.Query → query；in.SessionID 忽略（本入口无会话概念，不报错——
//     有会话要求的驱动用会话制适配器）；in.OnEvent → WithOnEvent（末个生效）。
//   - 返回：成功 → Status=succeeded + Final=终答；失败 → Status 按
//     statusOf 映射 + 原样返回 error（失败信息在 error，不塞 Result）。
//   - 事件：agentrun.Event 四字段透传进 iface.Event；Type 沿用原生词表
//     （reasoning/text/tool_call/…），不做翻译。
//   - 取消/超时：ctx 错沿 Run 原样返回，statusOf 按 errors.Is 分类。
func (a *Agent) Run(ctx context.Context, in iface.Input) (iface.Result, error) {
	opts := make([]RunOption, 0, len(a.Opts)+1)
	opts = append(opts, a.Opts...)
	if in.OnEvent != nil {
		cb := in.OnEvent
		opts = append(opts, WithOnEvent(func(e Event) {
			cb(iface.Event{
				Type:   e.Type,
				Text:   e.Text,
				Tool:   e.Tool,
				CallID: e.CallID,
			})
		}))
	}
	out, err := Run(ctx, a.Cfg, in.Query, opts...)
	if err != nil {
		return iface.Result{Status: statusOf(err)}, err
	}
	return iface.Result{Status: statusSucceeded, Final: out}, nil
}

// err → 词表状态（字面量对齐 contract 常量，见常量块注释）。
func statusOf(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return statusTimedOut
	case errors.Is(err, context.Canceled):
		return statusCancelled
	default:
		return statusFailed
	}
}
