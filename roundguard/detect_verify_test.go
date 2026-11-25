package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── 以下为反向验证用例（每一档 / 豁免面 / 连续性打断 / 窗口边界 / 次序）──

// TestDetectTiersPerShape 钉住**五个形态各自的两档**（首档 Warn / 停止档 Stalled）。
//
// 反向验证：把定档判据写成「L >= 阈值 ⇒ 停止档」⇒ 首档各行红；
// 写成「L > 阈值 ⇒ 首档」⇒ 停止档各行红；某一形态漏判 ⇒ 该行红。
func TestDetectTiersPerShape(t *testing.T) {
	cases := []struct {
		name  string
		shape roundguard.Shape
		first []roundguard.Observation
		again []roundguard.Observation
	}{
		{"repeat", roundguard.ShapeRepeat, repeatSeq(3), repeatSeq(4)},
		{"error_spiral", roundguard.ShapeErrorSpiral, failSeq(3), failSeq(4)},
		{"oscillation", roundguard.ShapeOscillation, altSeq(6), altSeq(7)},
		{"drift", roundguard.ShapeDrift, driftSeq(3), driftSeq(4)},
		{"idle", roundguard.ShapeIdle, idleSeq(3), idleSeq(4)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := roundguard.Detect(c.first, roundguard.Config{})
			if f.Shape != c.shape {
				t.Fatalf("形态应判 %q，实得 %q", c.shape, f.Shape)
			}
			if !f.Warn || f.Stalled {
				t.Fatalf("刚凑够阈值应为首档（Warn 且非停止档）：%+v", f)
			}
			g := roundguard.Detect(c.again, roundguard.Config{})
			if g.Shape != c.shape {
				t.Fatalf("形态应判 %q，实得 %q", c.shape, g.Shape)
			}
			if !g.Stalled || g.Warn {
				t.Fatalf("再犯一次应判停止档：%+v", g)
			}
		})
	}
}

