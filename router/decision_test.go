package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oagentkit/llm/llmtest"
)

// stubDecisionFace 决策面桩：记录收到的候选集/槽位规格与调用次数。
type stubDecisionFace struct {
	route string
	conf  float64
	slots map[string]string
	err   error
	calls int
	// 决策面收到的候选集与槽位规格（断言「与 LLM 面同一候选集」）。
	gotRoutes []Route
	gotSlots  []SlotSpec
}

func (f *stubDecisionFace) Classify(_ context.Context, _ string, routes []Route, slots []SlotSpec) (Decision, error) {
	f.calls++
	f.gotRoutes = routes
	f.gotSlots = slots
	if f.err != nil {
		return Decision{}, f.err
	}
	return Decision{Route: f.route, Confidence: f.conf, Slots: f.slots}, nil
}

// newDecisionRouter 装一个带 LLM 桩 + 决策面桩的路由器。
func newDecisionRouter(t *testing.T, face *stubDecisionFace, minConf float64, shadow bool) (*Router, *llmtest.Model, *[]Decision, *[]error) {
	t.Helper()
	llm := &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{{Content: `{"route":"explain","confidence":0.9,"reason":"LLM 桩判定"}`}}}
	var observed []Decision
	var observedErrs []error
	r, err := New(&Config{
		Model:                 llm,
		Routes:                testRoutes(),
		Slots:                 []SlotSpec{{Name: "plan", Description: "是否要求出方案"}},
		DecisionFace:          face,
		DecisionMinConfidence: minConf,
		DecisionShadow:        shadow,
		DecisionObserver: func(d Decision, err error) {
			observed = append(observed, d)
			observedErrs = append(observedErrs, err)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r, llm, &observed, &observedErrs
}

// TestDecisionFaceWins 决策面达门槛 ⇒ 采纳，且**LLM 分类调用没发生**
// （省一次模型调用是这一面的收益来源）。
func TestDecisionFaceWins(t *testing.T) {
	face := &stubDecisionFace{route: "bug-fix", conf: 0.9, slots: map[string]string{"plan": "true"}}
	r, llm, observed, _ := newDecisionRouter(t, face, 0.6, false)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Route != "bug-fix" || d.Source != SourceDecision {
		t.Fatalf("决策面应胜出: %+v", d)
	}
	if llm.Calls != 0 {
		t.Errorf("决策面胜出时不应再调 LLM（实际调了 %d 次）", llm.Calls)
	}
	if d.Slots["plan"] != "true" {
		t.Errorf("槽位应透传: %+v", d.Slots)
	}
	if len(*observed) != 1 || (*observed)[0].Source != SourceDecision {
		t.Errorf("观察钩子应记录一次决策面结果: %+v", *observed)
	}
	// 决策面与 LLM 面**同一候选集**（不另造一份路由表）。
	if len(face.gotRoutes) != len(testRoutes()) {
		t.Errorf("决策面收到的候选集 = %d 条, want %d", len(face.gotRoutes), len(testRoutes()))
	}
	if len(face.gotSlots) != 1 || face.gotSlots[0].Name != "plan" {
		t.Errorf("决策面收到的槽位规格 = %+v", face.gotSlots)
	}
}

// TestDecisionFaceLowConfidenceFallsBack 置信未达门槛 ⇒ 回落 LLM 面。
func TestDecisionFaceLowConfidenceFallsBack(t *testing.T) {
	face := &stubDecisionFace{route: "bug-fix", conf: 0.3}
	r, llm, _, observedErrs := newDecisionRouter(t, face, 0.6, false)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Source != SourceLLM || d.Route != "explain" {
		t.Fatalf("低置信应回落 LLM 面: %+v", d)
	}
	if llm.Calls != 1 {
		t.Errorf("回落路径必须真的调了 LLM（实际 %d 次）", llm.Calls)
	}
	if len(*observedErrs) != 1 || (*observedErrs)[0] == nil {
		t.Errorf("观察钩子应记录低置信原因: %+v", *observedErrs)
	}
}

// TestDecisionFaceErrorFallsBack 端点故障 ⇒ 回落 LLM 面（fail-open：
// 判定面是加速件，不得阻断主链）。
func TestDecisionFaceErrorFallsBack(t *testing.T) {
	face := &stubDecisionFace{err: errors.New("端点不可达")}
	r, llm, _, observedErrs := newDecisionRouter(t, face, 0.6, false)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Source != SourceLLM {
		t.Fatalf("端点故障应回落 LLM 面: %+v", d)
	}
	if llm.Calls != 1 {
		t.Errorf("回落路径必须真的调了 LLM（实际 %d 次）", llm.Calls)
	}
	if len(*observedErrs) != 1 || (*observedErrs)[0] == nil {
		t.Errorf("观察钩子应记录故障原因: %+v", *observedErrs)
	}
}

// TestDecisionFaceUnknownRouteFallsBack 决策面给的路由名不在表内 ⇒ 回落
// （「自信但答非所问」不能进主链）。
func TestDecisionFaceUnknownRouteFallsBack(t *testing.T) {
	face := &stubDecisionFace{route: "no-such-route", conf: 0.99}
	r, llm, _, _ := newDecisionRouter(t, face, 0.6, false)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Source != SourceLLM {
		t.Fatalf("未知路由应回落 LLM 面: %+v", d)
	}
	if llm.Calls != 1 {
		t.Errorf("回落路径必须真的调了 LLM（实际 %d 次）", llm.Calls)
	}
}

// TestDecisionFaceShadowDoesNotAdopt 影子模式：行为面零变更（仍由 LLM 判），
// 但决策面**被调用**且结果进了观察钩子。
func TestDecisionFaceShadowDoesNotAdopt(t *testing.T) {
	face := &stubDecisionFace{route: "bug-fix", conf: 0.95}
	r, llm, observed, observedErrs := newDecisionRouter(t, face, 0.6, true)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Source != SourceLLM || d.Route != "explain" {
		t.Fatalf("影子模式不得改变行为: %+v", d)
	}
	if llm.Calls != 1 {
		t.Errorf("影子模式仍走 LLM 面（实际 %d 次）", llm.Calls)
	}
	if face.calls != 1 {
		t.Errorf("影子模式仍应调用决策面（实际 %d 次）", face.calls)
	}
	if len(*observed) != 1 || (*observed)[0].Route != "bug-fix" {
		t.Errorf("影子结果应进观察钩子: %+v", *observed)
	}
	if len(*observedErrs) != 1 || (*observedErrs)[0] == nil ||
		!strings.Contains((*observedErrs)[0].Error(), "影子") {
		t.Errorf("影子模式应记录影子标记: %+v", *observedErrs)
	}
}

// TestDecisionFaceDropsUnknownSlots 决策面给的未配置槽位名一律丢弃
// （与 LLM 面的槽位自守恒同纪律：防幻觉槽位名透传给消费方）。
func TestDecisionFaceDropsUnknownSlots(t *testing.T) {
	face := &stubDecisionFace{route: "bug-fix", conf: 0.9,
		slots: map[string]string{"plan": "true", "hack": "x"}}
	r, _, _, _ := newDecisionRouter(t, face, 0.6, false)
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Slots["plan"] != "true" {
		t.Errorf("已配置槽位应保留: %+v", d.Slots)
	}
	if _, ok := d.Slots["hack"]; ok {
		t.Errorf("未配置槽位应被丢弃: %+v", d.Slots)
	}
}

// TestClassifyWithoutDecisionFaceUnchanged 不配置决策面时行为与从前一致
// （Source=llm；LLM 正常被调用）。
func TestClassifyWithoutDecisionFaceUnchanged(t *testing.T) {
	llm := &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{{Content: `{"route":"bug-fix","confidence":0.9,"reason":"修代码"}`}}}
	r, err := New(&Config{Model: llm, Routes: testRoutes()})
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Classify(context.Background(), "帮我修这个空指针")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if d.Route != "bug-fix" || d.Source != SourceLLM {
		t.Fatalf("无决策面时行为应与从前一致: %+v", d)
	}
	if llm.Calls != 1 {
		t.Errorf("LLM 应被调用一次（实际 %d 次）", llm.Calls)
	}
}
