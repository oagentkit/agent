# 回合守卫 `roundguard`：模型侧循环的次数预算、结局分类与无进展检测（设计）

日期：2026-11-21（设计）
状态：**设计待评审**（未实施；本轮不写代码）
关联：docs/GAP-ANALYSIS-20261107.md（本仓上一轮外部参照系刷新在 2026-11-07；本设计是它之后的第一轮）
外部参照：见 §2.3

## 0. 一句话

模型侧的 agent loop（LLM ↔ 工具调用反复往返）在 2026 年已被公认为「已解题」，
但**循环周边**的判定口径至今散在各家宿主手里；本仓把其中三件此前**没有任何
机制面承接**的事——**次数预算**、**回合结局分类**、**无进展（循环）检测**——做成
一个机制包 `roundguard`：纯判据、零领域词、内核不调模型。

## 1. 问题陈述

### 1.1 外部事实：循环本体已收敛，差异在周边

六家主流框架（Claude Agent SDK / OpenAI Agents SDK / LangGraph / smolagents /
Vercel AI SDK / CrewAI）的循环本体是同一段代码：

```go
for {
    resp := llm(messages, tools)
    if resp.ToolCalls == nil { break }                    // 文本回复 = 终止信号
    msgs = append(msgs, execute(resp.ToolCalls)...)       // 工具结果 = 继续信号
}
```

真正的分歧只有五处：谁判定停、上下文满了怎么办、权限按什么顺序判、失败能不能重试、
以及**「这一圈还要不要转下去」由谁说了算**。最后一项在 2026 年被单独拎出来：

- **次数预算**：主流实现的 `maxTurns` / `max_steps` / `max_seconds` / `max_tokens` /
  `per_tool_caps` 五件套，且要求**在模型调用前检查**（调用后检查意味着最后一次昂贵调用
  已经发生）。
- **无进展检测**：失控循环有五种形态（精确重复 / 震荡 / 缓慢漂移 / 错误重试螺旋 /
  只规划不动手），每种需要不同检测器；朴素去重只能挡住第一种。
- **结局分类**：一轮的结局不止「有工具调用 / 没工具调用」两种——还有交接、暂停、
  超限、中止、报错。OpenAI Agents SDK 用四分支判别联合表达这件事，Claude Agent SDK
  用 `stop_reason`，Pi 用三层退出条件。

### 1.2 仓内现状：这一维是空的

实测（2026-11-21）：

```
grep -rE 'MaxTurns|maxSteps|MaxRounds|轮次上限|步数上限|StepBudget|TurnBudget' --include='*.go' .
⇒ 零命中
```

本仓已有三样「量」的判据，但**没有第四次**：

| 已有 | 回答的问题 | 量纲 |
|---|---|---|
| `budget` | 「下一轮要发多少」（输入侧 token 估算） | token |
| `cost` | 「这一单已经花了多少」（已发生消费记账） | 金额 / 次数（只记不判） |
| `deadline` | 「还有多久」（会话级墙钟锚点，跨续跑轮不重置） | 墙钟 |
| **`roundguard`（本设计）** | **「还能转几圈 / 这一圈是不是白转」** | **次数** |

另有一份 2026-09 的实证（arXiv:2609.00050 的解读）把 agent 执行拆成图工程
（证据门控）+ 循环工程（有界恢复次数）+ 零信任 harness 三个正交抽象；其消融实验中
**去掉 recovery loop，同模型同任务集的验证完成率从 95.0% 掉到 12.9%**——决定 agent
是否有界可审计的是 harness，不是模型能力。这正是本仓站的位置。

### 1.3 为什么不是宿主的事

「次数够不够」「这一圈有没有白转」这两问与**谁来跑模型**无关，与**跑多少圈**有关。
放在宿主侧的后果与本仓其它机制被推给宿主时完全同形：每个宿主各写一遍组合逻辑，
而组合逻辑里恰好藏着必须全局一致的判定——

- 同一个循环，在 A 宿主被算成「第 12 圈，超了」、在 B 宿主被算成「第 8 圈，还能转」；
- 同一段动作序列，在 A 宿主被判「重复」而在 B 宿主被判「正常推进」；
- 一轮失败后能不能续跑，A 宿主放行、B 宿主拦下。

两边都没写错，只是各判一套，且**没有任何机器能发现**。这与 `guardset` 面对的是
同一类问题（守卫怎么装配）、与 `retryplan` 面对的是同一类问题（组合逻辑归内核，
判定归各自机制）——故本包与它们同族。

### 1.4 五家实现的实测对照（2026-11-21 克隆到本地逐行读）

| 实现 | 上限维度 | 触顶处置 | 仍产出终答？ | 「停止 vs 失败」可区分？ | 无进展检测 | 重复调用检测 |
|---|---|---|---|---|---|---|
| `smhanov/agentloop`（Go，MIT，143★） | maxToolCalls(5) / maxSteps(2×5+2) | 工具预算：禁工具 + user 通知，要求模型用已有证据作答；maxSteps：`ErrBudgetExhausted` | 工具预算 ✅ / maxSteps ❌ | ✅（`StatusStopped` vs `StatusFailed`） | ❌ | ✅（连续同调用同输出；三档处置 + `Repetitive` 豁免） |
| `openai/openai-agents-python`（21k★） | max_turns(10) | 抛 `MaxTurnsExceeded`，**除非装了 max-turn handler**（由 handler 产出合成输出） | 装了 handler ✅ / 没装 ❌ | ✅（异常 vs 正常返回） | ❌ | ❌ |
| `pydantic/pydantic-ai`（21k★） | request_limit(50) / tool_calls_limit / input·output·total tokens / per-request tokens / cost_limit | 一律抛 `UsageLimitExceeded`（**硬**） | ❌ | ✅（异常） | ❌ | ❌ |
| `huggingface/smolagents`（30k★） | max_steps(20) | `_handle_max_steps_reached()` → **单独再调一次模型**产出终答，步骤标 `AgentMaxStepsError`，state=`max_steps_error` | ✅（带错误标记） | ~（有错误标记，终答仍返回） | ❌（但有 `planning_interval`：**周期性强制重规划**） | ❌ |
| `anthropics/claude-agent-sdk-python`（8.6k★） | max_turns | `error_max_turns`（错误） | ❌ | ✅（`terminal_reason`：completed / max_turns / aborted_streaming / aborted_tools） | ❌ | ❌ |

**三条跨实现结论**：

1. **上限触顶的处置没有共识**——五家有三种口径：硬失败（pydantic-ai / Claude）、
   强制收尾（agentloop 工具预算 / OpenAI handler / smolagents 终答化）、失败但仍留
   终答（smolagents）。⇒ 本包**不把「软 / 硬」写死成两个字段的语义**（r1 那版是
   过拟合到 agentloop 一家），改为「**额度维度（事实）+ 处置口径（可配）**」（§6.1.1）。
2. **「回合」是国际共识词，且定义逐字同义**——OpenAI 的原话是「A turn is defined as
   one AI invocation (including any tool calls that might occur)」；pydantic-ai 的
   `request_limit` 注释说它「bounds the number of turns of the agent loop」；
   agentloop 的 `Steps` 注释说「one step = one inference plus its tool executions」。
   三家都指**模型往返**。⇒ 只有本仓的 `eventbus` 用 turn 指编排会话运行 ⇒ §3 的三分
   表是必要的，且「回合」这个中文名与国际实现对得上。
3. **无进展（震荡 / 漂移 / 空转 / 错误螺旋）检测：五家全是空白**——逐仓 grep
   `oscillat|stagnat|no_progress|drift`（排除测试与 schema / 时钟语境）命中 0 处；
   唯一的循环级检测是 agentloop 的**单一形态**重复调用检测。⇒ 本包机制三是
   **没有对照实现可抄**的那一块：没有外部先例可反查，风险只能靠自带的反向验证用例
   兜住（§10）。

## 2. 目标与非目标

### 2.1 目标

1. `roundguard` 包：三件套（回合结局分类 / 次数预算 / 无进展检测）+ 一条组合结论，
   全部为**纯函数或进程内计数器**，零领域词，内核**不调模型**。
2. 三条判据各自可被单独取用，也有一条把三者合成的**单结论**出口。
3. 全量登记面同步落位（policy / 生命周期 / 错误语义 / README 包表 / 词表登记），
   并带**反向验证用例**（坏实现必须会红）。

### 2.2 非目标（防范围蔓延）

- **不提供模型侧 turn runner**：不写「调 LLM → 执行工具 → 回灌」的驱动循环。
  内核不调模型是本仓的第一纪律（`budget.Summarizer` 是既有先例：内核给接口，
  宿主实现）。
- **不重判「该不该重试」**：那是 `toolfail` + `retryplan` 的活。本包只判
  「预算耗尽 / 无进展」这两种**续跑必然再撞同一问题**的情形（§6.4）。
- **不重判会话终态**：`finalize.Route` 的 Cause→终态四分路由不动，本包不产出
  ctx Cause、不碰 `contract` 状态词表。
- **不做 token / 成本预算**：`budget` / `cost` 不动。
- **不做墙钟**：`deadline` 不动。
- **不做工具调用配对**：`watchdog` 的 INV1/INV2/INV3 不动（§8 有逐条划界）。
- **不做跨进程回合账**：本包的账是**进程内**的。跨进程续跑时回合数**不随之累计**
  （§8 末条），要跨进程累计需宿主落库——本仓不替宿主选存储形态。

