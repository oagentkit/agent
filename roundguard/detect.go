package roundguard

// Observation 一回合的可机判事实（宿主注入）。
//
// ⚠️ **指纹与归一指纹都由宿主算**：内核若自己决定「什么是易变部分」（数字？
// 时间戳？行号？），就成了领域解释者——那是宿主的事。内核只对**宿主算好的两组
// 字符串**做集合判定（与 budget.Item 只携带 ID+Tokens、cost.Dims 是开放键同
// 一条纪律）。
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
	// 只看参数的去重会把「换个页码再查一次」这类有意义的重试误判成循环。
	ResultChanged bool
	// Exempt 本回合**不参与无进展判定**（重复计数与空转计数都不参与）。
	//
	// 两个已知的必须豁免的场景：
	// ① 轮询 / 查状态类动作——「输出变了才是进展信号」（agentloop 的 Repetitive
	//    标记同款）；
	// ② **刻意的规划步**——smolagents 的 planning_interval 会每隔 N 步插一个规划
	//    回合，而规划回合**必然零工具调用**，按空转判据会被误伤。
	Exempt bool
}

// Shape 无进展形态（判定词表；主人表 roundShapeAll，枚举出口 AllShapes）。
type Shape string

const (
	// ShapeNone 未命中任何形态。
	ShapeNone Shape = "none"
	// ShapeRepeat 精确重复：同指纹连着来。
	ShapeRepeat Shape = "repeat"
	// ShapeOscillation 震荡：A B A B …
	ShapeOscillation Shape = "oscillation"
	// ShapeDrift 缓慢漂移：归一后同、原指纹不同。
	ShapeDrift Shape = "drift"
	// ShapeErrorSpiral 错误重试螺旋：连着失败。
	ShapeErrorSpiral Shape = "error_spiral"
	// ShapeIdle 只规划不动手：有回合、零工具、未收敛。
	ShapeIdle Shape = "idle"
)

// Valid 报告该形态是否在词表内。
func (s Shape) Valid() bool { return roundShapeAll[s] }

// Finding 无进展检测结果。
type Finding struct {
	// Stalled 已达**停止档**：应当温和叫停。
	Stalled bool
	// Warn 首档命中：只该**警告回灌**，不停止——给模型一次自我修正的机会。
	Warn bool
	// Shape 形态。
	Shape Shape
	// Since 该形态连续成立的起点（全局 1 起；0 = 判不出）。
	Since int
}

// Config 无进展检测参数（零值 = 缺省）。
type Config struct {
	// Window 看最近多少个回合（0 = DefaultWindow）。
	//
	// **下限会被向上钳位到 2×Repeat+2**：震荡形态的最小命中窗口是 2×Repeat 个
	// 回合，再加一次「再犯」才到停止档 ⇒ 窗口不够时停止档**不可达**，而判据
	// 不可达等于判据不存在。故此处向上钳位（不是判不出）。
	Window int
	// Repeat 连续几回合构成重复（0 = DefaultRepeat）。
	//
	// **下限会被向上钳位到 2**：取 1 是**退化档**——「尾部同指纹连续长度」恒 ≥ 1
	// ⇒ 任意单个回合即判重复（首档）；且漂移形态在 Repeat=1 下**永不成立**
	// （单元素的「原指纹不全等」恒假 ⇒ 漂移那一档没有落点）⇒ 钳位（不是判不出）。
	//
	// 为什么**钳位**而不是报错：与 Window 的向上钳位同款——判据不可达等于判据
	// 不存在，而「这一档不可达」时最该做的是**把档位抬回可达区间**，而不是让
	// 宿主在装配期被拒（本包的 Config 没有 Validate 语义，零值/越界一律按缺省
	// 与钳位处理）。
	Repeat int
}

const (
	// DefaultWindow 缺省观察窗口（回合数）。
	//
	// 8 = 2×DefaultRepeat+2 ⇒ 每一形态的停止档都可达。
	DefaultWindow = 8
	// DefaultRepeat 缺省重复阈值（回合数）。
	DefaultRepeat = 3
)

