# agent CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.10.1] — 2026-10-20

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

## [v0.10.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（llm v0.11.0 / textx v0.5.0 / obs v0.7.0） (13)

## [v0.9.1] — 2026-10-19

- 杂务(deps)：oekit/logx 地板 v0.7.0 → v0.7.1

## [v0.9.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（textx v0.4.0 / obs v0.5.0 / pkg v0.15.1 / concurrency v0.17.1）

## [v0.8.0] — 2026-10-19

### 重构(agent)!：import 跟随 llm 包上移，依赖地板升到 v0.9.0

BREAKING CHANGE: 依赖 github.com/oagentkit/llm v0.9.0（该版起 import 路径为
github.com/oagentkit/llm，原 oagentkit/llm/llm 已移除）。
- agentrun/ reflection/ router/ 三处 import 由 llm/llm 改为 llm
- go.mod 地板 v0.7.2 → v0.9.0，go.sum 按直连 VCS 重算
llmtest 与 wrap 路径不变。

## [v0.7.0] — 2026-10-19

- 杂务(deps)：oekit 依赖升级 + 迁出被强推的 tag
- 杂务(deps)：oekit 依赖升级（llm v0.7.0→v0.7.1、obs v0.4.0→v0.4.1、concurrency v0.16.0→v0.17.0）

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
- 杂务(deps)：跟进最新 tag
- 杂务(deps)：oekit 依赖升级
- 杂务(deps)：llm v0.4.0 → v0.5.0（本轮新发 tag 的连锁升级）
- 杂务(deps)：跨仓依赖 llm v0.4.0 / obs v0.2.1，并移除已不需要的 runtime
- 杂务(deps)：oekit/concurrency v0.12.0 -> v0.13.1
- 杂务(deps)：oekit 三模块升至最新（pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0）
- 杂务(deps)：oekit/concurrency v0.6.0 -> v0.9.0

- CI：接入 org 共享门禁（规则本体在 oagentkit/.github）
