package agentrun

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// newShellExecTool 名字含变更类动词段的测试工具（执行成功，便于让首轮在工具生效后
// 以「空最终答复」失败——真实场景里副作用已在首轮落地）。
func newShellExecTool(t *testing.T) tool.BaseTool {
	t.Helper()
	st, err := utils.InferTool("shell_exec", "执行命令",
		func(_ context.Context, in *echoIn) (*echoOut, error) {
			return &echoOut{Echo: in.Text}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestIsMutatingTool 下划线分段精确匹配：动词段命中即变更类；子串误报不发生。
func TestIsMutatingTool(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"shell_exec", true},
		{"service_restart", true},
		{"kill", true},
		{"use_skill", false},          // 子串含 kill 但分段不匹配——不误判
		{"get_running_config", false}, // 子串含 run 但分段不匹配
		{"read_file", false},
		{"KILL_ALL", true}, // 大小写归一
	}
	for _, tc := range cases {
		if got := IsMutatingTool(tc.name); got != tc.want {
			t.Errorf("IsMutatingTool(%q) = %v, 期望 %v", tc.name, got, tc.want)
		}
	}
}

// TestIsMutatingToolCanonicalNames 规范名（`server.工具名`）形态必须被守卫覆盖
// （agent#90）：模型按规范名调用时动词段此前与 server 前缀被 `.` 黏连、永不落单，
// 守卫对整类规范名恒 false——副作用重复执行只在「模型这次发的是裸名还是规范名」
// 这一不可控变量上赌运气。
//
// 判别式钉住三件事：① 规范名与裸名判定一致（守卫不随模型写法漂移）；
// ② server 名含连字符（ask-ops）不截错位——「剥首个 . 或 - 前缀」式修法会把
// `ask-ops.send_email` 截成 `ops.send_email`，动词段照样黏连；③ 分段仍是精确
// 匹配，连字符加入分隔符全集不让 use_skill 类子串误报复活（含连字符变体）。
func TestIsMutatingToolCanonicalNames(t *testing.T) {
	mutating := []string{
		"srv.send_email",                // 规范名（agent#90 主形态）
		"srv.send-email",                // 连字符裸名经规范名前缀
		"github.create_issue",           // issue 报的另一实跑形态
		"mcp_github.create_issue",       // 下划线 server 前缀
		"ask-ops.send_email",            // 连字符 server 名：前缀剥法会截错位，规范化不受影响
		"send-email",                    // 连字符裸名（此前同样漏判）
		"ask-ops.run_readonly_commands", // 与裸名 run_readonly_commands 判定一致（已知的 run* 误报方向，豁免集/classifier 校正）
	}
	for _, name := range mutating {
		if !IsMutatingTool(name) {
			t.Errorf("IsMutatingTool(%q) = false，规范名漏判会放行整体重跑 → 副作用重复执行", name)
		}
	}
	nonMutating := []string{
		"srv.use_skill",     // 子串含 kill 但分段不匹配——连字符加入后不得退回子串匹配
		"ask-ops.use-skill", // 同上，连字符变体
		"srv.list_posts",    // 分段 posts ≠ post
		"github.get_commit", // get 段非动词表项；commit 无动词段
		"srv.read-file",     // 连字符只读
		"srv.get-running-config",
	}
	for _, name := range nonMutating {
		if IsMutatingTool(name) {
			t.Errorf("IsMutatingTool(%q) = true，分段精确匹配被规范化破坏", name)
		}
	}
}

// TestSideEffectTracker 观测器：tool_call 事件置位，其余事件不动；Wrap 透传原回调。
func TestSideEffectTracker(t *testing.T) {
	var tracker SideEffectTracker
	var passed []Event
	wrapped := tracker.Wrap(func(ev Event) { passed = append(passed, ev) })

	wrapped(Event{Type: EventToolCall, Tool: "echo"})
	wrapped(Event{Type: EventToolCall, Tool: "shell_exec"})
	wrapped(Event{Type: EventToolResult, Tool: "shell_exec"})

	if !tracker.Called() {
		t.Fatal("变更类工具调用后 Called 应为 true")
	}
	if tracker.MutatingTool() != "shell_exec" {
		t.Fatalf("MutatingTool = %q，期望 shell_exec（首个变更类工具）", tracker.MutatingTool())
	}
	if len(passed) != 3 {
		t.Fatalf("事件应全量透传: %d", len(passed))
	}

	var idle SideEffectTracker
	idle.Observe(Event{Type: EventToolCall, Tool: "read_file"})
	if idle.Called() {
		t.Fatal("只读工具不应置位")
	}
}

// TestRunWithRetrySkipsAfterMutation 首轮变更类工具已生效后失败 → 不整体重跑
// （只打 1 次请求），错误说明跳过原因并包裹原始错误。
func TestRunWithRetrySkipsAfterMutation(t *testing.T) {
	// 首轮：shell_exec 真实执行（副作用落地）→ 空最终答复 → 运行失败。
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "shell_exec", arguments: `{"text":"reboot -h"}`}},
		mockResponse{content: ""},
	)

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newShellExecTool(t)},
	}, "执行重启", WithRetry("带反馈重跑"))
	if err == nil {
		t.Fatal("应失败")
	}
	if !strings.Contains(err.Error(), "跳过重试") || !strings.Contains(err.Error(), "shell_exec") {
		t.Fatalf("错误应说明副作用守卫: %v", err)
	}
	if n := m.calls.Load(); n != 2 {
		t.Fatalf("守卫生效不应发起第二次运行（2 次请求=首轮 ReAct 的工具轮+答复轮）: calls = %d", n)
	}
	if !strings.Contains(err.Error(), "未产出最终文本") && !strings.Contains(err.Error(), "最终答复为空") {
		t.Fatalf("应保留原始失败原因（%%w 可解包）: %v", err)
	}
}

