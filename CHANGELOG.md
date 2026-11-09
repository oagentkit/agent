# agent CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.14.6] — 2026-11-09

### 修复：agent 第三轮 issue 清零（#88 #89 #90 #91）

* fix(agentrun): ErrIterationBudgetExhausted 改成因分类判据——末轮工具失败不再误标为预算耗尽（#88）
* fix(dispatch): NewGuard 对 maxDepth<=0 构造期 panic——深度漏配装配期暴露而非运行期静默全拒（#89）
* fix(agentrun): IsMutatingTool 分段覆盖规范名形态（server.工具名），豁免集跨形态互认（#90）
* fix(agentrun): WithRetry 仅限流失败采信 Retry-After——陈旧建议不得吞掉非限流失败的重试（#91）

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.40.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.39.0` | `v0.40.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/textx v0.10.5

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/textx` | `v0.10.4` | `v0.10.5` |
| `github.com/oekit/concurrency` | `v0.30.1` | `v0.31.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.21.5

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/llm` | `v0.21.4` | `v0.21.5` |
| `github.com/oagentkit/textx` | `v0.10.3` | `v0.10.4` |
| `github.com/oekit/concurrency` | `v0.29.0` | `v0.30.1` |
| `github.com/oekit/logx` | `v0.8.11` | `v0.8.12` |
| `github.com/oekit/pkg` | `v0.38.0` | `v0.39.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.14.5] — 2026-11-08

- 与 v0.14.4 相比无代码差异（版本号/打包变更）

## [v0.14.4] — 2026-11-08

### 修复(agentrun)：streamDeltas 全部退出路径 exactly-once Close 流（#77）

eino 契约（schema/stream.go）：reader 必须恰好 Close 一次——无论循环正常
结束还是 break/return 提前退出；且 Close 不幂等（裸 Pipe 流 double-close
会 panic）。streamDeltas 此前只有 Recv 错误路径关流，干净 EOF break、零
分片与 concat 错误 return 三条路径全漏——与 llm#80 同缺陷形态（llm
v0.21.0 发版注记点名移交本仓）。ADK 事件流是 Copy 扇出的子副本：子流不
Close 则父流 closedNum 永不收口，上游生产者 goroutine 挂死在 Send 上；
每个流式轮次还滞留一个未关副本。
修法：defer + sync.Once——EOF/错误/取消/panic 全部退出路径恰好关一次，
once 兜住 Close 不幂等（Recv 错误路径的手动关流移除，统一由 once 收口），
defer 兜住 panic 路径。
失败测试先行（Copy(2) 扇出桩，llm#80 同款探针——已关子流再 Recv 报
ErrRecvAfterClosed，未关子流恒可重复 Recv 出缓存 io.EOF）：
TestStreamDeltasClosesOnEveryExitPath 钉住四条退出路径
① 干净 EOF（正常完成）② 零分片（agent#46 形态）
③ Recv 错误（回归不退化）  ④ concat 错误（分片角色不一致）。
close #77

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.21.4

llm v0.21.4（WithRateLimitRetry 模型级 429 退避装饰，llm#100）落地，地板同步追平。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。


## [v0.14.3] — 2026-11-08

- 杂务(deps)：依赖地板追到最新——github.com/oekit/logx v0.8.10

logx v0.8.10 落地，发版时刻地板时效门禁要求追平（matrix-tools#188 发版链）。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。

- 杂务(deps)：依赖地板追到最新——concurrency v0.28.4 / logx v0.8.9 / pkg v0.37.4 + 矩阵新 tag（按本仓依赖取齐）

发版时刻地板时效门禁（verify_tag_floor --ref HEAD）要求追平（matrix-tools#188 发版链）。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。


## [v0.14.2] — 2026-11-08

- 杂务(deps)：依赖地板追平——基座 v0.28.3/v0.37.3/v0.8.8（oekit BOM v0.2.7 对齐轮）
- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.21.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/llm` | `v0.21.0` | `v0.21.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(build)：补统一 Makefile——本地 ci 门禁与共享 CI 同口径（agentkit#138）

Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.14.1] — 2026-11-07

### 修复(clarify)：选项匹配精确优先+否定前缀防护——「不是」不再命中「是」（#69）

Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.21.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/llm` | `v0.20.0` | `v0.21.0` |
| `github.com/oekit/concurrency` | `v0.27.0` | `v0.28.0` |
| `github.com/oekit/logx` | `v0.8.4` | `v0.8.6` |
| `github.com/oekit/pkg` | `v0.35.2` | `v0.37.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.14.0] — 2026-11-07

### 修复(agentrun)：钉死 MaxIterations 计的是模型调用次数，并让「预算耗尽」可判定（#60）

## 根因
`Config.MaxIterations` 的注释写「ReAct 循环轮数上限」，实现原样转发给 eino
`adk.ChatModelAgentConfig.MaxIterations`——而 eino 的语义是 **ChatModel
generation cycles**：每次模型调用计一次，**含产出终稿的那一次**。
于是按「循环轮数」直觉填 1 的配置：纯问答能跑通（1 次调用即终稿），但只要模型
决定调工具，工具轮之后那次出终稿的调用就会被 eino 拒（`exceeds max iterations`），
整轮失败——而最终错误文案「agent 未产出最终文本（iterations=1）」看着像模型失语，
实际是**预算配小了**。这不是转发的 off-by-one（转发是忠实的），是契约语义错位。
## 修法
三处各自钉一个面：
1. **文档钉语义**：包注释、`DefaultMaxIterations`、`Config.MaxIterations` 一律
   写明「模型调用次数（含终稿轮），不是工具轮数」；
2. **算术下界成常量**：新增 `MinIterationsForToolRound = 2`——1 次调用产出
   tool_calls、1 次出终稿，这是下界不是经验值，装配期可据此告警；
3. **预算耗尽可判定**：新增哨兵 `ErrIterationBudgetExhausted` +
   `iterationBudgetError` 包装（`Error()`/`Unwrap()` **逐字透传**原错误，只额外
   让 `errors.Is` 成立——既有按文案判断的消费方零感知）。
判据取**确定性计数**而非文案匹配：`reactHandler` 数 assistant 消息数（一轮生成
一条 = 一次模型调用），`modelCalls >= budget` 即耗尽。匹配 eino 的
`exceeds max iterations` 文案不行——那是上游实现细节，改一个字判据就静默失效。
顺带把 `cfg.maxIterations()` 在 `run` 内只算一次（`budget`）：喂给 eino 的与
耗尽判据用的必须是同一个数，否则会出现「日志说 12、实际 10」的自相矛盾。
## 反向验证（两条，均实测红）
| 注入 | 报错 |
|---|---|
| 去掉 `if h.modelCalls >= budget` 标记 | `应判定为预算耗尽（ErrIterationBudgetExhausted）` |
| 无条件标成预算耗尽 | `上游故障不是预算耗尽——误判会让调用方去调大 MaxIterations 而不是重试/换端点` |
## 用例（`agentrun/maxiterations_semantics_test.go`）
- `TestMaxIterationsCountsModelCallsNotToolRounds`：**成对**断言——同一脚本
  [工具轮, 终稿] 在预算 1 下报 `ErrIterationBudgetExhausted`、在预算
  `MinIterationsForToolRound`(=2) 下成功返回终稿。只断言「预算 1 失败」证明不了
  语义（任何 bug 都能让它失败）；
- `TestModelFailureIsNotBudgetExhaustion`：负对照——上游 500 不得被判成预算耗尽，
  否则调用方会去调大预算而不是重试/换端点；
- `TestBudgetExhaustedRequiresZeroToolRounds`：纯问答预算 1 仍成功——「够不够」
  取决于是否需要工具轮，不是数字本身。
