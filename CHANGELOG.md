# agent CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

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
