package router

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// slotCfg 构造「槽位名 → 已配置」表。
func slotCfg(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// TestSanitizeSlotsTruncatesByRune 槽位值截断走 rune 口径（agent#57）。
//
// 按字节 `v[:64]` 切会在多字节字符中间断开，产出**非法 UTF-8**——下游任何
// 按字符串处理的环节（JSON 编码、日志、匹配）都会拿到坏数据，而且没有任何
// 报错点。
func TestSanitizeSlotsTruncatesByRune(t *testing.T) {
	many := strings.Repeat("中", 100)  // 300 字节 / 100 rune
	fit := strings.Repeat("文", 30)    // 90 字节、30 rune —— 超字节上限但未超 rune 上限
	exact := strings.Repeat("字", 64)  // 恰好 64 rune
	ascii := strings.Repeat("a", 100) // 100 字节 / 100 rune

	cases := []struct {
		name string
		in   string
		want int // 期望 rune 数
	}{
		{"超 rune 上限的中文", many, slotValueCap},
		{"超字节上限但未超 rune 上限的中文", fit, 30}, // 旧实现会在这里切坏
		{"恰好上限的中文", exact, slotValueCap},
		{"超上限的 ASCII", ascii, slotValueCap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeSlots(map[string]string{"k": tc.in}, slotCfg("k"))
			got, ok := out["k"]
			if !ok {
				t.Fatal("槽位应保留")
			}
			if !utf8.ValidString(got) {
				t.Fatalf("截断结果必须是合法 UTF-8（按字节切会切在字符中间）: %q", got)
			}
			if n := utf8.RuneCountInString(got); n != tc.want {
				t.Fatalf("rune 数 = %d, want %d（截断口径必须是 rune 不是字节）", n, tc.want)
			}
			// 未超上限的值必须**原样**保留——旧实现在这条上也会切
			if tc.want == utf8.RuneCountInString(tc.in) && got != tc.in {
				t.Fatalf("未超上限的值不得被改动:\n got=%q\nwant=%q", got, tc.in)
			}
		})
	}
}

// TestSanitizeSlotsKeepsShortValues 负对照：短值不受影响（截断逻辑不能误伤）。
func TestSanitizeSlotsKeepsShortValues(t *testing.T) {
	out := sanitizeSlots(map[string]string{
		"plan": "true",
		"zh":   "修复空指针",
	}, slotCfg("plan", "zh"))
	if out["plan"] != "true" {
		t.Fatalf("短值应原样: %q", out["plan"])
	}
	if out["zh"] != "修复空指针" {
		t.Fatalf("短中文值应原样: %q", out["zh"])
	}
}