// Detect 无进展检测（纯函数；只看最近 Window 个回合）。
//
// 判定顺序（取病因最具体的那条）：错误螺旋 → 精确重复 → 震荡 → 缓慢漂移 → 空转。
//
// 定档：形态的**连续成立长度 L** == 阈值 ⇒ 首档（Warn，给模型一次自我修正机会）；
// L > 阈值 ⇒ 停止档（Stalled，温和叫停）。
func Detect(obs []Observation, cfg Config) Finding {
	window := cfg.Window
	if window <= 0 {
		window = DefaultWindow
	}
	k := cfg.Repeat
	if k <= 0 {
		k = DefaultRepeat
	}
	// 下界向上钳位到 2（与窗口的向上钳位同款）：k=1 时漂移形态**永不可达**
	// （漂移要求「归一指纹全等且原指纹不全等」，而单元素的「原指纹不全等」
	// 恒假 ⇒ 那一档没有落点），且任意单个回合即被判重复 ⇒ 退化档。
	//
	// 为什么**钳位**而不是报错：判据不可达 = 判据不存在；把档位抬回可达区间
	// 比让宿主在装配期被拒更符合本包的其余口径（零值/越界一律按缺省与钳位处理）。
	if k < 2 {
		k = 2
	}
	if min := 2*k + 2; window < min {
		window = min
	}
	seq, idx := windowRounds(obs, window)
	if len(seq) == 0 {
		return Finding{Shape: ShapeNone}
	}
	if L := trailingFailed(seq); L >= k {
		return tierFinding(idx, ShapeErrorSpiral, L, k)
	}
	if L := trailingSameFingerprint(seq); L >= k {
		return tierFinding(idx, ShapeRepeat, L, k)
	}
	if L := trailingAlternating(seq); L >= 2*k {
		return tierFinding(idx, ShapeOscillation, L, 2*k)
	}
	// 漂移带「原指纹不全等」这一条（spec §7.3 第 4 行）：归一后同但**原指纹
	// 全等**的那串是「重复」（或被进展打断），不是漂移。
	if L, ok := driftCandidate(seq, k); ok {
		return tierFinding(idx, ShapeDrift, L, k)
	}
	if L := trailingIdle(seq); L >= k {
		return tierFinding(idx, ShapeIdle, L, k)
	}
	return Finding{Shape: ShapeNone}
}

// windowRounds 取窗口内**未豁免**的回合，并给出它们在原序列中的下标（用于报 Since）。
func windowRounds(obs []Observation, window int) ([]Observation, []int) {
	start := len(obs) - window
	if start < 0 {
		start = 0
	}
	seq := make([]Observation, 0, window)
	idx := make([]int, 0, window)
	for i := start; i < len(obs); i++ {
		if obs[i].Exempt {
			continue
		}
		seq = append(seq, obs[i])
		idx = append(idx, i)
	}
	return seq, idx
}

// trailingFailed 尾部连续失败长度。
func trailingFailed(seq []Observation) int {
	n := 0
	for i := len(seq) - 1; i >= 0; i-- {
		if !seq[i].Failed {
			break
		}
		n++
	}
	return n
}

// trailingSameFingerprint 尾部同指纹连续长度。
//
// 「结果变了」是进展 ⇒ streak 重新起算（与 agentloop 的
// "a repeated call whose output changed is progress, so the streak restarts" 同款）。
func trailingSameFingerprint(seq []Observation) int {
	if len(seq) == 0 {
		return 0
	}
	last := seq[len(seq)-1]
	if last.ResultChanged {
		return 1
	}
	n := 1
	for i := len(seq) - 2; i >= 0; i-- {
		if seq[i].Fingerprint != last.Fingerprint || seq[i].ResultChanged {
			break
		}
		n++
	}
	return n
}

// trailingAlternating 尾部周期 2 交替的连续长度（周期 1 属「重复」，返回 0）。
func trailingAlternating(seq []Observation) int {
	n := len(seq)
	if n < 3 {
		return 0
	}
	last, prev := seq[n-1], seq[n-2]
	if last.Fingerprint == prev.Fingerprint || last.ResultChanged || prev.ResultChanged {
		return 0
	}
	length := 2
	for i := n - 3; i >= 0; i-- {
		if seq[i].ResultChanged || seq[i].Fingerprint != seq[i+2].Fingerprint {
			break
		}
		length++
	}
	return length
}

