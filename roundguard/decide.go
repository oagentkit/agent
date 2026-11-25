package roundguard

// Snapshot 一回合末尾的完整事实（宿主组装）。
//
// 三项事实缺一不可：结局（这一回合以什么结束）、用量（额度还剩多少）、
// 回合历史（最近这几圈是不是在原地打转）。内核只对这三组**机械事实**做判定，
// 不解释工具语义、不猜「什么是易变部分」（指纹仍由宿主算，见 detect.go）。
type Snapshot struct {
	// Outcome 本回合的结局事实。
	Outcome Outcome
	// Used 当前用量快照。
	Used Used
	// Obs 回合历史（含本回合，按时间序）。
	Obs []Observation
}

// Decision 一条可执行结论（组合判定面）。
//
// 宿主拿到这一条就能直接决定「继续 / 温和叫停 / 失败即止」——不必自己把
// 结局分类、额度判定、无进展检测三张判据重新拼一遍（各宿主各拼一套 =
// 同一段循环在不同宿主得到不同结论，且没有任何机器能发现）。
type Decision struct {
	// Reason 结局分类。
	//
	// 多数取值是**模型侧事实**（终答 / 要工具 / 交接 / 报错 / 中止），
	// 三个取值是**内核判出的停法**（强制收尾 / 预算耗尽 / 无进展）——
	// 混进同一张表，宿主才能用一张词表判「这一回合怎么结束的」。
	Reason Reason
	// Continue 允许**按原样**进入下一回合。
	//
	// ⚠️ false 有**多种含义**，靠 Reason 区分，宿主不得一律当失败处理：
	// ReasonFailed / ReasonNoOutcome / ReasonAborted / ReasonBudgetExhausted
	// 是失败型（Reason.IsFailure()==true）；ReasonForcedFinal 是转强制收尾
	// （产物仍可用）；ReasonStalled 是温和叫停（也不是失败）。
	//
	// ⚠️ 无进展**首档**时本项仍为 true（只该警告回灌、不停止，见 Warn）；
	// 停止档才为 false。
	Continue bool
	// Limit 命中的额度类别（LimitNone = 未因额度叫停）。
	//
	// ⚠️ 无进展命中时**首档与停止档都置 LimitStalled**（命中的确是「无进展」
	// 这一维），单看本项判不出停不停——是否叫停看 Continue 与 Warn 两项。
	Limit Limit
	// Shape 无进展形态（仅 LimitStalled 时有效；未命中时为 ShapeNone）。
	Shape Shape
	// Warn 无进展**首档**：只该警告回灌，不停止。
	Warn bool
	// Used 判定时的用量快照（留痕用）。
	//
	// ⚠️ 本字段与 `Snapshot.Used` **共享同一份 `Tools` 映射**（不是副本）：
	// 宿主若在判定之后改写自己的用量表，这份留痕会跟着变。判定本身不受影响
	// （判定在返回前就已做完）——但「留痕」读起来是当时的用量，别拿它当审计
	// 凭据用。宿主若要审计凭据，自己留一份副本。
	Used Used
}

// Decide 合成一条结论：结局 → 额度（按 Policy 处置）→ 无进展。
//
// 判定优先级（顺序即语义）：
//
//  1. **结局终局优先**：模型给了终答 / 交接 / 报错 / 中止 ⇒ 以结局为准，
//     **不被额度打断**（模型说完了就是完了——哪怕刚好撞上上限）。
//  2. 否则（结局要求继续）：额度用尽 ⇒ 按 Budget.Policy 处置——
//     PolicyForceFinal ⇒ ReasonForcedFinal（转强制收尾，产物仍可用）；
//     PolicyFailFast ⇒ ReasonBudgetExhausted（失败型收敛）。
//  3. 否则：无进展命中 ⇒ ReasonStalled（**按档位决定 Continue**：
//     首档继续观察并回灌警告，停止档温和叫停）。
//  4. 否则：Continue=true，照常进入下一回合。
//
// ⚠️ Continue=false 有**三种含义**，靠 Reason 区分，宿主不得一律当失败处理：
// ReasonFailed / ReasonNoOutcome / ReasonAborted / ReasonBudgetExhausted 是
// **失败型**（IsFailure()==true）；ReasonForcedFinal 是**转强制收尾**；
// ReasonStalled 是**温和叫停**。
//
// **本函数不判「能不能续跑」**：那两件事归 retryplan（该走哪条路径）与
// toolfail（该不该重试）——本包抢过来判，就是两个包判同一件事，口径必然漂移。
// 本包只判「**这一回合怎么结束、还能不能按原样再转一圈**」。
//
// 前置条件：**Budget 须先经 Validate**（与三个 Check* 同一前提——未校验的
// 负值上限在那里是 fail-open）。
func Decide(s Snapshot, b Budget, cfg Config) Decision {
	// 零值即**合法取值**，不是「未设」：LimitNone = 未因额度叫停，
	// ShapeNone = 未命中任何形态。与 Reason 的零值语义不同（Reason 零值
	// 不在词表内），故这两个字段显式初始化，不留空串。
	d := Decision{Reason: ReasonNoOutcome, Limit: LimitNone, Shape: ShapeNone, Used: s.Used}
	route := Classify(s.Outcome)
	d.Reason = route.Reason
	if !route.Continue {
		// 结局已是终局：不给额度/停滞改写它的机会。
		return d
	}
	// 额度判定（调用前口径的同一份判据）：先回合数，后工具调用总数。
	// 两者都用尽时以**回合数**为准（更粗的那一维先撞——续跑必然再撞它）。
	if v := CheckRounds(b, s.Used); !v.Allowed {
		d.Limit = v.Limit
		d.Reason = budgetStopReason(b.effectivePolicy())
		return d
	}
	if v := CheckToolCalls(b, s.Used); !v.Allowed {
		d.Limit = v.Limit
		d.Reason = budgetStopReason(b.effectivePolicy())
		return d
	}
	// 无进展检测。
	f := Detect(s.Obs, cfg)
	if f.Shape != ShapeNone {
		d.Limit = LimitStalled
		d.Shape = f.Shape
		d.Warn = f.Warn
		d.Reason = ReasonStalled
		// 首档（Warn）只该**警告回灌**、不停止 ⇒ 继续观察下一回合；
		// 停止档才温和叫停。这是一档之差的落点（见 detect.go 的定档规则）。
		d.Continue = !f.Stalled
		return d
	}
	d.Continue = true
	return d
}

// budgetStopReason 额度用尽后的结论（按处置口径分流）。
//
// 分流只此一处：两种口径若各写各的分支，两条分支迟早漂成同一个值
// （那正是「处置口径可配」名存实亡的形态）。
func budgetStopReason(p StopPolicy) Reason {
	if p == PolicyFailFast {
		return ReasonBudgetExhausted
	}
	return ReasonForcedFinal
}
