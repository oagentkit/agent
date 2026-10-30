package agentrun

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"

	"github.com/oagentkit/llm/llmtest"
)

// 本文件覆盖「模型用裸名调带 server 前缀的工具」这一形态的能力暴露
// （issue agent#36：实测 2026-10-30 扁鹊生产实弹 215）。
//
// 形态：
//   暴露名（模型看到的、也是 eino ToolsNode 索引的键）= `server.工具名`
//   （mcpkit v0.9.0 起 = `Info().Name`；mcpkit 的 `wireName` 另存真实调用名）；
//   模型实际请求的名字 = **裸名**（按直觉推断「工具名就是那个短名」）。
//
// 后果：eino 报 `tool ... not found in toolsNode indexes` ⇒ 该步**整步失败**
// （事件错误向上抛、链路降级；变更类工具已执行时连整步重试都被跳过）。
//
// 本仓（编排层）不拥有 `server.工具名` 的命名约定（那是 mcpkit 的契约），
// 因此本仓的职责是**把 eino 已有的两个正解暴露出来**，让下游能注入：
//   ① ToolAliases —— 声明式别名：两种写法都命中，模型不必自我纠正；
//   ② UnknownToolsHandler —— 幻觉兜底：未知工具返回可解释观察而非整步失败。

// newPrefixedTool 造一个**规范名带 server 前缀**的工具（模拟 mcpkit 暴露名）。
func newPrefixedTool(t *testing.T, canonical, desc string) tool.BaseTool {
	t.Helper()
	st, err := utils.InferTool(canonical, desc,
		func(_ context.Context, in *echoIn) (*echoOut, error) {
			return &echoOut{Echo: in.Text}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRunToolAliasesResolveBareName 别名把模型的裸名解析回规范名 ⇒ 该步不再整步失败。
//
// 这是 issue 的主诉求（方案 A）：模型发 `run_readonly_commands`，
// 而索引键是 `ask-ops.run_readonly_commands` ⇒ 注册别名后命中。
func TestRunToolAliasesResolveBareName(t *testing.T) {
	// 模型发的是**裸名**（与规范名不同）
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "echo", arguments: `{"text":"hi"}`}},
		mockResponse{content: "工具结果已消化，最终结论"},
	)

	// 规范名带 server 前缀（MCP 暴露名形态）
	prefixed := newPrefixedTool(t, "srv.echo", "回显输入")

	var toolEvents []string
	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "使用 echo 工具后给出结论", Model: cm,
		Tools:       []tool.BaseTool{prefixed},
		ToolAliases: map[string]compose.ToolAliasConfig{"srv.echo": {NameAliases: []string{"echo"}}},
	}, "调用工具", WithOnEvent(func(e Event) {
		if e.Type == EventToolCall {
			toolEvents = append(toolEvents, e.Tool)
		}
	}))
	if err != nil {
		t.Fatalf("别名命中后不应整步失败：%v", err)
	}
	if out != "工具结果已消化，最终结论" {
		t.Fatalf("out = %q", out)
	}
	if len(toolEvents) != 1 {
		t.Fatalf("应恰好 1 次工具调用，实际 %d：%v", len(toolEvents), toolEvents)
	}
}

// TestRunWithoutToolAliasesUnknownToolFails 未注册别名时**保持原行为**（整步失败）。
//
// 这条钉住「nil/空 = 零行为」的契约：暴露新能力不能悄悄改变存量路径。
func TestRunWithoutToolAliasesUnknownToolFails(t *testing.T) {
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "echo", arguments: `{"text":"hi"}`}},
		mockResponse{content: "不该走到这里"},
	)

	prefixed := newPrefixedTool(t, "srv.echo", "回显输入")

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "使用 echo 工具后给出结论", Model: cm,
		Tools: []tool.BaseTool{prefixed},
	}, "调用工具")
	if err == nil {
		t.Fatal("未注册别名且模型发裸名时应当失败（eino 找不到该工具）")
	}
	if !strings.Contains(err.Error(), "not found in toolsNode indexes") {
		t.Fatalf("err = %v，期望含 eino 的索引未命中报错", err)
	}
}

