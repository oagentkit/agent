package agentrun

// WithRetry 的 Retry-After 采信门控（agent#91）：服务端退避建议只对「本次失败
// 确为限流」采信——sink 不随尝试清除（httpjson 契约明言门控归调用方重试策略），
// 而 sink 每 run 仅一份：FailoverModel 吸收链节的中途 429 切备端点后运行继续，
// 末次失败往往与限流无关（空终稿/工具故障）。不分成因的采信有两种恶果：
//   - 建议超钳制（>5min，SelectRetryDelay 返回 0）→ 直接返回首轮错误，重试被
//     陈旧建议**吞掉**；
//   - 建议未超钳制 → 先白等一轮限流窗口再重跑一个与限流无关的失败。
//
// 用例经真实传输层驱动 sink 写入（llm provider CaptureRetryAfter → httpjson
// RetryAfterTransport → 429 响应回写 ctx sink），不 mock sink 内部——门控读的
// 就是消费方运行时的那份数据。

import (
	"context"
	"testing"
	"time"

	"github.com/oagentkit/llm"
)

// newRateLimitedProvider 恒 429（可带 Retry-After 头）的模型端点，经 llm provider
// 挂 CaptureRetryAfter 传输层——429 的服务端建议经此真实写进 ctx sink。
func newRateLimitedProvider(t *testing.T, retryAfter string) (*mockOpenAI, *llm.OpenAIProvider) {
	t.Helper()
	m, _ := newMockOpenAI(t, mockResponse{status: 429, retryAfter: retryAfter})
	p, err := llm.NewOpenAIProvider(context.Background(), llm.OpenAIProviderConfig{
		BaseURL:           m.srv.URL + "/v1",
		APIKey:            "k",
		Model:             "mock",
		CaptureRetryAfter: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, p
}

// TestWithRetryIgnoresStaleRetryAfterForNonRateLimitFailure 主场景（agent#91 失败
// 推演）：主端点 429 且 Retry-After: 700（超 5min 钳制）→ FailoverModel 吸收故障
// 切备端点 → 备端点首轮空答复（FinalMissingError，与限流无关）→ 重试必须照常进行，
// 不得被陈旧建议吞掉，也不得先白等 700s。
//
// 判别式用 ctx 超时钉死而非计时器：修法错误时（不分成因采信）hint=700s 走
// SelectRetryDelay=0 分支直接返回首轮错误；若改成「未超钳制才吞」的变体则会卡进
// time.After(700s) 撞 ctx 截止返回 ctx.Err——两种坏形态都过不了「重试轮出终稿」
// 这条断言。
func TestWithRetryIgnoresStaleRetryAfterForNonRateLimitFailure(t *testing.T) {
	pMock, p := newRateLimitedProvider(t, "700")
	_, backup := newMockOpenAI(t,
		mockResponse{content: ""},   // 首轮：空终稿 → FinalMissingError（与限流无关）
		mockResponse{content: "终稿"}, // 重试轮
	)
	fm := llm.NewFailoverModel(
		llm.ChainLink{Model: p.Model(), Name: "main"},
		llm.ChainLink{Model: backup, Name: "backup"},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := Run(ctx, Config{
		Name: "test", Instruction: "inst", Model: fm, MaxIterations: 4,
	}, "分析", WithRetry("带反馈重跑"))
	if err != nil {
		t.Fatalf("非限流失败的重试不得被陈旧 Retry-After 建议（700s>5min 钳制）吞掉: %v", err)
	}
	if out != "终稿" {
		t.Fatalf("out = %q, want 终稿", out)
	}
	// sink 确实被主端点的 429 写入过（否则本用例没踩到门控分支，测了个寂寞）：
	// 主端点应恰好被两轮各打了一次。
	if n := pMock.calls.Load(); n != 2 {
		t.Fatalf("主端点应被两轮各请求一次（sink 由其 429 写入）: calls = %d", n)
	}
}

// TestWithRetryStillSkipsRerunOnHugeRateLimitHint 正向保留：本次失败**确为限流**
// 且建议超钳制（700s > 5min）——超长限流窗口原地重跑没有意义，不重试（既有行为，
// agent#91 门控不得把这条也放开）。
func TestWithRetryStillSkipsRerunOnHugeRateLimitHint(t *testing.T) {
	m, p := newRateLimitedProvider(t, "700")

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: p.Model(), MaxIterations: 4,
	}, "分析", WithRetry("带反馈重跑"))
	if err == nil {
		t.Fatal("限流失败（建议超钳制）应如实上抛")
	}
	if n := m.calls.Load(); n != 1 {
		t.Fatalf("建议超钳制时不原地重跑: calls = %d, want 1", n)
	}
}

// TestWithRetryWaitsServerHintOnRateLimitFailure 正向保留：本次失败确为限流且建议
// 未超钳制（1s ≤ 5min）——采信建议等待后重试成功（既有行为不回退）。
func TestWithRetryWaitsServerHintOnRateLimitFailure(t *testing.T) {
	m, _ := newMockOpenAI(t,
		mockResponse{status: 429, retryAfter: "1"}, // 首轮：限流，带 1s 建议
		mockResponse{content: "终稿"},                // 重试轮
	)
	p, err := llm.NewOpenAIProvider(context.Background(), llm.OpenAIProviderConfig{
		BaseURL: m.srv.URL + "/v1", APIKey: "k", Model: "mock", CaptureRetryAfter: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: p.Model(), MaxIterations: 4,
	}, "分析", WithRetry("带反馈重跑"))
	if err != nil || out != "终稿" {
		t.Fatalf("限流失败应按服务端建议等待后重试成功: %v %q", err, out)
	}
	// 等待确实发生（服务端建议优先于立即重跑）；下界断言对时钟粒度留余量。
	if el := time.Since(start); el < 900*time.Millisecond {
		t.Fatalf("应采信 Retry-After: 1s 等待后重试，实际耗时 %v", el)
	}
	if n := m.calls.Load(); n < 2 {
		t.Fatalf("应发起重试: calls = %d", n)
	}
}
