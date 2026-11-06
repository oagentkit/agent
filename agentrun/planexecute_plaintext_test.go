package agentrun

import (
	"context"
	"strings"
	"testing"

	"github.com/oagentkit/llm/llmtest"
)

// planExecPlain 装配最小 plan-execute 三件套：planner 按脚本出 plan 与收尾响应，
// executor 恒输出一段文本。
func planExecPlain(t *testing.T, final llmtest.Resp) (*PlanExecuteResult, error) {
	t.Helper()
	planner := &llmtest.ToolModel{Model: &llmtest.Model{Script: []llmtest.Resp{
		planCall("单步"),
		final,
	}}}
	executor := &llmtest.ToolModel{Model: &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{
		{Content: "步骤执行完成"},
	}}}
	return PlanAndExecute(context.Background(), PlanExecuteConfig{
		Planner:  planner,
		Executor: executor,
		MaxSteps: 3,
	}, "目标")
}

// TestPlanAndExecutePlainTextFinal 非 JSON 信封的终稿按纯文本返回（agent#58）。
//
// 失效形态：`respond` 是**期望**的收尾方式而非唯一方式，模型直接给纯文本答复
// 是常见形态。`sawRespond` 已为真（BreakLoop 触发、任务确实完成），旧实现却因
// 嗅不到信封把整轮结果丢掉上抛「respond 信封异常」。
func TestPlanAndExecutePlainTextFinal(t *testing.T) {
	res, err := planExecPlain(t, llmtest.Resp{
		ToolCalls: llmtest.ToolCall("respond", "最终答复：纯文本，未套信封"),
	})
	if err != nil {
		t.Fatalf("纯文本终稿应视为合法，不得整轮失败: %v", err)
	}
	if res.Text != "最终答复：纯文本，未套信封" {
		t.Fatalf("Text = %q, want 原样纯文本", res.Text)
	}
}

// TestPlanAndExecuteBlankFinalIsReportedAsMissing 空终稿要报「未产出终稿」，
// 不能报成「信封异常」，也不能把空串/空白当成合法答案交出去（agent#58）。
//
// 两句话指向完全不同的排查方向：前者是模型没给答案，后者会让人去查序列化/
// 协议层。「空」有两种形态，都得覆盖：载荷全空（`{"response":""}`）与
// 载荷只有空白（`{"response":"   "}`）。
func TestPlanAndExecuteBlankFinalIsReportedAsMissing(t *testing.T) {
	for _, payload := range []string{`{"response":""}`, `{"response":"   "}`, `{"response":"\n\t "}`} {
		t.Run(payload, func(t *testing.T) {
			res, err := planExecPlain(t, llmtest.Resp{
				ToolCalls: llmtest.ToolCall("respond", payload),
			})
			if err == nil {
				t.Fatalf("空白终稿应报错，实得 Text=%q", res.Text)
			}
			if !strings.Contains(err.Error(), "未产出终稿") {
				t.Fatalf("应报「未产出终稿」，实得: %v", err)
			}
			if strings.Contains(err.Error(), "信封异常") {
				t.Fatalf("不得再报「信封异常」（会把排查引向协议层）: %v", err)
			}
		})
	}
}

// TestPlanAndExecuteEnvelopeStillUnwrapped 回归：信封形态仍解包为纯文本。
func TestPlanAndExecuteEnvelopeStillUnwrapped(t *testing.T) {
	res, err := planExecPlain(t, respondCall("最终答复：资料已汇总"))
	if err != nil {
		t.Fatalf("信封终稿应成功: %v", err)
	}
	if res.Text != "最终答复：资料已汇总" {
		t.Fatalf("Text = %q（信封应解包）", res.Text)
	}
}

// TestPlanAndExecuteWhitespaceOnlyContentIsNotAnAnswer 非信封形态的纯空白
// 内容同样是「没给答案」，不得当成终稿返回（成对对照：纯文本非空仍合法）。
func TestPlanAndExecuteWhitespaceOnlyContentIsNotAnAnswer(t *testing.T) {
	planner := &llmtest.ToolModel{Model: &llmtest.Model{Script: []llmtest.Resp{
		planCall("单步"),
		{ToolCalls: llmtest.ToolCall("respond", "   ")},
	}}}
	executor := &llmtest.ToolModel{Model: &llmtest.Model{RepeatLast: true, Script: []llmtest.Resp{
		{Content: "步骤执行完成"},
	}}}
	_, err := PlanAndExecute(context.Background(), PlanExecuteConfig{
		Planner:  planner,
		Executor: executor,
		MaxSteps: 3,
	}, "目标")
	if err == nil {
		t.Fatal("纯空白终稿应报错")
	}
	if !strings.Contains(err.Error(), "未产出终稿") {
		t.Fatalf("应报「未产出终稿」，实得: %v", err)
	}
}
