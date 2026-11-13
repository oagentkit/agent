package clarify

// 2026-11-11 issue 清零战回归测试（#108 #109）：钉住序数双层后缀与最长匹配。
// 2026-11-13 清零战役追加（#129）：多字否定词的二元翻转。

import "testing"

// TestOrdinalIndexDoubleSuffix_Issue108 「序数词缀 + 尾标点」双层后缀必须剥净
// ——中文回答常态带尾句读，单遍过表只剥一层时用户的明确选择被当换话题丢弃。
func TestOrdinalIndexDoubleSuffix_Issue108(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"第一个。", 1}, {"第1个。", 1}, {"第二个、", 2}, {"选2。", 2},
		{"第3个.", 3}, {"选项1。", 1}, {"第2个", 2}, {"第一个", 1},
	}
	for _, c := range cases {
		n, ok := OrdinalIndex(c.in)
		if !ok || n != c.want {
			t.Errorf("OrdinalIndex(%q) = (%d,%v), want (%d,true)", c.in, n, ok, c.want)
		}
	}
	// 普通句子不误伤：剥离不动点后剩余 >1 rune。
	if _, ok := OrdinalIndex("1个进程在跑。"); ok {
		t.Error("普通句子不应误判为序数指代")
	}
}

// TestResolveAnswerLongestContainsMatch_Issue109 Contains 轮取最长命中
// ——泛词在前（推荐声明序）时短选项截胡带前缀回答的形态修掉。
func TestResolveAnswerLongestContainsMatch_Issue109(t *testing.T) {
	options := []string{"删除", "删除数据库"}
	// 带前缀的宽松回答须命中长选项。
	picked, _, ok := ResolveAnswer(options, "放弃", "请删除数据库")
	if !ok || picked != "删除数据库" {
		t.Fatalf("「请删除数据库」应消解为「删除数据库」，实际 (%q,%v)", picked, ok)
	}
	// 精确形态不变。
	picked, _, ok = ResolveAnswer(options, "放弃", "删除数据库")
	if !ok || picked != "删除数据库" {
		t.Fatalf("精确形态漂移: (%q,%v)", picked, ok)
	}
	// 只有短选项的既有形态保持。
	picked, _, ok = ResolveAnswer(options, "放弃", "帮我删除")
	if !ok || picked != "删除" {
		t.Fatalf("短选项命中漂移: (%q,%v)", picked, ok)
	}
	// 否定形态防护不受最长匹配影响。
	picked, isFb, ok := ResolveAnswer([]string{"同意", "拒绝"}, "不清楚，全面查", "不同意")
	if !ok || isFb || picked != "拒绝" {
		t.Fatalf("二元否定翻转漂移: (%q,%v,%v)", picked, isFb, ok)
	}
}

// TestResolveAnswerMultiCharNegation_Issue129 多字否定词（不是/不同意/不可以/
// 不会/不能/不需要 + 选项词）必须进否定检测——否定判定是整串精确拼接
// （a == neg+opt），词表缺多字词时拼接不成立 ⇒ 否定检测落空 ⇒ 第二轮宽松
// Contains 把**被否定的选项**命中返回：用户答「不同意删除数据库」曾被消解成
// picked=删除数据库（ok=true），明确拒绝的操作被选中执行（P2）。
func TestResolveAnswerMultiCharNegation_Issue129(t *testing.T) {
	options := []string{"删除数据库", "保留数据"}
	fb := "不清楚，全面查"
	// 多字否定词形态：一律翻转到另一选项，绝不得命中被否定的选项。
	for _, answer := range []string{
		"不是删除数据库", "不同意删除数据库", "不可以删除数据库",
		"不会删除数据库", "不能删除数据库", "不需要删除数据库",
	} {
		picked, isFb, ok := ResolveAnswer(options, fb, answer)
		if !ok || isFb || picked != "保留数据" {
			t.Errorf("%s: ResolveAnswer = (%q,%v,%v)，want (保留数据,false,true)",
				answer, picked, isFb, ok)
		}
	}
	// 对照组：词表内既有形态不回退（#69 语义保持）。
	for _, answer := range []string{"不要删除数据库", "别删除数据库", "不用删除数据库", "没有删除数据库"} {
		picked, _, ok := ResolveAnswer(options, fb, answer)
		if !ok || picked != "保留数据" {
			t.Errorf("%s 应翻转到「保留数据」: (%q,%v)", answer, picked, ok)
		}
	}
	// 否定无法定位到单一选项时不翻转、也不得命中被否定选项（与 #69 三元语义一致）。
	picked, _, ok := ResolveAnswer([]string{"同意", "拒绝", "弃权"}, fb, "不同意")
	if ok || picked != "" {
		t.Fatalf("三元下多字否定不应翻转: (%q,%v)", picked, ok)
	}
}
