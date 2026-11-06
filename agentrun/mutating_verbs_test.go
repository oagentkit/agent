package agentrun

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// TestMutatingVerbsCoverOutboundSideEffects 对外投递/发布类动词必须在表内
// （agent#56）。
//
// 漏判方向是**危险方向**：`IsMutatingTool` 说「不变更」⇒ 重试守卫放行整体
// 重跑 ⇒ 邮件重发 / 报告重发 / 资金重复转账。误判只是放弃一次合法重试。
func TestMutatingVerbsCoverOutboundSideEffects(t *testing.T) {
	mutating := []string{
		"send_email", "send_message",
		"post_comment", "post_webhook",
		"publish_report", "publish_release",
		"put_object", "patch_config", "upload_artifact",
		"transfer_funds", "grant_access", "revoke_token", "rotate_keys",
		"dispatch_job", "emit_event",
	}
	for _, name := range mutating {
		if !IsMutatingTool(name) {
			t.Errorf("IsMutatingTool(%q) = false，漏判会放行整体重跑 → %s 重复执行", name, name)
		}
	}
}

// TestMutatingVerbsStillAvoidSubstringFalsePositives 负对照：补动词不得让
// 「分段精确匹配」退化成子串匹配。
//
// 这是本次改动最容易犯的错：把 send/post/put 加进去很容易顺手改成
// strings.Contains，那样 use_skill（含 kill）、get_running_config（含 run）
// 会重新被误判——而它们的误判正是 agent#45 花两轮生产实弹换来的。
func TestMutatingVerbsStillAvoidSubstringFalsePositives(t *testing.T) {
	nonMutating := []string{
		"use_skill",              // 子串含 kill
		"get_running_config",     // 子串含 run
		"read_file",              // 无动词段
		"list_posts",             // 子串含 post 但分段是 posts
		"get_input_schema",       // 子串含 put 但分段是 input
		"search_sentences",       // 子串含 send 但分段是 sentences
		"describe_grants_policy", // 子串含 grant 但分段是 grants
	}
	for _, name := range nonMutating {
		if IsMutatingTool(name) {
			t.Errorf("IsMutatingTool(%q) = true，分段精确匹配已退化成子串匹配", name)
		}
	}
}

// TestMutatingVerbsAreLowercase 词表本身必须全小写：IsMutatingTool 先 ToLower
// 名字再查表，表里若混入大写项，那条动词永远不会命中（静默失效）。
func TestMutatingVerbsAreLowercase(t *testing.T) {
	for v := range MutatingVerbs {
		if v != lower(v) {
			t.Errorf("MutatingVerbs 含非小写项 %q——ToLower 后永不命中，等于没加", v)
		}
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// newNamedEchoTool 构造任意工具名的可调用工具（动词表用例要按名字触发判定）。
func newNamedEchoTool(t *testing.T, name string) tool.BaseTool {
	t.Helper()
	st, err := utils.InferTool(name, "执行",
		func(_ context.Context, in *echoIn) (*echoOut, error) {
			return &echoOut{Echo: in.Text}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRunWithRetrySkipsAfterOutboundVerb 端到端：此前漏判的动词现在能挡住整体重跑
// （agent#56）。
//
// 这是 issue 报的真实危害面：`send_email` 不在动词表 ⇒ 判「非变更类」⇒ 重试守卫
// 放行整体重跑 ⇒ **邮件重发**。逐条断言「不发起第二次运行」是唯一能证明守卫真的
// 拦住的证据——只断言「返回了错误」抓不到「错了但重试照跑」。
func TestRunWithRetrySkipsAfterOutboundVerb(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "send_email", arguments: `{"text":"账单已发"}`}},
		mockResponse{content: ""}, // 首轮空答复 ⇒ 首轮失败
	)

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newNamedEchoTool(t, "send_email")},
	}, "发账单邮件", WithRetry("带反馈重跑"))
	if err == nil {
		t.Fatal("首轮空答复应失败")
	}
	if !strings.Contains(err.Error(), "跳过重试") || !strings.Contains(err.Error(), "send_email") {
		t.Fatalf("应触发副作用守卫并点名 send_email（漏判就是邮件重发）: %v", err)
	}
	// 2 次请求 = 首轮的工具轮 + 答复轮；出现第 3 次即证明守卫没拦住
	if n := m.calls.Load(); n != 2 {
		t.Fatalf("守卫生效不应发起第二次运行: calls = %d", n)
	}
}

// TestRunWithRetryKeepsReadOnlyToolRetryable 负对照：真正只读的工具仍照常重试。
//
// 补动词最容易造成的可接受损失是「多判了几个变更类」；但如果把判定做坏成
// 「一律算变更」，重试能力整体消失，而上面那些用例照样绿。
func TestRunWithRetryKeepsReadOnlyToolRetryable(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "list_posts", arguments: `{"text":"x"}`}},
		mockResponse{content: ""},
		mockResponse{content: "重试成功"},
	)

	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newNamedEchoTool(t, "list_posts")}, // 分段 posts ≠ post
	}, "列一下", WithRetry("带反馈重跑"))
	if err != nil || out != "重试成功" {
		t.Fatalf("只读工具应照常重试成功: %v %q", err, out)
	}
	if n := m.calls.Load(); n < 3 {
		t.Fatalf("应发起第二次运行: calls = %d", n)
	}
}