### 2.3 外部参照（调研于 2026-11-21）

| 来源 | 取用点 |
|---|---|
| <https://yavchn.parkscomputing.com/story/hn/47491045> | 六框架循环同形；周边工程才是差异所在 |
| <https://staging-cuttlesoft-com.vercel.app/blog/2026/04/14/the-agent-loop-one-pattern-three-frameworks> | OpenAI/Pydantic/LangGraph 三实现同题对拍 |
| <https://openai.github.io/openai-agents-js/guides/running-agents> | Runner 四分支结局判别联合、`maxTurns` 默认 10 |
| <https://console.anthropic.com/docs/zh-CN/agent-sdk/permissions> | 权限五级判定序（本包不实现，只作对照） |
| <https://agentguides.dev/ai-agents/agent-loop> | 「一轮」的准确定义与停止方式清单；两条追加纪律 |
| <https://www.technologyonthe.net/2026/10/loop-detection-and-step-budgets-for-llm.html> | 硬预算四件套 + **调用前检查**；五种失控形态 |
| <https://codex.danielvaughan.com/2026/09/06/agentic-cloud-engineering-graph-loop-zero-trust-harness-codex-cli> | 图/循环/零信任三抽象；recovery loop 的 95.0%→12.9% 消融 |
| <https://www.wqlblog.cn/posts/Pi_Agent_Core_%e6%9e%b6%e6%9e%84%e5%89%96%e6%9e%90/> | Pi 双层循环与三层退出条件 |
| <https://github.com/smhanov/agentloop>（MIT，Go 1.21+） | **Go 的模型侧 loop 库实读**（2026-11-21 克隆到本地逐行读）：软硬两级预算与强制收尾、重复检测三档处置、中止尾部配对修复、压缩的临时视图替换、导出 API 面上限门禁 —— 本设计 §6.1.1 / §7.5 / §13 的主要依据 |
| <https://github.com/openai/openai-agents-python> | **源码实读**：turn 的定义（one AI invocation + 其工具调用）、`DEFAULT_MAX_TURNS=10`、max-turn handler 的合成输出路径 |
| <https://github.com/pydantic/pydantic-ai> | **源码实读**：`UsageLimits` 多维限额（request / tool_calls / 三种 token / per-request / cost）、`check_before_request` 的调用前检查、`request_limit` 的「回合 ≠ 请求数」口径 |
| <https://github.com/huggingface/smolagents> | **源码实读**：`max_steps` 默认 20 与触顶后的 `_handle_max_steps_reached`（再调一次模型出终答 + 标错误）、`planning_interval` 的周期性强制重规划 |
| <https://github.com/anthropics/claude-agent-sdk-python> | **源码实读**：`max_turns` 选项、`error_max_turns` 子类型、`terminal_reason`（completed / max_turns / aborted_streaming / aborted_tools） |

## 3. 命名：必须先解决的一个撞词

**本仓已经有「轮次」这个词，含义与本设计的单位不同。**

| 词 | 现有含义 | 主人 | 本设计是否沿用 |
|---|---|---|---|
| **轮次**（turn） | **一次编排会话运行**（`eventbus.EventTurnStarted/EventTurnEnded` 开闭括号；watchdog INV3 判它的配对） | `eventbus` / `watchdog` | ❌ 已被占用，**不沿用** |
| **环节**（step） | 宿主定义的**业务步骤**（`statemachine.StepStatus` 转移表） | `statemachine` / `contract` | ❌ 已被占用 |
| **回合**（round） | —— | 本设计 | ✅ **新词** |

选「回合」的理由：① 语义最贴（一次模型往返 = 一次发言机会）；② 与「轮次」「环节」
在中文里可辨；③ 代码标识 `round` 不与仓内任何现有标识撞（`turn` 撞 `eventbus`，
`step` 撞 `statemachine`，`loop` 撞 `worker.Loop`（worker 领取驱动）与
`finalize` 所说的「宿主编排循环」）。

⚠️ **这不是文字游戏**：若本包也叫 `turn`，宿主代码里会同时出现
`eventbus.EventTurnStarted`（编排轮次）与 `roundguard.ReasonTurnLimit`（模型回合），
两个同形不同义的名字共用一个词——正是本仓包头反复声明要避免的「同词不同域」，
而那种撞车在本仓**已被证明会真实发生**（`delegate.Status.failed` 与
`contract.StatusFailed` 的同形异义，issue #358 用类型别名处理）。

**外部印证（2026-11-21 实读三家源码）**：OpenAI Agents SDK 的原话是「A turn is
defined as one AI invocation (including any tool calls that might occur)」；
pydantic-ai 的 `request_limit` 注释说它「bounds the number of turns of the agent
loop」；agentloop 的 `Steps` 注释说「one step = one inference plus its tool
executions」——**三家指的全是「模型往返」，与我们定的「回合」逐字同义**。
⇒ 国际实现里 `turn` / `step` 都指模型往返；**只有本仓的 `eventbus` 用 turn 指编排
会话运行**。这既说明三分表是必要的，也说明中文「回合」与国际实现对得上。

**备选（评审时可推翻）**：`modelturn`（包名长但绝对无歧义）、`iterationguard`
（与 Go 1.23+ 的 `iter` 包名族太近，不取）。**本设计推荐 `roundguard`**。

## 4. 总体形态

```
                    ┌──────────────────────── roundguard ────────────────────────┐
   宿主装配：       │                                                            │
   · Budget         │   CheckRounds(b, used)      → Verdict   （模型调用前）      │
   · Config         │   CheckTool(b, used, tool)  → Verdict   （工具执行前）      │
   · Meter          │   Classify(outcome)         → Route     （回合末：结局）    │
   · Observation[]  │   Detect(obs, cfg)          → Finding   （回合末：无进展）  │
                    │   Decide(snapshot, b, cfg)  → Decision  （回合末：单结论）  │
                    └────────────────────────────────────────────────────────────┘
```

- **四个纯函数 + 一个进程内计数器**（`Meter`）+ 一个组合入口（`Decide`）。
- **零内部依赖**：只依赖 `github.com/oekit/pkg/errors`（哨兵）与标准库。
  不 import `budget` / `cost` / `deadline` / `watchdog` / `contract`——
  它们是**平级的不同维度**，互相不引（§8 逐条划界）。
- **不调模型**：`Outcome` / `Observation` 是宿主注入的机械事实，内核不解释其语义。

### 4.1 判定面清单（本包提供什么）

| 机制 | 回答的问题 | 纯函数 | 注入面 |
|---|---|---|---|
| 结局分类 | 这一回合结束了，意味着什么 | `Classify` | `Outcome`（宿主从模型回复映射） |
| 次数预算 | 还能不能再转一圈 / 还能不能再调这个工具 | `CheckRounds` / `CheckTool` | `Budget` + `Used` |
| 无进展检测 | 最近这几圈是不是白转 | `Detect` | `Observation[]` |
| 组合 | 一条可执行结论 | `Decide` | 以上全部 |

### 4.2 组合结论的形状

```go
// Snapshot 一回合末尾的完整事实（宿主组装）。
type Snapshot struct {
    Outcome Outcome
    Used    Used
    Obs     []Observation // 回合历史（含本回合，按时间序）
}

// Decision 一条可执行结论（组合判定面）。
type Decision struct {
    Reason   Reason // 结局分类
    Continue bool   // 允许**按原样**进入下一回合
    Limit    Limit  // 命中的上限（LimitNone = 未因上限叫停）
    Shape    Shape  // 无进展形态（仅 LimitStalled 时有效）
    Warn     bool   // 无进展**首档**：只该警告回灌，不停止
    Used     Used   // 判定时的用量快照（留痕用）
}

// Decide 合成一条结论：结局 → 额度用尽（按 Budget.Policy 处置）→ 无进展。
func Decide(s Snapshot, b Budget, cfg Config) Decision
```

**判定优先级（顺序即语义）**：

1. **结局终局优先**：模型给了终答 / 交接 / 报错 / 中止 ⇒ 以结局为准，
   **不被预算打断**（模型说完了就是完了——哪怕刚好撞上上限）。
2. 否则（结局要求继续）：**额度用尽** ⇒ 按 `Budget.Policy` 处置——
   `PolicyForceFinal` ⇒ `ReasonForcedFinal`（转强制收尾，产物仍可用）；
   `PolicyFailFast` ⇒ `ReasonBudgetExhausted`（失败型收敛）。
   **额度两维的次序：先回合数、后工具调用总数**——两者同时用尽时报**回合数**那一维
   （回合数 = 本回合不能开始，是**外层**边界；工具调用总数 = 不能再调工具但本回合
   可继续，是**内层**边界；外层先撞 ⇒ 报外层那一维，留痕与处置才对得上）。
3. 否则：无进展命中 ⇒ `ReasonStalled`（按档位决定 `Continue`）。
4. 否则：`Continue=true`，照常进入下一回合。

⚠️ `Continue=false` 有**三种含义**，靠 `Reason` 区分，宿主不得一律当失败处理：
`ReasonFailed`/`ReasonNoOutcome`/`ReasonAborted`/`ReasonBudgetExhausted` 是
**失败型**（`IsFailure()==true`）；`ReasonForcedFinal` 是**转强制收尾**；
`ReasonStalled` 是**温和叫停**。

## 5. 机制一：回合结局分类

### 5.1 词表（判定词表，十值）

