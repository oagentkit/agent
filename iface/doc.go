// Package iface Agent 执行面的协议中立声明：「一个 Agent 是什么、一次执行怎么读」。
//
// 生命周期: Experimental——agent#174 起步定级；未过一个 minor 无 breaking
// 窗口（api-compat.md 晋级条件一），0.x 阶段按 org 通行约定演进。
//
// # 为什么这个包在 Agent 层（agent#174 裁决）
//
// 「Agent 是什么」是 Agent 层的自我描述（三层边界：Agent = 智能行为与决策编排）。
// 接口放装配层会让思想层的自我描述反向依赖装配层：agentrun 想**显式**声明实现
// （编译期断言 + godoc 锚点）就必须 import 装配层——tier 3 → tier 5 违反分层；
// 靠 Go 结构化类型隐式满足则「实现没实现」无从查证。故接口归本仓，
// 消费方向永远是顺向：装配层 / 宿主 → agent（tier ≥ 3 → tier 3）。
//
// # 窄接口：只定语义，不定装配
//
// 一个方法：Run(ctx, Input) (Result, error)。不承诺回合、工具轮、审批、
// 快照等执行模型/装配专有概念——那些留在各入口原生类型与适配器的原生句柄上
// （与 runtime ADR-0010「共享的是语义与读面，不是控制流抽象」同向；
// 本包是该 ADR 已背书的入口形态 Run(ctx, input) → Result 的类型化）。
//
// # 状态词表不另立
//
// Result.Status 的取值遵循 kernel contract 词表（succeeded / failed /
// timed_out / cancelled …）。本包是 tier 3、**不 import kernel**，因此
// **不定义任何状态常量**——另立常量集就是词表分叉（contract#354/#357 收编过的
// 形态）。字面量与 contract 常量逐字对齐，contract Stable 只增不改；
// 跨仓对齐由适配器测试钉住。
//
// # 事件是原生词表的透传，不是第二套词表
//
// Event.Type 沿用入口原生词表（agentrun 的 reasoning/text/…、eventbus 的
// agent.round_start/…），iface 不做翻译层——两套词表的对照见 agent#174 ADR
// 对照表。需要统一词表的消费方按 ADR-0010 的判据走 kernel eventbus。
package iface
