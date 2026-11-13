package router

// 2026-11-11 issue 清零战回归测试（#107）：钉住决策面采纳路径的门槛自守。
// 2026-11-13 清零战役追加（#130）：门槛值域构造期校验。

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/oagentkit/llm/llmtest"
)

// TestDecisionFaceRespectsGlobalGate_Issue107 决策面采纳前必须过全局闸
// （路由表合法性 + 全局 MinConfidence）——「门槛自守」承诺此前只对 LLM 面
// 成立，决策面产出的非法路由名/低于全局门槛的结果直达分发面。
func TestDecisionFaceRespectsGlobalGate_Issue107(t *testing.T) {
	// 全局 MinConfidence=0.8 > 决策面门槛 0.3：低置信决策面结果不得采纳，
	// 须回落 LLM 面（LLM 桩给出 0.9 合法结果）。
	face := &stubDecisionFace{route: "bug-fix", conf: 0.5}
	llm := &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{{Content: `{"route":"bug-fix","confidence":0.9,"reason":"ok"}`}}}
	r, err := New(&Config{
		Model: llm, Routes: testRoutes(),
		MinConfidence:         0.8,
		DecisionFace:          face,
		DecisionMinConfidence: 0.3,
	})
	if err != nil {
		t.Fatal(err)
	}
	d, cerr := r.Classify(context.Background(), "帮我修个 bug")
	if cerr != nil {
		t.Fatalf("决策面 0.5 过决策面门槛(0.3)但低于全局门槛(0.8)，应回落 LLM 面成功，实际 err=%v", cerr)
	}
	if d.Source != SourceLLM {
		t.Fatalf("应回落 LLM 面（source=llm），实际 source=%s", d.Source)
	}
	if llm.Calls != 1 {
		t.Fatalf("LLM 面应被调用 1 次，实际 %d", llm.Calls)
	}

	// 非法路由名：不得采纳，回落 LLM 面。
	face2 := &stubDecisionFace{route: "no-such-route", conf: 0.99}
	llm2 := &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{{Content: `{"route":"explain","confidence":0.9,"reason":"ok"}`}}}
	r2, err := New(&Config{Model: llm2, Routes: testRoutes(), DecisionFace: face2})
	if err != nil {
		t.Fatal(err)
	}
	d2, cerr2 := r2.Classify(context.Background(), "解释一下")
	if cerr2 != nil || d2.Source != SourceLLM {
		t.Fatalf("非法路由应回落 LLM 面: d=%+v err=%v", d2, cerr2)
	}

	// NaN 置信度：不得穿过门槛静默采纳（clamp01 不拦 NaN）。
	face3 := &stubDecisionFace{route: "bug-fix", conf: math.NaN()}
	llm3 := &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{{Content: `{"route":"bug-fix","confidence":0.9,"reason":"ok"}`}}}
	r3, err := New(&Config{Model: llm3, Routes: testRoutes(), MinConfidence: 0.5, DecisionFace: face3})
	if err != nil {
		t.Fatal(err)
	}
	if _, cerr3 := r3.Classify(context.Background(), "修 bug"); cerr3 != nil {
		t.Fatalf("NaN 应被拦（回落 LLM 面成功），实际 err=%v", cerr3)
	}
}

// TestNewRejectsOutOfRangeThresholds_Issue130 门槛值域构造期校验：
// 负值把门槛静默关掉（「置信度 < 负阈值」恒假 ⇒ 低置信度全放行）、
// 百分比误填 85 让一切分类永远低于门槛（无差别永远 fallback）——
// 两种配置事故都必须在 New 就报错，不得带病上线（「门槛自守」的值域维度）。
func TestNewRejectsOutOfRangeThresholds_Issue130(t *testing.T) {
	stub := func() *llmtest.Model {
		return &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{
			{Content: `{"route":"bug-fix","confidence":0.9,"reason":"ok"}`},
		}}
	}
	cases := []struct {
		name string
		cfg  *Config
	}{
		{"负值 MinConfidence（手滑 -0.5）", &Config{Model: stub(), Routes: testRoutes(), MinConfidence: -0.5}},
		{">1 MinConfidence（百分比误填 85）", &Config{Model: stub(), Routes: testRoutes(), MinConfidence: 85}},
		{"NaN MinConfidence", &Config{Model: stub(), Routes: testRoutes(), MinConfidence: math.NaN()}},
		{"负值 DecisionMinConfidence", &Config{Model: stub(), Routes: testRoutes(), DecisionMinConfidence: -0.1}},
		{">1 DecisionMinConfidence", &Config{Model: stub(), Routes: testRoutes(), DecisionMinConfidence: 85}},
		{"NaN DecisionMinConfidence", &Config{Model: stub(), Routes: testRoutes(), DecisionMinConfidence: math.NaN()}},
	}
	for _, c := range cases {
		if _, err := New(c.cfg); err == nil {
			t.Errorf("%s 应构造期报错（agent#130）", c.name)
		} else if !strings.Contains(err.Error(), "须在 [0,1]") {
			t.Errorf("%s 报错文案应指明值域: %v", c.name, err)
		}
	}
	// 合法值域边界保留：0 = 不设门槛（既有语义），0.5/1 常规值不得误伤。
	for _, v := range []float64{0, 0.5, 1} {
		cfg := &Config{Model: stub(), Routes: testRoutes(), MinConfidence: v, DecisionMinConfidence: v}
		if _, err := New(cfg); err != nil {
			t.Errorf("MinConfidence=DecisionMinConfidence=%v 应合法: %v", v, err)
		}
	}
}
