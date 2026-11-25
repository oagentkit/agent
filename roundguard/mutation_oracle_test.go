package roundguard_test

import (
	"github.com/oekit/pkg/errors"

	"github.com/oagentkit/agent/roundguard"
)

// ── 规格 oracle：把 spec 写成**可执行的第二份实现**，与生产实现逐条对照 ──
//
// 为什么要有它：本机制**全行业空白**（五家主流实现零命中，见 spec §1.4）⇒ 没有
// 可抄的对照实现 ⇒ **「坏实现会红」是本包唯一的正确性来源**。逐个判据零散钉仍
// 漏掉两类变异：①「单条判据都过、组合起来才错」；②「改了一个阈值/一个位置，
// 而没有任何用例覆盖那个位置」。
//
// 故这里按 **spec**（不是按实现）独立写成第二份判定，逐条与实现对照。两条纪律：
//
//  1. oracle 的**算法刻意与生产实现不同**（生产向后扫描连续段，oracle 正向维护
//     连续段长度）⇒ 两边共错同一处的概率低得多；
//  2. oracle 里的**词表取值一律引用本包常量**（不得手抄字面量）。
//
// 合成数据：夹具只用中性词面（指纹用字母串、工具标识用 read / write）。

// errSynthetic 合成失败事实（判据只消费「有没有失败」这一位机械事实）。
var errSynthetic = errors.New("合成失败")

// ── 结局分类 oracle（spec §5.3）──

// classifyArm 结局分类的一个臂（**次序即语义**）。
type classifyArm struct {
	name   string
	hit    func(roundguard.Outcome) bool
	reason roundguard.Reason
	cont   bool
}

// classifyArms 结局分类的判定臂（按 spec §5.3 的先后排定：先具体后笼统）。
var classifyArms = []classifyArm{
	{"中止", func(o roundguard.Outcome) bool { return o.Aborted }, roundguard.ReasonAborted, false},
	{"报错", func(o roundguard.Outcome) bool { return o.Failed != nil }, roundguard.ReasonFailed, false},
	{"交接", func(o roundguard.Outcome) bool { return o.Handoff }, roundguard.ReasonHandedOff, false},
	{"要工具", func(o roundguard.Outcome) bool { return o.ToolCalls > 0 }, roundguard.ReasonToolCalls, true},
	{"强制收尾", func(o roundguard.Outcome) bool { return o.ForcedFinal }, roundguard.ReasonForcedFinal, false},
	{"终答", func(o roundguard.Outcome) bool { return o.Completed }, roundguard.ReasonCompleted, false},
	{"原样再问", func(o roundguard.Outcome) bool { return o.Paused }, roundguard.ReasonPaused, true},
}

// classifyOracle 按臂表判定回合结局（第一命中的臂胜出）。
func classifyOracle(o roundguard.Outcome) roundguard.Route {
	for _, arm := range classifyArms {
		if arm.hit(o) {
			return roundguard.Route{Reason: arm.reason, Continue: arm.cont}
		}
	}
	return roundguard.Route{Reason: roundguard.ReasonNoOutcome}
}

// ── 无进展检测 oracle（spec §7.2 / §7.3 / §7.5）──

// oracleFailRun 尾部连续失败长度（正向扫描）。
//
// ⚠ **「结果变了」不参与**：失败事实与「结果变了」无关——结果变了的失败**仍是
// 失败** ⇒ 连续性**不**被打断（spec §7.3 第 1 行）。
func oracleFailRun(seq []roundguard.Observation) int {
	run := 0
	for _, o := range seq {
		if o.Failed {
			run++
		} else {
			run = 0
		}
	}
	return run
}

// oracleFingerprintRun 尾部同指纹连续长度（正向扫描）。
//
// ⚠ **「结果变了」打断**连续性（它表达进展，不是重复）：末轮 RC ⇒ 长度重起算
// （只算它自己）；中间的 RC ⇒ 那一段从它之后重新起算（spec §7.3 第 2 行）。
func oracleFingerprintRun(seq []roundguard.Observation) int {
	if len(seq) == 0 {
		return 0
	}
	if seq[len(seq)-1].ResultChanged {
		return 1
	}
	run := 0
	for i, o := range seq {
		if i == 0 {
			run = 1
			continue
		}
		if o.Fingerprint == seq[i-1].Fingerprint && !o.ResultChanged && !seq[i-1].ResultChanged {
			run++
		} else {
			run = 1
		}
	}
	return run
}