// TestDetectOscillationBeatsDrift 钉住判定次序：震荡优先于漂移。
//
// 交替的指纹 + 相同的归一指纹，两条都成立 ⇒ 取**形态更具体**的那条。
// 反向验证：把漂移排到震荡之前 ⇒ 红。
func TestDetectOscillationBeatsDrift(t *testing.T) {
	seq := make([]roundguard.Observation, 0, 6)
	for i := 0; i < 6; i++ {
		fp := "a"
		if i%2 == 1 {
			fp = "b"
		}
		seq = append(seq, obs(fp, roundguard.Observation{Normalized: "pN"}))
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeOscillation {
		t.Fatalf("震荡与漂移同成立时应判 %q，实得 %q", roundguard.ShapeOscillation, f.Shape)
	}
}

// TestDetectDriftBeatsIdle 钉住判定次序：漂移优先于空转。
//
// 「归一指纹相同 + 原指纹不同 + 零工具」的回合同时满足空转 ⇒ 取形态更具体的那条。
// 反向验证：把空转排到漂移之前 ⇒ 红。
func TestDetectDriftBeatsIdle(t *testing.T) {
	f := roundguard.Detect(driftSeq(3), roundguard.Config{})
	if f.Shape != roundguard.ShapeDrift {
		t.Fatalf("漂移与空转同成立时应判 %q，实得 %q", roundguard.ShapeDrift, f.Shape)
	}
}

// TestDetectDriftRequiresDifferentFingerprints 钉住漂移的**定义**：归一后同、
// **原指纹不同**。同指纹的连着来是「重复」（或被进展打断），不是漂移。
//
// 反向验证：去掉「原指纹不全等」这一条 ⇒ 一串「同指纹 + 有归一指纹 + 结果变了」
// 的回合会被判成漂移 ⇒ 本用例红（那是假阳：序列明明显示了进展）。
func TestDetectDriftRequiresDifferentFingerprints(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{Normalized: "pN"}),
		obs("a", roundguard.Observation{Normalized: "pN"}),
		obs("a", roundguard.Observation{Normalized: "pN", ResultChanged: true}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeDrift {
		t.Fatalf("原指纹全等不该判漂移：%+v", f)
	}
}

// TestDetectExemptBreaksIdleCount 钉住豁免面：**空转计数**同样跳过豁免回合。
//
// 反向验证：让豁免只对重复生效（空转照数）⇒ 红。
func TestDetectExemptBreaksIdleCount(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{Exempt: true}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeIdle {
		t.Fatalf("豁免回合不该计入空转：%+v", f)
	}
	if f.Shape != roundguard.ShapeNone {
		t.Fatalf("两个非豁免回合凑不够任何形态，应判 %q，实得 %q", roundguard.ShapeNone, f.Shape)
	}
}

// TestDetectExemptBreaksErrorSpiralCount 钉住豁免面：**失败螺旋计数**同样跳过豁免回合。
//
// 反向验证：让豁免只对重复生效（失败照数）⇒ 红。
func TestDetectExemptBreaksErrorSpiralCount(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{Failed: true}),
		obs("a", roundguard.Observation{Failed: true, Exempt: true}),
		obs("a", roundguard.Observation{Failed: true}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeErrorSpiral {
		t.Fatalf("豁免回合不该计入失败螺旋：%+v", f)
	}
}

// TestDetectResultChangedBreaksEarlierStreak 钉住连续性打断的**另一侧**：结果变了
// 的那一轮**打断**连续性（它自己不算进那一段）——尾部那一段只算它之后的回合。
//
// 反向验证：去掉重复判据里对**前轮**「结果变了」的打断 ⇒ 连续长度被多算一轮 ⇒
// 三连同指纹被误判成重复 ⇒ 红。
func TestDetectResultChangedBreaksEarlierStreak(t *testing.T) {
	seq := []roundguard.Observation{
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{ResultChanged: true}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeRepeat {
		t.Fatalf("结果变了那一轮打断连续性，不该判重复：%+v", f)
	}
}

// TestDetectDriftBreaksOnLastResultChanged 钉住**末轮**「结果变了」对漂移的
// 打断：漂移判据必须与重复判据**同口径**（末轮结果变了 ⇒ 连续长度重起算）。
//
// 触发形态**不需要宿主违约**：`ResultChanged` 的含义是「与上一轮**同指纹**的
// 结果不同」，而末轮复用了更早出现过的指纹（如 p0）时，宿主合法置真。
//
// 反向验证：去掉 `trailingSameNormalized` 里的末轮 RC 分支 ⇒ 这串序列被判成
// 漂移（三轮首档 / 四轮直接停止档）⇒ 本用例红。
func TestDetectDriftBreaksOnLastResultChanged(t *testing.T) {
	three := []roundguard.Observation{
		{Fingerprint: "p0", Normalized: "pN", Tools: 1},
		{Fingerprint: "p1", Normalized: "pN", Tools: 1},
		{Fingerprint: "p0", Normalized: "pN", Tools: 1, ResultChanged: true},
	}
	f := roundguard.Detect(three, roundguard.Config{})
	if f.Shape == roundguard.ShapeDrift {
		t.Fatalf("末轮结果变了不该判漂移：%+v", f)
	}
	if f.Shape != roundguard.ShapeNone || f.Warn || f.Stalled {
		t.Fatalf("末轮有进展的序列不该判任何形态：%+v", f)
	}

	four := []roundguard.Observation{
		{Fingerprint: "p0", Normalized: "pN", Tools: 1},
		{Fingerprint: "p1", Normalized: "pN", Tools: 1},
		{Fingerprint: "p0", Normalized: "pN", Tools: 1},
		{Fingerprint: "p1", Normalized: "pN", Tools: 1, ResultChanged: true},
	}
	g := roundguard.Detect(four, roundguard.Config{})
	if g.Shape == roundguard.ShapeDrift {
		t.Fatalf("末轮结果变了不该判漂移：%+v", g)
	}
	if g.Shape != roundguard.ShapeNone || g.Warn || g.Stalled {
		t.Fatalf("末轮有进展的序列不该判任何形态：%+v", g)
	}
}

// TestDetectOscillationBreaksOnEarlierResultChanged 钉住震荡判据里**前轮**
// 「结果变了」的打断：交替序列中任一**非末轮**出现进展标记 ⇒ 那一段连续性从
// 该处重新起算 ⇒ 不判震荡。
//
// ⚠ 覆盖**全部非末轮位置**（2026-11-21 补）：震荡判据里有两处 RC 判据——循环体
// 里的 `seq[i].ResultChanged ||`（管位置 0..n-3）与末对的 `prev.ResultChanged`
// （管位置 n-2）。先前只钉了位置 0..3 ⇒ **末对那一位没有任何用例钉住**
// （实测：删掉 `prev.ResultChanged` ⇒ 整套 roundguard 用例仍全绿）。
//
// 反向验证：删掉 `trailingAlternating` 循环里的 `seq[i].ResultChanged ||`
// ⇒ 六轮交替被误判成震荡（首档）⇒ 本用例红；
// 删掉末对的 `prev.ResultChanged` ⇒ 第 4 轮出现进展标记仍被判成震荡 ⇒ 本用例红。
func TestDetectOscillationBreaksOnEarlierResultChanged(t *testing.T) {
	base := altSeq(6)
	for i := range base {
		base[i].Tools = 1 // 有工具调用 ⇒ 不落到空转那一档
	}
	// 全部非末轮位置逐个钉（末轮那条由 TestDetectResultChangedBreaksOscillation 钉）。
	for pos := 0; pos < len(base)-1; pos++ {
		seq := make([]roundguard.Observation, len(base))
		copy(seq, base)
		seq[pos].ResultChanged = true
		f := roundguard.Detect(seq, roundguard.Config{})
		if f.Shape == roundguard.ShapeOscillation {
			t.Fatalf("第 %d 轮出现进展标记不该判震荡：%+v", pos, f)
		}
		if f.Shape != roundguard.ShapeNone {
			t.Fatalf("第 %d 轮出现进展标记不该判任何形态：%+v", pos, f)
		}
	}
}

// TestDetectResultChangedBreaksOscillation 钉住连续性打断：结果变了 ⇒ 震荡不成立。
//
// 反向验证：去掉震荡判据里的「结果变了」打断 ⇒ 六回合交替被误判成震荡 ⇒ 红。
func TestDetectResultChangedBreaksOscillation(t *testing.T) {
	seq := altSeq(6)
	seq[len(seq)-1].ResultChanged = true
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeOscillation {
		t.Fatalf("结果变了不该判震荡：%+v", f)
	}
}

// TestDetectResultChangedBreaksDrift 钉住连续性打断：结果变了 ⇒ 漂移不成立。
//
// 反向验证：去掉漂移判据里的「结果变了」打断 ⇒ 红。
func TestDetectResultChangedBreaksDrift(t *testing.T) {
	seq := driftSeq(3)
	seq[0].ResultChanged = true
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape == roundguard.ShapeDrift {
		t.Fatalf("结果变了不该判漂移：%+v", f)
	}
}

// TestDetectWindowIsClampedUp 钉住**窗口下限**：窗口小于 2×Repeat+2 时**向上钳位**
// （否则震荡形态的停止档永不可达——判据不可达等于判据不存在）。
//
// 反向验证：去掉钳位 ⇒ Window=1 时只看最后一个回合 ⇒ 连着四次同指纹也判不出来 ⇒ 红。
func TestDetectWindowIsClampedUp(t *testing.T) {
	f := roundguard.Detect(repeatSeq(4), roundguard.Config{Window: 1, Repeat: 3})
	if f.Shape != roundguard.ShapeRepeat {
		t.Fatalf("窗口被钳位后仍应判 %q，实得 %q", roundguard.ShapeRepeat, f.Shape)
	}
	if !f.Stalled {
		t.Fatalf("窗口被钳位后停止档应可达：%+v", f)
	}
}

// TestDetectWindowTruncatesStreak 钉住**窗口上界**：只看最近 Window 个回合。
//
// 反向验证：把窗口裁剪去掉 ⇒ 连续段起点算成第 1 回合（Since=1）而不是窗口内
// 能看到的起点 ⇒ 红。
func TestDetectWindowTruncatesStreak(t *testing.T) {
	f := roundguard.Detect(repeatSeq(12), roundguard.Config{Window: 8, Repeat: 3})
	if f.Shape != roundguard.ShapeRepeat || !f.Stalled {
		t.Fatalf("窗口内的长重复应判停止档：%+v", f)
	}
	if f.Since != 5 {
		t.Fatalf("连续段起点应是窗口内第 5 回合（1 起），实得 %d", f.Since)
	}
}

// TestDetectSincePointsAtStreakStart 钉住 Since 的口径：形态**连续成立**的起点
// （全局 1 起）。
//
// 反向验证：把「+1」写成「不加」（0 起）⇒ 红。
func TestDetectSincePointsAtStreakStart(t *testing.T) {
	seq := []roundguard.Observation{
		obs("b", roundguard.Observation{}),
		obs("b", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
		obs("a", roundguard.Observation{}),
	}
	f := roundguard.Detect(seq, roundguard.Config{})
	if f.Shape != roundguard.ShapeRepeat {
		t.Fatalf("应判 %q，实得 %q", roundguard.ShapeRepeat, f.Shape)
	}
	if f.Since != 3 {
		t.Fatalf("连续段从第 3 回合起（1 起），实得 %d", f.Since)
	}
}

// TestDetectZeroConfigEqualsDefaults 钉住零值 = 缺省：Config{} 与显式写缺省值同结论。
func TestDetectZeroConfigEqualsDefaults(t *testing.T) {
	seq := repeatSeq(4)
	a := roundguard.Detect(seq, roundguard.Config{})
	b := roundguard.Detect(seq, roundguard.Config{
		Window: roundguard.DefaultWindow,
		Repeat: roundguard.DefaultRepeat,
	})
	if a != b {
		t.Fatalf("零值配置应与显式缺省同结论：%+v vs %+v", a, b)
	}
}