`mockResponse` 顺带加 `status` 字段（非 0 时回该 HTTP 状态码）以表达「上游故障」
形态；`Error()` 文案兼容钉仍在既有 `TestRunWithRetrySalvageOnIterationsExhausted`
里逐字校验。

### 修复(agentrun)：非信封纯文本终稿不再判死——空终稿改报「未产出终稿」（#58）

`unwrapRespond` 的文档说「非该形态原样返回」，但调用方写的是
```go
answer := unwrapRespond(lastAssistant)
if answer == "" || answer == lastAssistant {
    return nil, fmt.Errorf("... respond 信封异常（%q）", ...)
}
```
即**任何**没解析成 `{"response":...}` 的终稿都整轮失败。而 `sawRespond` 此时
已为真（BreakLoopAction 已触发、任务确实完成），于是把已经跑完的全部结果丢掉、
只上抛一句「信封异常」。
`respond` 是**期望**的收尾方式，不是唯一方式：模型直接给纯文本答复是常见形态。
现在非信封形态按合法纯文本终稿返回。
「空」是另一回事——真的没有答案，当然要报；但旧实现报不出，因为
- 纯空白终稿（`"   "`）会被当成非空答案**静默返回一串空白**；
- `{"response":""}` 压根不解包（旧 `unwrapRespond` 要求 `probe.Response != ""`），
  于是把 `{"response":""}` 这段 **JSON 原文**当终稿交出去。
故本次一并把解包判据从「载荷非空」改成「信封键存在」：键存在即解包，空/空白/
`null` 载荷一律解包成空串，再由同一条 `strings.TrimSpace(answer) == ""` 判定
报「未产出终稿（max_steps=%d，最终答复为空）」。
错误文案也要换：旧那句「信封异常」会把人引向序列化/协议层，而真实情况是模型
没给答案——两句话指向完全不同的排查方向。
## 行为变更
`unwrapRespond({"response":""})` 由「原样返回」变为「返回空串」。既有用例
`TestUnwrapRespond` 里 `{"response":""}` 原本被钉在「不应改动」那组，本次移到
「应解包为空」那组——**改的是不变量本身**（信封键存在即解包），不是凑绿；
`{"response":null}` 同理（`encoding/json` 把 null 解进 string 是 no-op，故与
空串统一）。非信封形态（`纯文本`、`{"other":1}`、`""`、畸形 JSON）仍逐字原样
返回，未变。
## 反向验证
两处改动各自单独回退，都红：
1. 判定退回 `answer == "" || answer == lastAssistant`（解包修复保留）——
   ```
   TestPlanAndExecutePlainTextFinal: 纯文本终稿应视为合法，不得整轮失败:
       agentrun: plan-execute respond 信封异常（"最终答复：纯文本，未套信封"）
   TestPlanAndExecuteBlankFinalIsReportedAsMissing/{"response":""}:
       应报「未产出终稿」，实得: respond 信封异常（"{\"response\":\"\"}"）
   TestPlanAndExecuteBlankFinalIsReportedAsMissing/{"response":"___"}:
       空白终稿应报错，实得 Text="   "
   TestPlanAndExecuteWhitespaceOnlyContentIsNotAnAnswer:
       应报「未产出终稿」，实得: respond 信封异常（"   "）
   ```
   第一条与 issue 描述的失效形态逐字一致；第三条正是「静默返回空白」。
2. 解包退回「载荷非空才解包」（判定修复保留）——
   ```
   TestPlanAndExecuteBlankFinalIsReportedAsMissing/{"response":""}:
       空白终稿应报错，实得 Text="{\"response\":\"\"}"
   TestUnwrapRespond: "{\"response\":\"\"}" 应解包为空/空白，得到 "{\"response\":\"\"}"
   ```
   即 JSON 原文被当成终稿交出去。两条都红过才恢复。
## 用例
新增 `agentrun/planexecute_plaintext_test.go` 四条：
- `TestPlanAndExecutePlainTextFinal`：非信封终稿原样返回（issue 的主诉）。
- `TestPlanAndExecuteBlankFinalIsReportedAsMissing`：三个子用例覆盖
  全空 / 纯空白 / 换行制表符三种「空」，且断言**不得**再出现「信封异常」。
- `TestPlanAndExecuteWhitespaceOnlyContentIsNotAnAnswer`：非信封形态的纯空白
  内容同样不算答案（成对：非空纯文本仍合法，见第一条）。
- `TestPlanAndExecuteEnvelopeStillUnwrapped`：回归——正常信封仍解包。
`go vet` + `go test -race -count=1 ./...` 全绿。

### 修复(agentrun)：补 MutatingVerbs 的对外投递/写入/凭据动词——漏判即副作用重复执行（#56）

动词表缺 `send/post/publish/put/patch/transfer/grant/rotate/dispatch/emit`，
`send_email`、`publish_report`、`put_object`、`transfer_funds`、`rotate_keys`
这些工具的分段全不命中 ⇒ `IsMutatingTool` 判「非变更类」⇒ 首轮失败后重试守卫
**放行整体重跑** ⇒ 邮件重发 / 报告重发 / 资金重复转账。
这是**方向不对称**的缺陷：文档自认「漏判/误判都是必然的」，但
- 漏判 ⇒ 放行重跑 ⇒ 副作用重复执行（不可撤回）；
- 误判 ⇒ 放弃一次合法重试（代价小，且可经 `NonMutatingTools` /
  `MutabilityClassifier` 显式校正回来）。
所以词表侧的取舍必须是**宁可多收**。按此补三类：
- 对外投递/发布：`send` `post` `publish` `emit` `dispatch`
- 写入/覆写：`put` `patch` `upload`（REST 动词与对象存储命名法）
- 权限与凭据：`grant` `revoke` `transfer` `rotate`（改的都是「谁能做什么」）
已知的误判倾向写进注释：`post_process` / `get_commit` 这类「动词恰好撞上写动词
的读操作」会命中——属误判方向，按上面的取舍接受。
## 反向验证
只撤回这三行新增动词（其余不动）：
```
IsMutatingTool("send_email") = false，漏判会放行整体重跑 → send_email 重复执行
IsMutatingTool("publish_report") = false …
IsMutatingTool("transfer_funds") = false …
（11 条全红）
```
## 用例取舍
三个钉 + 两条端到端：
- `TestMutatingVerbsCoverOutboundSideEffects`：issue 点名的 15 个工具名逐个断言。
- `TestMutatingVerbsStillAvoidSubstringFalsePositives`：**这是本次最容易犯的错**
  —— 补 `send/post/put` 时顺手把分段匹配改成 `strings.Contains`，`use_skill`
  （含 kill）、`get_running_config`（含 run）、`list_posts`（含 post）、
  `get_input_schema`（含 put）会全部重新误判，而那正是 agent#45 花两轮生产
  实弹换来的修正。用例把它们逐条钉住。
- `TestMutatingVerbsAreLowercase`：`IsMutatingTool` 先 `ToLower` 再查表，
  表里混入大写项即**永不命中**（静默失效，看起来「加了」其实没加）。
- `TestRunWithRetrySkipsAfterOutboundVerb`：端到端——`send_email` 工具 + 首轮
  空答复，断言错误里点名 `send_email` 且**模型调用次数恰为 2**（出现第 3 次即
  证明守卫没拦住）。只断言「返回了错误」抓不到「错了但重试照跑」。
- `TestRunWithRetryKeepsReadOnlyToolRetryable`：**成对的反向对照**——`list_posts`
  必须仍可重试。否则把判定做坏成「一律算变更」也能让前面几条全绿，而重试能力
  整体消失。
`go vet` + `go test -race -count=1 ./agentrun/...` 绿。

### 修复(router)：槽位值截断改 rune 口径——中文槽位值不再被切成非法 UTF-8（#57）