// oracleNormalizedRun 尾部归一指纹连续长度（正向扫描；口径与同指纹那条**同款**）。
//
// ⚠ 没有归一指纹 ⇒ 无法判漂移（内核不猜「什么是易变部分」）⇒ 长度 0。
func oracleNormalizedRun(seq []roundguard.Observation) int {
	if len(seq) == 0 {
		return 0
	}
	if seq[len(seq)-1].Normalized == "" {
		return 0
	}
	if seq[len(seq)-1].ResultChanged {
		return 1
	}
	run := 0
	for i, o := range seq {
		if i == 0 {
			run = 1
			continue
		}
		if o.Normalized != "" && o.Normalized == seq[i-1].Normalized &&
			!o.ResultChanged && !seq[i-1].ResultChanged {
			run++
		} else {
			run = 1
		}
	}
	return run
}

// oracleOscillationRun 尾部周期 2 交替的连续长度（正向维护「以 i 结尾的最长
// 周期 2 段」）。
//
// 口径：相邻不等**且** f[i] == f[i+2]（周期 1 的 A A A 不算震荡）；任一位置
// 出现「结果变了」⇒ 那一段重新起算（spec §7.3 第 3 行）。
func oracleOscillationRun(seq []roundguard.Observation) int {
	n := len(seq)
	if n < 3 {
		return 0
	}
	last, prev := seq[n-1], seq[n-2]
	if last.Fingerprint == prev.Fingerprint || last.ResultChanged || prev.ResultChanged {
		return 0
	}
	alt := make([]int, n)
	for i, o := range seq {
		switch {
		case o.ResultChanged:
			alt[i] = 0
		case i == 0:
			alt[i] = 1
		case o.Fingerprint == seq[i-1].Fingerprint:
			alt[i] = 1
		case alt[i-1] == 0:
			// 前一轮带进展标记 ⇒ 那一段连续性被它打断 ⇒ 不能跨过去。
			alt[i] = 1
		case i >= 2 && o.Fingerprint == seq[i-2].Fingerprint:
			alt[i] = alt[i-1] + 1
		default:
			alt[i] = 2
		}
	}
	return alt[n-1]
}

// oracleDriftRun 尾部归一指纹连续长度（带「原指纹不全等」这道守卫）。
//
// ⚠ 守卫的触发条件：归一后同**且原指纹全等** ⇒ 那不是漂移（那串归重复形态判走，
// spec §7.3 第 4 行与「重复优先于漂移」那段）。
func oracleDriftRun(seq []roundguard.Observation) int {
	L := oracleNormalizedRun(seq)
	if L == 0 {
		return 0
	}
	run := seq[len(seq)-L:]
	for _, o := range run[1:] {
		if o.Fingerprint != run[0].Fingerprint {
			return L
		}
	}
	return 0
}

// oracleIdleRun 尾部「零工具且未收敛」的连续长度（正向扫描）。
//
// ⚠ **「结果变了」不参与**：空转看的是「有没有动手 / 收没收敛」，与结果是否变化
// 无关 ⇒ 连续性**不**被打断（spec §7.3 第 5 行）。
func oracleIdleRun(seq []roundguard.Observation) int {
	run := 0
	for _, o := range seq {
		if o.Tools == 0 && !o.Settled {
			run++
		} else {
			run = 0
		}
	}
	return run
}

// oracleTier 定档（L == 阈值 ⇒ 首档；L > 阈值 ⇒ 停止档）+ 连续段起点。
func oracleTier(idx []int, shape roundguard.Shape, L, threshold int) roundguard.Finding {
	since := 0
	if len(idx) >= L {
		since = idx[len(idx)-L] + 1
	}
	f := roundguard.Finding{Shape: shape, Since: since}
	if L > threshold {
		f.Stalled = true
	} else {
		f.Warn = true
	}
	return f
}

