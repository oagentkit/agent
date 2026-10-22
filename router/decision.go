// 决策面（System One 等非自回归判定模型）接入：**判定**与**生成**分家。
//
// # 为什么 router 需要这一面
//
// router 现有的分类执行体是 LLM（自回归生成）：有 messages 概念、输出散文、要一次
// 模型调用。但「把输入路由到**有限候选集**」这件事本身不需要生成——它是非自回归
// **判定模型**（System One 协议：Jev 托管 / Laya 自托管 / Kev / Clef / CLM）的主场：
// 一次前向 pass 答完全部问题，输出**有限标签 + 概率分布 + 校准置信度**，毫秒级、
// 几乎免费。
//
// 于是 router 有了**三种执行体**：
//
//	词面（确定性，本包 KeywordHit）→ 决策面（判定，毫秒级）→ LLM 面（生成，秒级）
//
// 决策面夹在词面与 LLM 面之间：它比词面能理解语义，比 LLM 面快且便宜。
//
// # 编排语义（fail-open）
//
// 决策面**先于** LLM 面被问；达门槛即采纳（省一次 LLM 分类调用），未达门槛 /
// 端点故障 / 影子模式 ⇒ **一律回落 LLM 面**——判定面是加速件，**不得阻断主链**。
//
// # 置信口径由实现方负责
//
// 决策面返回的置信度语义**取决于实现方**：不同后端给法不一致（实测：某后端的
// choice 用自有公式、noul 直接给概率；另一后端按公开公式给）⇒ 同一门槛在两端
// 含义会漂移。实现方应当**按统一口径本地算**（从概率分布算，见对应客户端包的
// EffectiveConfidence），本包只负责比门槛。
//
// 另：口径**随候选集规模变化**（候选越多、同一 p_top 折出的置信度越低）⇒
// 门槛须随候选集标定，不能跨候选集移植。
package router

import (
	"context"
	"errors"
	"fmt"
)

// 判定来源常量（Decision.Source）。
const (
	// SourceLLM 判定来源：LLM 分类面（**零值即 llm**，向后兼容既有消费方）。
	SourceLLM = "llm"
	// SourceDecision 判定来源：决策面（System One 等非自回归判定模型）。
	SourceDecision = "decision"
)

// errDecisionShadow 影子模式的内部标记：决策面被调用但**只记不采**。
// 只进观察钩子，不外泄给调用方（调用方看到的是 LLM 面的正常结果）。
var errDecisionShadow = errors.New("router: 决策面影子模式（只记不采）")

// DecisionFace 决策面的窄接口。
//
// 实现方（客户端包）负责：把候选集映射成判定题的候选、把槽位规格映射成题目、
// 以及**按自己的口径算校准置信度**。本包不关心它怎么实现，只关心编排语义
// （门槛/回落/影子/来源标注）。
//
// 实现约定：
//   - 返回的 Route 必须**恰好等于**候选集里的某个名字（本包会校验，不在表内即拒）；
//   - 返回的 Confidence 应是**校准置信度**（0-1，越大越确信）；
//   - 返回的 Slots 只保留**本包配置过的槽位名**（其余一律丢弃，防幻觉槽位名透传）。
type DecisionFace interface {
	// Classify 用给定候选集与槽位规格做一次判定。
	//
	// state 是待判定的原始输入；routes/slots 是本包的候选集与槽位规格
	// （实现方据此组题）。返回的 Decision 只需填 Route/Confidence/Slots。
	Classify(ctx context.Context, state string, routes []Route, slots []SlotSpec) (Decision, error)
}

// DecisionObserver 决策面结果观察钩子（可选；观测面注册，如指标计数 / 一致率统计）。
//
// 调用时机：决策面被调用后**每次**回调一次——
//   - 采纳时：err = nil；
//   - 未过门槛 / 端点故障：err 非 nil（原因在 err 里）；
//   - 影子模式：err = errDecisionShadow 的同义错误（**只记不采**）。
//
// 钩子在**分类热路径**上被调用，实现方须自行保证线程安全；不得在钩子里做重活。
type DecisionObserver func(d Decision, err error)

// classifyByDecisionFace 问一次决策面并做本包侧的自守恒校验
// （候选集合法性 + 置信门槛 + 槽位名自守恒）。
//
// 返回 (decision, err)：
//   - err == nil ⇒ 决策面给出了**可采纳**的结果；
//   - err != nil ⇒ 不可采纳，调用方应回落 LLM 面（err 供观察钩子与日志用）。
func (r *Router) classifyByDecisionFace(ctx context.Context, input string) (Decision, error) {
	d, err := r.decFace.Classify(ctx, input, r.routes, r.slots)
	d.Source = SourceDecision
	d.Confidence = clamp01(d.Confidence) // 越界置信度统一钳位（与 LLM 面同款）
	d.Slots = sanitizeSlots(d.Slots, r.slotByName)
	if err == nil {
		if _, ok := r.byName[d.Route]; !ok {
			err = fmt.Errorf("router: 决策面结果 %q 不在路由表", d.Route)
		} else if r.decMinCF > 0 && d.Confidence < r.decMinCF {
			err = fmt.Errorf("router: 决策面置信度 %.2f 低于阈值 %.2f", d.Confidence, r.decMinCF)
		}
	}
	return d, err
}

// observeDecision 决策面结果观察（钩子非空才调；钩子 panic 不影响主链）。
func (r *Router) observeDecision(d Decision, err error) {
	if r.decObserver == nil {
		return
	}
	r.decObserver(d, err)
}
