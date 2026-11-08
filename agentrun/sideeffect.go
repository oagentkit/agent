// 副作用感知的重试守卫：变更类（非幂等）工具一旦执行，整体重跑 = 重复副作用
// （脚本执行/服务操作类工具在首轮已真机生效）。重试入口据此跳过整体重试，
// 宁可如实失败交上层降级/人工，不做「重跑一次赌成功」。
//
// 变更类判定是保守的动词段启发式：按下划线分段**精确**匹配动词表——子串匹配会把
// use_skill（含 kill）、get_running_config（含 run）误判成变更类，白白放弃合法重试；
// 偏保守原则保留在词表侧——拿不准的执行动词都收进来。
package agentrun

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MutatingVerbs 变更类工具动词段表（可扩展：调用方按领域补充后 IsMutatingTool 即生效；
// 并发读取安全——构建期扩展、运行期只读）。
var MutatingVerbs = map[string]bool{
	"exec": true, "execute": true, "run": true, "restart": true, "kill": true,
	"reboot": true, "shutdown": true, "halt": true, "start": true, "stop": true,
	"install": true, "remove": true, "delete": true, "del": true, "create": true,
	"write": true, "set": true, "update": true, "apply": true, "deploy": true,
	"enable": true, "disable": true, "flush": true, "truncate": true, "mount": true,
	"umount": true, "unmount": true, "killall": true, "chmod": true, "chown": true,
	// 对外投递/发布类（agent#56）：这些动词的共同点是**执行即产生不可撤回的外部
	// 效果**——邮件已发出、报告已公开、资金已转账、密钥已轮换。首轮失败后整体
	// 重跑不是「再试一次」，是「再做一次」。
	"send": true, "post": true, "publish": true, "emit": true, "dispatch": true,
	// 写入/覆写类：REST 动词与对象存储命名法（put_object / patch_config）
	"put": true, "patch": true, "upload": true,
	// 权限与凭据变更：授权/转账/轮换/吊销都改的是「谁能做什么」
	"grant": true, "revoke": true, "transfer": true, "rotate": true,
}

// IsMutatingTool 工具名是否变更类（非幂等）：小写后按分隔符分段精确匹配动词表。
//
// 分隔符全集 = 下划线 + 点 + 连字符（agent#90）：本仓的规范名契约是带前缀形态
// （`server.工具名`，MCP 场景，见 Config.ToolAliases），模型可能按规范名调用
// （`srv.send_email`）也可能按裸名调用（`send_email`）。只按 `_` 分段时规范名的
// 动词段与 server 前缀被 `.` 黏连（`srv.send_email` → [srv.send email]），动词段
// 永不落单，守卫对整类规范名恒 false——副作用守卫的有效性由「模型这次发的是
// 裸名还是规范名」这一不可控变量决定，漏判方向是邮件重发/资金重复转账。连字符
// 一并算分隔符：server 名自身可含连字符（`ask-ops.run_readonly_commands`），
// 「剥首个 . 或 - 前的前缀」式修法会被它截错位；规范化则对任意组合成立，且
// `send-email` 类连字符裸名也一并覆盖。
//
// ⚠️ 词表漏判是**危险方向**（agent#56）：漏判 ⇒ 放行整体重跑 ⇒ 副作用重复执行
// （邮件重发/报告重发/资金重复转账）；误判只是放弃一次合法重试，代价小得多。
// 所以拿不准的动词一律收进来，宁可在跟踪侧用 NonMutatingTools / MutabilityClassifier
// 校正回来。已知的误判倾向：`post_process` / `get_commit` 这类「动词恰好撞上
// 写动词的读操作」会命中——它们属于「误判方向」，按上面的取舍接受。
//
// ⚠️ 启发式边界（oagentkit/agent#45，bianque 两轮生产实弹）：动词段匹配对
// 「run_readonly_commands」类命名会误判（"run" 段命中）——名字启发式在工具名
// 自由命名的世界里漏判/误判都是必然的。名字拿不准的域外工具走显式通道：
//   - Config.NonMutatingTools 豁免集（MCP ReadOnlyHint / curated 白名单为据）；
//   - Config.MutabilityClassifier 整体接管判定（显式声明优先于一切启发式）。
func IsMutatingTool(name string) bool {
	for _, seg := range splitToolName(name) {
		if MutatingVerbs[seg] {
			return true
		}
	}
	return false
}

