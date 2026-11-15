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

// TestOrdinalIndexMultiDigit_Issue147 多位数序数（阿拉伯数字）与汉字十位合成
// ——单 rune 限制曾让「第10个」及以上整类解析失败，选项数 >9 时用户的明确
// 选择被当换话题丢弃。
func TestOrdinalIndexMultiDigit_Issue147(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"第10个", 10, true},
		{"第10", 10, true},
		{"第 10 个", 10, true},
		{"第10个。", 10, true},
		{"选项12", 12, true},
		{"10", 10, true},
		{"第十个", 10, true},
		{"第十", 10, true},
		{"第十一", 11, true},
		{"第二十个", 20, true},
		{"第二十三", 23, true},
		{"九十九", 99, true},
		// 普通句子仍不误伤（剥缀后残留非纯数字）。
		{"第10个进程在跑", 0, false},
		{"第十个进程在跑", 0, false},
		{"十全十美", 0, false},
		{"第", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		n, ok := OrdinalIndex(c.in)
		if ok != c.ok || (ok && n != c.want) {
			t.Errorf("OrdinalIndex(%q) = (%d,%v), want (%d,%v)", c.in, n, ok, c.want, c.ok)
		}
	}
}

// TestResolveAnswerOrdinalBeyondNine_Issue147 选项数 >9 时「第10个」必须消解到
// 第 10 个选项，而不是落到 Contains/换话题路径。
func TestResolveAnswerOrdinalBeyondNine_Issue147(t *testing.T) {
	options := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"}
	fallback := "不清楚，全面查"
	for _, c := range []struct {
		answer string
		want   string
	}{
		{"第10个", "J"},
		{"第十个", "J"},
		{"第十一个", "K"},
		{"第12个", "L"},
		{"第 10 个。", "J"},
	} {
		picked, isFb, ok := ResolveAnswer(options, fallback, c.answer)
		if !ok || isFb || picked != c.want {
			t.Errorf("%s: ResolveAnswer = (%q,%v,%v), want (%q,false,true)", c.answer, picked, isFb, ok, c.want)
		}
	}
	// 越界序数仍按「放弃消解」处理（由调用方按换话题/逃生路径处置）。
	if picked, _, ok := ResolveAnswer(options, fallback, "第13个"); ok || picked != "" {
		t.Fatalf("越界序数不应消解: (%q,%v)", picked, ok)
	}
}

// TestResolveAnswerEnglishNegation_Issue146 英文否定与中文同权：否定词表纯中文
// 时「not yes」会被 Contains 当成选择了 yes（二元题翻转防护对英文整体缺席）。
func TestResolveAnswerEnglishNegation_Issue146(t *testing.T) {
	fb := "not sure"
	// 二元翻转：英文否定 → 另一个选项。
	for _, c := range []struct {
		name    string
		options []string
		answer  string
		want    string
	}{
		{"not yes → no", []string{"yes", "no"}, "not yes", "no"},
		{"never yes → no", []string{"yes", "no"}, "never yes", "no"},
		{"don't yes → no", []string{"yes", "no"}, "don't yes", "no"},
		{"dont yes → no", []string{"yes", "no"}, "dont yes", "no"},
		{"no thanks → later", []string{"thanks", "later"}, "no thanks", "later"},
		{"not delete database → keep snapshot", []string{"delete database", "keep snapshot"}, "not delete database", "keep snapshot"},
	} {
		picked, isFb, ok := ResolveAnswer(c.options, fb, c.answer)
		if !ok || isFb || picked != c.want {
			t.Errorf("%s: ResolveAnswer(%q) = (%q,%v,%v), want (%q,false,true)",
				c.name, c.answer, picked, isFb, ok, c.want)
		}
	}
	// 精确命中仍优先：「no」本身就是选项时直接选中，不被否定逻辑吞掉。
	if picked, _, ok := ResolveAnswer([]string{"yes", "no"}, fb, "no"); !ok || picked != "no" {
		t.Fatalf("精确选项 no 应命中: (%q,%v)", picked, ok)
	}
	// 非二元且否定无法定位（不被否定的最长/唯一选项也不存在）→ 不翻转。
	if picked, _, ok := ResolveAnswer([]string{"yes", "maybe", "ok"}, fb, "not yes"); ok || picked != "" {
		t.Fatalf("三元下英文否定不应翻转: (%q,%v)", picked, ok)
	}
	// 中文既有形态不回退。
	if picked, _, ok := ResolveAnswer([]string{"是", "否"}, fb, "不是"); !ok || picked != "否" {
		t.Fatalf("中文否定翻转漂移: (%q,%v)", picked, ok)
	}
}
