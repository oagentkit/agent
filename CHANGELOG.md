# agent CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.10.6] — 2026-10-23

### 新增(router)：决策面 —— System One 等非自回归判定模型可先于 LLM 分类

router 此前只有**一条**分类执行体：LLM（自回归生成）。但「把输入路由到**有限候选集**」
这件事本身不需要生成——它是非自回归**判定模型**（System One 协议：Jev 托管 / Laya 自托管 /
Kev / Clef / CLM）的主场：一次前向 pass 答完全部问题，输出**有限标签 + 概率分布 + 校准
置信度**，毫秒级、几乎免费。

⇒ router 有了**三种执行体**：词面（确定性，本包 KeywordHit）→ 决策面（判定，毫秒级）
→ LLM 面（生成，秒级）。决策面夹在词面与 LLM 面之间：比词面能理解语义，比 LLM 面快且便宜。

* `DecisionFace` 窄接口：实现方（客户端包）负责把候选集/槽位规格映射成判定题，并
  **按自己的口径**返回校准置信度；本包只管编排语义。
* `Config.DecisionFace` / `DecisionMinConfidence` / `DecisionShadow` / `DecisionObserver`。
* `Decision.Source`（llm | decision）：判定来源可观测（零值即 llm，向后兼容）。
* 编排：**决策面先于 LLM 面**；达门槛即采纳（省一次 LLM 分类调用），未达门槛 / 端点故障
  ⇒ 回落 LLM 面（fail-open：判定面是加速件，**不得阻断主链**）。
* 影子模式（`DecisionShadow`）：调用决策面但**只记不采**（结果进 `DecisionObserver`，
  行为面零变更）——上线前量一致率与延迟的标准姿势。
* 自守恒：决策面给的路由名不在表内 ⇒ 拒（回落）；槽位只保留**本包配置过的槽位名**
  （与 LLM 面同纪律，防幻觉槽位名透传）；置信度越界钳位到 [0,1]。

取舍：
* **不直连客户端实现**：本包只认窄接口，具体客户端（System One 协议）落在模型侧
  （oagentkit/llm#35 / PR #36）——遵循「编排层不得直连底层实现」的仓纪律，且本仓
  **不新增仓依赖边**。
* **决策面先行而非 LLM 先行**：判定模型的输出**可校准**（概率分布算出的置信度），
  而 LLM 的自评置信度不可靠；代价是「判定面自信但判错」时 LLM 无机会纠正 ⇒ 用
  影子模式先量一致率、门槛随候选集标定来兜。
* **门槛口径提示**：判定置信度随候选集规模变化（候选越多、同一 p_top 折出的置信度越低）
  ⇒ 门槛不可跨候选集移植，须随候选集标定。

门禁：gofmt -l（空）/ go vet / go build ./... / go test -race -count=1 ./... 全过
（router 新增 7 条：采纳/低置信回落/端点故障回落/未知路由回落/影子不采纳/槽位自守恒/
无决策面时行为不变）。

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
