package agentrun

// 三行为契约测试——中断→继续 / 用户介入→恢复 / 派发子 Agent 失败→回父
// （oagentkit/agent#176，行为契约的消费方半边）。
//
// # 两仓各跑一半（本单与 yi-nology/goagent-kernel#523 的分工，不重叠）
//
// kernel#523 是**机制半边**：内核自身链条（创建→领取→…→终态）的组合正确性，
// 故障注入矩阵钉住 checkpoint 单调闸、对账三态、终态落账——「机制自己对不对」。
// 本文件是**消费方半边**：agent 层触发中断/介入/派发失败时，机制面接得住、
// 终态落得下、恢复不重复副作用——「消费方拿机制拼起来对不对」。两侧判据同形
// （完成台账、哨兵身份、终态词表）但各测各的面：kernel 有故障注入矩阵，这里
// 只有 agent 层可观察的行为（事件面、错误分类、状态词表、守卫拒绝）。
//
// # contract mock 的边界（tier3 不 import kernel，分层红线）
//
// 本仓不 import yi-nology/goagent-kernel，机制语义用 contract mock 钉住——
// mock 只复刻**消费方要依赖的两条语义**，不复刻机制实现：
//   - 完成台账可读 + 落账序号严格递增（回退拒绝）——对形 kernel checkpoint
//     Snapshot/Commit/ErrStaleSeq（kernel checkpoint/cas.go 单调闸语义）；
//   - 用户取消唯一哨兵按值认身份、同文案复制件不认领——对形 kernel
//     contract.ErrUserCancelled（#333 同名哨兵分叉事故的教训：身份是值不是文案）。
// 跨仓「两侧同形」的对账由 goagent-runtime/bridge/agentiface 适配器测试兜
// （那仓两头都能 import，kernel#524 出口条件 5 已留实跑证据）。

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/oagentkit/agent/dispatch"
	"github.com/oagentkit/agent/iface"
	"github.com/oagentkit/agent/roundguard"
)

// ---------- contract mock：完成台账 + 单调闸（对形 kernel checkpoint） ----------

// errMockStaleSeq 落账序号回退拒绝（对形 kernel checkpoint.ErrStaleSeq
// 「快照序号未严格递增，被单调闸拒绝」——恢复点不是回退，被拒的重试永久拒）。
var errMockStaleSeq = errors.New("mockCheckpoint: 落账序号回退（严格递增被拒）")

// mockCheckpoint 消费方半边的 checkpoint contract mock。
//
// 台账语义与 agentloop 消费 kernel checkpoint 的形状对齐（每步完成后落账、
// 恢复时读台账跳过已完成）：done 记「已可信完成」的步骤集，Commit 序号
// 严格递增。机制半边（跨进程 CAS、故障注入恢复）在 kernel checkpoint 包，
// 本 mock 不承担。
type mockCheckpoint struct {
	mu   sync.Mutex
	seq  int
	done map[string]bool
}

func newMockCheckpoint() *mockCheckpoint {
	return &mockCheckpoint{done: map[string]bool{}}
}

// Commit 落账：seq 必须严格递增（对形 SeqGate）；回退/重复序号拒绝且不写入。
func (c *mockCheckpoint) Commit(seq int, steps ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if seq <= c.seq {
		return errMockStaleSeq
	}
	c.seq = seq
	for _, s := range steps {
		c.done[s] = true
	}
	return nil
}

func (c *mockCheckpoint) IsDone(step string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done[step]
}

