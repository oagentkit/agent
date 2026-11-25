// Package roundguard 回合守卫：模型侧循环的**结局分类 / 次数预算 / 无进展检测**
// 与它们的**组合结论**——跨宿主共享的判据。
//
// 本包自 goagent-kernel 平移而来（归属评测 2026-11-26：单 agent 循环判据，
// 与多智能体编排无关）。API 承诺：0.x 阶段 minor 允许 breaking（org 通行约定）。
//
// # 它补的是哪一段
//
// 主流模型侧 loop 的**循环本体**已经收敛成同一段代码（模型请求工具就执行、
// 回灌结果、继续；模型给终答就停）——差异全在**循环周边**：这次还能不能转、
// 这一回合以什么结束、这一圈是不是白转。这三问与「谁来跑模型」无关，与「跑
// 多少圈」有关；放在宿主侧的后果是**每个宿主各判一套**：同一段循环在 A 宿主算
// 「第 12 圈超了」、在 B 宿主算「第 8 圈还能转」，且没有任何机器能发现。
//
// 本包把这三问收成**机制面**：纯函数，零领域词，**内核不调模型**。
// **四件套**：结局分类（Reason 词表 + Classify）/ 次数预算（Budget / Used /
// Meter / 三个 Check*）/ 无进展检测（Shape 词表 + Config + Detect）/ 组合结论
// （Snapshot + Decision + `Decide`）——`Decide` 把前三条判据收成**一条可执行
// 结论**（结局终局优先 → 额度按处置口径分流 → 无进展兜底）：宿主拿到结论就能
// 直接决定「继续 / 温和叫停 / 失败即止」，不必自己把三张判据重新拼一遍。
//
// 边界不变：**判据归内核，模型侧归宿主**——本包只给结论，不调模型、不写产物、
// 不解释工具语义。
//
// # 与既有机制划界（同词不同域，互指不重复）
//
//   - vs `budget`：budget 管**输入侧 token 估算**（「下一轮要发多少」），
//     本包管**次数**。量纲不同，不合并。
//   - vs `cost`：cost 是**已发生消费的记账**，本包是**约束**——同 budget/cost
//     的既有切分：记与判分属两包。
//   - vs `deadline`：deadline 是**墙钟**，本包是**次数**。
//   - vs `watchdog`：watchdog 判**工具调用配对**与**编排轮次**括号配对；
//     本包的单位是**模型回合**（一次模型往返），与它不互相替代。
//   - vs `finalize` / `contract`：finalize 是**会话级** Cause→终态路由；
//     本包是**回合级**结局分类，不产出 ctx Cause、不碰会话状态词表。
//   - vs `retryplan` / `toolfail`：本包**不重判**「该不该重试」，只在「额度用尽」
//     「无进展」这两种**续跑必然再撞**的情形下判不得续跑。
//
// # 三分命名
//
//   - **轮次**（turn）：一次**编排会话运行**——`eventbus.EventTurnStarted/Ended`。
//   - **环节**（step）：宿主定义的**业务步骤**——`statemachine.StepStatus`。
//   - **回合**（round）：模型侧**一次往返**（一次模型调用 + 其工具执行）——本包。
//
// # 单元词表
//
// 回合结局（Reason，10 值）、额度类别（Limit，5 值）、处置口径（StopPolicy，
// 2 值）与无进展形态（Shape，6 值）是本包当前的判定词表：取值决定结论，
// 别处出现必须引用本包的常量，不得手抄（登记面见 architecture-policy.yaml 的
// word_tables 段）。
package roundguard

// Reason 回合结局（判定词表；主人表 roundReasonAll，枚举出口 AllRoundReasons）。
//
// 零值是**空串**——不在词表内（Valid 判假），即「未设 / 读不懂」，**不冒充任何
// 结局**。与 statemachine.StepStatus 的零值处理同款。
type Reason string

const (
	// ReasonCompleted 模型给出终答，本回合不再要工具。
	ReasonCompleted Reason = "completed"
	// ReasonToolCalls 模型请求工具 ⇒ 允许进入下一回合。
	ReasonToolCalls Reason = "tool_calls"
	// ReasonHandedOff 模型请求交接给别的执行方 ⇒ 收敛并移交。
	ReasonHandedOff Reason = "handed_off"
	// ReasonPaused 服务端要求原样再问（pause_turn）⇒ 允许进入下一回合。
	ReasonPaused Reason = "paused"
	// ReasonFailed 模型调用或工具执行明确报错 ⇒ 失败型收敛。
	ReasonFailed Reason = "failed"
	// ReasonNoOutcome 本回合无任何产出 ⇒ 失败型收敛（保守侧）。
	ReasonNoOutcome Reason = "no_outcome"
	// ReasonAborted 被中止（用户取消 / 停机 / 接管）⇒ 失败型收敛。
	ReasonAborted Reason = "aborted"
	// ReasonBudgetExhausted 额度用尽且按**失败**口径处置 ⇒ 失败型收敛。
	ReasonBudgetExhausted Reason = "budget_exhausted"
	// ReasonForcedFinal 额度用尽但按**强制收尾**口径处置 ⇒ 产物仍可用。
	ReasonForcedFinal Reason = "forced_final"
	// ReasonStalled 无进展 ⇒ 温和叫停（不是失败）。
	ReasonStalled Reason = "stalled"
)

// String 机器可读短码（展示文案归宿主）。
func (r Reason) String() string { return string(r) }

// Valid 报告该取值是否在回合结局词表内。
func (r Reason) Valid() bool { return roundReasonAll[r] }

// IsFailure 报告该结局是否为**失败型**收敛。
//
// 为什么要单独一个判据而不是让宿主自己列清单：「被温和收停」与「真的坏了」在
// 宿主侧必须可区分——前者仍有可用产出（模型已给出终答、或证据已齐备只差收束），
// 后者只有半截 transcript。两类混在一张「都收敛了」的清单里，宿主就会把「被上限
// 温和收停」也当故障上报（假红）。
//
// 与上游模型侧实现的同构：agentloop 的 AnswerStatus 把 StatusStopped（hook 跳过
// / 重复检测命中）与 StatusFailed（传输/hook/ctx 错误）分成两个值。
func (r Reason) IsFailure() bool {
	switch r {
	case ReasonFailed, ReasonNoOutcome, ReasonAborted, ReasonBudgetExhausted:
		return true
	}
	return false
}
