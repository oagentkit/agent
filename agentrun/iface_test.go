package agentrun

import (
	"context"
	"errors"
	"testing"

	"github.com/oagentkit/agent/iface"
)

// iface 适配器（agent#174）：三处映射——参数 / 结果 / 事件，外加状态词表分类。
// 测试复用本包既有的 newMockOpenAI 网关替身（非 llmtest 桩：桩吞不掉真实调用链）。

func TestIfaceAdapterSuccess(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{content: "最终答案"})
	a := &Agent{Cfg: Config{Name: "t", Instruction: "inst", Model: cm}}

	got, err := a.Run(context.Background(), iface.Input{Query: "问题"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 字面量与 contract 常量逐字对齐（contract.StatusSucceeded = "succeeded"）
	if got.Status != "succeeded" {
		t.Fatalf("Status = %q, want succeeded", got.Status)
	}
	if got.Final != "最终答案" {
		t.Fatalf("Final = %q, want 最终答案", got.Final)
	}
}

func TestIfaceAdapterFailure(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{status: 500})
	a := &Agent{Cfg: Config{Name: "t", Instruction: "inst", Model: cm}}

	got, err := a.Run(context.Background(), iface.Input{Query: "问题"})
	if err == nil {
		t.Fatal("上游 500 应当返回 error")
	}
	if got.Status != "failed" {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.Final != "" {
		t.Fatalf("失败时 Final 应为空，实得 %q", got.Final)
	}
}

func TestIfaceAdapterEvents(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{content: "答案"})
	a := &Agent{Cfg: Config{Name: "t", Instruction: "inst", Model: cm}}

	var types []string
	_, err := a.Run(context.Background(), iface.Input{
		Query:   "问题",
		OnEvent: func(e iface.Event) { types = append(types, e.Type) },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(types) == 0 {
		t.Fatal("OnEvent 应当至少收到一个事件（agentrun 原生词表透传）")
	}
	// 词表透传：Type 必须是 agentrun 原生字面量，不是 iface 自造词
	found := false
	for _, ty := range types {
		switch ty {
		case EventText, EventTextDelta, EventReasoning, EventReasoningDelta:
			found = true
		}
	}
	if !found {
		t.Fatalf("事件 Type 不在 agentrun 原生词表内：%v", types)
	}
}

func TestIfaceAdapterSessionIDIgnored(t *testing.T) {
	// agentrun 无会话概念：SessionID 忽略但不报错（有会话要求的驱动用会话制适配器）
	_, cm := newMockOpenAI(t, mockResponse{content: "ok"})
	a := &Agent{Cfg: Config{Name: "t", Instruction: "inst", Model: cm}}
	got, err := a.Run(context.Background(), iface.Input{Query: "q", SessionID: "s-1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("Status = %q", got.Status)
	}
}

func TestIfaceStatusOf(t *testing.T) {
	// 状态分类：字面量对齐 contract 常量（contract.StatusCancelled/TimedOut/Failed）
	cases := []struct {
		err  error
		want string
	}{
		{context.Canceled, "cancelled"},
		{context.DeadlineExceeded, "timed_out"},
		{errors.New("boom"), "failed"},
		{wrappedDeadline{}, "timed_out"},
	}
	for _, c := range cases {
		if got := statusOf(c.err); got != c.want {
			t.Errorf("statusOf(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

// wrappedDeadline 模拟调用链里被 wrap 过的超时错（errors.Is 必须穿透）。
type wrappedDeadline struct{}

func (wrappedDeadline) Error() string { return "deadline exceeded" }
func (wrappedDeadline) Unwrap() error { return context.DeadlineExceeded }