// TestRunWithEventsAndRetrySkipsAfterMutation 事件回调路径同一守卫；观测事件照常全量回调。
func TestRunWithEventsAndRetrySkipsAfterMutation(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "shell_exec", arguments: `{"text":"x"}`}},
		mockResponse{content: ""},
	)

	var toolCalls []string
	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newShellExecTool(t)},
	}, "执行", WithRetry("重跑"), WithOnEvent(func(ev Event) {
		if ev.Type == EventToolCall {
			toolCalls = append(toolCalls, ev.Tool)
		}
	}))
	if err == nil || !strings.Contains(err.Error(), "跳过重试") {
		t.Fatalf("应触发副作用守卫: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0] != "shell_exec" {
		t.Fatalf("首轮工具调用事件应照常回调: %v", toolCalls)
	}
	if n := m.calls.Load(); n != 2 {
		t.Fatalf("不应发起第二次运行: calls = %d", n)
	}
}

// TestRunWithRetryAfterMutationAllowed RetryAfterMutation=true：恢复无条件重试。
func TestRunWithRetryAfterMutationAllowed(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "shell_exec", arguments: `{"text":"x"}`}},
		mockResponse{content: ""},
		mockResponse{content: "重试成功"},
	)

	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools:              []tool.BaseTool{newShellExecTool(t)},
		RetryAfterMutation: true,
	}, "执行", WithRetry("带反馈重跑"))
	if err != nil || out != "重试成功" {
		t.Fatalf("显式允许后应重试成功: %v %q", err, out)
	}
	if n := m.calls.Load(); n < 2 {
		t.Fatalf("应发起第二次运行: calls = %d", n)
	}
}

// TestRunWithRetryNonMutatingExempt NonMutatingTools 豁免集：命中动词启发式的工具名
// 被显式豁免（只读执行器命名误报校正）后，首轮调用不触发守卫，失败照常重试。
func TestRunWithRetryNonMutatingExempt(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "shell_exec", arguments: `{"text":"x"}`}},
		mockResponse{content: ""},
		mockResponse{content: "重试成功"},
	)

	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools:            []tool.BaseTool{newShellExecTool(t)},
		NonMutatingTools: map[string]bool{"shell_exec": true},
	}, "执行", WithRetry("带反馈重跑"))
	if err != nil || out != "重试成功" {
		t.Fatalf("豁免后应照常重试成功: %v %q", err, out)
	}
	if n := m.calls.Load(); n < 3 {
		t.Fatalf("应发起第二次运行: calls = %d", n)
	}
}

