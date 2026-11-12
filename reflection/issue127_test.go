package reflection

// 本文件钉 oagentkit/agent#127 的回归：Stalled（稿差判停）路径下 Result.Text
// 必须与 Rounds 末条同稿。Round 只在批判前记录（Draft = 被批判的那一稿），
// 而判停路径的 res.Text 是修订后的重复稿——该稿从未被批判、从未进过 Rounds，
// 消费方按「Rounds[last].Draft 是交付稿」读审计留痕时对不上（另外三条终止
// 路径 Text 与末条都一致，唯独 Stalled 脱节）。

import (
	"context"
	"testing"

	"github.com/oagentkit/llm/llmtest"
)

// TestRefineStalledRoundRecord_Issue127 判停路径：重复稿记进 Rounds，
// Text == Rounds 末条 Draft，批判留痕复用上一轮 verdict。
func TestRefineStalledRoundRecord_Issue127(t *testing.T) {
	draft := "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnop"
	// 修订稿与初稿仅差尾部一字符 ⇒ Similarity >= 0.98 ⇒ 判停。
	revised := "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmno"
	fm := &llmtest.Model{Script: []llmtest.Resp{
		{Content: draft},
		{Content: `{"pass":false,"issues":["x"]}`},
		{Content: revised},
	}}
	res, err := Refine(context.Background(), &Config{
		Model: fm, Task: "t", Rubric: "r", MaxIterations: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stalled || res.Converged {
		t.Fatalf("应判停且不收敛: stalled=%v converged=%v", res.Stalled, res.Converged)
	}
	if res.Text != revised {
		t.Fatalf("Text 应为重复稿: %q", res.Text)
	}
	// 核心不变式：Text 与 Rounds 末条同稿（本条 issue 的缺陷本体）。
	if len(res.Rounds) != 2 {
		t.Fatalf("判停路径应有两轮记录（初稿 + 重复稿）: %d", len(res.Rounds))
	}
	last := res.Rounds[len(res.Rounds)-1]
	if last.Draft != res.Text {
		t.Fatalf("Rounds 末条 Draft 必须等于 Text（agent#127）: last=%q text=%q", last.Draft, res.Text)
	}
	// 重复稿未再批判：Pass=false，批判留痕复用上一轮 verdict（审计仍可追到
	// 「这一稿被评成什么」——相似度 ≥0.98，批判对重复稿同样成立）。
	if last.Pass {
		t.Fatal("重复稿不应记 Pass=true（本轮未再批判、上一轮判的是不通过）")
	}
	// 复用语义：末条 Critique 与上一轮同源（Raw 是否有值是另一面的既有状态，
	// 本条只钉「复用」而不是「重新批判」）。
	if last.Critique != res.Rounds[0].Critique {
		t.Fatalf("末条批判留痕应复用上一轮: last=%q prev=%q", last.Critique, res.Rounds[0].Critique)
	}
	if len(last.Issues) != 1 || last.Issues[0] != "x" {
		t.Fatalf("Issues 应带上一轮评审发现: %+v", last.Issues)
	}
	// 初稿轮次不受影响。
	if res.Rounds[0].Draft != draft {
		t.Fatalf("初稿轮次应保留: %q", res.Rounds[0].Draft)
	}
}

// TestRefineTextAlwaysMatchesLastRound_Issue127 不变式面：三条终止路径
// （收敛 / 达上限 / 判停）Text 都必须等于 Rounds 末条 Draft——任一路径
// 脱节即本条回归。
func TestRefineTextAlwaysMatchesLastRound_Issue127(t *testing.T) {
	cases := []struct {
		name   string
		script []llmtest.Resp
		max    int
		stall  bool
	}{
		{
			name: "收敛",
			script: []llmtest.Resp{
				{Content: "d1"},
				{Content: `{"pass":false,"issues":["x"]}`},
				{Content: "d2"},
				{Content: `{"pass":true,"issues":[]}`},
			},
			max: 3,
		},
		{
			name: "达上限",
			script: []llmtest.Resp{
				{Content: "d1"},
				{Content: `{"pass":false,"issues":["x"]}`},
				{Content: "d2"},
				{Content: `{"pass":false,"issues":["y"]}`},
				{Content: "d3"},
				{Content: `{"pass":false,"issues":["z"]}`},
			},
			max: 3,
		},
		{
			name: "稿差判停",
			script: []llmtest.Resp{
				{Content: "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnop"},
				{Content: `{"pass":false,"issues":["x"]}`},
				{Content: "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmno"},
			},
			max:   3,
			stall: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := &llmtest.Model{Script: tc.script}
			res, err := Refine(context.Background(), &Config{
				Model: fm, Task: "t", Rubric: "r", MaxIterations: tc.max,
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.Stalled != tc.stall {
				t.Fatalf("Stalled=%v, want %v", res.Stalled, tc.stall)
			}
			if len(res.Rounds) == 0 {
				t.Fatal("任何终止路径都应有轮次记录")
			}
			if last := res.Rounds[len(res.Rounds)-1]; last.Draft != res.Text {
				t.Fatalf("Text 必须等于 Rounds 末条 Draft: text=%q last=%q", res.Text, last.Draft)
			}
		})
	}
}