// Done 台账快照（排序副本——消费方对账用，不能反向改台账）。
func (c *mockCheckpoint) Done() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.done))
	for s := range c.done {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// stepCounter 步骤执行计数：证明「已完成步骤不重复执行」的硬事实面。
type stepCounter struct {
	mu sync.Mutex
	n  map[string]int
}

func newStepCounter() *stepCounter { return &stepCounter{n: map[string]int{}} }

func (c *stepCounter) hit(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n[name]++
}

func (c *stepCounter) get(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[name]
}

type stepIn struct {
	Text string `json:"text" jsonschema:"description=参数"`
}

type stepOut struct {
	Step string `json:"step"`
}

// newStepTool 契约测试步骤工具：台账已记完成 ⇒ 重放结果不落执行计数
// （恢复路径），未完成 ⇒ 真执行并计数（中断前路径）。这就是消费方半边
// 「恢复后从断点继续、不重复执行已完成步骤」的执行体语义——与 agentloop
// 侧 effects 台账挡重放同形（机制在 kernel，这里钉消费方组合行为）。
func newStepTool(t *testing.T, name string, ckpt *mockCheckpoint, runs *stepCounter) tool.BaseTool {
	t.Helper()
	st, err := utils.InferTool(name, "契约测试步骤工具",
		func(_ context.Context, _ *stepIn) (*stepOut, error) {
			if ckpt.IsDone(name) {
				return &stepOut{Step: name + ":replayed"}, nil
			}
			runs.hit(name)
			return &stepOut{Step: name}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// ---------- 行为一：中断→继续（预算断流后恢复不重放已完成步骤） ----------

// TestContractInterruptResumeSkipsCompletedSteps 中断→继续：
// 预算断流（ErrIterationBudgetExhausted）打断在途执行后，已完成步骤经
// 事件面对账落进台账；恢复跑从断点继续——已完成步骤重放不执行、未完成
// 步骤真执行，整链每步至多执行一次。
//
// 断点事实面钉的是 agentrun 的事件契约：tool_call/tool_result 按 CallID 配对
// （Event.CallID），恢复驱动只能从 tool_result 集推「哪些步骤已可信完成」——
// 事件面丢配对 ⇒ 台账无从落起，这就是本行为对 agent 层的可观察要求。
func TestContractInterruptResumeSkipsCompletedSteps(t *testing.T) {
	ckpt := newMockCheckpoint()
	runs := newStepCounter()

	// ---- 进程 1：执行 step_a、step_b 后预算断流 ----
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c-a", name: "step_a", arguments: `{"text":"a"}`}},
		mockResponse{toolCall: &mockToolCall{id: "c-b", name: "step_b", arguments: `{"text":"b"}`}},
		mockResponse{toolCall: &mockToolCall{id: "c-c", name: "step_c", arguments: `{"text":"c"}`}},
	)
	tools := []tool.BaseTool{
		newStepTool(t, "step_a", ckpt, runs),
		newStepTool(t, "step_b", ckpt, runs),
		newStepTool(t, "step_c", ckpt, runs),
	}
	var completed []string
	var seenCallIDs = map[string]bool{}
	_, err := Run(context.Background(), Config{
		Name: "worker", Instruction: "inst", Model: cm, Tools: tools,
		MaxIterations: 2, DisableStreaming: true,
	}, "长任务", WithOnEvent(func(e Event) {
		if e.Type == EventToolResult && e.CallID != "" {
			// 断点事实面：tool_result 到达 ⇒ 该步骤已执行完（配对键 CallID）
			if !seenCallIDs[e.CallID] {
				seenCallIDs[e.CallID] = true
				completed = append(completed, e.Tool)
			}
		}
	}))
	if !errors.Is(err, ErrIterationBudgetExhausted) {
		t.Fatalf("预算断流应报 ErrIterationBudgetExhausted，实得 %v", err)
	}
	if runs.get("step_a") != 1 || runs.get("step_b") != 1 {
		t.Fatalf("中断前应恰好各执行一次，实得 a=%d b=%d", runs.get("step_a"), runs.get("step_b"))
	}
	if runs.get("step_c") != 0 {
		t.Fatalf("step_c 未到断点不得执行，实得 %d", runs.get("step_c"))
	}

	// 断点落账（恢复驱动职责）：已完成步骤进台账，序号单调
	if err := ckpt.Commit(1, completed...); err != nil {
		t.Fatalf("落账失败: %v", err)
	}
	if got := ckpt.Done(); len(got) != 2 || got[0] != "step_a" || got[1] != "step_b" {
		t.Fatalf("台账应为 [step_a step_b]，实得 %v", got)
	}
	// 恢复点不是回退：重复/回退序号被拒（对形 ErrStaleSeq——被拒的重试永久拒）
	if err := ckpt.Commit(1, "step_c"); !errors.Is(err, errMockStaleSeq) {
		t.Fatalf("序号不递增应拒，实得 %v", err)
	}

	// ---- 进程 2：共享台账恢复——模型按历史重放同一调用，台账挡下重复执行 ----
	_, cm2 := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c-a", name: "step_a", arguments: `{"text":"a"}`}},
		mockResponse{toolCall: &mockToolCall{id: "c-b", name: "step_b", arguments: `{"text":"b"}`}},
		mockResponse{toolCall: &mockToolCall{id: "c-c", name: "step_c", arguments: `{"text":"c"}`}},
		mockResponse{content: "长任务完成"},
	)
	tools2 := []tool.BaseTool{
		newStepTool(t, "step_a", ckpt, runs),
		newStepTool(t, "step_b", ckpt, runs),
		newStepTool(t, "step_c", ckpt, runs),
	}
	out, err := Run(context.Background(), Config{
		Name: "worker", Instruction: "inst", Model: cm2, Tools: tools2,
		MaxIterations: 4, DisableStreaming: true,
	}, "继续")
	if err != nil {
		t.Fatalf("恢复跑失败: %v", err)
	}
	if out != "长任务完成" {
		t.Fatalf("终答 = %q", out)
	}
	// 至多一次：已完成步骤恢复后不得再执行，未完成步骤恰好补上
	if runs.get("step_a") != 1 || runs.get("step_b") != 1 || runs.get("step_c") != 1 {
		t.Fatalf("整链每步至多执行一次，实得 a=%d b=%d c=%d",
			runs.get("step_a"), runs.get("step_b"), runs.get("step_c"))
	}
	if err := ckpt.Commit(2, "step_c"); err != nil {
		t.Fatalf("恢复后补账应过（序号递增）: %v", err)
	}
}

// ---------- 行为二：用户介入→恢复（落终态，不被误判成停机） ----------

// errUserCancelled 用户取消唯一哨兵（contract mock，对形 kernel
// contract.ErrUserCancelled「全仓唯一哨兵」的消费方语义）。
var errUserCancelled = errors.New("agentrun contract mock: 用户取消")

// TestContractUserCancelLandsCancelledNotShutdown 用户介入→恢复：
// context.WithCancelCause 装因、cancel(errUserCancelled) 携带唯一哨兵的形态
// 介入后，执行落**非空终态 cancelled**——不被误判成进程停机。
//
// 回归形态对标 kernel #333（同名哨兵分叉事故）：超时/取消 cause 落到收尾侧
// 兜底臂 ⇒ 判成停机 ⇒ Status() 返回空串 ⇒ 会话永久滞留 running。消费方半边
// 的判据有三条：
//  1. 终态必须是非空词表值（空串 = 滞留 running 的失败形态，直接判红）；
//  2. 身份按值不按文案：cause 是哨兵本值，同文案复制件 errors.Is 不认领；
//  3. 回合级走法不误判成故障：roundguard.Classify(Aborted) ⇒ ReasonAborted，
//     不是 ReasonFailed（中断后走续跑收口，不走失败兜底）。
func TestContractUserCancelLandsCancelledNotShutdown(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(errUserCancelled)

	// 介入发生在工具执行中途：cancel 后循环收场，下一跳模型调用吃到 ctx 错
	poke, err := utils.InferTool("poke", "触发用户介入",
		func(_ context.Context, _ *stepIn) (*stepOut, error) {
			cancel(errUserCancelled) // 用户介入：专属 cause 装进取消（唯一哨兵身份）
			return &stepOut{Step: "poked"}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	_, cm := newMockOpenAI(t,
		mockResponse{toolCall: &mockToolCall{id: "c-1", name: "poke", arguments: `{"text":"x"}`}},
		mockResponse{content: "不应到达"},
	)
	a := &Agent{Cfg: Config{
		Name: "worker", Instruction: "inst", Model: cm,
		Tools: []tool.BaseTool{poke}, MaxIterations: 3, DisableStreaming: true,
	}}
	got, runErr := a.Run(ctx, iface.Input{Query: "任务"})
	if runErr == nil {
		t.Fatal("用户介入后 Run 应报错返回")
	}

	// 判据 1：非空终态，且是 cancelled——#333 形态（空串=滞留 running）在此判红
	if got.Status == "" {
		t.Fatal("终态为空串 = 会话滞留 running（kernel #333 形态），必须落词表终态")
	}
	if got.Status != statusCancelled {
		t.Fatalf("用户介入应落 cancelled，实得 %q（误判成停机/故障/超时都是错路由）", got.Status)
	}
	if errors.Is(runErr, ErrIterationBudgetExhausted) {
		t.Fatal("用户介入不得被归成预算耗尽（处置完全不同：一个是调预算，一个是等人）")
	}

	// 判据 2：哨兵身份按值认（同文案复制件不认领——#333 同名哨兵分叉的教训）
	cause := context.Cause(ctx)
	if !errors.Is(cause, errUserCancelled) {
		t.Fatalf("cause 应认领唯一哨兵，实得 %v", cause)
	}
	textCopy := errors.New(errUserCancelled.Error()) // 同文案、不同值
	if errors.Is(cause, textCopy) {
		t.Fatal("同文案复制件被认领——身份退化成按文案比对，#333 分叉会重演")
	}

	// 判据 3：回合级分类走中断臂，不走故障臂
	route := roundguard.Classify(roundguard.Outcome{Aborted: true})
	if route.Reason != roundguard.ReasonAborted {
		t.Fatalf("Aborted 应归 ReasonAborted，实得 %s", route.Reason)
	}
	if route.Continue {
		t.Fatal("中断回合不得继续（收口语义）")
	}
	// 取消与超时是两条处置（调用方看 cause 走不同恢复策略），词表必须分开
	if statusOf(fmt.Errorf("wrap: %w", context.DeadlineExceeded)) != statusTimedOut {
		t.Fatal("超时应落 timed_out——与 cancelled 串档会让恢复策略选错")
	}
}

// ---------- 行为三：派发子 Agent 失败→回父（父侧落终态，不留 running 残留） ----------

// fakeEdges 测试用拓扑边源（EdgeSource 最小实现）。
type fakeEdges map[string][]string

func (e fakeEdges) Slugs() []string {
	out := make([]string, 0, len(e))
	for s := range e {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func (e fakeEdges) DispatchAllow(slug string) []string { return e[slug] }

// failAgent 恒失败的子 Agent（iface.Agent 测试替身）。
type failAgent struct{ err error }

func (f failAgent) Run(context.Context, iface.Input) (iface.Result, error) {
	return iface.Result{Status: statusFailed}, f.err
}

// parentDriver 父侧驱动：派发/子跑期间 state=running，任何出口都必须落
// 词表终态。「running 残留」= Run 返回后 state 还停在 running/空串——那正是
// 滞留 running 的消费方形态，每条出口逐一判红。
type parentDriver struct {
	state string
}

func (p *parentDriver) dispatchTo(g *dispatch.Guard, callee string, depth int, sub iface.Agent) (string, error) {
	p.state = "running"
	if err := g.Assert("parent", callee, depth); err != nil {
		p.state = statusOf(err)
		return p.state, err
	}
	res, err := sub.Run(context.Background(), iface.Input{Query: "子任务"})
	if err != nil {
		p.state = statusOf(err)
		return p.state, err
	}
	p.state = res.Status
	return p.state, nil
}

// TestContractDispatchFailureReturnsToParentTerminal 派发子 Agent 失败→回父：
// 守卫拒绝（结构化 DenyError，按失败兜底如实上报不得转述降级）与子 Agent
// 失败两条路径，父侧都落终态、不留 running 残留。
func TestContractDispatchFailureReturnsToParentTerminal(t *testing.T) {
	g := dispatch.NewGuard(fakeEdges{"parent": {"sub_ok"}}, 2)

	// 路径 A：矩阵外派发被拒——DenyError 结构化身份 + Reason 常量（审计按常量比对）
	p := &parentDriver{}
	st, err := p.dispatchTo(g, "sub_denied", 1, failAgent{err: errors.New("不达")})
	if err == nil {
		t.Fatal("矩阵外派发应被拒绝")
	}
	var de *dispatch.DenyError
	if !errors.As(err, &de) {
		t.Fatalf("拒绝必须是结构化 *DenyError（不得转述降级），实得 %T", err)
	}
	if de.Reason != dispatch.ReasonNotAllowed {
		t.Fatalf("Reason = %s, want %s", de.Reason, dispatch.ReasonNotAllowed)
	}
	if st == "" || st == "running" {
		t.Fatalf("拒绝后父侧必须落终态，实得 %q", st)
	}

	// 路径 B：深度超限拒绝（递归派发上限的消费方读面）
	p2 := &parentDriver{}
	_, err = p2.dispatchTo(g, "sub_ok", 3, failAgent{err: errors.New("不达")})
	if !errors.As(err, &de) || de.Reason != dispatch.ReasonDepthExceeded {
		t.Fatalf("深度超限应为 DenyError/depth_exceeded，实得 %v", err)
	}
	if p2.state == "" || p2.state == "running" {
		t.Fatalf("深度拒绝后父侧必须落终态，实得 %q", p2.state)
	}

	// 路径 C：子 Agent 失败回到父——父侧落 failed，running 不残留
	p3 := &parentDriver{}
	st3, err := p3.dispatchTo(g, "sub_ok", 1, failAgent{err: errors.New("子任务炸了")})
	if err == nil {
		t.Fatal("子 Agent 失败应回传父侧")
	}
	if st3 != statusFailed {
		t.Fatalf("子失败父侧应落 failed，实得 %q", st3)
	}
	if p3.state == "" || p3.state == "running" {
		t.Fatalf("running 残留：%q", p3.state)
	}

	// 路径 D：子 Agent 失败经回合分类也是失败臂（不误判成中断/交接）
	if r := roundguard.Classify(roundguard.Outcome{Failed: err}); r.Reason != roundguard.ReasonFailed {
		t.Fatalf("子失败应归 ReasonFailed，实得 %s", r.Reason)
	}
}