```go
// Reason 回合结局（判定词表：backing roundReasonAll / enum AllRoundReasons / predicate Reason.Valid）。
type Reason string

const (
    ReasonCompleted        Reason = "completed"          // 模型给出终答，本回合不再要工具
    ReasonToolCalls        Reason = "tool_calls"         // 模型请求工具 → 允许进入下一回合
    ReasonHandedOff        Reason = "handed_off"         // 模型请求交接给别的执行方 → 收敛并移交
    ReasonPaused           Reason = "paused"             // 服务端要求原样再问 → 允许进入下一回合
    ReasonFailed           Reason = "failed"             // 模型调用或工具执行明确报错 → **失败型**收敛
    ReasonNoOutcome        Reason = "no_outcome"         // 本回合无任何产出 → **失败型**收敛（保守侧）
    ReasonAborted          Reason = "aborted"            // 被中止（用户取消 / 停机 / 接管）→ **失败型**收敛
    ReasonBudgetExhausted  Reason = "budget_exhausted"   // 额度用尽且按**失败**口径处置 → **失败型**收敛
    ReasonForcedFinal      Reason = "forced_final"       // 额度用尽但按**强制收尾**口径处置 → 产物仍可用
    ReasonStalled          Reason = "stalled"            // 无进展 → **温和叫停**（不是失败）
)

// IsFailure 报告该结局是否为**失败型**收敛。
//
// 为什么要单独一个判据而不是让宿主自己列清单：「被温和收停」与「真的坏了」
// 在宿主侧必须可区分——前者仍有可用产出（模型已给出终答、或证据已齐备只差
// 收束），后者只有半截 transcript。两类混在一张「都收敛了」的清单里，
// 宿主就会把「被上限温和收停」也当故障上报（假红）。
//
// 与上游模型侧实现的同构：agentloop 的 AnswerStatus 把 StatusStopped
// （hook 跳过 / 重复检测命中）与 StatusFailed（传输/hook/ctx 错误）分成两个值。
func (r Reason) IsFailure() bool {
    switch r {
    case ReasonFailed, ReasonNoOutcome, ReasonAborted, ReasonBudgetExhausted:
        return true
    }
    return false
}
```

**为什么把「强制收尾」「额度用尽」「无进展」也放进结局词表**：它们不是模型的语义，
是内核判出来的停法；混进同一张表，宿主才能用**一张词表**判「这一回合怎么结束的」，
而不用再分「模型说的」与「内核说的」两套。

⚠️ **失败型与温和型必须可区分**（`IsFailure()`）：`failed`/`no_outcome`/`aborted`/
`budget_exhausted` 是**失败型**（只有半截 transcript）；`forced_final`/`stalled` 是
**温和型**（产出仍在，只是被内核收住）。

**为什么「没产出」与「报错」分开**：`failed` 是**有**错误，`no_outcome` 是**什么都没有**
（模型既没要工具也没给终答）。按本仓既有纪律（`budget` 的
`ErrPlanMismatch` vs `ErrEmptyPlan`：「按同一条理由合并成一种，等于让宿主看到
`ErrPlanMismatch` 却在错误的地方找原因」），两者的现场不同，处置也不同
（前者多半要查模型/工具侧，后者查宿主接线），不合并。

**零值语义**：`Reason` 是字符串类型，**零值为空串**——空串**不在词表内**
（`Reason.Valid()` 判假），即「未设 / 读不懂」，**不冒充任何结局**（与
`statemachine.StepStatus` 的零值处理同款：读不懂 ≠ 合法迁移）。
「忘填结局会不会被当成完成」的防线**不在 Reason 上，在 `Route` 上**：
`Route.Continue` 的零值是 `false` ⇒ 宿主自造一个零值 `Route` 时，本包判**收敛**
而非继续。`Classify` 的八个分支穷尽 ⇒ 它的返回值必在词表内（非法值不可能从本包
流出）。

### 5.2 输入面

```go
// Outcome 一回合结束后的机械事实（宿主注入；内核不调模型、不认识工具语义）。
type Outcome struct {
    ToolCalls int    // 本回合请求的工具调用次数（>0 ⇒ 模型还要工具）
    Completed bool   // 模型给出了终答
    Handoff   bool   // 模型请求交接
    Paused    bool   // 服务端要求原样再问（pause_turn）
    Failed    error  // 模型调用或工具执行的错误（nil = 无）
    Aborted   bool   // ctx 被取消
    // ForcedFinal 本回合是「工具预算耗尽后的强制收尾步」：宿主已禁用工具，
    // 要求模型用已收集的证据给出终答。它是**产出仍然可用**的收束，不是失败。
    ForcedFinal bool
}
```

### 5.3 判定

```go
// Route 结局分类结果。
type Route struct {
    Reason   Reason
    Continue bool   // 允许进入下一回合
}

// Classify 回合结局分类（纯函数）。
func Classify(o Outcome) Route
```

判定顺序（**先具体后笼统**，取证据最硬的那条）：

| 序 | 条件 | Reason | Continue |
|---|---|---|---|
| 1 | `o.Aborted` | `ReasonAborted` | false |
| 2 | `o.Failed != nil` | `ReasonFailed` | false |
| 3 | `o.Handoff` | `ReasonHandedOff` | false |
| 4 | `o.ToolCalls > 0` | `ReasonToolCalls` | **true** |
| 5 | `o.ForcedFinal` | `ReasonForcedFinal` | false |
| 6 | `o.Completed` | `ReasonCompleted` | false |
| 7 | `o.Paused` | `ReasonPaused` | **true** |
| 8 | 都不成立（零值形态） | `ReasonNoOutcome` | false |

**为什么「要工具」排在「给终答」前面**：模型可以同时给文本与工具调用；此时
「还要工具」才是决定下一步的事实（文本是给用户看的，工具调用才是控制流）。
把终答排前面会让「模型一边收尾一边补一个工具调用」被误判成已完成——而工具
**还没执行**，判完成就是谎报已完成。

**为什么「强制收尾」排在「给终答」前面**：强制收尾步本身就是「模型用已有证据作答」的
那一步——它同时满足 `Completed`。若把 `Completed` 排前面，强制收尾会被判成普通终答，
宿主就分不出「这是被上限逼出来的收尾」还是「模型自己写的结论」——两者的后续处置
不同（前者要留痕说「是被预算收住的」）。

**为什么「暂停」排在最后**：它是**最少证据**的一条——只说明「本回合什么实质都没发生」。
前面任何一条成立就说明本回合有实质内容，不该被降级成「暂停」。

**零值形态的处理**：`Outcome{}` ⇒ `ReasonNoOutcome` + `Continue=false`。
宿主忘了填 `Outcome` 时，本包判**收敛**（保守侧），而不是「无事发生所以继续」
（那会让一个空循环空转到超时）。

## 6. 机制二：回合次数预算

### 6.1 预算面

```go
// Budget 回合预算（宿主给；内核不解释工具标识的含义）。
//
// **额度维度（事实）与处置口径（策略）分开**——前者各家一致，后者没有共识
// （详见 §6.1.1 与 §1.4 的五家实测对照）。
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
func (b Budget) Validate() error
```

⚠️ **`Validate` 与「零值 = 不限」的关系**：`MaxRounds == 0`、`MaxToolCalls == 0`、
`MaxPerTool[v] == 0` 都是**显式关闭**对应上限（合法的「不限」）。**负值一律非法**
（装配期报错）——把一个写错的负值默默当成「不限」跑一整轮，是典型的 fail-open；
宁可装配期报错。另：`MaxPerTool` 的**键不得为空串**（空标识无法计数，等于声明了
一个无法判的上限）。

### 6.1.1 额度维度 vs 处置口径（本设计最重要的一处取舍）

**先分清两件事**：

- **额度维度（事实）**：跑了多少回合 / 多少工具调用 / 某工具调了多少次。
  这一层各家**完全一致**——都是计数器 + 「用尽了吗」。
- **处置口径（策略）**：用尽之后怎么办——硬失败，还是转强制收尾（拿已有证据作答）？
  这一层**五家没有共识**（§1.4 实测出三种口径）。

**证据（2026-11-21 逐仓实读）**：pydantic-ai 与 Claude 是硬失败；agentloop 把工具
预算做成强制收尾（禁工具 + user 通知 + 要求模型用已有证据作答，那一步仍算完成）；
OpenAI 默认抛 `MaxTurnsExceeded`，但**装了 max-turn handler 就由 handler 产出
合成输出**；smolagents 触顶后**单独再调一次模型**产出终答、同时把步骤标成错误。

**为什么不能把「软 / 硬」写死成两个字段的语义**：r1 曾把「工具预算 = 软（强制收尾）/
回合上限 = 硬（失败）」写成固定语义——那是**过拟合到 agentloop 一家**：同一份代码在
pydantic-ai 宿主里就是硬失败。⇒ 本设计改为「**额度维度（事实）+ 处置口径（可配）**」：

```go
// StopPolicy 额度用尽后的处置口径。
type StopPolicy string

const (
    // PolicyForceFinal 强制收尾：不再给工具（模型面），要求模型用已收集的
    // 证据给出终答；产物仍然可用（不是失败）。
    PolicyForceFinal StopPolicy = "force_final"
    // PolicyFailFast 直接失败：本次运行以失败收场。
    PolicyFailFast StopPolicy = "fail_fast"
)
```

**缺省 = `PolicyForceFinal`**：腰斩最接近答案的那一刻，代价是丢弃全部已采集的证据
（这正是 r1 认定的错）。宿主可显式改成 `PolicyFailFast`（并留痕）。

