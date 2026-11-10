package clarify

// 2026-11-11 issue 清零战回归测试（#108 #109）：钉住序数双层后缀与最长匹配。

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
