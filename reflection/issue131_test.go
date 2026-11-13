package reflection

// 本文件钉 oagentkit/agent#131：Round.Critique 必须带上 Critic 原始 JSON
// （审计留痕），不得恒为空串。

import (
	"testing"

	"github.com/oagentkit/llm/llmtest"
)

// TestCritiqueRawRecorded_Issue131 critiqueVerdict.Raw 此前标 json:"-" 且
// 全仓无任何赋值点 ⇒ Round.Critique 恒空，下游读它做可观测/回放拿到的永远
// 是空。修法：critiqueVerdict.UnmarshalJSON 在解码时截获原始字节写入 Raw。
func TestCritiqueRawRecorded_Issue131(t *testing.T) {
	const wantRaw = `{"pass":true,"issues":[]}`
	fm := &llmtest.Model{Script: []llmtest.Resp{
		{Content: "d1"},
		// 带 markdown 围栏：留痕口径 = ExtractJSON 提取后的 Critic JSON。
		{Content: "```json\n" + wantRaw + "\n```"},
	}}
	res, err := Refine(t.Context(), &Config{Model: fm, Task: "t", Rubric: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Converged {
		t.Fatalf("应收敛: %+v", res)
	}
	if len(res.Rounds) != 1 {
		t.Fatalf("应有一轮记录: %d", len(res.Rounds))
	}
	got := res.Rounds[0].Critique
	if got == "" {
		t.Fatal("Critique 恒为空（agent#131 本体）：Raw 未被写入")
	}
	if got != wantRaw {
		t.Fatalf("Critique 应为提取后的 Critic 原始 JSON: got=%q want=%q", got, wantRaw)
	}
}

// TestCritiqueRawRecordedNonEmptyOnStalled_Issue131 判停路径复用上一轮
// verdict 的留痕语义不变（#127），但复用的留痕本身不得再是空串。
func TestCritiqueRawRecordedNonEmptyOnStalled_Issue131(t *testing.T) {
	draft := "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnop"
	revised := "初稿内容ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmno"
	fm := &llmtest.Model{Script: []llmtest.Resp{
		{Content: draft},
		{Content: `{"pass":false,"issues":["x"]}`},
		{Content: revised},
	}}
	res, err := Refine(t.Context(), &Config{Model: fm, Task: "t", Rubric: "r", MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stalled {
		t.Fatal("应判停（复现 #127 路径）")
	}
	if len(res.Rounds) != 2 {
		t.Fatalf("应有两轮记录: %d", len(res.Rounds))
	}
	for i, r := range res.Rounds {
		if r.Critique == "" {
			t.Fatalf("第 %d 轮 Critique 不得为空（agent#131）", i+1)
		}
	}
	if res.Rounds[1].Critique != res.Rounds[0].Critique {
		t.Fatalf("判停轮留痕应复用上一轮: last=%q prev=%q",
			res.Rounds[1].Critique, res.Rounds[0].Critique)
	}
}