// TestSideEffectTrackerExempt 豁免集大小写归一；未豁免工具照常置位。
func TestSideEffectTrackerExempt(t *testing.T) {
	tracker := SideEffectTracker{exempt: map[string]bool{"run_readonly_command": true}}
	tracker.Observe(Event{Type: EventToolCall, Tool: "run_readonly_command"})
	tracker.Observe(Event{Type: EventToolCall, Tool: "RUN_READONLY_COMMAND"}) // 大小写归一
	if tracker.Called() {
		t.Fatal("豁免工具不应置位")
	}
	tracker.Observe(Event{Type: EventToolCall, Tool: "run_approved_command"})
	if !tracker.Called() || tracker.MutatingTool() != "run_approved_command" {
		t.Fatalf("未豁免变更类工具应照常置位: %q", tracker.MutatingTool())
	}
}

// TestRunWithRetryReadOnlyRetries 只读工具失败仍照常重试（守卫不误伤）。
func TestRunWithRetryReadOnlyRetries(t *testing.T) {
	_, cm := newMockOpenAI(t,
		mockResponse{content: ""},
		mockResponse{content: "重试成功"},
	)

	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
	}, "第一问", WithRetry("第二问"))
	if err != nil || out != "重试成功" {
		t.Fatalf("无变更类工具应照常重试: %v %q", err, out)
	}
}

// MutabilityClassifier 显式判定整体接管（agent#45）：run_readonly_commands 命中
// "run" 动词段被启发式误判（bianque 两轮生产实弹）——显式通道下按注解读真值，
// 优先于启发式与豁免集。
func TestMutabilityClassifierOverrides(t *testing.T) {
	// 启发式基线：误判复现（钉住问题本身）
	if !IsMutatingTool("run_readonly_commands") {
		t.Fatal("启发式误判基线：run_readonly_commands 应命中 run 段（问题复现）")
	}
	// classifier 整体接管：只读执行器判只读
	tr := SideEffectTracker{
		classifier: func(name string) bool { return name == "shell_exec" },
	}
	tr.Observe(Event{Type: EventToolCall, Tool: "run_readonly_commands"})
	if tr.MutatingTool() != "" {
		t.Fatalf("classifier 判只读不得计变更类: %q", tr.MutatingTool())
	}
	tr.Observe(Event{Type: EventToolCall, Tool: "shell_exec"})
	if tr.MutatingTool() != "shell_exec" {
		t.Fatalf("classifier 判变更应记录: %q", tr.MutatingTool())
	}
	// 接管语义：豁免集在 classifier 模式下不再参与（判定单源）
	tr2 := SideEffectTracker{
		exempt:     map[string]bool{"shell_exec": true},
		classifier: func(string) bool { return true },
	}
	tr2.Observe(Event{Type: EventToolCall, Tool: "shell_exec"})
	if tr2.MutatingTool() != "shell_exec" {
		t.Fatal("classifier 非 nil 时整体接管（豁免集不参与）")
	}
}