⚠️ **处置口径不按额度维度分别配置**——那会让「同一个上限在不同宿主得到不同处置」
变成常态，而那正是本包存在的理由（§1.3）。一个 policy 管全部额度 ⇒ 跨宿主口径一致。

⚠️ **处置口径属判定语义，归内核**；但「能不能强制收尾」最终取决于模型面有没有那个
接缝（能不能禁工具再问一次）——宿主若没有该接缝，只能用 `PolicyFailFast`。

**强制收尾那一步不可否决**：内核判出的停优先于宿主的否决——否则宿主可以在最后一刻
把强制收尾翻回「继续调工具」，预算就形同虚设。

### 6.2 用量面

```go
// Used 当前用量（回合数 + 工具调用总数 + 按工具的调用次数）。
type Used struct {
    Rounds    int
    ToolCalls int            // 工具调用总数（MaxToolCalls 额度用）
    Tools     map[string]int // 按工具的调用次数（单工具上限用）
}

// Meter 进程内回合账（只数不判；跨进程不累计，见 §8 末条）。
type Meter struct { /* … */ }

func NewMeter() *Meter
func (m *Meter) Round() int                  // 记一回合并返回累计回合数
func (m *Meter) Tool(toolID string) int      // 记一次工具调用并返回该工具累计次数
func (m *Meter) Used() Used                  // 导出用量快照
```

⚠️ **回合数 ≠ 模型请求数**：本包的 `Rounds` 数的是**回合**（一次模型往返），
不是「发往 provider 的请求数」——fallback 重试、provider 侧重试、续跑请求都可能让
请求数大于回合数。pydantic-ai 的 `request_limit` 注释明确写了这条区分（"bounds the
number of turns of the agent loop, not the number of requests sent to the provider"）
⇒ 口径必须写死，否则两个宿主会把同一个数字当两件事用。

### 6.3 判定面（**调用前**检查）

```go
// Limit 上限类别。
type Limit string

const (
    LimitNone      Limit = "none"        // 未因上限叫停
    LimitRounds    Limit = "rounds"      // 回合数额度用尽（处置按 Budget.Policy）
    LimitToolCalls Limit = "tool_calls"  // 工具调用总额度用尽（处置按 Budget.Policy）
    LimitPerTool   Limit = "per_tool"    // 单工具额度用尽 ⇒ 该次调用被拒
    LimitStalled   Limit = "stalled"     // 无进展（由 Detect 产出，见 §7）
)

// Verdict 准入判定结论。
type Verdict struct {
    Allowed bool
    Limit   Limit
    Detail  string  // 机械留痕（命中的工具标识等）；**展示文案归宿主**
}

// CheckRounds 判定下一回合能否开始（**模型调用前**检查；回合数额度）。
func CheckRounds(b Budget, used Used) Verdict

// CheckToolCalls 判定本回合还能不能再调工具（**模型调用前**检查；工具调用总额度）。
func CheckToolCalls(b Budget, used Used) Verdict

// CheckTool 判定某工具的下一次调用能否进行（**工具执行前**检查；单工具上限）。
func CheckTool(b Budget, used Used, toolID string) Verdict
```

**三个 `Check*` 只判「额度用尽没有」这个事实**，不判处置——处置由 `Budget.Policy`
在 `Decide` 里决定（§4.2）。`Verdict.Limit` 是**命中的额度维度**。

**额度两维的判定次序**（`Decide` 里，§4.2 同款）：**先回合数（`CheckRounds`）、
后工具调用总数（`CheckToolCalls`）**；两者同时用尽 ⇒ 报**回合数**那一维
（`LimitRounds`）。理由：回合数是**外层**边界（撞上它 ⇒ 本回合根本不能开始），
工具调用总额是**内层**边界（撞上它 ⇒ 本回合还能继续，只是不能再调工具）；外层先撞
⇒ 报外层那一维——否则宿主拿到的 `Limit` 会指向一个**还能继续**的维度，处置与留痕
都跟着错。

**为什么必须调用前检查**：调用后检查意味着**最后一次昂贵调用已经发生**。
本包的三个 `Check*` 都是**纯函数**（输入用量快照，输出判定），宿主在两个时点各调：

| 时点 | 调用 | `Allowed=false` 的含义 |
|---|---|---|
| 模型调用前 | `CheckRounds` | **本回合不能开始** ⇒ 失败型收敛，不再调模型 |
| 模型调用前 | `CheckToolCalls` | **不能再调工具**（但本回合可继续）⇒ 转入强制收尾 |
| 工具执行前 | `CheckTool` | **这一次的调用被拒** ⇒ 宿主回灌一条失败结果给模型（配对不破），本回合继续 |

⚠️ 三个 `Allowed=false` 的语义**不同**：混用会得出「预算耗尽就整个停下来」的错误结论，
把「逼出答案」做成「腰斩答案」（§6.1.1）。

**为什么按工具也要有上限**：主流实现都带 `per_tool_caps`——一个**有副作用**的工具
（发信、建单）上限应当远低于只读工具。总回合数管不住「同一个副作用工具连调 50 次」。

### 6.4 「不得续跑」判据

本包**只**在两种情形下判「不得续跑」，因为这两种情形**续跑必然再撞同一个问题**：

| 情形 | 判据 | 理由（与 `retryplan` 同族的取舍） |
|---|---|---|
| 回合数上限用尽 | `LimitRounds` | 续跑会再撞同一个上限——「没有幂等键就不许猜着重试」的同款保守侧 |
| 无进展 | `LimitStalled` | 续跑会再转同一段死循环 |

⚠️ **处置口径决定这条表的适用范围**：`PolicyFailFast` 下「额度用尽」一律落到本表；
`PolicyForceFinal` 下，`LimitRounds`/`LimitToolCalls` 走的是**强制收尾**（本回合
仍会再走一次模型调用，只是不再给工具），只有 `LimitStalled` 才真的「不许续跑」。

其余情形（尤其 `ReasonFailed`）**本包不判**「能不能续跑」——那是
`toolfail`（该不该重试）+ `retryplan`（原样重发 / 幂等键重放）的活。
本包抢过来判，就是**两个包判同一件事** ⇒ 口径必然漂移。

## 7. 机制三：无进展（循环）检测

### 7.1 输入面

```go
// Observation 一回合的可机判事实（宿主注入）。
type Observation struct {
    // Fingerprint 本回合动作指纹（宿主算）：同一动作同一指纹。
    Fingerprint string
    // Normalized 抹掉易变部分后的指纹（宿主算）：page=1/2/3 ⇒ "page=N"。
    Normalized string
    // Tools 本回合工具调用数。
    Tools int
    // Failed 本回合是否出现失败结果。
    Failed bool
    // Settled 本回合是否已收敛（终答 / 交接）。
    Settled bool
    // ResultChanged 本回合的结果与上一轮**同指纹**的结果不同（true = 有进展）。
    //
    // 为什么必须显式给这个事实：「相同调用但输出变了」是**进展**，不是重复。
    // 只看参数的去重会把「换个页码再查一次」这类**有意义的重试**误判成循环，
    // 也会把「同一个轮询工具连续返回不同状态」误判成失控（而轮询类动作
    // 本就靠输出变化表达进展）。
    ResultChanged bool
    // Exempt 本回合**不参与无进展判定**（重复计数与空转计数都不参与）。
    //
    // 两个已知的必须豁免的场景：
    // ① 轮询 / 查状态类动作——「输出变了才是进展信号」（agentloop 的
    //    `Repetitive` 标记同款）；
    // ② **刻意的规划步**——smolagents 的 `planning_interval` 会每隔 N 步插一个
    //    规划回合，而规划回合**必然零工具调用**。按「连续 k 回合零工具且未收敛」
    //    的空转判据，它会被误判成停滞（`planning_interval=1` 时必然误判）。
    //    宿主把这类回合标 Exempt 即可豁免。
    Exempt bool
}
```

⚠️ **归一化由宿主算，内核不猜**：内核若自己决定「什么是易变部分」（数字？时间戳？
行号？），就成了领域解释者——那是宿主的事。内核只对**宿主算好的两组字符串**做
集合判定（与 `budget.Item` 只携带 ID+Tokens、`cost.Dims` 是开放键同一条纪律）。

### 7.2 形态与判定

```go
// Shape 无进展形态。
type Shape string

const (
    ShapeNone         Shape = "none"
    ShapeRepeat       Shape = "repeat"        // 精确重复：同指纹连着来
    ShapeOscillation  Shape = "oscillation"   // 震荡：A B A B
    ShapeDrift        Shape = "drift"         // 缓慢漂移：归一后同、原指纹不同
    ShapeErrorSpiral  Shape = "error_spiral"  // 错误重试螺旋：连着失败
    ShapeIdle         Shape = "idle"          // 只规划不动手：有回合、零工具、未收敛
)

// Finding 无进展检测结果。
type Finding struct {
    // Stalled 已达**停止档**：应当温和叫停。
    Stalled bool
    // Warn 首档命中：只该**警告回灌**，不停止——给模型一次自我修正的机会
    // （处置分档见 §7.5）。
    Warn bool
    Shape Shape
    Since int  // 从第几回合起（1 起；0 = 判不出）
}

// Config 无进展检测参数（零值 = 缺省）。
type Config struct {
    // Window 看最近多少回合（0 = DefaultWindow = 8 = 2×DefaultRepeat+2）。
    //
    // 下限会被向上钳位到 2×Repeat+2——震荡形态的停止档需要那么多回合才可达；
    // 窗口不够时向上钳位（不是判不出）。
    Window int
    // Repeat 连续几回合构成重复（0 = DefaultRepeat = 3）。
    // 至少取 2：取 1 是退化档（任意单个回合即判重复，且漂移那一档永不成立）。
    // ⇒ **向下界向上钳位到 2**（不是报错）：判据不可达 = 判据不存在，
    //   把档位抬回可达区间比让宿主在装配期被拒更符合本包的其余口径。
    Repeat int
}

// Detect 无进展检测（纯函数；只看最近 Window 个回合）。
func Detect(obs []Observation, cfg Config) Finding
```

