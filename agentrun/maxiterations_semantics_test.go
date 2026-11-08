package agentrun

// MaxIterations 的语义钉死：计的是**模型调用次数**（含终稿轮），不是工具轮数
// （agent#60）。
//
// 这组用例的作用不是「测出 bug」，而是把契约的两半各自钉成一个可判定的量：
//   - 语义：预算 1 + [工具轮, 终稿] 必然失败、预算 2 同脚本必然成功——这是
//     「1 轮工具要花 2 次调用」的算术下界，谁把 MaxIterations 理解成工具轮数
//     就必须同时改掉这两条（成对断言，不是单边）；
//   - 可区分：预算耗尽与上游故障必须能用 errors.Is 分开判——前者调大预算重跑
//     就对了，后者重试同样预算是合理的。判据是 eino 的公开哨兵
//     adk.ErrExceedMaxIterations（不是计数也不是文案匹配：计数在「末轮模型已发
//     tool_calls 而工具失败」时与预算耗尽同值——agent#88；文案是上游实现细节，
//     改一个字判据就静默失效）。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// TestMaxIterationsCountsModelCallsNotToolRounds 一轮工具调用要花 2 次模型调用。
//
// 判别式在「成对」上：只断言「预算 1 失败」是证明不了语义的——任何 bug 都能让
// 它失败；必须同时断言「同一脚本预算 2 成功」，才钉住「失败的原因是差一次终稿
// 调用」而不是别的什么。
func TestMaxIterationsCountsModelCallsNotToolRounds(t *testing.T) {
	tc := &mockToolCall{id: "c1", name: "echo", arguments: "{}"}
	echo := newEchoTool(t)

	// 预算 1：工具轮就花光了，终稿那一次被 eino 拒。
	_, cm1 := newMockOpenAI(t,
		mockResponse{toolCall: tc},  // 第 1 次调用：工具轮
		mockResponse{content: "终稿"}, // 第 2 次调用：终稿
	)
	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm1,
		Tools: []tool.BaseTool{echo}, MaxIterations: 1,
	}, "x")
	if err == nil {
		t.Fatal("MaxIterations=1 + [工具轮, 终稿] 必须失败——1 次调用出不了终稿")
	}
	if !errors.Is(err, ErrIterationBudgetExhausted) {
		t.Fatalf("应判定为预算耗尽（ErrIterationBudgetExhausted），实得 %v", err)
	}
	// 文案仍逐字透传 eino 的原错误：既有按文案判断的消费方零感知。
	if !strings.Contains(err.Error(), "iterations") && !strings.Contains(err.Error(), "iteration") {
		t.Fatalf("原始错误文案应原样透传（wrapper 只加 errors.Is 语义，不改 Error()），实得 %q", err.Error())
	}

	// 同一脚本、预算 2：工具轮 + 终稿刚好够。
	_, cm2 := newMockOpenAI(t,
		mockResponse{toolCall: tc},
		mockResponse{content: "终稿"},
	)
	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm2,
		Tools: []tool.BaseTool{echo}, MaxIterations: MinIterationsForToolRound,
	}, "x")
	if err != nil {
		t.Fatalf("MaxIterations=%d 应刚好够「一轮工具 + 终稿」，实得 %v", MinIterationsForToolRound, err)
	}
	if out != "终稿" {
		t.Fatalf("out = %q, want 终稿", out)
	}
}

// TestModelFailureIsNotBudgetExhaustion 负对照：上游故障不得被误判成预算耗尽。
//
// 没有这条，「一切错误都标成预算耗尽」实现同样能过上面那条用例——而那样判
// 反而有害：调用方会把网关故障当配置问题，去调大 MaxIterations 而不去重试/换端点。
func TestModelFailureIsNotBudgetExhaustion(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{status: 500})
	echo := newEchoTool(t)

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{echo}, MaxIterations: 3,
	}, "x")
	if err == nil {
		t.Fatal("上游 500 应报错")
	}
	if errors.Is(err, ErrIterationBudgetExhausted) {
		t.Fatalf("上游故障不是预算耗尽——误判会让调用方去调大 MaxIterations 而不是重试/换端点: %v", err)
	}
}

// TestBudgetExhaustedRequiresZeroToolRounds 纯问答（无工具）下预算 1 就够：
// 一次调用直接出终稿，不消耗第二次。
//
// 这条是上面语义的另一面：预算的含义是「模型调用次数」，所以「够不够」取决于
// 是否需要工具轮，而不是配置里那个数字本身。
func TestBudgetExhaustedRequiresZeroToolRounds(t *testing.T) {
	_, cm := newMockOpenAI(t, mockResponse{content: "直接答"})
	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm, MaxIterations: 1,
	}, "x")
	if err != nil {
		t.Fatalf("纯问答预算 1 应成功（1 次调用即终稿），实得 %v", err)
	}
	if out != "直接答" {
		t.Fatalf("out = %q", out)
	}
}

// newFailingTool 执行即失败的工具（模拟 MCP 工具超时/网关故障形态）。
func newFailingTool(t *testing.T, name string) tool.BaseTool {
	t.Helper()
	st, err := utils.InferTool(name, "执行即失败",
		func(_ context.Context, _ *echoIn) (*echoOut, error) {
			return nil, errors.New("MCP 工具超时（模拟）")
		})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestToolFailureAtBudgetIsNotBudgetExhaustion 末轮工具失败不得被误标成预算耗尽
// （agent#88）。
//
// MaxIterations=1：第 1 次（= 最后一次）模型调用返回 tool_calls、随后**工具执行
// 失败**——此时数出来的模型调用数恰好等于预算，旧判据 `modelCalls >= budget`
// 会把任何工具故障（MCP 超时/ToolsNode 报错）盖上 ErrIterationBudgetExhausted，
// 契约文档指示消费方据此「调大预算重跑」——真实根因是工具故障，调大预算重跑
// 只会重放整轮采集（成本放大）且根因被掩盖。判据必须是成因分类：eino 以公开
// 哨兵 adk.ErrExceedMaxIterations 拒绝预算外的下一次模型调用，错误链 errors.Is
// 可判；工具失败不含该哨兵，不得打标。错误文案必须保真透传真实根因。
func TestToolFailureAtBudgetIsNotBudgetExhaustion(t *testing.T) {
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "collect", arguments: `{"text":"x"}`}},
	)
	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newFailingTool(t, "collect")}, MaxIterations: 1,
	}, "x")
	if err == nil {
		t.Fatal("工具执行失败应报错")
	}
	if errors.Is(err, ErrIterationBudgetExhausted) {
		t.Fatalf("末轮工具失败不是预算耗尽——误标会让调用方调大预算重跑（重放整轮采集、掩盖根因）: %v", err)
	}
	if !strings.Contains(err.Error(), "MCP 工具超时") {
		t.Fatalf("错误文案应保真透传工具侧根因: %v", err)
	}
}