`sanitizeSlots` 用 `len(v) > 64`（字节）判定后 `v[:64]` 切片。多字节字符被切在
中间 ⇒ 产出**非法 UTF-8**，而没有任何报错点：下游按字符串处理的环节
（JSON 编码、日志、匹配）拿到坏数据，只会表现为「奇怪的行为」。
仓内其余截断都是 rune 口径（`clarify` 的 `utf8.RuneCountInString` 判定），
此处是漏网之鱼。
## 改动
`if utf8.RuneCountInString(v) > slotValueCap { v = string([]rune(v)[:slotValueCap]) }`：
`RuneCountInString` 判定不分配，只有真的超限才付一次转换成本。
顺带在 `slotValueCap` 上写明计量口径是 rune 与理由——这个常量原来只写「长度上限」，
「长度」按字节还是按字符恰好是这次事故的缝隙。
## 既有断言也改了（这是改不变量本身，不是凑绿）
`router_test.go:193` 原来是 `len(got) != slotValueCap` —— 用字节数断言一个
**错误的口径**。它的输入正是 80 个中文，所以旧断言在旧实现下绿、
在新实现下红：这不是新代码破坏了契约，而是契约本身从「64 字节」改成
「64 个字符」。改成 `utf8.RuneCountInString` 并在注释里写明原因。
⚠️ 这是**行为变更**：64 字节 → 64 字符，多字节槽位值的实际上限随之变大
（中文从 21 字到 64 字）。发版说明需带上。
## 反向验证
逐字退回字节截断（含移除 `unicode/utf8` 导入）：
| 子用例 | 结果 |
|---|---|
| 超 rune 上限的中文（100 字） | 红 —— 结果尾部是悬挂字节 `…中\xe4`，非法 UTF-8 |
| **超字节上限但未超 rune 上限的中文（30 字）** | 红 —— 本不该被截断的值被截坏 |
| **恰好 64 rune 的中文** | 红 —— 正好在上限上的值也被切坏 |
| 超上限的 ASCII | 绿（ASCII 下两种口径等价，符合预期） |
`go vet` + `go test -race -count=1 ./...`（agent 全仓六包）绿。

### 修复(router)：空关键词不再命中一切，也不再让扫描原地挂死（#59）

根因：hitScan 的唯一前进量是 `start = pos + len(kw)`，而 kw=="" 时
`strings.Index(s,"")` 恒返回 0、前进量恒 0。于是同一条空词分出两种**静默**失效：
- 恒真族：boundary=false 的路径首轮就 `return true` ⇒ 词表里一个空项让这条意图
  吞掉全部输入（KeywordHit / KeywordHitExcept）；
- 挂死族：boundary=true 的变体（KeywordHitBoundary / KeywordHitBoundaryExcept）
  与被作废后「继续找下一处」的 KeywordPostNegated 都不 return，循环原地踏步
  ⇒ goroutine 永久占用 CPU 且永不返回。没有报错、没有日志，只有热路径 hang。
为什么在原语层守门而不是「加个校验」：router 的关键词是**包级纯函数**，词表由
消费方自己持有（router.New 校验的是路由名/槽位名），本包没有「词表」这个对象，
构造期无处可校验。故契约写进 keyword.go 文件头并在五个入口落守卫：
**空关键词不构成命中**（四个命中入口 + KeywordPostNegated 一律 false）。
理由不是「空串是任意串的子串」这条数学事实，而是词表语义：词表是数据面
（消费方从配置/yaml 加载），空串是装配错误，让它命中一切等于把整张词表退化成
恒真判定。判定从严——宁可某条意图不被词表兜底（还有 LLM 分类面兜），
不把一切都归给它。
顺带（同一类装配错误）：maskedAt 遇到空 mask 现在显式 `continue`。空区间容不下
任何非空命中，所以「跳过」与「放行」在**非空命中下结论相同**，但语义上必须写清
——把空 mask 当成「包含一切」会让整张排除表恒返回 false（已用成对用例钉住
「空 mask 不排除 / 非空 mask 照常排除」）。此处是语义澄清，不是行为修复。
反向验证（三处，各自只拆自家守卫）：
- 去掉 hitScan 的空词守门 ⇒ 恒真族三条打印「空关键词不得命中」，
  挂死族两条由看门狗打印「空关键词使扫描原地踏步（永不返回）」
- 去掉 KeywordPostNegated 的空词守门 ⇒ 该子用例 2.00s 后由看门狗判红
- 把空 mask 改成 `return true`（当作包含一切）⇒ 空 mask 用例红
用例设计说明：挂死族不直接写在测试体里，而是走 2s 看门狗 + t.Fatalf——
否则旧代码下整包要等 10 分钟 go test 超时，报错说不清是哪条入口。

- 杂务(deps)：依赖地板追到最新——textx v0.10.0 / concurrency v0.27.0 / logx v0.8.4 / pkg v0.35.2

## [v0.13.0] — 2026-11-06

### 修复(agentrun)：流式零分片成因区分——断流上抛重试，真空答复保持原形态（#46）

零分片此前直接 return (空消息, nil)——传输/网关断流（连接开即断）与模型
真空答复两类成因被合并成同一成功形态，下游据「空内容」触发整程重试重放
工具采集，瞬断被放大成完整重放。
- 零分片（干净 EOF 即断）显式报错「传输/网关侧断流」——走生成重试
- 有分片但内容空（真空答复）保持空消息成功——仍走既有「空答复」判定面
  （错误文案区分空答复/没答复的注释语义不变）
TestStreamDeltasZeroChunkCause：零分片报错含成因 + 空内容分片保持成功形态。
全仓 -race 绿。
关联 oagentkit/agent#46
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

### 修复(agentrun,router,clarify)：观察钩子 recover 兑现 + mutability 显式通道 + 词表严格解码（#42 #43 #44 #45）

agent#42/#44——「钩子 panic 不影响主链」注释与实现对齐：
- router.observeDecision 补 recover（此前裸调，宿主指标回调 panic 沿
  Classify/Run 击穿调用方 goroutine；恢复即静默终结该次观察——主链不受
  牵连是优先级）
- agentrun.WithOnEvent doc 消歧：明确「回调 panic 会击穿调用方 goroutine，
  本包不代为 recover，消费方自行 recover」——此前「消费失败不影响事件面
  完整性」的措辞给了错误的安全暗示
agent#43——clarify 词表 typo 静默降级两道修复：
- LoadVocab 改 yaml KnownFields 严格解码：terms: 误写 items:/term: 不再
  静默忽略（加载「成功」但注入段缺术语的慢性降级），拼错键解析层即报错
- 新增 ValidateTermsStrict（opt-in）：非 vague 条目 Terms 为空的结构层
  兜底。不并入 Validate——词条只做路由、无术语展示是合法形态（既有测试
  钉住的语义），需要注入完备性的宿主装配期显式调用
