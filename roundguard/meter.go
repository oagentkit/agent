package roundguard

import "sync"

// Meter 进程内回合账（只数不判）。
//
// **进程内**：跨进程续跑时回合数**不随之累计**——跨进程累计需要存储形态，那是宿主
// 资产；要跨进程累计的宿主须自行落库并把 Used 灌回本包。
//
// **并发安全**：带锁。判据包不该把加锁方式留给调用方各自发明——宿主各写一遍加锁
// 方式 = 同一种并发缺陷的 N 个变体。
//
// **零值可用**：`var m roundguard.Meter`（或把它当 struct 字段零值起步）即可
// 直接记账，不必先调 NewMeter——Go 的默认契约是零值可用，导出类型的零值不可用
// 会在宿主循环里变成**运行期 panic**（见 Tool 里的惰性初始化）。
type Meter struct {
	mu      sync.Mutex
	rounds  int
	calls   int
	perTool map[string]int
}

// NewMeter 构造一个空的回合账。
//
// 与零值等效（零值 Meter 同样可用）——保留它是为了给「显式构造」一个入口，
// 以及让宿主在代码里一眼看出用的是哪种起步方式。
func NewMeter() *Meter {
	return &Meter{perTool: make(map[string]int)}
}

// Round 记一回合并返回累计回合数。
func (m *Meter) Round() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rounds++
	return m.rounds
}

// Tool 记一次工具调用并返回该工具的累计调用数。
func (m *Meter) Tool(toolID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	// 惰性初始化：零值 Meter 的 perTool 是 nil map，往 nil map 写会 panic
	//（assignment to entry in nil map）⇒ 宿主按 Go 惯例写 'var m roundguard.Meter'
	// 就会在运行期炸掉，且编译期完全看不出来。
	//
	// 为什么在**锁内**初始化：map 的并发写必须与读同锁，否则「懒初始化」本身
	// 就是一个数据竞争（两个 goroutine 同时发现 nil 并各建一张表 ⇒ 计数丢失）。
	if m.perTool == nil {
		m.perTool = make(map[string]int)
	}
	m.perTool[toolID]++
	return m.perTool[toolID]
}

// Used 导出用量快照（副本——调用方改写它不影响记账）。
func (m *Meter) Used() Used {
	m.mu.Lock()
	defer m.mu.Unlock()
	tools := make(map[string]int, len(m.perTool))
	for k, v := range m.perTool {
		tools[k] = v
	}
	return Used{Rounds: m.rounds, ToolCalls: m.calls, Tools: tools}
}