// TestSideEffectTrackerExemptCanonicalForms 豁免集跨形态互认（agent#90）：豁免键按
// 裸名填充、模型发规范名（`server.工具名`）时，剥前缀裸名查找要能命中——否则已
// 豁免的只读工具被误判变更类，合法重试被挡死（bianque 实弹的规范名变体）。反向
// （豁免键=规范名、模型发规范名）exact 查找本就命中；模型发裸名而豁免键只有规范名
// 时无法穷举 server 前缀，仍按失配处理（无害方向：多算变更类只是放弃重试）。
func TestSideEffectTrackerExemptCanonicalForms(t *testing.T) {
	// 豁免键=裸名，模型发规范名：剥前缀命中
	tr := SideEffectTracker{exempt: map[string]bool{"run_readonly_command": true}}
	tr.Observe(Event{Type: EventToolCall, Tool: "ask-ops.run_readonly_command"})
	if tr.Called() {
		t.Fatal("豁免键=裸名、调用=规范名：剥前缀裸名应命中豁免集")
	}
	// 豁免键=规范名，模型发规范名：exact 命中（既有行为不回退）
	tr2 := SideEffectTracker{exempt: map[string]bool{"ask-ops.run_readonly_command": true}}
	tr2.Observe(Event{Type: EventToolCall, Tool: "ask-ops.run_readonly_command"})
	if tr2.Called() {
		t.Fatal("豁免键=规范名、调用=规范名：exact 应命中豁免集")
	}
	// 未豁免的规范名变更类工具照常置位（豁免集不扩大成全放行）
	tr3 := SideEffectTracker{exempt: map[string]bool{"run_readonly_command": true}}
	tr3.Observe(Event{Type: EventToolCall, Tool: "srv.send_email"})
	if !tr3.Called() || tr3.MutatingTool() != "srv.send_email" {
		t.Fatalf("未豁免的规范名变更类工具应置位: %q", tr3.MutatingTool())
	}
}

// TestRunWithRetrySkipsAfterCanonicalNameMutation 端到端（agent#90 实跑场景）：
// 模型按**规范名** `srv.send_email` 调用——工具真实执行（副作用落地）、首轮空答复
// 失败后，副作用守卫必须挡住整体重跑。此前守卫只按 `_` 分段，规范名的动词段与
// server 前缀被 `.` 黏连（[srv.send email]）恒判非变更类 ⇒ 整体重跑 ⇒ 服务端收到
// 3 次请求、邮件重发。逐条断言「不发起第二次运行」是唯一能证明守卫真拦住的证据。
func TestRunWithRetrySkipsAfterCanonicalNameMutation(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "srv.send_email", arguments: `{"text":"账单已发"}`}},
		mockResponse{content: ""}, // 首轮空答复 ⇒ 首轮失败
	)

	_, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{newNamedEchoTool(t, "srv.send_email")},
	}, "发账单邮件", WithRetry("带反馈重跑"))
	if err == nil {
		t.Fatal("首轮空答复应失败")
	}
	if !strings.Contains(err.Error(), "跳过重试") || !strings.Contains(err.Error(), "srv.send_email") {
		t.Fatalf("规范名调用也应触发副作用守卫并点名工具: %v", err)
	}
	// 2 次请求 = 首轮的工具轮 + 答复轮；出现第 3 次即证明守卫没拦住（邮件重发）
	if n := m.calls.Load(); n != 2 {
		t.Fatalf("守卫生效不应发起第二次运行（规范名漏判=副作用重复执行）: calls = %d", n)
	}
}

// TestRunWithRetryNonMutatingExemptCanonical 规范名只读工具 + 裸名豁免键：豁免生效
// 后失败照常重试（守卫对规范名形态不误伤）。
func TestRunWithRetryNonMutatingExemptCanonical(t *testing.T) {
	m, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c1", name: "ask-ops.run_readonly_commands", arguments: `{"text":"x"}`}},
		mockResponse{content: ""},
		mockResponse{content: "重试成功"},
	)

	out, err := Run(context.Background(), Config{
		Name: "test", Instruction: "inst", Model: cm,
		Tools:            []tool.BaseTool{newNamedEchoTool(t, "ask-ops.run_readonly_commands")},
		NonMutatingTools: map[string]bool{"run_readonly_commands": true}, // 裸名豁免键
	}, "采集", WithRetry("带反馈重跑"))
	if err != nil || out != "重试成功" {
		t.Fatalf("规范名只读工具豁免后应照常重试成功: %v %q", err, out)
	}
	if n := m.calls.Load(); n < 3 {
		t.Fatalf("应发起第二次运行: calls = %d", n)
	}
}