// oracleDetect 按 spec §7.3 的判定次序复刻无进展检测（整段判定，含窗口钳位）。
func oracleDetect(obs []roundguard.Observation, cfg roundguard.Config) roundguard.Finding {
	k := cfg.Repeat
	if k <= 0 {
		k = roundguard.DefaultRepeat
	}
	// 下界向上钳位到 2（spec §7.2）：k=1 时漂移形态永不可达（退化档）。
	if k < 2 {
		k = 2
	}
	window := cfg.Window
	if window <= 0 {
		window = roundguard.DefaultWindow
	}
	if min := 2*k + 2; window < min {
		window = min
	}
	seq := make([]roundguard.Observation, 0, window)
	idx := make([]int, 0, window)
	for i := len(obs) - window; i < len(obs); i++ {
		if i < 0 || obs[i].Exempt {
			continue
		}
		seq = append(seq, obs[i])
		idx = append(idx, i)
	}
	if len(seq) == 0 {
		return roundguard.Finding{Shape: roundguard.ShapeNone}
	}
	type cand struct {
		shape roundguard.Shape
		L     int
		W     int
	}
	cands := []cand{
		{roundguard.ShapeErrorSpiral, oracleFailRun(seq), k},
		{roundguard.ShapeRepeat, oracleFingerprintRun(seq), k},
		{roundguard.ShapeOscillation, oracleOscillationRun(seq), 2 * k},
		{roundguard.ShapeDrift, oracleDriftRun(seq), k},
		{roundguard.ShapeIdle, oracleIdleRun(seq), k},
	}
	for _, c := range cands {
		if c.L < c.W {
			continue
		}
		return oracleTier(idx, c.shape, c.L, c.W)
	}
	return roundguard.Finding{Shape: roundguard.ShapeNone}
}

// ── 组合结论 oracle（spec §4.2）──

// oracleBudgetStopReason 额度用尽后的结论（按处置口径分流）。
func oracleBudgetStopReason(b roundguard.Budget) roundguard.Reason {
	if b.Policy == roundguard.PolicyFailFast {
		return roundguard.ReasonBudgetExhausted
	}
	return roundguard.ReasonForcedFinal
}

// oracleDecide 按 spec §4.2 的判定次序复刻组合结论。
//
// ⚠ 额度两维的次序**独立写死**：先回合数、后工具调用总数（两者同时用尽 ⇒ 报回合数
// 那一维，见 spec §4.2 / §6.3）。
func oracleDecide(s roundguard.Snapshot, b roundguard.Budget, cfg roundguard.Config) roundguard.Decision {
	d := roundguard.Decision{Reason: roundguard.ReasonNoOutcome, Limit: roundguard.LimitNone, Shape: roundguard.ShapeNone}
	route := classifyOracle(s.Outcome)
	d.Reason = route.Reason
	if !route.Continue {
		return d
	}
	if b.MaxRounds > 0 && s.Used.Rounds >= b.MaxRounds {
		d.Limit = roundguard.LimitRounds
		d.Reason = oracleBudgetStopReason(b)
		return d
	}
	if b.MaxToolCalls > 0 && s.Used.ToolCalls >= b.MaxToolCalls {
		d.Limit = roundguard.LimitToolCalls
		d.Reason = oracleBudgetStopReason(b)
		return d
	}
	f := oracleDetect(s.Obs, cfg)
	if f.Shape != roundguard.ShapeNone {
		d.Limit = roundguard.LimitStalled
		d.Shape = f.Shape
		d.Warn = f.Warn
		d.Reason = roundguard.ReasonStalled
		d.Continue = !f.Stalled
		return d
	}
	d.Continue = true
	return d
}

// sameDecision 比两条组合结论（逐字段；Decision.Used 内含映射 ⇒ 结构体不可比）。
func sameDecision(got, want roundguard.Decision) bool {
	return got.Reason == want.Reason && got.Continue == want.Continue &&
		got.Limit == want.Limit && got.Shape == want.Shape && got.Warn == want.Warn
}