### 7.3 判定顺序（取病因最具体的那条）

设 `w` = 最近 `min(len(obs), Window)` 个回合，`k` = `Repeat`；
**参与计算的回合** = 窗口内 `Exempt == false` 的回合（豁免的回合不计数，但占窗口位置）。

**「结果变了」（`ResultChanged`）与连续性的关系**——**逐形态不同**（不是通则）：
「结果变了」表达的是「**同一动作**的结果不同」⇒ 对**按指纹 / 归一指纹计连续性**的
形态（重复 / 震荡 / 漂移）它是**进展** ⇒ **打断**连续性；对**按失败事实 / 按有没有
动手**计连续性的形态（错误螺旋 / 空转）它**不参与**判定 ⇒ **不打断**连续性。
下表最后一列逐形态写死这一条（2026-11-21 补：先前这里只写了一句通则，而实现里
错误螺旋 / 空转两条判据**忽略** RC ⇒ 声明与执行不一致，且两条口径都没有用例钉住）。

| 序 | 条件（**尾部连续成立长度 L**） | Shape | 最小命中窗口 W | 「结果变了」的作用 |
|---|---|---|---|---|
| 1 | 尾部连续 `Failed` 全为 true，L ≥ k | `ShapeErrorSpiral` | k | **不参与**（失败事实与「结果变了」无关：结果变了的失败**仍是失败** ⇒ 连续性不被打断） |
| 2 | 尾部连续 `Fingerprint` 全等，L ≥ k | `ShapeRepeat` | k | **打断**（末轮 RC ⇒ 长度重起算，只算它自己；中间的 RC ⇒ 该段从它之后重新起算） |
| 3 | 尾部呈周期 2 交替：相邻不等且 `f[i]==f[i+2]`，L ≥ 2k（`A A A` 是周期 1，故不与「重复」混淆） | `ShapeOscillation` | 2k | **打断**：**末位 / 次末位**带 RC ⇒ 整段不算震荡（长度归零）；更早的位置带 RC ⇒ 该处连续性从它之后**重新起算** |
| 4 | 尾部连续 `Normalized` 全等 **且** 原 `Fingerprint` 不全等，L ≥ k | `ShapeDrift` | k | **打断**（与第 2 行**同口径**：末轮 RC ⇒ 长度重起算） |
| 5 | 尾部连续 `Tools == 0` 且 `Settled` 全为 false，L ≥ k | `ShapeIdle` | k | **不参与**（空转看的是「有没有动手 / 收没收敛」：出了新结果但仍零工具、仍未收敛 ⇒ **仍是空转**） |

**「原指纹全等」那一子情形归谁判（重复优先于漂移）**：第 4 行的「原指纹不全等」是
一道**守卫**。在当前判定链上它**不可达**——「原指纹全等」的那串必被**第 2 行
（重复形态）先判走**。这依赖两处耦合：**① 重复分支先于漂移分支；② 两者用同一个
阈值 k**。故那一子情形的结论恒为「重复」，且**对两条分支的先后不敏感**（守卫挡住了
漂移分支 ⇒ 次序换不换都不影响结论）。

⚠ 正因守卫在当前耦合下不可达，它**曾经是一句惰性断言**：删掉它，整套用例仍全绿
（2026-11-21 实测）。⇒ 两条纪律：① 耦合一旦被改动（漂移分支的阈值被调到低于重复
分支），守卫立刻变成**承重判据**；② 为此守卫的契约由**白盒用例**直钉
（`driftCandidate`，见 `roundguard/mutation_internal_test.go`），删掉守卫 ⇒ 用例
当场红。

**为什么改成「尾部连续成立长度 L」**：定长窗口写法（「最近 k 回合全等」）的结论是
**二值**的（命中 / 不命中），而处置要**三档**（不判 / 首档 / 停止档）——二值判据
分不出首档与停止档，于是每次命中都直接停，模型没有一次自我修正的机会。尾部连续
长度是**连续量**，天然带档位信息：L == W ⇒ 首档，L > W ⇒ 停止档（见下）。

**窗口下限为什么是 2×Repeat+2**：停止档要求 L ≥ W+1，而震荡形态的 W = 2k
（= 6）⇒ 停止档至少要 2k+1 个回合才**可达**；窗口小于这个数时，那一档**永不可达**
（判据不可达 = 判据不存在）。故 `Window` 向上钳位到 2×Repeat+2（= 8）——比可达下界
（2k+1）**多留一轮余量**：窗口内的**豁免回合占位置不计数**，留这一轮才能保证
「窗口里有一个豁免回合」时停止档仍可达（窗口取 2k+1 时，一个豁免回合就把可判的
回合数压回 2k ⇒ 那一档又不可达）。窗口不够时**向上钳位**（不是判不出——判不出
等于把「钝」当成「没有判据」）。

**定档规则（命中 ≠ 立刻停）**：每种形态求出它在尾部的**连续成立长度 L**
（该形态的模式连续成立的最大长度；W = 该形态的最小命中窗口）：

- `L == W` ⇒ **首档**（`Warn=true`）：刚好凑够一次——值得提醒模型一次。
- `L > W` ⇒ **停止档**（`Stalled=true`）：模式还在延续 ⇒ 温和叫停。

⚠️ 这一档之差是 §7.5 的分档处置的**判据来源**——没有它，「给模型一次自我修正机会」
就没有可判的落点。

**为什么错误螺旋排在重复前面**：`A A A`（同动作连着来）同时命中「重复」与「错误螺旋」。
前者只说**形状**，后者额外携带**失败事实**——处置完全不同（重复 ⇒ 可能调参就能过；
错误螺旋 ⇒ 这条路根本走不通）。按证据强度取更硬的那条。

**为什么「空转」排在最后**：空转（只说不做）是最弱的信号——很多正常回合也零工具
（例如模型在组织语言）。它只在其它四条都不成立时成立。

### 7.4 失败事实与进展事实从哪来

宿主在工具执行后把失败**如实回灌**给模型（模型据此自行修正），同时**记一条**
`Observation{Failed: true}`。失败回灌本身是模型侧的事（内核不调模型），
本包只消费那条**机械事实**。

同理，「结果变了」由宿主记为 `Observation{ResultChanged: true}`——内核不比对输出
内容（那是模型的产物，且可能很大），只消费这条布尔事实。

### 7.5 处置分三档（不要一命中就停）

**证据（agentloop 的重复调用处置）**：该实现对「相同调用 + 相同输出」的连续重复
**不是**一命中就停，而是三档——

1. 达到重复阈值的那一次**仍然执行**，但把警告**追加**到真实输出之后回灌给模型
   （警告带一段输出摘录作证据，摘录有长度上限），
2. **再犯一次**才停止，且停止发生在**执行之前**（该次调用不执行，但仍给模型回灌
   一条说明为什么停的结果），
3. 另有 `Repetitive` 标记：**轮询 / 查状态类工具豁免**重复检测——「输出变了才是
   进展信号」。

**为什么照抄这个分档**：重复循环的成因常常是**模型没意识到自己在重复**。一次带证据的
警告回灌比内核直接掐死更可能让模型换路；而「再犯一次才停」保证了警告不是摆设。
一命中就停会把「模型还需要一次提示」的场合变成「任务无产出地失败」。

**本设计的落法**（判定面只出**事实与档位**，**回灌文案归宿主**）：

| 档 | `Finding` | 宿主该做什么 |
|---|---|---|
| 首档 | `Warn=true` | 把警告回灌给模型（不停止），下一回合继续观察 |
| 停止档 | `Stalled=true` | 温和叫停（不是失败）；把「为什么停」也回灌给模型 |
| 豁免 | `Exempt=true`（由 Observation 给出） | 该回合不参与计数 |

⚠️ 「温和叫停」与「失败」的可区分性由 `Reason.IsFailure()` 保证（§5.1）。

## 8. 边界：与既有机制逐条划界