// splitToolName 工具名分段：小写后按下划线/点/连字符切分（连续分隔符不产空段）。
// 分段是**精确**匹配的前提——子串匹配会把 use_skill（含 kill）、get_running_config
// （含 run）误判成变更类（agent#45），切分只改分隔符全集、不退回子串。
func splitToolName(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '_' || r == '.' || r == '-'
	})
}

// bareToolName 剥离 `server.` 命名空间前缀后的小写裸名（无 `.` 前缀时原样小写）。
// MCP 规范名为 `server.工具名`（Config.ToolAliases 契约），模型两种写法都可能发；
// 豁免集常按裸名填充，规范名调用要剥前缀才能命中（agent#90）。
func bareToolName(name string) string {
	n := strings.ToLower(name)
	if i := strings.IndexByte(n, '.'); i >= 0 {
		return n[i+1:]
	}
	return n
}

// SideEffectTracker 观测一次 agent 运行中是否调用过变更类工具（事件回调包装器，
// 观测不截断：Wrap 后事件照常透传给原回调）。exempt 为工具名豁免集
// （Config.NonMutatingTools 注入；零值=无豁免，纯动词启发式）；
// classifier 非 nil 时整体接管判定（Config.MutabilityClassifier，显式声明
// 优先于启发式与豁免集——「名字拿不准」的域外工具走声明而非重猜）。
type SideEffectTracker struct {
	mu         sync.Mutex
	called     string                 // 首个变更类工具名；""=未调用
	exempt     map[string]bool        // 豁免集：命中即不计变更类（只读执行器命名误报的校正缝）
	classifier func(name string) bool // 显式判定（nil=启发式+豁免集）
}

// Observe 处理一个运行事件（接 OnEvent 回调）。
//
// 豁免集查找按工具名的两种形态归一比对（agent#90）：模型发的原样名、小写名、
// 以及剥掉 `server.` 前缀后的小写裸名——MCP 规范名为 `server.工具名`，模型可能
// 发规范名也可能发裸名，豁免集常按裸名填充，只做 exact/小写 exact 查找时规范名
// 调用会整类失配（守卫对已豁免的只读工具误报变更类，白白挡死合法重试）。
func (t *SideEffectTracker) Observe(ev Event) {
	if ev.Type != EventToolCall {
		return
	}
	mutating := false
	if t.classifier != nil {
		mutating = t.classifier(ev.Tool) // 显式判定整体接管（含豁免语义）
	} else {
		if t.exempt[ev.Tool] || t.exempt[strings.ToLower(ev.Tool)] || t.exempt[bareToolName(ev.Tool)] {
			return
		}
		mutating = IsMutatingTool(ev.Tool)
	}
	if mutating {
		t.mu.Lock()
		if t.called == "" {
			t.called = ev.Tool
		}
		t.mu.Unlock()
	}
}

// Wrap 包装事件回调：记录变更类工具调用并透传事件（onEvent 可为 nil）。
func (t *SideEffectTracker) Wrap(onEvent func(Event)) func(Event) {
	return func(ev Event) {
		t.Observe(ev)
		if onEvent != nil {
			onEvent(ev)
		}
	}
}

// Called 是否已调用过变更类工具。
func (t *SideEffectTracker) Called() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.called != ""
}

// MutatingTool 首个被调用的变更类工具名（未调用返回 ""）。
func (t *SideEffectTracker) MutatingTool() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.called
}

// runWithMeta run 的内部形态：额外返回首轮运行中首个被调用的变更类工具名
// （Config.NonMutatingTools 豁免集经 tracker 生效——判定输入校正，守卫语义不变）。
func runWithMeta(ctx context.Context, cfg Config, query string, onEvent func(Event)) (string, string, error) {
	tracker := SideEffectTracker{exempt: cfg.NonMutatingTools, classifier: cfg.MutabilityClassifier}
	out, err := run(ctx, cfg, query, tracker.Wrap(onEvent))
	return out, tracker.MutatingTool(), err
}

// mutationSkipErr 重试守卫触发：首轮已执行变更类工具，整体重跑会重复副作用。
func mutationSkipErr(tool string, err error) error {
	return fmt.Errorf("agentrun: 首轮已调用变更类工具 %q，跳过重试防副作用重复（确需重试设 Config.RetryAfterMutation=true）: %w", tool, err)
}
