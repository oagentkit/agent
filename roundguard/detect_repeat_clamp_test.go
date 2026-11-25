package roundguard_test

import (
	"testing"

	"github.com/oagentkit/agent/roundguard"
)

// ── Config.Repeat 的下界钳位（终审 I4）──
//
// # 这个用例在钉什么
//
// 「k = 1」是**退化档**：①「尾部同指纹连续长度」恒 ≥ 1 ⇒ 任意单个回合即判重复
// （首档）；② 漂移形态要求「归一指纹全等**且**原指纹不全等」，而单元素的
// 「原指纹不全等」恒假 ⇒ 漂移那一档**永不可达**。
//
// 这与本包刚修过的「默认窗口 6 ⇒ 震荡停止档永不可达」**完全同型**——那里修的
// 是「一档不可达」，这里是「k=1 ⇒ 一形态不可达 + 阈值退化成 1」。
//
// ⇒ 判据：**k=1 被向上钳位到 2**（与窗口向上钳位同款，不报错）。钳位之后：
// ① 单回合**不再**判重复；② 漂移形态**重新可达**。
//
// # 反向验证（坏实现会红）
//
// 删掉 Detect 里的「if k < 2 { k = 2 }」⇒ k=1 时单回合又判成重复首档、漂移
// 又不可达 ⇒ 本文件的四条用例同时红 ⇒ 钳位的承重夹具在这里。
//
// 合成数据：指纹用字母串（a / b / c），工具标识用 read / write。

// TestDetectRepeatClampedFromOne 单回合在 k=1 下**不得**判成重复（钳位后 k=2）。
//
// 为什么必须钉这条：k=1 时「尾部同指纹连续长度」恒 ≥ 1 ⇒ 任意单个回合即命中
// 重复首档 ⇒ 宿主每转一圈就被警告一次（首档本意是「给模型一次自我修正机会」，
// 而退化档下它变成了每圈都响的噪声）。
func TestDetectRepeatClampedFromOne(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
	}

	f := roundguard.Detect(seq, roundguard.Config{Repeat: 1})
	if f.Shape != roundguard.ShapeNone {
		t.Fatalf("k=1 被钳到 2 后单回合不应命中任何形态（实得 %q）", f.Shape)
	}
}

// TestDetectDriftReachableAtRepeatOne 钉住 k=1 下漂移形态**重新可达**。
//
// 为什么必须钉这条：漂移是本包五种形态里**唯一**要求「原指纹不全等」的——
// 单元素的「原指纹不全等」恒假 ⇒ k=1 时漂移那一档**没有落点**（判据不可达 =
// 判据不存在）。钳位到 2 之后，两回合的「归一指纹同、原指纹不同」即可命中。
func TestDetectDriftReachableAtRepeatOne(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
	}

	f := roundguard.Detect(seq, roundguard.Config{Repeat: 1})
	if f.Shape != roundguard.ShapeDrift {
		t.Fatalf("k=1 被钳到 2 后漂移形态应可达（实得 %q）", f.Shape)
	}
}

// TestDetectRepeatOneEqualsRepeatTwo 钉住「k=1 的结论与 k=2 逐字相同」。
//
// 为什么单独一条：前两条钉的是**具体形态**（单回合不判重复、漂移可达）；
// 这一条钉的是**钳位语义本身**——k=1 与 k=2 的输入空间必须产出同一个结论集。
// 若实现把 k=1 钳到别的值（或不钳），这条立刻红。
func TestDetectRepeatOneEqualsRepeatTwo(t *testing.T) {
	seqs := [][]roundguard.Observation{
		{{Fingerprint: "a", Normalized: "n", Tools: 1}},
		{{Fingerprint: "a", Normalized: "n", Tools: 1}, {Fingerprint: "a", Normalized: "n", Tools: 1}},
		{{Fingerprint: "a", Normalized: "n", Tools: 1}, {Fingerprint: "b", Normalized: "n", Tools: 1}},
		{{Fingerprint: "a", Tools: 1}, {Fingerprint: "b", Tools: 1}, {Fingerprint: "a", Tools: 1}},
	}

	for i, seq := range seqs {
		one := roundguard.Detect(seq, roundguard.Config{Repeat: 1})
		two := roundguard.Detect(seq, roundguard.Config{Repeat: 2})
		if one != two {
			t.Fatalf("序列 %d：k=1（应钳到 2）判 %+v，k=2 判 %+v——两者必须相同", i, one, two)
		}
	}
}

// TestDetectRepeatOutOfRangeClampAndDefault 钉住越界 k 的**两条不同口径**：
//
//	k = 1        → **向上钳位到 2**（漂移形态要可达 ⇒ 阈值不得退化成 1）；
//	k = 0 / 负数 → **按缺省处理**（k = DefaultRepeat），**不是**钳到 2。
//
// 为什么单独一条：两条口径**不同**，别让实现把 1 也当成缺省（那会静默把阈值
// 抬到 3，与「至少取 2」的契约不符），也别把 0/负数当成钳到 2（那会把「没配
// 阈值」误读成「只要 2 回合就重复」）。
//
// 终审复审 F4 指出：原用例的 k=0/-1 分支只有 `if k == 1 &&` 守卫 ⇒ 那个分支
// **只被计算、从未被断言**（死代码）⇒「0/负数走缺省」这条口径没有任何断言
// 钉住，将来被改成别的值也不会红；用例名「BelowOneClampedToTwo」还与它自己
// 的注释相矛盾。本条把两条口径都补成**真断言**，名字也随之改成实话。
func TestDetectRepeatOutOfRangeClampAndDefault(t *testing.T) {
	seq := []roundguard.Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
	}

	wantClamp := roundguard.Detect(seq, roundguard.Config{Repeat: 2})
	wantDefault := roundguard.Detect(seq, roundguard.Config{Repeat: roundguard.DefaultRepeat})

	// k=1：向上钳位到 2（钳位被删 ⇒ 这里立刻红）
	if got := roundguard.Detect(seq, roundguard.Config{Repeat: 1}); got != wantClamp {
		t.Fatalf("k=1 应钳到 2：判 %+v，k=2 判 %+v", got, wantClamp)
	}

	// k=0 / 负数：按缺省处理（= DefaultRepeat），**不是**钳到 2
	for _, k := range []int{0, -1, -7} {
		got := roundguard.Detect(seq, roundguard.Config{Repeat: k})
		if got != wantDefault {
			t.Fatalf("k=%d 应按缺省（DefaultRepeat=%d）处理：判 %+v，缺省判 %+v",
				k, roundguard.DefaultRepeat, got, wantDefault)
		}
	}
}