| 既有机制 | 它回答 | 本包 | 划界理由 |
|---|---|---|---|
| `budget` | 下一轮要发多少（token 估算） | 回合**次数** | 量纲不同：token ≠ 次数。两者并列为「量的四个维度」 |
| `cost` | 这一单已经花了多少（记账） | 回合次数**约束** | 同 `budget`/`cost` 的既有切分：**记**与**判**分属两包 |
| `deadline` | 还有多久（墙钟） | 回合次数 | 墙钟 ≠ 次数。两者互补不重叠 |
| `watchdog` | 工具调用配对（INV1/INV2）与**编排轮次**括号配对（INV3） | 模型**回合**的次数与进展 | **单位不同**：watchdog 的「轮」= 编排会话运行（`eventbus.EventTurnStarted`）；本包的「回合」= 模型一次往返。两者不互相替代 |
| `finalize` | 会话级 Cause→终态四分路由 | 回合级结局分类 | 轴不同：本包**不产出 ctx Cause**、不碰 `contract` 状态词表 |
| `contract` / `statemachine` | 会话状态 / 环节状态 | 回合结局（第三根轴） | 回合结局不是会话状态也不是环节状态，**不复用**既有词表 |
| `retryplan` / `toolfail` | 该不该重试 / 怎么重试 | 只判「续跑必然再撞」的两种情形 | 本包不抢「失败能不能重试」的判定 |
| `guardset` | 多守卫拒绝合成 | 一次只管一个维度 | 本包不聚合多守卫 |
| `steer` | 运行中用户接管 | 回合末判定接管后如何收敛 | 接管 Cause 由 `steer` 产出，本包只在 `ReasonAborted` 侧消费事实 |
| `delegate` / `handoff` | 委托 / 跨进程交接 | `ReasonHandedOff`（模型请求交接） | 本包只判「模型要求移交」这一事实，交接契约不重做 |
| **模型侧 loop 库**（agentloop 等） | 驱动整个循环（调模型 / 执行工具 / 回灌） | **判据层**，不驱动 | 见 §13：两者不竞争，是**驱动层 + 判据层**的上下关系 |
| **压实：视图替换 vs 真源替换** | —— | 本仓 `budget.Compact` 是**真源替换 + 原文归档句柄** | 模型侧库（agentloop 的 Compactor / RebuildContext）通常是**临时视图替换**（真源保留，只换发给模型的那份）。两者可共存：视图替换可回看，真源替换省内存（原文已归档） |
| **强制收尾这一步的否决权** | —— | **内核判出的停不可否决** | 与 agentloop 的硬规则同构：强制收尾步「veto or not」都结束。否则宿主能在最后一刻翻回「继续调工具」，预算形同虚设 |
| **限额声明面** | —— | `budget`（估算）与 `roundguard`（约束）**分属两包** | pydantic-ai 把多种限额聚在一个 `UsageLimits` 类型里（含「请求前预估拦截」开关 `count_tokens_before_request`）。拆分在本仓的代价是宿主要装配两个对象，收益是「估」与「判」的口径不会互相污染；跨实现看**限额类型聚合是主流**（pydantic-ai），本仓的拆分是一个**可被推翻的取舍** |
| **跨进程回合账** | —— | **不提供** | 本包的 `Meter` 是**进程内**的。跨进程续跑时回合数**不随之累计**——跨进程累计需要存储形态，那是宿主资产。要跨进程累计的宿主须自行落库并把 `Used` 灌回本包（外部实现同样不做：agentloop 全程进程内） |

## 9. 登记面与门禁（先改声明面，再写代码）

### 9.1 `architecture-policy.yaml`

**(a) `modules` 段**（新增）：

```yaml
  - id: roundguard
    roots: [roundguard]
    requires: ["ext:github.com/oekit/pkg"]   # 零内部依赖：哨兵/错误用 pkg/errors
    owner: harness
    managed: true
    notes: |
      回合守卫：模型侧循环的次数预算 + 回合结局分类 + 无进展检测。
      零内部依赖是刻意的——budget/cost/deadline/watchdog 是**平级的不同量纲**，
      互相不引（引了就成环，且两处口径会漂）。回合结局是**第三根轴**，
      不复用 contract 会话状态 / statemachine 环节状态。
```

**(b) `thresholds.scan_roots`**：增 `roundguard`（漏了则「超长文件」「god object」
两条阈值对它完全隐形——policy 自校验当天即红）。

**(c) `word_tables.tables`**（三张判定词表，值由代码派生，policy 不写取值清单）：

```yaml
    - pkg: roundguard
      backing: roundReasonAll
      enum: AllRoundReasons
      predicate: Reason.Valid
      family: Reason
      desc: 回合结局词表（10 值）
    - pkg: roundguard
      backing: roundLimitAll
      enum: AllLimits
      predicate: Limit.Valid
      family: Limit
      desc: 回合上限类别词表（5 值）
    - pkg: roundguard
      backing: roundShapeAll
      enum: AllShapes
      predicate: Shape.Valid
      family: Shape
      desc: 无进展形态词表（6 值）
```

**(d) `word_tables.relations`**（同形取值必须声明关系）：

本包 `failed` 与 `contract`（会话失败）、`statemachine`（环节失败）、
`taskboard`（任务失败，**允许重新认领、非终态**）三张表同形 ⇒ 三条 `distinct`：

```yaml
    - a: {pkg: contract, enum: AllStatuses}
      b: {pkg: roundguard, enum: AllRoundReasons}
      relation: distinct
      anchors:
        a: [{all: true}]
        b: [{all: true}]
    # 同上（statemachine 环节状态与 contract 同族）
    - a: {pkg: statemachine, enum: AllStepStatuses}
      b: {pkg: roundguard, enum: AllRoundReasons}
      relation: distinct
      anchors:
        a: [{all: true}]
        b: [{all: true}]
    - a: {pkg: taskboard, enum: AllStates}
      b: {pkg: roundguard, enum: AllRoundReasons}
      relation: distinct
      anchors:
        a: [{all: true}]
        b: [{all: true}]
```

### 9.2 `api-lifecycle.yaml`

```yaml
  - pkg: roundguard
    level: Experimental
    since: v0.60.0        # 以实际发版版本为准（本文写作时上一版为 v0.51.0）
    reason: 回合守卫三件套刚下沉：结局分类的取值集合与「不得续跑」的判据边界仍待第二个宿主验证（比照 steer 的定级理由）。
```

包注释必须同步打标记：`生命周期: Experimental（v0.60.0 定级）——本包仍会改、删、拆，
不承诺任何兼容；升级风险自负；定级理由见 api-lifecycle.yaml，分级规则见
docs/api-compat.md。`

### 9.3 `docs/error-semantics.md`

本包只有一个导出哨兵（其余是判定结论，不是错误）：

```go
// ErrInvalidBudget 预算声明非法（负数上限等）。
var ErrInvalidBudget = errors.New("回合预算声明非法")
```

登记进错误语义总表的 `### roundguard` 小节：触发条件 = 装配期 `Budget.Validate`
判负值上限；调用方处置 = **修声明**（调用方配置错，重试无意义）。
⚠ 小节名必须与 `api-lifecycle.yaml` 的 `pkg` 完全一致，否则 `make apilifecheck` 红。

### 9.4 `README.md`

- **包表**新增一行（`| \`roundguard\` | <生产行数> | oekit/pkg | <描述> |`）：
  行数由 `make docsync` 实测（**25% 相对容差**），描述须与包头口径一致。
- **工程底座表**（生产代码行数 / 生产文件数 / 测试文件数）**判相等、无容差**
  ⇒ 新增包必然改动该表三项，实施时须同步刷新。

### 9.5 其它登记面

- `docs/AI-GUIDE.md`：包数与分组表（「51 个包分五组」「机制面 36 个」等计数）需同步。
- `docs/USAGE.md`：新增「宿主怎么接」的章节（§11 的接线示例）。
- `CHANGELOG.md`：发版时补记（按 tag 切段）。
- `Makefile`：**不动**（门禁自动覆盖新包）。

## 10. 测试与反向验证

### 10.1 必测的判据（每条都配「坏实现会红」的对照）

| 判据 | 正向用例 | 反向验证（变异必须红） |
|---|---|---|
| 回合上限 | `MaxRounds=3`，已用 2 ⇒ 允许；已用 3 ⇒ 拒绝且 `Limit=LimitRounds` | 把 `>=` 写成 `>` ⇒ 红 |
| 按工具上限 | `MaxPerTool["read"]=2`，`read` 已用 2 ⇒ 拒绝且 `Detail` 含工具标识；`write` 不受影响 | 去掉工具维度过滤 ⇒ 红 |
| 预算声明校验 | `MaxRounds=-1` ⇒ 报错 | 去掉负值分支 ⇒ 红 |
| 结局分类 | `{ToolCalls:1}` ⇒ `ReasonToolCalls`+继续；`{Completed:true}` ⇒ `ReasonCompleted`+收敛；**`{ToolCalls:1, Completed:true}` ⇒ `ReasonToolCalls`**（要工具优先）；零值 ⇒ `ReasonNoOutcome`+收敛；`{Failed:err}` ⇒ `ReasonFailed`；`{Aborted:true}` ⇒ `ReasonAborted` | 交换「要工具」与「给终答」的判定次序 ⇒ 红（§5.3 的理由即用例） |
| 重复检测 | `A A A` ⇒ `ShapeRepeat` | 去掉「全等」判定 ⇒ 红 |
| 震荡检测 | `A B A B A B` ⇒ `ShapeOscillation` | 把周期 2 判成周期 1 ⇒ 红 |
| 漂移检测 | `page=1/2/3`（`Normalized` 同）⇒ `ShapeDrift` | 用 `Fingerprint` 代替 `Normalized` 比较 ⇒ 红 |
| 错误螺旋 | 三连 `Failed` ⇒ `ShapeErrorSpiral` | 把它排在重复之后 ⇒ 红（次序即语义） |
| 空转检测 | 三回合零工具且未收敛 ⇒ `ShapeIdle` | 去掉 `Settled` 条件 ⇒ 红（会把正常回合判空转） |
| 组合结论 | 回合额度用尽 ⇒ 按 Policy 判 `ReasonForcedFinal`/`ReasonBudgetExhausted`+`Limit=LimitRounds`；命中停滞 ⇒ `ReasonStalled`+`Limit=LimitStalled`；**结局终局优先于预算**（`Completed` 且已超限 ⇒ `ReasonCompleted`） | 去掉「终局优先」分支 ⇒ 红 |
| 处置口径分流 | 同一额度用尽：`PolicyForceFinal` ⇒ `ReasonForcedFinal`（`IsFailure()==false`）；`PolicyFailFast` ⇒ `ReasonBudgetExhausted`（`IsFailure()==true`） | 让两种 policy 走同一条分支 ⇒ 红 |
| 失败/温和可区分 | `ReasonForcedFinal`/`ReasonStalled` ⇒ `IsFailure()==false`；`ReasonFailed`/`ReasonAborted`/`ReasonNoOutcome`/`ReasonBudgetExhausted` ⇒ `IsFailure()==true` | 让 `IsFailure` 对全部取值返回 true ⇒ 红 |
| 处置分档 | 首次命中 ⇒ `Warn=true` 且 `Stalled=false`；再犯 ⇒ `Stalled=true` | 让首次命中直接 `Stalled=true` ⇒ 红 |
| 进展不打断 | 连续同指纹但 `ResultChanged=true` ⇒ **不判重复** | 去掉 `ResultChanged` 打断 ⇒ 红 |
| 豁免面 | `Exempt=true` 的回合既不参与重复计数、也不参与空转判定 | 让豁免只对重复生效 ⇒ 红 |
| 处置口径分流 | `PolicyForceFinal` ⇒ 超限判 `ReasonForcedFinal`（`IsFailure()==false`）；`PolicyFailFast` ⇒ 判 `ReasonBudgetExhausted`（`IsFailure()==true`） | 让两种 policy 走同一条分支 ⇒ 红 |
| 回合数口径 | `Used.Rounds` 数的是**回合**不是 provider 请求数（同一个值两种解释） | 把 `Rounds` 改成记请求数 ⇒ 红（口径即契约） |

