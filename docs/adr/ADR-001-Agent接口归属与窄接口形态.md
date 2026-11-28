# ADR-001 · Agent 接口归属与窄接口形态

> **分面声明**（双 ADR 协调，matrix-tools#275）：本 ADR 是**仓内面 canon**（三入口对照表详版、四条归属理由、适配器落点与测试面以本篇为准）；治理面 canon 是 `oagentkit/matrix-tools` 的 `docs/adr/ADR-003-统一Agent执行接口的归属与形态.md`（三方案拒收面 / 门禁接线 / 与 ADR-001/002/ADR-0010 的集成关系）。两篇核心裁决逐条一致，实现锚点以 `oagentkit/agent/iface` 与 `yi-nology/goagent-runtime/agentiface` 为准。

| 项 | 内容 |
|---|---|
| **状态** | 已采纳（Accepted） |
| **日期** | 2026-11-28 |
| **背景** | oagentkit 演进评审 2.3-问题一（Agent 层自己没有接口面，「一个 Agent 是什么」无从谈起）+ 2.4-阶段一（优先稳定 Agent 接口）+ 问题三（统一执行语义）。issue `oagentkit/agent#174`。 |
| **决策** | ① 接口**放 Agent 层**：新包 `agent/iface`（`Run(ctx, Input) (Result, error)` 窄接口，纯声明、零依赖）；② **不放装配层**（拒收面见 §二）；③ 三入口各写适配器**不强迁存量**（agentrun 在本仓、agentloop 与 contract 在 runtime `bridge/agentiface`）；④ 状态词表**不另立**（Result.Status 遵循 kernel contract 词表，本包不定义常量）；⑤ 定级 Experimental 起步（api-lifecycle，matrix-tools#256 判据）。 |
| **后果** | 宿主/装配层拿到一个可查证的「Agent」契约（编译期断言 `var _ iface.Agent = (*Agent)(nil)` 可写、godoc 有锚点）；三套既有入口经适配器证明可承载，存量零迁移；新增执行入口时实现一个方法即可接入。 |
| **依据** | 三入口**实测签名**（2026-11-28，见 §一）；三层边界 `matrix-tools` ADR-001（Agent 层职责）；runtime `ADR-0010`（共享的是语义与读面，不是控制流抽象——本接口是其背书形态 `Run(ctx, input) → Result` 的类型化）；kernel `contract` 包（Stable，状态词表单源）。 |

> **一句话结论**：「Agent 是什么」归 Agent 层回答——`agent/iface` 一个方法的窄接口，
> 装配层与宿主顺向消费，三入口适配器证明可承载，词表归 kernel、本包一字不立。

---

## 一、三入口实况对照表（实测，非评审转述）

⚠️ **事实勘误**：评审原文写「`contract.Run/Step/Interrupt/Resume`——纯接口契约面」。
实测（2026-11-28）`goagent-kernel/contract` 包只有 `contract.go` + `status.go`，
**没有这四个方法**——它是编排**边界接口**（`RunContext` / `StepSink` / `Tools` /
`Clock`）+ 状态词表，本身不提供运行入口。三入口中的第三个因此是
「**宿主经 contract 边界驱动**」的形态（适配器 §四据此写）。红线 3（先验证再
动手）在本单的第一次应用。

