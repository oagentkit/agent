package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── 无进展检测：输入空间的全穷举对照 + 阈值 / 次序 / 连续性 ──

// mxOpts 合成回合的构造选项（中性词面：指纹用字母串与数字串）。
type mxOpts struct {
	fps   []string // 指纹表（循环取用）
	norm  string   // 归一指纹（空串 = 不给）
	tools int      // 工具调用数
	fail  bool     // 是否失败
}

// mxSeq 造一串合成回合。
func mxSeq(n int, o mxOpts) []roundguard.Observation {
	out := make([]roundguard.Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, roundguard.Observation{
			Fingerprint: o.fps[i%len(o.fps)],
			Normalized:  o.norm,
			Tools:       o.tools,
			Failed:      o.fail,
		})
	}
	return out
}

// mxSame / mxAlt / mxDiff 三张指纹表（分别对应「全等 / 周期 2 交替 / 互不相同」）。
var (
	mxSame = []string{"a"}
	mxAlt  = []string{"a", "b"}
	mxDiff = []string{"p0", "p1", "p2", "p3", "p4", "p5", "p6", "p7"}
)

// oracleRoundKinds 穷举用的**合成回合种类**（20 种，覆盖每一维的取值组合）。
//
// 刻意覆盖的交互：失败 + 结果变了（错误螺旋的 RC 口径）、零工具 + 结果变了
// （空转的 RC 口径）、豁免（豁免面）、归一指纹相同 / 不同（漂移与重复的分工）。
var oracleRoundKinds = []roundguard.Observation{
	{Fingerprint: "a", Tools: 1},
	{Fingerprint: "b", Tools: 1},
	{Fingerprint: "a", Normalized: "n", Tools: 1},
	{Fingerprint: "b", Normalized: "n", Tools: 1},
	{Fingerprint: "a", Tools: 1, ResultChanged: true},
	{Fingerprint: "a", Tools: 1, Failed: true},
	{Fingerprint: "a", Tools: 1, Failed: true, ResultChanged: true},
	{Fingerprint: "c", Tools: 0},
	{Fingerprint: "c", Tools: 0, ResultChanged: true},
	{Fingerprint: "c", Tools: 0, Settled: true},
	{Fingerprint: "a", Tools: 1, Exempt: true},
	{Fingerprint: "a", Normalized: "n", Tools: 1, Exempt: true},
	// 下面六种是为了让**每一对形态**都能在同一串里同时成立（次序才显形）：
	{Fingerprint: "a", Tools: 0},                                // 重复 × 空转
	{Fingerprint: "b", Tools: 0},                                // 震荡 × 空转
	{Fingerprint: "c", Normalized: "n", Tools: 0},               // 漂移 × 空转
	{Fingerprint: "b", Tools: 1, Failed: true},                  // 错误螺旋 × 震荡
	{Fingerprint: "b", Normalized: "n", Tools: 1, Failed: true}, // 错误螺旋 × 漂移
	{Fingerprint: "c", Tools: 0, Failed: true},                  // 错误螺旋 × 空转
	// 再两种：漂移与空转 / 错误螺旋与漂移要**同时**成立，光有「归一同 + 零工具」
	// 不够——漂移要求**原指纹不全等** ⇒ 同一对里还得有两个不同的原指纹。
	{Fingerprint: "d", Normalized: "n", Tools: 0},               // 漂移 × 空转（异指纹）
	{Fingerprint: "c", Normalized: "n", Tools: 1, Failed: true}, // 错误螺旋 × 漂移（异指纹）
}

// TestMutationMatrixDetectExhaustive 钉住无进展检测的**整个输入空间**。
//
// 穷举：20 种合成回合的**长度 0..4 的全部序列**（每组参数 168421 条）× 3 组参数
// （缺省 / 阈值 2 且窗口被钳位 / 阈值 4 且窗口充足）⇒ 每条序列都与**独立写成的
// 规格 oracle** 逐字段对照（形态 / 首档 / 停止档 / 连续段起点）。
//
// 为什么必须穷举：形态判定有 5 条尾部判据 + 5 个判定次序 + 窗口钳位 + 豁免面 +
// 「结果变了」的四种口径 ⇒ 逐对钉只能覆盖「两两同成立」，而「三形态同成立」
// 「结果变了落在中间位置」这两类落点只有穷举才盖得住。
//
// 反向验证：改任一条尾部判据 / 调任一次序 / 动窗口钳位 / 让 RC 参与或不参与
// 任一形态的连续性 ⇒ 必有一条序列分歧 ⇒ 红。
func TestMutationMatrixDetectExhaustive(t *testing.T) {
	configs := []roundguard.Config{
		{},                      // 缺省（阈值 3、窗口 8）
		{Repeat: 2, Window: 4},  // 窗口被向上钳位到 2k+2 = 6
		{Repeat: 4, Window: 20}, // 窗口充足（阈值 4）
	}
	for ci, cfg := range configs {
		for length := 0; length <= 4; length++ {
			seq := make([]roundguard.Observation, 0, length)
			var walk func(depth int)
			walk = func(depth int) {
				if depth == length {
					got := roundguard.Detect(seq, cfg)
					want := oracleDetect(seq, cfg)
					if got != want {
						t.Fatalf("参数组 %d / 序列 %+v：实现判 %+v，规格 oracle 判 %+v",
							ci, seq, got, want)
					}
					return
				}
				for _, kind := range oracleRoundKinds {
					seq = append(seq, kind)
					walk(depth + 1)
					seq = seq[:len(seq)-1]
				}
			}
			walk(0)
		}
	}
}

