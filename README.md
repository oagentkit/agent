# agent

Agent 编排——ReAct 运行样板/计划先行/反思循环/意图路由/多专家黑板/派发守卫 —— oagentkit 模块矩阵 **tier 3**（编排层，依赖 llm 等 tier2 门面）。

```
go get github.com/oagentkit/agent
```

| 包 | 职责 |
|---|---|
| `agentrun` | ReAct 运行样板与计划先行（PlanAndExecute）编排 |
| `reflection` | 反思循环 |
| `router` | 显式意图路由（三种执行体：词面 → **决策面**（System One 等非自回归判定模型，毫秒级）→ LLM 面（生成）） |
| `blackboard` | 无中心多专家协作黑板 |
| `dispatch` | 通用派发守卫：allow 矩阵 + 深度上限 + 自派发拒绝 |
| `clarify` | 澄清与标准化词表内核 |

## 定位

**tier 3** — 编排层，依赖 llm 等 tier2 门面。

- 本仓依赖的矩阵内仓：`llm`、`textx`。

## 外部依赖地板

- `github.com/cloudwego/eino`

## 与 oekit 的关系

本矩阵（oagentkit，Agent/LLM 编排工具链）**依赖** oekit（Open Easy Kit，Go 微服务工具链）：
`oekit/pkg` / `oekit/concurrency` / `oekit/logx` 等。oekit 是另一套独立矩阵，不是子品牌。

## 开发

```bash
go test ./... -race
go vet ./...
gofmt -l .
```

规范见 [AGENTS.md](AGENTS.md)。

## License

MIT
- **编排层不得直连底层实现。** 所有能力经 `llm`/`mcpkit`/`toolgate` 等门面进入，
  不得绕过门面直接调 provider/MCP client。
- 反思循环与黑板协作必须有**深度上限与终止条件**，防止无限循环。
- 派发守卫的 allow 矩阵是白名单，默认拒绝；并有**自派发拒绝**（防止无限递归）。