| | ① `agentrun.Run`（函数式，本仓） | ② `agentloop.Loop.Run` / `runtime.Session.Run`（结构体方法，runtime） | ③ contract 边界（kernel，Stable） |
|---|---|---|---|
| **参数** | `ctx, cfg Config, query string, opts ...RunOption`——query 即输入；装配在 `Config`（Model/Instruction/工具）；`WithRetry` 可选 | `ctx, sessionID, user`——会话锚点必填（`Session.Run` 免 sessionID）；装配在 `New(Config)`（Model/Tools/Store 必填；ApprovalGate/Joint/Budget 可选） | **无运行参数**——宿主把领域结构适配进 `RunContext`（ID/Status/SetStatus/CurrentStep/Field）+ `StepSink`，引擎只认接口 |
| **返回** | `(string, error)`——只有终答文本，成败全在 error | `(*Result, error)`——Result 带 RunID/Status/Final/Rounds/ToolCalls/Uncertain/EmitFailed/PendingCallID（挂起等审批的调用锚点） | 不返回执行结果——状态读面在 `RunContext.Status()`（词表常量 created/running/succeeded/failed/cancelled/timed_out…） |
| **事件** | `WithOnEvent(func(Event))` 回调；词表 reasoning/text/tool_call/…（含 delta 与 attempt_retry）；末个生效；**回调 panic 不代 recover** | **kernel eventbus 事件面**：agent.round_start/content_delta/tool_call/tool_result/round_end/session_status（联合提交/水位线判据，ADR-0010 Event 行）；`Config.Emitter` 可换发射面 | 不在本面——事件判据归 kernel eventbus（ADR-0010：Event 共享面 = eventbus） |
| **取消/中断** | ctx 取消/超时沿 eino 调用链传播 → 返回 ctx 错；**无会话、无恢复点**（失败即结束） | ctx 取消 → 在途步骤收场 → 快照可恢复（runtime W3 #40）；挂起态经 `Approve(callID, …)` 续跑；快照序号闸拒并发（ErrStaleSeq） | 不定义取消——ctx 由宿主/引擎传播；词表有 `interrupted`/`cancelled` 取值 |

差异一句话：**①是一次性函数、②是会话制可恢复循环、③是宿主状态边界**——
三者共享的只有「一次执行有输入、有终态、可观测、可取消」这四件事，
这四件事就是窄接口的全部内容。

## 二、裁决：接口放 Agent 层（`agent/iface`），不放装配层

**放 Agent 层的四条理由**：

1. **语义归属**：「一个 Agent 是什么」是 Agent 层的自我描述（matrix-tools
   ADR-001 三层职责：Agent = 智能行为与决策编排）。装配层回答「怎么把它们
   装起来」，不回答「Agent 是什么」——把身份面放进装配层，等于让 assembly
   反向定义思想层概念。
2. **实现关系可查证**：接口在本仓，`agentrun` 才能写编译期断言
   `var _ iface.Agent = (*Agent)(nil)` 与 godoc 锚点。若接口在装配层，
   tier 3 → tier 5 的 import 被分层禁止，agentrun 只能靠 Go 结构化类型
   **隐式**满足——「实现没实现」没有任何机器可查的痕迹（这正是评审问题一
   「无从谈起」在工程面的形态）。
3. **消费方向永远顺向**：需要这个接口的是宿主与装配层（tier ≥ 5），
   它们 import agent 是 5→3 顺向；agent 永远不需要 import 装配层。
   方向反了就会出现「思想层为了被识别而依赖装配层」的倒挂。
4. **定级链现成**：agent 已按 matrix-tools#256 接入 api-lifecycle
   （第一批），新包 `iface` 直接按判据标 Experimental 起步；
   接口放装配层会让「Agent 接口」的定级挂在装配层账上（其定级是第二批），
   阶段一「优先稳定 Agent 接口」的验收反而悬空。

**为什么不放装配层（拒收面）**：

- ❌ 「适配器都在装配层，接口放装配层最省事」——适配器看得见三套语义是
  **tier 5 的物理事实**，不等于**概念归属**在 tier 5；适配器放装配层
  （§四就是这么放的），接口不需要跟着走。
- ❌ 「runtime ADR-0010 已经裁了共享语义，接口当然归 runtime」——ADR-0010 裁的
  是**执行模型之间**（Loop vs Pipeline）共享什么、不共享什么；本单裁的是
  **入口形态**的类型化。两者互为镜像（本单与 runtime#39 的关系，issue 关联
  已写）：ADR-0010 背书形态 `Run(ctx, input) → Result`，本单把它落成类型，
  状态词表仍归 contract——**没有第二套语义，只有同一套语义的类型化**。
- ❌ 「先放 runtime，等消费方多了再搬」——搬 = 破坏性改 import 路径
  （跨 org breaking，matrix-tools ADR-001 §三改名裁决同款成本论证）；
  起点放错，代价是全矩阵下游。