// TestMutationMatrixDetectThresholdIsK 钉住**每个形态的命中阈值恰为 k**
// （震荡为 2k）——不多不少。
//
// 为什么单列：阈值若被放宽（如「L >= k-1」也算命中），形态会在更短的串上命中，
// 而「某形态达标」的用例（用的是恰好 k 长的串）**照样全绿** ⇒ 只有把
// 「差一轮」的那一档也钉住才红。
//
// 这条同时钉住 spec §7.3 的耦合②：**重复与漂移同用阈值 k**（任一形态的阈值被
// 单独改动 ⇒ 本用例红）。
func TestMutationMatrixDetectThresholdIsK(t *testing.T) {
	k := roundguard.DefaultRepeat
	cases := []struct {
		name  string
		below []roundguard.Observation // L == 阈值-1 ⇒ 不该命中
		at    []roundguard.Observation // L == 阈值 ⇒ 首档命中
		shape roundguard.Shape
	}{
		{"重复", mxSeq(k-1, mxOpts{fps: mxSame, tools: 1}), mxSeq(k, mxOpts{fps: mxSame, tools: 1}), roundguard.ShapeRepeat},
		{"错误螺旋", mxSeq(k-1, mxOpts{fps: mxSame, tools: 1, fail: true}), mxSeq(k, mxOpts{fps: mxSame, tools: 1, fail: true}), roundguard.ShapeErrorSpiral},
		{"漂移", mxSeq(k-1, mxOpts{fps: mxDiff, norm: "n", tools: 1}), mxSeq(k, mxOpts{fps: mxDiff, norm: "n", tools: 1}), roundguard.ShapeDrift},
		{"空转", mxSeq(k-1, mxOpts{fps: mxDiff}), mxSeq(k, mxOpts{fps: mxDiff}), roundguard.ShapeIdle},
		{"震荡", mxSeq(2*k-1, mxOpts{fps: mxAlt, tools: 1}), mxSeq(2*k, mxOpts{fps: mxAlt, tools: 1}), roundguard.ShapeOscillation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := roundguard.Detect(c.below, roundguard.Config{})
			if f.Shape != roundguard.ShapeNone || f.Warn || f.Stalled {
				t.Fatalf("差一轮不该命中任何形态（阈值恰为 k）：%+v", f)
			}
			g := roundguard.Detect(c.at, roundguard.Config{})
			if g.Shape != c.shape {
				t.Fatalf("刚够阈值应判 %q，实得 %q", c.shape, g.Shape)
			}
			if !g.Warn || g.Stalled {
				t.Fatalf("刚够阈值应为首档：%+v", g)
			}
		})
	}
}

// TestMutationMatrixDetectShapeOrderMatrix 钉住**判定次序**：两个形态同时成立
// 时取**更具体**的那条。
//
// 为什么单列：次序是「证据硬度」的编码（错误螺旋携带失败事实 ⇒ 比重复硬；
// 震荡 / 漂移是形态 ⇒ 比空转硬），而每条形态**单独**的用例完全看不出次序——
// 只把两条形态拼在一起才显形。
//
// ⚠ 「重复 vs 漂移」这一对**不可同时成立**（原指纹全等 ⇒ 漂移分支被守卫挡住，
// 见 spec §7.3）：它的落点在**白盒**用例里直钉（mutation_internal_test.go）。
func TestMutationMatrixDetectShapeOrderMatrix(t *testing.T) {
	cases := []struct {
		name string
		seq  []roundguard.Observation
		want roundguard.Shape
	}{
		{"错误螺旋压过重复", mxSeq(3, mxOpts{fps: mxSame, tools: 1, fail: true}), roundguard.ShapeErrorSpiral},
		{"错误螺旋压过震荡", mxSeq(6, mxOpts{fps: mxAlt, tools: 1, fail: true}), roundguard.ShapeErrorSpiral},
		{"错误螺旋压过漂移", mxSeq(3, mxOpts{fps: mxDiff, norm: "n", tools: 1, fail: true}), roundguard.ShapeErrorSpiral},
		{"错误螺旋压过空转", mxSeq(3, mxOpts{fps: mxDiff, fail: true}), roundguard.ShapeErrorSpiral},
		{"重复压过空转", mxSeq(3, mxOpts{fps: mxSame}), roundguard.ShapeRepeat},
		{"震荡压过漂移", mxSeq(6, mxOpts{fps: mxAlt, norm: "n", tools: 1}), roundguard.ShapeOscillation},
		{"震荡压过空转", mxSeq(6, mxOpts{fps: mxAlt}), roundguard.ShapeOscillation},
		{"漂移压过空转", mxSeq(3, mxOpts{fps: mxDiff, norm: "n"}), roundguard.ShapeDrift},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := roundguard.Detect(c.seq, roundguard.Config{})
			if f.Shape != c.want {
				t.Fatalf("应判 %q，实得 %q", c.want, f.Shape)
			}
		})
	}
}