**反向验证矩阵**（2026-11-21 补，Task 6）：上表是「逐条判据 × 一条对照」，而**矩阵**
把它铺成一张可执行的表（`roundguard/mutation_test.go`：机制分组 → 判据 → **变异类**
→ 钉住它的用例），外加**两条穷举对照**（结局分类 128 种输入组合；无进展检测 20 种
合成回合 × 长度 0..4 的全部序列 × 3 组参数）与**一条组合面穷举**（`Decide` 240 种
组合）——每条都与**独立按本 spec 写成的 oracle** 逐字段对照。

为什么要从「逐条」升级到「矩阵 + 穷举」：① 逐条钉只覆盖「两两同成立」，而「三形态
同成立」「组合起来才错」这类落点只有穷举才盖得住；② 穷举对照把「好实现能过」升级成
「坏实现会红」——实现的任何一处改动（哪怕只在一个位置、一个阈值）都会与 oracle 分歧。
白盒那一半（承重守卫只在未导出面上）在 `roundguard/mutation_internal_test.go`。

### 10.2 其余门禁

- **逐包覆盖率** `PKG_COV_MIN=70`（新包默认无豁免）⇒ 必须带测试且达标。
- **单文件 400 行上限** ⇒ 预切分：`roundguard.go`（包头+结局）/`budget.go`/
  `meter.go`/`detect.go`/`decide.go`/`word.go`（词表与枚举出口）/`errors.go`。
- **errors 单包**：只用 `github.com/oekit/pkg/errors`。
- **禁裸 goroutine**：本包无并发面（纯函数 + 进程内计数器，计数器非并发安全 ⇒
  包注释写明**调用方自行加锁**，或改用 `sync.Mutex`？**取后者**：判据包不该把
  并发留给调用方猜）。
  （实施时二选一，倾向带锁以免宿主各自发明加锁方式。）
- **禁 printf 回调**：`logf func(string, ...any)` 一类签名一律不进本包。
- **测试夹具用合成数据**：不得出现「内存分析Agent」「本机」「故障诊断」等词。

## 11. 宿主接线示例（伪代码，宿主实现）

⚠️ **额度判定只留 `Decide` 一处**：宿主侧**不要**另开一套「回合数额度 / 工具调用
总额度」的判定分支——`Decide` 内部调的就是 `CheckRounds` / `CheckToolCalls`
（§6.3），宿主再判一遍会出三个问题：

1. **重复判定**：两套口径各判各的，漂了没人知道；
2. **死代码**：`Decide` 给出终局结论之后，宿主那套分支**永远执行不到**；
3. **处置口径分叉**：本地布尔（`forceFinal` 之类）绕过 `Budget.EffectivePolicy()`，
   把「零值缺省 = 强制收尾」误判成「没设」。

⇒ 处置口径一律取 `Budget.EffectivePolicy()`（**别直接读 `Budget.Policy`**——零值
是合法缺省，不是第三种口径）；停法一律按 `d.Reason` 分流。

```go
// —— 宿主侧（模型面）：内核不调模型，这段在宿主仓 ——
b := roundguard.Budget{
    MaxRounds:    40,                              // 回合数上限
    MaxToolCalls: 20,                              // 工具调用总数上限
    MaxPerTool:   map[string]int{"send_email": 2}, // 单工具上限
    Policy:       roundguard.PolicyForceFinal,     // 缺省即此；改 FailFast 即失败即止
}
cfg := roundguard.Config{}
meter := roundguard.NewMeter()
var obs []roundguard.Observation

for {
    // ① 模型调用：这一回合**能不能开始**由上一回合末的 Decide 说了算（见 ③）
    //    —— 额度判定只做一次、就在 Decide 里，宿主侧不再另开一套额度分支
    resp := model.Call(ctx, msgs, tools)
    meter.Round()

    // ② 工具执行前：单工具上限 ⇒ 否决这一次，但**配对不破**
    //    （单工具额度没有内部对应面，只能宿主在工具执行前自己判）
    var results []toolResult
    for _, tc := range resp.ToolCalls {
        if v := roundguard.CheckTool(b, meter.Used(), tc.Name); !v.Allowed {
            results = append(results, toolResult{ID: tc.ID, Err: errors.New(v.Detail)}) // 回灌失败结果
            continue
        }
        res := execTool(ctx, tc)
        meter.Tool(tc.Name)
        results = append(results, res)
    }

    // ③ 回合末：**每回合一条** Observation（工具调用折叠进本回合的计数与指纹）
    obs = append(obs, hostRoundObservation(resp, results))
    d := roundguard.Decide(roundguard.Snapshot{
        Outcome: hostOutcome(resp, results), // 宿主把本回合的模型回复映射成 Outcome
        Used:    meter.Used(),
        Obs:     obs,
    }, b, cfg)

    if !d.Continue {
        if d.Reason == roundguard.ReasonForcedFinal {
            // 软：额度用尽但按「强制收尾」口径处置 ⇒ 再给模型一次**不带工具**的
            // 收尾机会，让它用已收集的证据给出终答（产物仍可用，不是失败）
            final := model.Call(ctx, msgs, noTools())
            meter.Round()
            out := hostOutcome(final, nil)
            out.ForcedFinal = true // 标记这是「额度耗尽后的强制收尾步」
            return finish(roundguard.Decide(roundguard.Snapshot{
                Outcome: out,
                Used:    meter.Used(),
                Obs:     obs,
            }, b, cfg))
        }
        // 其余停法按 Reason 处置：budget_exhausted / failed / aborted / no_outcome
        // 是失败型（IsFailure()==true）；stalled 是温和叫停（产物仍可用）
        return finish(d)
    }
    msgs = append(msgs, assistantMsg(resp), toolResultsMsg(results))
    if d.Warn {
        // 首档：把警告回灌给模型（**文案归宿主**），给它一次自我修正的机会。
        // ⚠️ 必须排在 assistantMsg / toolResultsMsg **之后**：警告是针对**这一回合**的
        //    判据 ⇒ 排在它前面 = 模型先看到警告、后看到触发警告的那一回合
        //    （**语义倒置**；而配对不破 ⇒ 没有编译错误、没有断言能抓到它）。
        // ⚠️ 这个位置**不会**漏掉警告：Warn=true ⇒ Continue=true（首档只警告不停止，
        //    见 decide.go 的 tierFinding：Stalled 与 Warn 互斥）⇒ 上面那个
        //    !d.Continue 分支在 Warn=true 时根本不会进。
        msgs = append(msgs, hostWarnMessage(d.Shape))
    }
}
```

**接线纪律两条**（写进 USAGE）：

1. **失败要回灌、要记账**：工具失败既作为 `tool_result` 回灌给模型（模型自行修正），
   又作为 `Observation.Failed` 记一条——只回灌不记账 ⇒ 无进展检测永远看不到错误螺旋。
2. **指纹要稳定，且要带上「结果变了」**：指纹由宿主算，同一逻辑动作必须给同一指纹；
   归一指纹要抹掉 **易变部分**（页码/时间戳/行号），否则漂移检测永远不命中。
   「相同调用但输出变了」必须显式记为 `ResultChanged: true`——那是**进展**，
   不记就会把有意义的重试误判成循环。轮询 / 查状态类动作要记 `Exempt: true`。

## 12. 未决点（评审需拍板）

