package agentrun

// 2026-11-15 issue 清零战回归测试：agent#144（MinIterationsForToolRound
// 装配期消费）与 agent#148（FinalMissingError.Iterations 报实际调用数）。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// TestConfigValidateRejectsToolRoundBudget_Issue144 配置了工具时 MaxIterations
// 低于 MinIterationsForToolRound 必须在 Validate 就报错（agent#60 的装配期防线
// 此前只有常量与注释、零消费点）。
//
// 未配工具的纯问答路径保留 MaxIterations=1 的合法性（一次调用即终稿），
// 0 = 缺省 12 不查。
func TestConfigValidateRejectsToolRoundBudget_Issue144(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{content: "x"})
	echo := newEchoTool(t)
	base := Config{Name: "test", Instruction: "inst", Model: cm}

	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			"Tools + MaxIterations=1",
			func() Config { c := base; c.Tools = []tool.BaseTool{echo}; c.MaxIterations = 1; return c }(),
			true,
		},
		{
			"ToolsFactory + MaxIterations=1",
			func() Config {
				c := base
				c.ToolsFactory = func() []tool.BaseTool { return nil }
				c.MaxIterations = 1
				return c
			}(),
			true,
		},
		{
			"Tools + MaxIterations=MinIterationsForToolRound",
			func() Config {
				c := base
				c.Tools = []tool.BaseTool{echo}
				c.MaxIterations = MinIterationsForToolRound
				return c
			}(),
			false,
		},
		{
			"无工具 + MaxIterations=1（纯问答合法）",
			func() Config { c := base; c.MaxIterations = 1; return c }(),
			false,
		},
		{
			"MaxIterations=0（缺省 12 不查）",
			base,
			false,
		},
	}
	for _, c := range cases {
		err := c.cfg.Validate()
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: 应装配期报错（agent#144）", c.name)
			} else if !strings.Contains(err.Error(), "MinIterationsForToolRound") {
				t.Errorf("%s: 报错文案应点名 MinIterationsForToolRound: %v", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: 不应报错，实得 %v", c.name, err)
		}
	}
}

// TestFinalMissingErrorReportsActualCalls_Issue148 空终稿路径经 Run 直达：
// Iterations 必须是实际模型调用数（本用例 1），而不是配置预算（缺省 12）——
// 错误文案是排障唯一入口，报预算会把「1 次调用就空答复」误导成「12 轮都没出终稿」。
func TestFinalMissingErrorReportsActualCalls_Issue148(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{content: ""})
	_, err := Run(context.Background(), Config{Name: "test", Instruction: "inst", Model: cm}, "x")
	if err == nil {
		t.Fatal("空终稿应报错")
	}
	var fme *FinalMissingError
	if !errors.As(err, &fme) {
		t.Fatalf("应为 FinalMissingError，实得 %T: %v", err, err)
	}
	if !fme.EmptyContent {
		t.Fatalf("应为空终稿形态: %+v", fme)
	}
	if fme.Iterations != 1 {
		t.Fatalf("实际 1 次模型调用，Iterations 应报 1（不是预算 12），实得 %d", fme.Iterations)
	}
	if !strings.Contains(err.Error(), "iterations=1") {
		t.Fatalf("错误文案应报实际调用数 iterations=1: %q", err.Error())
	}
	if strings.Contains(err.Error(), "iterations=12") {
		t.Fatalf("错误文案不得报配置预算 iterations=12: %q", err.Error())
	}
}

// TestFinalMissingErrorBothShapesUseModelCalls_Issue148 !sawFinal 与
// EmptyContent 两处构造共用 finalMissingError，统一报实际调用数。
func TestFinalMissingErrorBothShapesUseModelCalls_Issue148(t *testing.T) {
	h := &reactHandler{modelCalls: 3, lastReasoning: "末轮思考"}
	for _, empty := range []bool{false, true} {
		e := h.finalMissingError(empty)
		if e.Iterations != 3 {
			t.Errorf("EmptyContent=%v: Iterations = %d, want 3（实际调用数）", empty, e.Iterations)
		}
		if e.EmptyContent != empty {
			t.Errorf("EmptyContent = %v, want %v", e.EmptyContent, empty)
		}
		if e.LastReasoning != "末轮思考" {
			t.Errorf("LastReasoning 应透传: %q", e.LastReasoning)
		}
	}
}
