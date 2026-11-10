package router

// 2026-11-11 issue 清零战回归测试（#107）：钉住决策面采纳路径的门槛自守。

import (
	"context"
	"math"
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