agent#45——变更性判定显式通道：Config.MutabilityClassifier 非 nil 时整体
接管 IsMutatingTool 启发式与 NonMutatingTools 豁免集（判定单源）——
run_readonly_commands 命中 run 段的误判（bianque 两轮生产实弹）不再要求
每个下游维护豁免集；名字拿不准的域外工具走显式声明而非重猜（mcpkit
ReadOnly 注解经此喂数据单源）。
测试：TestObserveDecisionPanicContained / TestLoadVocabStrictKeys（拼错键
报错+合法通过+严格检查两态）/ TestMutabilityClassifierOverrides（误判
基线复现+接管+豁免集不参与）。全仓 -race 绿。
关联 oagentkit/agent#42 oagentkit/agent#43 oagentkit/agent#44 oagentkit/agent#45
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.35.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.34.1` | `v0.35.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.20.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/llm` | `v0.19.0` | `v0.20.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/textx v0.9.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/textx` | `v0.8.0` | `v0.9.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.25.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.24.0` | `v0.25.1` |
| `github.com/oekit/logx` | `v0.8.2` | `v0.8.3` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/textx v0.8.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/textx` | `v0.7.2` | `v0.8.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.34.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.34.0` | `v0.34.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/llm v0.19.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/llm` | `v0.18.1` | `v0.19.0` |
| `github.com/oagentkit/textx` | `v0.7.1` | `v0.7.2` |
| `github.com/oekit/concurrency` | `v0.23.2` | `v0.24.0` |
| `github.com/oekit/logx` | `v0.8.0` | `v0.8.2` |
| `github.com/oekit/pkg` | `v0.31.0` | `v0.34.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.12.0] — 2026-10-30

### 新增(agentrun)：暴露 ToolsNodeConfig.ToolAliases / UnknownToolsHandler

## 问题
模型调用工具时若用**裸名**（`run_readonly_commands`），而工具在 ToolsNode 索引里的键是
**带 server 前缀的暴露名**（`ask-ops.run_readonly_commands`），eino 直接报：
```
[NodeRunError] tool run_readonly_commands not found in toolsNode indexes
```
该步骤**整步失败**（agentrun 事件错误向上抛、链路降级）；变更类工具已执行时
连整步重试都会被跳过 ⇒ 一条链少一环结论。
（issue agent#36；实测 2026-10-30 扁鹊生产实弹 215。）
## 根因
`agentrun/agentrun.go` 与 `agentrun/planexecute.go` 两处 ToolsNode 硬编码：
```go
ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
```
⇒ 下游无法注入 eino 已有的两个正解。
## 方案
在 `Config` 与 `PlanExecuteConfig` 上**同时**暴露两个能力（两者互补，不是二选一）：
```go
// ToolAliases 工具名别名表（可选）：键=规范名（tool.Info().Name，MCP 场景为 server.工具名）
ToolAliases map[string]compose.ToolAliasConfig
// UnknownToolsHandler 幻觉工具兜底（可选）：签名与 eino 一致
UnknownToolsHandler func(ctx context.Context, name, input string) (string, error)
```
- **ToolAliases** 解决**已知**的写法差异 ⇒ 两种写法都命中，模型不必自我纠正（首选）；
- **UnknownToolsHandler** 解决**未知**的幻觉 ⇒ 模型编了个工具名时拿到可解释观察而非整步失败。
两处都接到 `compose.ToolsNodeConfig`：`agentrun.go` 的 ReAct 路径与 `planexecute.go` 的
Executor 路径——后者有独立的 `Tools` 字段与独立的构造点，只改一处会漏。
### 为什么本仓不自动推导裸名别名
本仓是**编排层**，不拥有 `server.工具名` 的命名约定——那是 mcpkit 的契约
（`mcp/catalog.go`：`info.Name` = 暴露名、`wireName` = 真实调用名）。
本仓拿不到「哪个前缀是 server 名」的知识 ⇒ 自动推导会变成猜。
故本仓的职责是**暴露能力**，由知道 server 命名的下游显式注册。
### 一处与 issue 原文的更正
issue 里方案 B 的签名写作
`func(ctx context.Context, toolName string, args map[string]any) (string, error)`，
而 eino v0.9.21 的真实签名是
`func(ctx context.Context, name, input string) (string, error)`
—— `name` 与 `input` **都是 string**（input 是参数原文 JSON，不是 map）。
按 issue 原文的签名实现会编译不过。此处按 eino 真实签名暴露。
## 兼容性
纯增量：默认 nil ⇒ 行为与现在完全一致
（eino 侧 `aliasConfigs == nil` 时跳过；`unknownToolHandler == nil` 时直接报错）。
⚠️ 装配期需注意：别名若与**另一个工具的规范名或已注册别名**冲突，
eino 的 `applyAliasConfigs` 会让整个 ToolsNode 构造失败
⇒ 同名裸名来自两个 server 时只能择一，或改用 `UnknownToolsHandler` 兜底。
## 验证
- `go build ./...` / `go vet ./...` / `gofmt -l` / `go mod tidy -diff` 全绿
- `go test ./...` 全绿；`go test -race ./...` 全绿
- 新增 5 个用例：
  - `TestRunToolAliasesResolveBareName` —— 模型发**裸名**、规范名带 `srv.` 前缀 ⇒ 别名命中、该步不失败
  - `TestRunWithoutToolAliasesUnknownToolFails` —— 未注册别名 ⇒ **保持原行为**（整步失败，钉住「nil = 零行为」）
  - `TestRunUnknownToolsHandler` —— 模型调不存在的工具 ⇒ 处理器被调 1 次、收到正确 name/input、该步不失败
  - `TestRunUnknownToolsHandlerError` —— 处理器返回 error ⇒ 该步仍失败（处理器是缝，不是吞一切的洞）
  - `TestPlanExecuteToolAliases` —— plan-execute 链路的 Executor 同样支持别名
- **变异验证**：把接线退回 `compose.ToolsNodeConfig{Tools: tools}`
  ⇒ 用例必须转红（实测报 `[NodeRunError] tool 不存在的工具 not found in toolsNode indexes`）
## 发版
本仓是发布件：本次发 **v0.12.0**（新增公共 API ⇒ minor 版本）。
按 org CONTRIBUTING 顺序：特性 PR 合入 main → 预写 CHANGELOG → 打 annotated tag。
🤖 Generated with [Claude Code](https://claude.com/claude-code)

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.2

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.23.1` | `v0.23.2` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.1

oekit 基座又发了一版（`concurrency` v0.23.1，2026-10-29 02:19 CST）⇒ 上一轮抬到
v0.23.0 的地板已经落后 ⇒ 再抬一格。
本仓只用到 `concurrency` 的少量 API ⇒ 实测 go build / go vet / go test -race 全绿，
**零行为差异**；go 指令未被顶起。
只改 go.mod / go.sum ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.0 github.com/oekit/logx v0.8.0 github.com/oekit/pkg v0.31.0

oekit 基座发了新版本（pkg v0.31.0 / concurrency v0.23.0 / logx v0.8.0）⇒ 本仓的
依赖地板落后 ⇒ 抬到最新，让**下一次发版**不再带出落后地板（打 tag 时刻的地板
必须是当时的最新——`verify_tag_floor` / `release.py doctor` 都在查这条）。
兼容性实测：本仓只用这些基座包的少量 API ⇒ go build / go vet / go test -race
全绿，**零行为差异**；go 指令未被顶起（本仓的 go 地板不动）。

- 杂务(deps)：依赖地板追平上游本轮新版本——github.com/oagentkit/llm@v0.18.1、github.com/oagentkit/textx@v0.7.1、github.com/oekit/concurrency@v0.22.0、github.com/oekit/pkg@v0.28.0

本仓 HEAD 的依赖地板已落后于各依赖的最新版本（`verify_tag_floor.py --org oagentkit --ref HEAD` 判红）。
本仓一旦发版，就会把落后地板发出去——而 tag 受 protect-tags ruleset 保护、打完不能重打，
只能靠后续版本补齐。
本版只抬依赖地板，不改任何行为：
- github.com/oagentkit/llm → v0.18.1
- github.com/oagentkit/textx → v0.7.1
- github.com/oekit/concurrency → v0.22.0
- github.com/oekit/pkg → v0.28.0
对应 issue matrix-tools#143。


- 文档(agent)：刷正 README 的外部依赖清单 —— 与 go.mod 实测对账