// TestMutationMatrixDetectErrorSpiralIgnoresResultChanged 钉住**错误螺旋与
// 「结果变了」的关系**：结果变了**不参与**失败螺旋的连续性判定。
//
// 为什么这样定口径：失败事实是「这一回合失败了」；「结果变了」表达的是「同一
// 动作的结果不同」——结果变了的**失败仍然是失败** ⇒ 连续性不被打断（否则
// 「失败—换个姿势再失败—再失败」这种真实死循环会被当成进展放过）。
//
// 反向验证：让失败螺旋尊重 RC ⇒ 本用例红（形态翻成无形态）。
func TestMutationMatrixDetectErrorSpiralIgnoresResultChanged(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a", Tools: 1, Failed: true},
		{Fingerprint: "a", Tools: 1, Failed: true, ResultChanged: true},
		{Fingerprint: "a", Tools: 1, Failed: true},
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeErrorSpiral {
		t.Fatalf("结果变了不打断失败螺旋，应判 %q，实得 %q", roundguard.ShapeErrorSpiral, f.Shape)
	}
	if !f.Warn || f.Stalled {
		t.Fatalf("三连失败刚够阈值应为首档：%+v", f)
	}
}

// TestMutationMatrixDetectIdleIgnoresResultChanged 钉住**空转与「结果变了」的
// 关系**：结果变了**不参与**空转的连续性判定。
//
// 为什么这样定口径：空转看的是「有没有动手 / 收没收敛」，与输出内容无关；一轮
// 出了新结果但**仍然零工具、仍未收敛**，它**仍然是空转**（否则模型每轮换个说法
// 就能无限绕过空转判据）。
//
// 反向验证：让空转尊重 RC ⇒ 本用例红（形态翻成无形态）。
func TestMutationMatrixDetectIdleIgnoresResultChanged(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a"},
		{Fingerprint: "b", ResultChanged: true},
		{Fingerprint: "c"},
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeIdle {
		t.Fatalf("结果变了不打断空转，应判 %q，实得 %q", roundguard.ShapeIdle, f.Shape)
	}
	if !f.Warn || f.Stalled {
		t.Fatalf("三连空转刚够阈值应为首档：%+v", f)
	}
}

// TestMutationMatrixDetectOscillationRestartsAfterResultChanged 钉住**「结果变了」
// 之后的轮次重新起算**：进展标记打断的那一段不算，其后的轮次从**下一轮**重新
// 起算（连续段起点随之后移）。
//
// 为什么单列：前面那条用例钉的是「有没有被打断」，这条钉的是「打断之后从哪儿
// 重新数」——起算点错一轮（起点算成被打断的那一轮 / 算成整段起点）时，前一条
// 仍全绿。
//
// 反向验证：让进展标记之后的轮次接着数（不重新起算）⇒ 本用例红。
func TestMutationMatrixDetectOscillationRestartsAfterResultChanged(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a", Tools: 1},
		{Fingerprint: "b", Tools: 1},
		{Fingerprint: "a", Tools: 1, ResultChanged: true},
		{Fingerprint: "b", Tools: 1},
		{Fingerprint: "a", Tools: 1},
		{Fingerprint: "b", Tools: 1},
		{Fingerprint: "a", Tools: 1},
		{Fingerprint: "b", Tools: 1},
		{Fingerprint: "a", Tools: 1},
	}
	f := roundguard.Detect(seq, roundguard.Config{Window: 16})
	if f.Shape != roundguard.ShapeOscillation {
		t.Fatalf("进展标记之后的六轮交替应重新凑够震荡：%+v", f)
	}
	if !f.Warn || f.Stalled {
		t.Fatalf("刚凑够 2k 轮应为首档：%+v", f)
	}
	if f.Since != 4 {
		t.Fatalf("连续段应从进展标记之后的那一轮（第 4 轮）重新起算，实得 %d", f.Since)
	}
}
