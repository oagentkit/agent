# AGENTS.md — oagentkit/agent

> Agent 编排——ReAct 运行样板/计划先行/反思循环/意图路由/多专家黑板/派发守卫
> module：`github.com/oagentkit/agent` · org：[oagentkit = Open Agent Kit](https://github.com/oagentkit)
> 矩阵分层：**tier 3**（编排层，依赖 llm 等 tier2 门面）

## 本仓包含

```
`agentrun/`
`reflection/`
`router/`
`blackboard/`
`dispatch/`
`clarify/`
```

## 硬规则

### 分层与依赖

- 依赖严格自底向上，**禁止反向依赖**。本仓是 tier 3，只允许依赖 tier ≤ 3 的仓。
- 本仓依赖的矩阵内仓：`llm`、`textx`。
- 改 module 路径必须同步：全部 import、go.mod、README 安装行。

### 错误处理

- 一律使用 `github.com/oekit/pkg/errors`；业务错误用 BizError。
- 所有 error 禁止 `_` 忽略。

### 并发安全

- 禁止裸 `go func`：用 `github.com/oekit/concurrency/async` 的 GoSafe 系列（panic 被 recover）。
- 共享状态用 mutex / atomic 保护；包级全局可变状态用 `atomic.Pointer[T]`。

### API 设计

- 函数选项模式命名 `XxxOption`（禁止裸 `Option`，多包 co-import 会冲突）。
- 导出符号 PascalCase + godoc 注释；包文档放 `doc.go`。

### 日志

- 日志走 `oekit/logx`，组件 logger 用 `logx.Delegating("oagentkit-agent")`。
- 面向用户的最终输出落日志前过 `guardx/fence` 的不可信文本中和。

## 仓特有纪律

- **编排层不得直连底层实现。** 所有能力经 `llm`/`mcpkit`/`toolgate` 等门面进入，
  不得绕过门面直接调 provider/MCP client。
- 反思循环与黑板协作必须有**深度上限与终止条件**，防止无限循环。
- 派发守卫的 allow 矩阵是白名单，默认拒绝；并有**自派发拒绝**（防止无限递归）。

## 明确不属于本仓的

| 能力 | 归属 |
|---|---|
| 文本/差异/会话/报告 | `github.com/oagentkit/textx` |
| 事件总线/流控/子进程/热插拔/钩子 | `github.com/oagentkit/runtime` |
| 脱敏/出口围栏/权限/沙箱解析/审计 | `github.com/oagentkit/guardx` |
| DB 队列与选主 | `github.com/oagentkit/queue` |
| 通用 errors/stringx/encoding/mathx | `github.com/oekit/pkg` |
| goroutine 池/单飞/重试/令牌桶 | `github.com/oekit/concurrency` |