| # | 未决点 | 本设计的默认取值 | 备选 |
|---|---|---|---|
| 1 | 包名 | `roundguard` | `modelturn`（更明确、包名更长） |
| 2 | 「回合」单位的中文名 | 回合 / round | 「迭代」（与 Go `iter` 包名族太近，不取） |
| 3 | `Meter` 是否并发安全 | **带锁**（宿主不必各自发明加锁方式） | 无锁 + 包注释要求调用方加锁 |
| 4 | `Outcome.Completed` 与 `ToolCalls>0` 同时成立时 | 「要工具」优先（§5.3） | 终答优先（会让未执行的工具被吞） |
| 5 | `Config.Window` / `Repeat` 缺省值 | 8 / 3（原定 6 / 3，因 §7.3 定档需 L>W ⇒ 窗口 6 时震荡停止档不可达，故改为 8 = 2×3+2 并向上钳位） | 4 / 2（更敏） |
| 6 | 跨进程回合累计 | **不做**（§8 末条；外部实现同样全程进程内） | 做（需新增存储面契约，范围显著变大） |
| 7 | 额度用尽后的**缺省处置口径** | `PolicyForceFinal`（腰斩最接近答案的那一刻，代价最大） | `PolicyFailFast`（pydantic-ai / Claude 的主流口径：失败即止） |
| 8 | 导出 API 面上限 | **设门禁**（见 §13 的抄法） | 不设（导出面随实现自由生长） |
| 9 | 处置口径是否**按额度维度分别配置** | **不配置**（一个 policy 管全部额度 ⇒ 跨宿主口径一致） | 允许分别配置（更灵活，但「同一上限不同宿主不同处置」的风险回来了） |
| 10 | 额度维度的**缺省值** | 不设缺省（全 0 = 不限，交给宿主显式声明） | 设缺省（agentloop 的缺省是 `MaxToolCalls=5`、`MaxSteps=2×5+2=12`） |

## 13. 与模型侧 loop 库的关系（2026-11-21 补）

**结论：不竞争，是上下两层。**

`smhanov/agentloop`（MIT，Go 1.21+，约 19.6k 行含测试，依赖只有姊妹项目 `llmhub` +
`yaml.v3`）已经把模型侧 loop 做成了一个可直接 import 的库：预算、hooks、skills、
压缩、流式、trace 全齐，另带 search / bash 两个 preset。

⇒ **模型侧 loop 不必由本仓或宿主重写**；宿主可以直接用它。

**那本包还有什么位置**：模型侧库是**驱动层**（谁去调模型、谁去执行工具），
本包是**判据层**（什么算停、什么算超限、什么算没进展）。差别在**可替换性**：

| | 模型侧库（agentloop） | 本包（roundguard） |
|---|---|---|
| 判据位置 | **内置**在库里（`dupCallLimit` 等是库的实现细节） | 独立判据包，宿主可换口径 |
| 跨宿主 | 库是单宿主直接 import | 多宿主共享同一份判定 |
| 覆盖范围 | 重复检测**单形态**（连续同调用同输出） | 五形态（重复 / 震荡 / 漂移 / 错误螺旋 / 空转） |
| 进程边界 | 全程进程内 | 进程内（跨进程账仍不做，§8） |

**接缝（很干净）**：agentloop 的 `Hooks` 正好是本包判据的落点——

| 库里的 hook | 本包对应的判定 | 宿主怎么用 |
|---|---|---|
| `AfterGenerate`（`Finish` 可翻转） | `Classify`（回合结局） | 判出的 `Continue=false` ⇒ 让 hook 结束本回合 |
| `AfterToolBatch`（`Skip` ⇒ 优雅停止） | `Decide`（组合结论） | `Finding.Stalled` / `LimitRounds` ⇒ 置 `Skip`，**温和叫停**（不是失败） |
| `BeforeGenerate`（`Skip` ⇒ 停止） | `CheckRounds` / `CheckToolCalls` | 额度用尽 ⇒ 置 `Skip`；若走 `PolicyForceFinal` 则不 Skip，改由库自己的强制收尾生效 |
| `BeforeToolCall`（`ErrSkipToolCall` 否决单次调用） | `CheckTool`（单工具上限） | 超限 ⇒ 否决该次调用，但**仍回灌一条失败结果**给模型（配对不破） |

⇒ 宿主用 agentloop 做驱动、用 roundguard 做判据，**两层不重复**：库的硬上限与
强制收尾是它的实现，我们的判据是可替换的那一层。

**另三家的接缝**（同一套判据接不同实现，宿主按自己用的那家接线即可）：

| 实现 | 它提供的接缝 | 本包对应的判定 | 备注 |
|---|---|---|---|
| OpenAI Agents SDK | `max_turns` + **max-turn handler**（handler 产出合成输出） | `Budget.Policy=PolicyForceFinal` 的等价物 | handler 是宿主可插拔的兜底，与我们的 policy 是同一件事的两种放法 |
| OpenAI Agents SDK | `RunHooks`（lifecycle 回调） | `Classify` / `Decide` | 与 agentloop 的 Hooks 同构 |
| pydantic-ai | `UsageLimits`（request / tool_calls / 三种 token / per-request / cost） | 只接**次数**维度（`MaxRounds`/`MaxToolCalls`/`MaxPerTool`） | token 与 cost 维度归本仓 `budget`/`cost`；它的 `check_before_request` 与我们的「调用前检查」同形 |
| pydantic-ai | `request_limit` 的口径 | `Used.Rounds`（回合 ≠ 请求数） | §6.2 的口径条款即抄它 |
| smolagents | `_handle_max_steps_reached()`（触顶后再调一次模型出终答） | `PolicyForceFinal` 的第二种实现 | 差别：它把该步标成错误（产物可用但带错误标记） |
| smolagents | `planning_interval`（周期性强制重规划） | `Observation.Exempt` 的第二个豁免场景 | §7.1 的真冲突：规划步零工具调用，会被空转判据误伤 |
| Claude Agent SDK | `max_turns` + `terminal_reason` | `LimitRounds` + `Reason` 词表 | 它是纯硬口径（`error_max_turns`），对应我们的 `PolicyFailFast` |

**另抄一条工程做法——导出 API 面上限门禁**：agentloop 用 `apibudget_test.go` 把三个
包的**导出符号总数**钉在天花板（引入时实测 196 = core 112 + search 51 + bash 33，
再 +10% 取整），理由是「加公开 API 必须是一次自觉行为，抬高天花板要单独一次变更并
附理由」。本包照抄：**`roundguard` 的导出符号总数应有上限**，超限即红，抬上限的
变更必须自带理由（§12 未决点 #8）。

**不要抄的**：agentloop 把「模型侧 loop」与「判据」焊在一个库里（判据内置、不可替换）。
本仓的纪律正相反——判定语义归内核、判定依据归宿主。模型侧库是**下游消费者**，
不是本包的竞品。

## 14. 变更历史

- 2026-11-21：初稿（设计待评审）。基于 2026-11-21 的外部参照系调研与仓内实测
  （`MaxTurns|maxSteps|…` 全仓零命中）成文。
- 2026-11-21（修订 r1）：实读 `smhanov/agentloop` 源码后改了六处判断——
  ① 预算拆**软硬两级**（工具预算耗尽 ⇒ 强制收尾而非失败；回合硬上限才是失败）；
  ② 结局词表加 `forced_final`，并新增 `Reason.IsFailure()` 判据区分
  **失败型收敛**与**温和叫停**；
  ③ `Observation` 加 `ResultChanged`（输出变了 = 进展）与 `Exempt`（轮询类豁免）；
  ④ 重复检测处置**分三档**（警告回灌 → 再犯才停），不再一命中就停；
  ⑤ §8 划界表补「压实：真源替换 vs 临时视图替换」与「强制收尾不可否决」；
  ⑥ 新增 §13：与模型侧 loop 库的关系（**驱动层 / 判据层**，不竞争）+ 抄它的
  **导出 API 面上限门禁**。
- 2026-11-21（修订 r2）：**五家实现对拍后**改三处判断——
  ① **r1 的「软硬两级」是过拟合到 agentloop 一家**：五家实测出三种触顶口径
  （pydantic-ai / Claude 硬失败；agentloop 工具预算强制收尾；OpenAI 装了 max-turn
  handler 就产出合成输出；smolagents 触顶后再调一次模型出终答并标错误）⇒ 改为
  **「额度维度（事实）+ 处置口径（可配）」**：新增 `StopPolicy`（缺省
  `PolicyForceFinal`）、`Budget.Policy`、`Decide` 按 policy 分流；§1.4 新增
  五家实测对照表。
  ② **「回合」拿到外部印证**：OpenAI「A turn is defined as one AI invocation
  (including any tool calls that might occur)」；pydantic-ai 的 `request_limit`
  「bounds the number of turns of the agent loop」；agentloop 的 Steps
  「one step = one inference plus its tool executions」——三家都指模型往返，
  只有本仓 `eventbus` 用 turn 指编排会话运行 ⇒ §3 三分表必要且中文名对得上。
  ③ **smolagents 的 `planning_interval` 暴露一个真冲突**：它每隔 N 步插一个规划
  回合，而规划回合必然零工具调用 ⇒ 会被我们的空转判据误判成停滞 ⇒
  `Observation.Exempt` 扩为通用豁免面（不再只管重复计数），§7.1 写明两个必须
  豁免的场景（轮询类动作 / 刻意规划步）。
  另补：pydantic-ai 的 `request_limit` 明确「回合数 ≠ provider 请求数」⇒ §6.2
  写死 `Used.Rounds` 口径；§8 补「限额声明面」一行（记为可被推翻的取舍）；
  §13 接缝表扩到四家；§11 示例按 Policy 更新；§12 未决点增至 10 条。