// trailingSameNormalized 尾部归一指纹相同的连续长度。
//
// **与 trailingSameFingerprint 同口径**：末轮「结果变了」⇒ 连续长度**重新起算**
// （只算末轮那一轮）。少了这一条，两条姊妹判据的「连续性」定义就不一致 ⇒
// 一串「末轮结果变了」的漂移序列会被判成漂移（假阳：序列明明显示了进展）。
func trailingSameNormalized(seq []Observation) int {
	if len(seq) == 0 {
		return 0
	}
	last := seq[len(seq)-1]
	if last.Normalized == "" {
		return 0 // 没有归一指纹 ⇒ 无法判漂移（内核不猜「什么是易变部分」）
	}
	if last.ResultChanged {
		return 1 // 末轮结果变了 ⇒ 连续长度重起算（与 trailingSameFingerprint 同口径）
	}
	n := 1
	for i := len(seq) - 2; i >= 0; i-- {
		if seq[i].Normalized != last.Normalized || seq[i].ResultChanged {
			break
		}
		n++
	}
	return n
}

// allSameFingerprint 报告这串回合的**原指纹**是否全等。
//
// 为什么漂移要带这一条：漂移的定义是「归一后同、**原指纹不同**」（spec §7.3
// 第 4 行）。少了它，一串「同指纹 + 宿主给了归一指纹 + 末轮结果变了（= 有进展）」
// 的回合会被判成漂移——那是假阳：序列明明显示了进展，却被判成无进展。
func allSameFingerprint(run []Observation) bool {
	if len(run) == 0 {
		return false
	}
	fp := run[0].Fingerprint
	for _, o := range run[1:] {
		if o.Fingerprint != fp {
			return false
		}
	}
	return true
}

// driftCandidate 漂移分支的候选判定：尾部归一指纹连续长度 ≥ k，**且**原指纹不全等。
//
// 为什么收成独立函数（判定语义一字未改，只把内联条件逐字收口）：
// 这条分支上挂着一道**承重守卫**（原指纹不全等），而它在整条判定链上**不可达**
// ——「原指纹全等」的那串必被**先判的重复形态**判走（两处耦合：① 重复分支先于
// 漂移分支；② 两者同用阈值 k，见 spec §7.3）⇒ 黑盒观察不到 ⇒ **守卫被删掉时
// 没有任何用例会红**（2026-11-21 实测：删守卫后整套 roundguard 用例仍全绿）。
//
// 收成函数是为了让这道守卫**可白盒直钉**：守卫的契约（该进⇒进、该挡⇒挡）从此
// 有用例，删掉守卫 ⇒ 白盒用例当场红 ⇒ 守卫从「惰性断言」变成「承重判据」。
func driftCandidate(seq []Observation, k int) (int, bool) {
	L := trailingSameNormalized(seq)
	if L < k {
		return 0, false
	}
	if allSameFingerprint(seq[len(seq)-L:]) {
		return 0, false
	}
	return L, true
}

// trailingIdle 尾部「零工具且未收敛」的连续长度。
func trailingIdle(seq []Observation) int {
	n := 0
	for i := len(seq) - 1; i >= 0; i-- {
		if seq[i].Tools != 0 || seq[i].Settled {
			break
		}
		n++
	}
	return n
}

// tierFinding 按连续成立长度定档：L == 阈值 ⇒ 首档；L > 阈值 ⇒ 停止档。
//
// 入参只有连续段的下标表（算 Since 用）——形态与连续段本身已由调用方判定完，
// 再传一份用不到的序列进来只会让人以为这里还要看序列。
func tierFinding(idx []int, shape Shape, L, threshold int) Finding {
	since := 0
	if len(idx) >= L {
		since = idx[len(idx)-L] + 1
	}
	f := Finding{Shape: shape, Since: since}
	if L > threshold {
		f.Stalled = true
	} else {
		f.Warn = true
	}
	return f
}