本仓 README 的「外部依赖」清单与**各仓 go.mod 的事实**对不上。
矩阵的单一数据源 `matrix-tools/modules.json` 也有同样漂移，已在
[matrix-tools#126](https://github.com/oagentkit/matrix-tools/pull/126) 同步刷正。
## 现状核实
```bash
$ grep -v indirect agent/go.mod | grep -E "^\s+[a-z].*/"
	github.com/cloudwego/eino v0.9.21
	github.com/cloudwego/eino-ext/components/model/openai v0.1.13
	github.com/oagentkit/llm v0.17.1
	github.com/oagentkit/textx v0.6.2
	gopkg.in/yaml.v3 v3.0.1
```
⇒ 第三方直接依赖是 **3 个**（eino、**eino-ext/components/model/openai**、**yaml.v3**），
本仓 README 只列了 1 个。
## 影响
`eino-ext/components/model/openai` 是重量级地板（拉 sonic 等）；漏列 ⇒ 读者低估依赖面。
## 改动
**只改清单那一行（`- `github.com/cloudwego/eino`` ⇒ 多两行 `- `github.com/cloudwego/eino-ext/components/model/openai`` 与 `- `gopkg.in/yaml.v3``）**，其余段落逐字未动。
## 验收
- [x] 清单与 `go.mod` 实测一致
- [x] 与 `matrix-tools/modules.json` 的登记一致（同期在 matrix-tools#126 刷正）
- [x] **纯 .md 改动** ⇒ 不需要 CHANGELOG 条目、不打 tag、不发版（无发版步骤）
关联 oagentkit/matrix-tools#125


## [v0.11.0] — 2026-10-23

- 杂务(deps)：依赖地板抬到 llm v0.17.1 / oekit/concurrency v0.20.2

抬 go.mod 的依赖地板，跟到本轮新版本：
- github.com/oagentkit/llm        v0.16.0 -> v0.17.1
- github.com/oekit/concurrency    v0.20.1 -> v0.20.2
本仓自身代码零改动，只改 go.mod / go.sum。
下游要做什么：**go mod tidy**——地板抬高对下游是**隐式破坏性变更**：
不跑就报 updates to go.mod needed，且报错不提真正原因。
为什么抬：本仓 tip（b91c82f，2026-10-22T22:19:17Z）的地板还钉在 llm v0.16.0 /
concurrency v0.20.1，而 llm 最新已是 v0.17.1（发布于 2026-10-23T07:58:26Z）、
基座 concurrency 最新已是 v0.20.2（发布于 2026-10-23T03:09:14Z）——**都比 tip 晚**。
矩阵的「发版时刻地板时效」门禁（verify_tag_floor.py --org oagentkit，默认 HEAD 口径）
此刻还绿，**只因 tip 的提交时刻早于这两个发布时刻**；用门禁自己的 check_ref() 复现
（同一份 go.mod、同一批 tag，只把 ref 时刻推进到现在）即刻报红：
llm 地板 v0.16.0 应为 v0.17.1、concurrency 地板 v0.20.1 应为 v0.20.2。
⇒ 下一次 push 到本仓就会把这条判据打红，而下一次打 tag 就会把落后地板发出去；
tag 受 protect-tags 保护不可重打（重打会让下游 go.sum 的 h1 对不上 ⇒ checksum mismatch）。
为什么无行为差异：llm v0.16.0 -> v0.17.1 的 diff 是**纯加法**——client.go 新增
Client.OnUsageDetailed 与 UsageRecord.CachedTokens（不碰 OnUsage 旧签名、不改 CostOf
计价口径），decision/ 与 port/ 是新包（本仓不 import）；本仓唯一 import 的 llm 根包
编译与测试在 v0.17.1 下全绿。concurrency v0.20.1 -> v0.20.2 的 diff 只动了 syncx/
（KeyedGate 的 release 幂等标记从裸 bool 改成 atomic CAS、新增 Gate.TryAcquire）
与它自己的 go.mod，而本仓只 import concurrency/async，一字未动。
门禁：go build / vet / test -race 通过，gofmt -l 空，go mod tidy -diff 空。
Closes #28


## [v0.10.6] — 2026-10-23

### 新增(router)：新增决策面 —— System One 等非自回归判定模型可先于 LLM 分类

router 此前只有一条分类执行体：LLM（自回归生成）。但「把输入路由到有限候选集」本身
不需要生成——它是非自回归判定模型（System One 协议：Jev/Laya/Kev/Clef/CLM）的主场：
一次前向 pass 答完全部问题，输出有限标签 + 概率分布 + 校准置信度，毫秒级、几乎免费。
⇒ router 有了三种执行体：词面（确定性 KeywordHit）→ 决策面（判定，毫秒级）→ LLM 面
（生成，秒级）。决策面比词面能理解语义，比 LLM 面快且便宜。
- DecisionFace 窄接口：实现方负责把候选集/槽位规格映射成判定题，并按自己的口径
  返回校准置信度；本包只管编排语义（不直连客户端实现，遵循「编排层不得直连底层
  实现」的仓纪律，且本仓不新增仓依赖边）。
- Config 新增 DecisionFace / DecisionMinConfidence / DecisionShadow /
  DecisionObserver；Decision 新增 Source（llm | decision，零值即 llm，向后兼容）。
- 编排：决策面先于 LLM 面；达门槛即采纳（省一次 LLM 分类调用），未达门槛/端点故障
  ⇒ 回落 LLM 面（fail-open：判定面是加速件，不得阻断主链）。
- 影子模式：调用决策面但只记不采（结果进 DecisionObserver，行为面零变更）——上线前
  量一致率与延迟的标准姿势。
- 自守恒：决策面给的路由名不在表内 ⇒ 拒（回落）；槽位只保留本包配置过的槽位名
  （防幻觉槽位名透传）；置信度越界钳位到 [0,1]。
取舍：决策面先行而非 LLM 先行——判定模型的输出可校准（概率分布算出的置信度），
而 LLM 的自评置信度不可靠；代价是「判定面自信但判错」时 LLM 无机会纠正 ⇒ 用影子
模式先量一致率、门槛随候选集标定来兜。门槛口径提示：判定置信度随候选集规模变化，
不可跨候选集移植。
门禁：gofmt -l（空）/ go vet / go build ./... / go test -race -count=1 ./... 全过
（router 新增 7 条用例）。

- 杂务(deps)：依赖地板跟到发版时刻的最新——oagentkit/llm v0.16.0（决策面客户端落地）/ oekit/concurrency v0.20.1 / oekit/pkg v0.22.0

## [v0.10.5] — 2026-10-22

### 修复(agentrun)：Usage 补 DurationMS —— 流式用量通道的耗时不再恒 0

agentrun.Usage 缺 duration 字段 ⇒ 消费方只能把本轮 LLM 调用耗时记成 0，
且「零耗时」与「没采到耗时」不可区分（面板上表现为「有数但全 0」，比没数据更坏）。
* Usage 新增 DurationMS int64 —— 与 llm.UsageRecord.DurationMS 同型，宿主映射闭包直接赋值
* 测量点：模型流交付 → 收流 concat 完成（生成窗口）；0 = 未采集
* 口径边界写进 godoc：不含「请求发出 → 首帧到达」的排队与首字延迟（TTFB）
测试：新增 TestRunStreamingUsageDuration —— mock 注入帧间隔（生成耗时），
断言每轮 DurationMS > 0 且 >= 注入的生成耗时；反证样本已验
（测量点改成恒 0 ⇒ 用例变红，还原 ⇒ 转绿）。
取舍：不用 eino callbacks 的 OnStart 起点 —— cfg.Model 由调用方构造、ADK 无
callbacks 注入点，llm 侧 NewUsageHandler 的起点写在模型调用的 ctx 里、在
agentrun 回填点不可见；要拿到它就得包一层模型（降级 ToolCallingChatModel
接口 + 模型调用与用量事件需 FIFO 对齐），代价大于收益。
本地：gofmt -l（空）/ go vet / go build ./... / go test -race -count=1 ./... 全过。

- 杂务(deps)：依赖地板跟到发版时刻的最新——oekit/pkg v0.21.0 / oekit/concurrency v0.20.0

打 tag 时刻的依赖地板必须是当时的最新（matrix-tools 的 verify_tag_floor 判据）。
agent 不直接 import 这两个模块（均为间接依赖），上游改动纯增量：
* oekit/pkg v0.20.0 -> v0.21.0：新增 pathx/filename.go（+243）与测试，无既有行为改动
* oekit/concurrency v0.19.0 -> v0.20.0：新增 syncx/gate.go（+49）与测试，
  既有文件仅 syncx/syncx.go 的包注释 1 行改写
⇒ 本仓行为零差异。
本地：gofmt -l（空）/ go vet / go build ./... / go test -race -count=1 ./... 全过 /
go mod tidy -diff（空）。


## [v0.10.4] — 2026-10-21

- 杂务(deps)：依赖地板跟到矩阵本轮新版本——llm v0.15.2

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）当场报红：
- github.com/oagentkit/llm@v0.15.2
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
## 为什么无行为差异
被抬的这几个版本**自身也都是纯依赖地板追平**（各仓代码零改动，只抬
`oekit/pkg` / `oekit/concurrency` / `guardx` / `llm` 四类地板之一），
本仓用到的转发面与符号集一字未动。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```


## [v0.10.3] — 2026-10-21

- 杂务(deps)：依赖地板跟到矩阵本轮新版本——textx v0.6.2

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）当场报红：
- github.com/oagentkit/textx@v0.6.2
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
## 为什么无行为差异
被抬的这几个版本**自身也都是纯依赖地板追平**（各仓代码零改动，只抬
`oekit/pkg` / `oekit/concurrency` / `guardx` / `llm` 四类地板之一），
本仓用到的转发面与符号集一字未动。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```


## [v0.10.2] — 2026-10-21

- 杂务(deps)：依赖地板追平上游本轮新版本——oagentkit/llm v0.15.0 oekit/concurrency v0.19.0 oekit/pkg v0.20.0

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是隐式 breaking：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 抬了哪几块、为什么无行为差异
**`github.com/oagentkit/llm` v0.14.2 → v0.15.0**
上游新增 `OpenAIProviderConfig.HTTPClient`（**可选**字段）：宿主可注入自己的
`*http.Client`，把出站守卫挂到 `Transport.DialContext` 上（写面校验拦不住
DNS rebinding，守卫必须在拨号层）。缺省 nil 时行为**逐字不变**。
**`github.com/oekit/concurrency` v0.18.0 → v0.19.0**
上游 diff 只动了 2 个包：
- `retry`：**抖动 panic 修复**——`rand.Int63n` 的契约是 n > 0，而「退避幅度为 0」
  是生产可达路径（cfg.Delay=0 / DelayHint 返回 0 / float64→int64 溢出成负数），
  旧实现在这些路径上直接 panic。改为钳到 [0, MaxInt64]。
- `syncx`：新增全局在飞闸门原语（`Gate`）+ 说明同步。
**`breaker` 与 `async` 两个包一字未动。**
**本仓实际 import 的 `oekit/concurrency` 子包：（本仓未直接 import oekit/concurrency 的任何子包）**
与上游改动相交的：**（无）**
**`github.com/oekit/pkg` v0.18.0 → v0.20.0**
上游 diff 只动了 5 个包：
- `errors`：**纯增量**——新增 `Errorf`（格式化 + `%w` 包装，nil-safe）。
  `New`/`As`/`Is`/`Unwrap`/`Wrap` 是同型透传，一字未动。
- `httpjson`：**退避溢出修复**——旧写法 `base << (attempt-1)` 在 attempt 较大时
  int64 溢出成负数，负数绕过 `MaxDelay` 封顶，`time.After(负数)` 立刻返回
  ⇒ **该轮退避彻底消失**。改为乘法 + 饱和封顶。正常 attempt 数（1..20）下
  结果**逐字相同**，差别只在溢出的病态路径。
- `urlguard`：**纯增量**——新增 `WithExactHosts`，未设置时行为与既有完全一致。
- `stringx`：**仅注释**（补 `TruncEllipsis` 的 n 语义说明）。
- `pathx`：新增包（`SanitizeRel` 等）。
**本仓实际 import 的 `oekit/pkg` 子包：（本仓未直接 import oekit/pkg 的任何子包）**
与上游改动相交的：**（无）**
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```


- 文档(AGENTS)：补「发版顺序」小节，指向 org CONTRIBUTING 单一副本

13 个业务仓的 AGENTS.md 此前对发版顺序零覆盖（只有 textx 有个
「### 版本与发版」标题，但里面只讲 SemVer 与 module 路径），
发版顺序只写在 org 规则仓的 .github/CONTRIBUTING.md 里，单仓读者看不到。
本次只加**指针 + 本仓相关提醒**，runbook 本体保持**单一副本**
（CONTRIBUTING §发版），避免 13 份副本各自漂移。
覆盖四条：
- CHANGELOG.md 由 matrix-tools/scripts/gen_changelogs.py 从 git 历史生成，不要手改
- tag 必须落在 CHANGELOG 提交之后（下游 go get 拿到的是 tag 快照）
- 警告：无参重生成会吞掉待发版段，--next-version 是唯一能产出待发版段的开关
- 本仓相关提醒（见正文）


## [v0.10.1] — 2026-10-21

### 修复(go)：Go 地板降到 go 1.24.0，CI go-version 同步到 1.24

承接 matrix-tools#22（tier0 地板被无谓抬高）与 #34（tier1+ 排查结论）——
本仓是 tier3 编排层，但依赖链上**没有任何包要求高于 1.24 的 Go 版本**。
## 依据
- **依赖链全兼容**：llm v0.13.1 / textx v0.6.1 的 tag 快照都是 go 1.24.0
- **代码兼容 1.24**：真实 go1.24.0 工具链（`GOTOOLCHAIN=go1.24.0`）
  build + test 全绿——实跑，不是推断
- **门槛是 go 指令不是 toolchain**（编译器实证）
## toolchain 行的取舍：删掉
toolchain 不构成门槛，但 `toolchain go1.27.2` 会让 `go` 命令**自动下载**
1.27.2，与「最低支持版本是 1.24」相悖。删掉，也与基座 oekit 六个基础仓一致。
## 依赖地板同步重钉
`require` 抬到 **llm v0.13.1 / textx v0.6.1**（它们本轮刚发的、
tag 快照里才是 go 1.24.0 的新 tag）——否则 MVS 会拉到旧 tag 的 1.26 地板，
降级形同虚设。obs v0.9.2 经 `go mod tidy` 后不再是直接依赖，已移除。
## 改动
- `go.mod`：`go 1.26.0` → `go 1.24.0`；删 `toolchain go1.27.2`；
  内部 require 抬到 llm v0.13.1 / textx v0.6.1
- `.github/workflows/ci.yml`：`go-version: '1.26'` → `'1.24'`
CI 的 go-version 与 go.mod 的 go 指令**成对**——跨层通用纪律，不因本次降级而放松。
## 验证
- `GOTOOLCHAIN=go1.24.0 go build ./...` → exit 0
- `GOTOOLCHAIN=go1.24.0 go test ./...` → 全绿
- `go mod tidy` → 零差异
门禁见 oagentkit/matrix-tools#35（`scripts/verify_go_floor.py` 判据 4：
tier1+ 不得「依赖链全兼容却声明更高」）。
Closes oagentkit/matrix-tools#34

- 杂务(deps)：oagentkit/llm 依赖地板抬到 v0.14.2

llm v0.14.1 → v0.14.2 是**定价覆盖键归一化**的修复（oagentkit/llm#26 的续），
非破坏性变更，本仓无需改代码，只抬地板。
为什么必须先发这一版：agent 的 v0.10.1 待发版段里带着「Go 地板降到 1.24.0」
与「oekit 依赖地板抬到 v0.18.0」两项，但 go.mod 里 llm 还钉在 v0.14.1。
按矩阵的「打 tag 时刻地板必须是当时的最新」纪律，先补上这一处地板，
再打 v0.10.1 的 tag——否则 tag 快照会把一个已落后的地板钉死。
本地门禁：go mod tidy 无差异 / go build / go vet 通过。

- 杂务(deps)：oekit 依赖地板抬到 v0.18.0

oekit 姊妹矩阵的依赖地板对齐到最新：
- github.com/oekit/pkg        v0.17.0 → v0.18.0
- github.com/oekit/concurrency v0.17.1 → v0.18.0
- github.com/oekit/logx       已是最新 v0.7.1，不动
oekit v0.18.0 的两个发布都是**纯增量**，无破坏性变更：
- pkg：jsonrepair 补抽数组原语 ExtractArray；
- concurrency：新增 `DoValue[T]`（带返回值的重试），`Do` 改为委托给它——
  退避序列、抖动、MaxDelay 封顶、DelayHint 优先、Retryable 门控、ctx 取消
  **只有一份实现**（此前这类调用方只能自己重写一遍退避，且常丢掉 DelayHint 语义）。
依据 oagentkit/matrix-tools#44 的还债流程：依赖地板对齐到发版时刻的最新。
本仓本轮不发 tag（不在发版集内），地板在 main 上就位，下次发版自然带上。


## [v0.10.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（llm v0.11.0 / textx v0.5.0 / obs v0.7.0） (13)

matrix-tools 的 verify_dep_floors.py 首次全矩阵扫描查出的地板漂移，本仓是受影响仓之一。
本轮按 tier 拓扑顺序推进：tier0 → tier1 → tier2 依次先发，本仓最后把地板跟到
它们的新 tag——否则就是「发了新的、旧的立刻变落后」，永远收敛不了。
所升版本与原钉版本相比源码零改动，逐版实测确认：
- textx / obs / llm 的新旧版本 全树逐字节相同（diff -rq 退出码 0）
- oekit/pkg v0.14.0→v0.15.1  仅新增 urlguard 包（纯标准库，零新增三方依赖）
- oekit/concurrency v0.17.0→v0.17.1  仅三个文档文件
- oekit/logx v0.7.0→v0.7.1  仅 CI 配置与文档
无 API 变更、无兼容风险。
本地门禁：go mod tidy -diff 无输出 / go build / go vet / gofmt / go test -race 全通过。
关联 oagentkit/matrix-tools#13


## [v0.9.1] — 2026-10-19

- 杂务(deps)：oekit/logx 地板 v0.7.0 → v0.7.1

matrix-tools 的新守卫 verify_dep_floors.py 在本仓实跑时又查出一处 issue #8
未列出的漂移：logx v0.7.1 已在远端，本仓仍钉 v0.7.0。
实测两版差异：仅 .github/workflows/ci.yml、新增 issue-to-project.yml、
AGENTS.md / CHANGELOG.md / README.md —— **零 .go 改动**，无 API 变更。
补完后本仓 6 条第一方依赖地板全部等于远端最新。
本地门禁：go mod tidy -diff 无输出 / go build / go vet / go test -race 全通过。


## [v0.9.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（textx v0.4.0 / obs v0.5.0 / pkg v0.15.1 / concurrency v0.17.1）

llm 已由 v0.8.0 抬到 v0.9.0，本提交补齐剩余四处漂移。四者与仓库
当前钉住的版本相比，源码零改动，逐版实测确认：
- textx v0.3.0 → v0.4.0       全树逐字节相同（diff -rq 退出码 0）
- obs   v0.4.1 → v0.5.0       全树逐字节相同（退出码 0）
- oekit/pkg v0.14.0 → v0.15.1 仅新增 urlguard 包（纯标准库，pkg 的 go.mod
                             连 require 块都没有，零新增三方依赖）
- oekit/concurrency v0.17.0 → v0.17.1 仅三个文档文件，无 .go 改动
本地门禁：go mod tidy -diff 无输出 / go build / go vet / go test -race 全通过。


## [v0.8.0] — 2026-10-19

### 重构(agent)!：import 跟随 llm 包上移，依赖地板升到 v0.9.0

BREAKING CHANGE: 依赖 github.com/oagentkit/llm v0.9.0（该版起 import 路径为
github.com/oagentkit/llm，原 oagentkit/llm/llm 已移除）。
- agentrun/ reflection/ router/ 三处 import 由 llm/llm 改为 llm
- go.mod 地板 v0.7.2 → v0.9.0，go.sum 按直连 VCS 重算
llmtest 与 wrap 路径不变。

## [v0.7.0] — 2026-10-19

- 杂务(deps)：oekit 依赖升级 + 迁出被强推的 tag

两项改动：
1. oekit `concurrency` v0.16.0 → v0.17.0（姊妹矩阵依赖地板对齐）。
   v0.16.0...v0.17.0 仅 2 个提交：KeyedLimiter.Create 原子登记修复 + 文档，
   无 breaking，未触及任何已有导出签名。
2. 依赖迁到未被强推过的 tag。
   原先依赖的 tag 在发布后被 force 移到重写后的 main，内容与
   sum.golang.org 记录不一致，走 proxy.golang.org / GOPROXY=direct 的构建
   会 SECURITY ERROR。本地不报错是因为模块缓存预热 + goproxy.cn 还留着
   移动前的字节。已为这些模块重打不可变的附注 tag 并迁移过来。
   全部 14 个仓已启用 tag 保护规则集，禁止 tag 更新与删除。
go.sum 的 h1 全部取自 sum.golang.org，仅就地替换目标模块的两行。

- 杂务(deps)：oekit 依赖升级（llm v0.7.0→v0.7.1、obs v0.4.0→v0.4.1、concurrency v0.16.0→v0.17.0）

本 PR 含两项改动。
## 1. oekit 依赖地板
姊妹矩阵 oekit 的 `concurrency` 出了 v0.17.0，依赖地板一并对齐。
升级前已核对 `oekit/concurrency` v0.16.0...v0.17.0 的全部 2 个提交：
- `feat(ratelimit): KeyedLimiter.Create 护栏内原子登记 (Fixes #33)` — `ratelimit/keyed.go` +28 行
- `docs: 矩阵总览表 DESC 口径同步` — 仅 AGENTS.md / README.md / CHANGELOG.md
无 breaking / remove 标记，未触及任何已有导出签名，故为纯补丁式升级。
## 2. 迁出被 force 移动过的 tag
本仓原先依赖的姊妹仓 tag（发布后被强推到了重写后的 main）内容已与
sum.golang.org 记录不一致，于是任何走 `proxy.golang.org` 或 `GOPROXY=direct`
的构建都会 SECURITY ERROR。本地往往不报错——模块缓存预热 + goproxy.cn
还留着移动前的字节，把问题掩盖了。
已为 8 个模块重打不可变的**附注 tag**（内容与被移动 tag 指向同一 commit，
只换版本号，让校验和数据库重新记录一次正确哈希），并把依赖迁过去。
`go.sum` 的 h1 全部取自 sum.golang.org，仅就地替换目标模块的两行
（zip h1 + go.mod h1），其余行不动。


## [v0.6.2] — 2026-10-19

- 与 v0.6.1 相比无代码差异（版本号/打包变更）

## [v0.6.1] — 2026-10-19

### 修复(deps)：go.sum 里 llm v0.7.0、obs v0.4.0 的 h1 是 tag 重写**之前**算出来的

org 那次全量 tag/历史重写把同名 tag 指到了新提交，于是同名 tag 对应新内容，
而本仓 go.sum 仍记着重写前的 h1。后果是硬失败，不是「下载慢一点」：
    verifying github.com/oagentkit/<MOD>@<VER>: checksum mismatch
    SECURITY ERROR … the bits may have been replaced on the origin server
本仓最后一次 CI 绿在重写之前，所以这条红一直潜伏着，下次触发 CI 才会炸。
修法是**只替换陈旧的那几行**，不整体重算 go.sum。理由有二：
  1. 逐条比对后确认只有 llm v0.7.0、obs v0.4.0 这几个版本在本仓是陈旧的，整体 tidy 会把无关
     条目一起搅动；
  2. 重算时若沿用共享模块缓存，`tidy` 会复用重写前下载的副本，把旧哈希原样写回
     ——**命令成功、依据是错的**（门面仓 agentkit PR #5 第一次就这么被骗过：
     本地全绿、CI 仍红）。所以核对与验证都用空 GOMODCACHE + GOPRIVATE 直连 VCS，
     让每个第一方模块现取现算，再用 CI 报的 downloaded 哈希与本地 go.sum 逐字对账。
三方模块条目一条未动，sumdb 校验面没被关掉。

## [v0.6.0] — 2026-10-18

### 新增：agent 仓建立——Agent 编排（6 包）

从 agentkit 单仓切出 tier3 编排层模块，module 路径 github.com/oagentkit/agent。
收编 6 个包：agentrun（ReAct + 计划先行）/ reflection（反思循环）/ router（意图路由）/
blackboard（多专家黑板）/ dispatch（派发守卫）/ clarify（澄清词表内核）
矩阵内依赖：llm、textx；llm 的传递依赖 obs、runtime 自动正确解析。
AGENTS.md 三条纪律：编排层不得绕过 llm/mcpkit/toolgate 门面直连底层、
反思与黑板协作必须有深度上限与终止条件、派发 allow 矩阵默认拒绝且拒绝自派发。
门禁：go build / go vet / go test -race 6 包全绿，gofmt 干净。

- 杂务(deps)：依赖地板升到「历史最新 + 0.1.0」

本轮做了版本线重置：清空全部历史 tag，按「历史最新 + 0.1.0」重打新版本
（如 llm v0.6.0 → v0.7.0、toolgate v0.6.1 → v0.7.0、agentkit v1.3.0 → v1.4.0）。
各仓 go.mod 的矩阵内依赖地板同步跟进到新版本号。
**go.sum 的 oagentkit 行已整段删除**，由 CI 的 `go mod tidy` 重算：
新 tag 的内容与任何历史版本都不同，`h1` 必须现算；本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现）。
重算依赖 `.github` 的 go-ci 配了 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 缓存着旧版本，若走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
（第二个提交会补上重算好的 go.sum。）
* chore(deps): 依赖地板升到「历史最新 + 0.1.0」
chore(deps): 依赖地板升到「历史最新 + 0.1.0」
本轮做了版本线重置：清空全部历史 tag，按「历史最新 + 0.1.0」重打新版本
（如 llm v0.6.0 → v0.7.0、toolgate v0.6.1 → v0.7.0、agentkit v1.3.0 → v1.4.0）。
各仓 go.mod 的矩阵内依赖地板同步跟进到新版本号。
**go.sum 的 oagentkit 行已整段删除**，由 CI 的 `go mod tidy` 重算：
新 tag 的内容与任何历史版本都不同，`h1` 必须现算；本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现）。
重算依赖 `.github` 的 go-ci 配了 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 缓存着旧版本，若走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
（第二个提交会补上重算好的 go.sum。）
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。

- 杂务(deps)：跟进最新 tag

oagentkit/llm v0.5.0→v0.6.0、oagentkit/textx v0.1.0→v0.2.0、oagentkit/obs v0.2.1→v0.3.0
go.sum 取 sumdb 权威 h1，仅就地替换目标 mod 两行。

- 杂务(deps)：oekit 依赖升级

concurrency v0.13.1→v0.16.0、pkg v0.13.0→v0.14.0
姊妹矩阵 oekit 的依赖地板此前被漏掉（上一轮只扫了 oagentkit 内部依赖），本次一并对齐到最新。
升级前已确认 oekit/concurrency v0.13.1..v0.16.0 的 8 个提交全是 feat/docs/chore，
无 breaking/remove 标记。其中 #28 订正了 `WithCooldown(0)` 的语义——
即 llms.txt 里警告过的那个陷阱，升上去正好拿到修正后的行为。
go.sum 的 h1 取自 sumdb，仅就地替换目标 mod 的两行、其余行不动。

- 杂务(deps)：llm v0.4.0 → v0.5.0（本轮新发 tag 的连锁升级）

llm v0.5.0 已发布（本次依赖地板升级），本仓跟进。
go.sum 的 h1 取自 sumdb，仅就地替换该 mod 的两行。

- 杂务(deps)：跨仓依赖 llm v0.4.0 / obs v0.2.1，并移除已不需要的 runtime

全 org 跨仓依赖地板统一升级。
llm v0.4.0 起不再依赖 runtime（熔断已全量委托 oekit/concurrency/breaker），
因此本仓的 runtime 传递依赖整条消失——go.mod 的 require 行与 go.sum 的两行一并删除。
这是 `go mod tidy` 的真实结果，不是手工猜测。
go.sum 的 h1 取自 sumdb（经 goproxy.cn 代理），即 `go mod tidy` 会写入的权威值。

- 杂务(deps)：oekit/concurrency v0.12.0 -> v0.13.1

本仓不使用上游 breaker（熔断只在 llm 仓），v0.13.0 的 #25 修复与
v0.13.1 的 #26（RetryableWithHeaders 头值 OWS 修剪）对本仓无直接影响，
本次为矩阵内版本对齐。
门禁：go mod tidy -diff / build / vet /
test -race / gofmt 全绿。

- 杂务(deps)：oekit 三模块升至最新（pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0）

拿到上游新增能力：
- concurrency #22 RetryableWithHeaders：403 头感知判定（次级限流 / 主限流耗尽）
- concurrency #23 FanInLimit：受限并发扇出
- concurrency #24 WithTokenClock、#21 Lockout（keyed lockout/penalty）
- logx #12 TraceHandler：给裸 slog.Handler 注入 trace_id
破坏性变更核对（均无影响）：
- logx v0.7.0 ParseLogLevel 签名改为 (Level, error) —— 我们未使用该函数
- concurrency v0.9.0 的 Slot.Panicked 废弃 —— rag 已迁到 Slot.Err + errors.As
门禁：go build / go vet / go test -race 全绿。

- 杂务(deps)：oekit/concurrency v0.6.0 -> v0.9.0

拿到上游显式三态状态机（#17）与 singleflight 可观测折叠（#18），并为本次
breaker 替换评估了语义差异。
用到的子包只有 async / singleflight / syncx —— 上游 breaker 包我们并未引用
（runtime/breaker 是本仓自己的三态实现），所以本次升级零 breaking。
关于 breaker 替换的现状：#17 已落地（closed/open/half-open + 代绑定 + 探测时限 +
开放期失败不延长冷却），但两侧的触发语义仍不对齐，已在上游补两个需求：
- #19 冷却期与探测失联时限共用 probeTimeout，无法独立配置
- #20 缺「连续失败 N 次即熔断」（trip）触发
因此本包暂不删除 —— llm.TestResilientBreakerTrips 钉的是 trip 语义，
上游 v0.9.0 只有速率型触发（dropRatio > 0），现在换会让该测试失败。


- CI：接入 org 共享门禁（规则本体在 oagentkit/.github）