## 三、形态（窄接口的全部内容）

```go
package iface // github.com/oagentkit/agent/iface —— 纯声明，零依赖（仅 stdlib）

type Input struct {
    Query     string        // 本轮输入
    SessionID string        // 会话锚点；空 = 一次性执行
    OnEvent   func(Event)   // 过程事件回调（可 nil；panic 不代 recover；不支持必须显式拒绝）
}

type Result struct {
    Status string            // 遵循 kernel contract 词表——本包不定义状态常量
    Final  string            // 终答（succeeded 时有值）
}

type Event struct {
    Type    string           // 沿用入口原生词表，不做翻译层
    Text    string
    Tool    string
    CallID  string
    Payload map[string]any
}

type Agent interface {
    Run(ctx context.Context, in Input) (Result, error)
}
```

三条自我约束（防窄接口长胖）：

1. **状态词表不另立**：Result.Status 只透传 contract 词表取值；本包不定义
   任何状态常量（另立常量集 = 词表分叉，contract#354/#357 收编过的形态）。
   字面量与 contract 常量逐字对齐，contract Stable 只增不改，跨仓对齐由
   适配器测试钉住。
2. **事件不做翻译层**：Event.Type 沿用入口原生词表（两套词表的对照在 §一
   事件列）；要统一词表的消费方按 ADR-0010 走 kernel eventbus，不在本包
   发明第三套。
3. **装配与丰富面外置**：回合数/工具数/Uncertain/挂起锚点等留在各入口
   原生 Result 上，经适配器的原生句柄取（如 `Agentloop.Loop`）；
   审批、恢复、重试都是装配语义，不进接口。

## 四、三入口适配器落点（不强迁存量）

| 入口 | 适配器 | 位置 | 测试 |
|---|---|---|---|
| agentrun | `agentrun.Agent`（结构体：Cfg + Opts，方法 Run） | 本仓 `agent/agentrun/iface.go` | 本仓 `iface_test.go`（httptest 网关替身：成功/失败/事件透传/SessionID 忽略/状态分类） |
| agentloop | `agentiface.Agentloop`（session 制；事件经 session 键路由的 Emitter 桥） | `goagent-runtime/bridge/agentiface`（tier 5 可同时看见三套语义，bridge/ 是既定适配层惯例） | runtime 侧同包测试 |
| contract 边界 | `agentiface.Contract`（宿主 `RunContext` 读面 + Exec 注入） | 同上 | runtime 侧同包测试（词表常量跨仓对齐钉在此处：两仓都能 import contract） |

- **不强迁存量**：`agentrun.Run` / `agentloop.Loop.Run` / 宿主 contract 驱动
  保持原签名；适配器是加法。
- **跨仓依赖顺序**：runtime 侧适配器 import `agent/iface` ⇒ agent 需先发版
  （v0.17.0）再 bump runtime 的地板——基座先行，同 matrix-tools#256 后续波次的
  地板时效纪律。

## 五、与既有裁决的关系

- `matrix-tools` ADR-001（三层边界）：本单是其 §一「Agent 层职责」的第一次
  接口级落地；越级边纪律不变（runtime → agent 是顺向，白名单已登记）。
- runtime ADR-0010（两模型分立、共享语义）：镜像单——它裁共享什么，本单裁
  形态是什么；两处都**不**承诺 Round/工具轮的统一抽象。
- kernel `contract`（Stable）：状态词表唯一单源；本单不复制、不包装、
  不定义平行常量。
- api-lifecycle（matrix-tools#256 / ADR-002）：`agent/iface` Experimental 起步；
  晋级走三维判据（无 breaking 窗口 + 真实宿主接入 + 故障测试证据）。

## 关联

- issue：`oagentkit/agent#174`（本 ADR 的执行单）、`matrix-tools#254`（总纲）
- 镜像/姊妹：`yi-nology/goagent-runtime#39`（W2 Loop 与 Pipeline 边界，ADR-0010）、
  `#38`（对象模型）、`#41`（产品入口）
- 词表单源：`yi-nology/goagent-kernel` `contract` 包（Stable）+ `docs/api-compat.md`