// TestRunUnknownToolsHandler 幻觉工具兜底：模型调用不存在的工具时由处理器接管，
// 返回的观察回给模型，**该步不再整步失败**。
func TestRunUnknownToolsHandler(t *testing.T) {
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "不存在的工具", arguments: `{"x":1}`}},
		mockResponse{content: "已看到兜底观察，最终结论"},
	)

	// 工具表里要有**别的**真实工具：零工具时 ADK 不建 ToolsNode，
	// 工具调用压根不会被派发 ⇒ 兜底无从触发（这不是兜底的问题，是没有节点可派发）。
	echoTool := newEchoTool(t)

	var gotName, gotInput string
	var calls atomic.Int32
	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{echoTool},
		UnknownToolsHandler: func(_ context.Context, name, input string) (string, error) {
			calls.Add(1)
			gotName, gotInput = name, input
			return "该工具不存在，请检查工具名", nil
		},
	}, "调用工具")
	if err != nil {
		t.Fatalf("兜底处理器接管后不应整步失败：%v", err)
	}
	if out != "已看到兜底观察，最终结论" {
		t.Fatalf("out = %q", out)
	}
	if calls.Load() != 1 {
		t.Fatalf("兜底处理器应被调用 1 次，实际 %d", calls.Load())
	}
	if gotName != "不存在的工具" {
		t.Fatalf("处理器收到的工具名 = %q", gotName)
	}
	if gotInput != `{"x":1}` {
		t.Fatalf("处理器收到的参数原文 = %q", gotInput)
	}
}

// TestRunUnknownToolsHandlerError 兜底处理器自己返回 error 时该步仍失败
// （处理器是「解释并降级」的缝，不是「吞掉一切」的洞）。
func TestRunUnknownToolsHandlerError(t *testing.T) {
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "不存在的工具", arguments: `{}`}},
		mockResponse{content: "不该走到这里"},
	)

	echoTool := newEchoTool(t)

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{echoTool},
		UnknownToolsHandler: func(_ context.Context, _, _ string) (string, error) {
			return "", errors.New("兜底也失败")
		},
	}, "调用工具")
	if err == nil {
		t.Fatal("处理器返回 error 时该步应当失败")
	}
}

// TestPlanExecuteToolAliases 计划-执行链路的 Executor 同样支持别名。
//
// PlanExecuteConfig 有独立的 Tools 字段与独立的 ToolsNode 构造点，
// 只改 Config 会漏掉这条路径。
func TestPlanExecuteToolAliases(t *testing.T) {
	// Planner 出计划 → Executor 先调工具（**发裸名**）再产出执行结果 → Replanner 判定完成。
	planner := &llmtest.ToolModel{Model: &llmtest.Model{RepeatLast: false, Script: []llmtest.Resp{
		planCall("执行回显"),
		respondCall("最终答复：已执行"),
	}}}
	executor := &llmtest.ToolModel{Model: &llmtest.Model{RepeatLast: false, Script: []llmtest.Resp{
		{ToolCalls: llmtest.ToolCall("echo", `{"text":"hi"}`)},
		{Content: "步骤执行完成"},
	}}}

	prefixed := newPrefixedTool(t, "srv.echo", "回显输入")

	res, err := PlanAndExecute(context.Background(), PlanExecuteConfig{
		Planner:  planner,
		Executor: executor,
		MaxSteps: 5,
		Tools:    []tool.BaseTool{prefixed},
		ToolAliases: map[string]compose.ToolAliasConfig{
			"srv.echo": {NameAliases: []string{"echo"}},
		},
	}, "目标")
	if err != nil {
		t.Fatalf("别名命中后不应失败：%v", err)
	}
	if res.Text != "最终答复：已执行" {
		t.Fatalf("Text = %q", res.Text)
	}
}
