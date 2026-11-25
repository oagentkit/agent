package roundguard_test

import (
	"go/ast"
	"slices"
)

// ── 导出面计数口径（判据在 exportbudget_test.go，这里是它的执行面）──
//
// 口径（什么算、什么不算）写在门禁文件的头注里——那里是**规则的声明面**，
// 本文件是**规则怎么落地**。两者分开放的理由与本仓「声明面 / 执行面分离」同款：
// 规则改在声明面，执行面不动；反过来，执行面被放松时，门禁文件旁的自检用例
// （exportbudget_rules_test.go 的 TestCountingRules*）会当场变红。
//
// 为什么全走 go/ast 而不逐行看文本：import / 声明 / 字段 / 方法的书写形态远不止
// 一种，逐行正则必然漏，且漏得毫无声响（与本仓 archcheck 当年同一个病）。
//
// 包级模型（类型、方法声明、接口方法集的跨文件汇总）与 AST 辅助在
// exportbudget_model_test.go。

// deliveryExposed 该类型名是否在**交付面**上：名字导出，或被某个导出别名指向。
//
// 为什么要有这个「或」：`type Alias = inner`（inner 未导出）时，宿主 import 得到
// `Alias` 就拿到了 inner 的字段与方法 ⇒ inner 的**成员面**在交付面上。
// 而「名字」仍不在（宿主拿不到 `inner`）⇒ 两件事要分开判。
func (m *packageModel) deliveryExposed(typeName string) bool {
	return ast.IsExported(typeName) || m.aliasExposed[typeName]
}

// countExportedSymbols 数一个文件里的导出符号数（口径见门禁文件头注）。
func countExportedSymbols(f *ast.File, m *packageModel) int {
	n := 0
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				// 包级函数。
				if d.Name.IsExported() {
					n++
				}
				continue
			}
			// 导出方法：方法名导出 **且** 接收者类型**在交付面上**
			//（名字导出，或被某个导出别名指向——`type Alias = inner` 时宿主
			// import 得到 Alias 就拿到了 inner 的方法；未导出类型上的方法宿主
			// import 不到——除非它被**提升**进某个导出类型，那一路在类型声明处
			// 按「提升方法」计，见 methodView）。
			if d.Name.IsExported() && m.deliveryExposed(receiverTypeName(d.Recv)) {
				n++
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						n++
						n += m.countTypeMembers(s)
						continue
					}
					// 未导出，但**被某个导出别名指向**（`type Alias = inner`）⇒
					// 它的成员面对宿主可见 ⇒ 计入；**名字**不计入（宿主拿不到
					// 这个名字）。
					if m.aliasExposed[s.Name.Name] {
						n += m.countTypeMembers(s)
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							n++
						}
					}
				}
			}
		}
	}
	return n
}

// countTypeMembers 数一个导出类型的**成员面**。
//
// 结构体：导出具名字段 + 导出嵌入字段 + 提升进来的导出方法。
// 接口：导出方法集 + 提升进来的导出方法。
func (m *packageModel) countTypeMembers(ts *ast.TypeSpec) int {
	switch t := ts.Type.(type) {
	case *ast.StructType:
		return m.countStructMembers(ts, t)
	case *ast.InterfaceType:
		return m.countInterfaceMembers(ts, t)
	}
	return 0
}

// countStructMembers 数导出结构体的成员面。
func (m *packageModel) countStructMembers(ts *ast.TypeSpec, st *ast.StructType) int {
	n := 0
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// 嵌入字段：嵌入的**导出**类型名本身成为本包的导出名（+1）。
			// 跨包嵌入的导出类型同样 +1（那个名字宿主调得到）；只是它的**提升
			// 方法**不在本包测量面内（口径的已知边界）。
			if ast.IsExported(embeddedFieldName(field.Type)) {
				n++
			}
			continue
		}
		for _, name := range field.Names {
			if name.IsExported() {
				n++
			}
		}
	}
	// 经嵌入**提升**上来的导出方法（深度 ≥1；深度 0 是本类型自己的方法，已在
	// FuncDecl 分支计过）。来源是**导出**嵌入类型的 ⇒ 已在它自己的声明处计过 ⇒
	// 不重复计。
	for _, e := range m.methodView(ts.Name.Name) {
		if e.depth >= 1 && !e.exported {
			n++
		}
	}
	return n
}

// countInterfaceMembers 数导出**接口的方法集**。
//
// 为什么接口方法必须计入：给导出接口加一个方法，宿主侧的实现方就被迫多实现
// 一个 ⇒ **破坏性变更**。只数「接口类型本身」的话，这类改动对计数完全不可见
// ⇒ 「精确计数」在一条增长路径上彻底失明。
//
// ⚠️ 嵌入的接口**名**不产生导出名（接口嵌入不产生字段名）⇒ 这里不计名字；
// 它贡献的**方法集**按 Go 的提升规则统一处理（下面那段）。
func (m *packageModel) countInterfaceMembers(ts *ast.TypeSpec, it *ast.InterfaceType) int {
	n := 0
	for _, field := range it.Methods.List {
		if len(field.Names) == 0 {
			continue // 嵌入接口：名字不计，方法集按提升规则处理
		}
		for _, name := range field.Names {
			if name.IsExported() {
				n++
			}
		}
	}
	for _, e := range m.methodView(ts.Name.Name) {
		if e.depth >= 1 && !e.exported {
			n++
		}
	}
	return n
}

// viewEntry 一个导出方法名在某类型的嵌入链上的落点。
type viewEntry struct {
	// depth 最浅深度：该类型自己声明的方法 = 0；直接嵌入的类型的方法 = 1；
	// 再往里每层 +1。
	depth int
	// exported 来源的**直接嵌入类型**是否导出 ⇒ 该名字已在它自己的声明处计过，
	// 不能在宿主类型这里再计一次（否则同一个方法被数两次）。
	exported bool
}

// methodView 解析类型 T 的方法视图：导出方法名 → 它在 T 的嵌入链上的落点。
//
// **只收导出方法名**：未导出的名字宿主调不到，不属交付面。
//
// Go 的选择器规则（**这就是本视图的口径**）：
// ① **浅层赢**：浅层出现过的名字，深层同名被**遮蔽**（不是歧义）。
// ② **同层多来源即歧义**：同一深度上多个嵌入类型带同名 ⇒ Go 里它是歧义选择器、**不可调用** ⇒ 不属交付面 ⇒ 不计。
//
// ⚠️ **方法集 ≠ 声明**：接口的方法集写在类型声明里（*ast.InterfaceType），**不是**
// FuncDecl ⇒ 只看声明就看不见嵌入接口贡献的方法集（声明面宣称覆盖、执行面没
// 实现 ⇒ 声明/执行漂移）。
func (m *packageModel) methodView(typeName string) map[string]viewEntry {
	return m.methodViewPath(typeName, map[string]bool{})
}

// methodViewPath methodView 的递归实现。
//
// path 是**路径式**防环（进入时标记、返回时清除）：同一类型可以在不同分支各
// 出现一次（那是合法的嵌入结构），只有「自己嵌自己」才要截断。
func (m *packageModel) methodViewPath(typeName string, path map[string]bool) map[string]viewEntry {
	out := map[string]viewEntry{}
	if typeName == "" || path[typeName] {
		return out
	}
	path[typeName] = true
	defer delete(path, typeName)

	// levels[深度][名字] = 各来源「直接嵌入类型是否导出」的列表。
	// 同一深度上多个来源 ⇒ 歧义 ⇒ 该名字不被提升。
	levels := map[int]map[string][]bool{}

	// 深度 0：T 自己声明的导出方法（FuncDecl）+ T 作为接口时它方法集里的导出方法。
	own := map[string]bool{}
	for name := range m.methods[typeName] {
		own[name] = true
	}
	for name := range m.ifaces[typeName] {
		own[name] = true
	}
	if len(own) > 0 {
		lvl := map[string][]bool{}
		for name := range own {
			lvl[name] = []bool{m.deliveryExposed(typeName)}
		}
		levels[0] = lvl
	}

	// 深度 ≥1：T 的各个**直接嵌入类型**各自贡献（方法集按 Go 的提升规则解析）。
	for _, inner := range m.embeddedTypes(typeName) {
		innerExported := m.deliveryExposed(inner)
		for name, e := range m.methodViewPath(inner, path) {
			lvl := levels[e.depth+1]
			if lvl == nil {
				lvl = map[string][]bool{}
				levels[e.depth+1] = lvl
			}
			lvl[name] = append(lvl[name], innerExported)
		}
	}

	// 浅层赢 + 同层多来源即歧义 ⇒ 只保留「最浅深度上恰好一个来源」的名字。
	depths := make([]int, 0, len(levels))
	for d := range levels {
		depths = append(depths, d)
	}
	slices.Sort(depths)
	claimed := map[string]bool{}
	for _, d := range depths {
		for name, sources := range levels[d] {
			if claimed[name] {
				continue // 浅层已占位 ⇒ 深层同名被遮蔽
			}
			if len(sources) > 1 {
				continue // 歧义 ⇒ 该名字不被提升（Go 里不可调用）
			}
			claimed[name] = true
			out[name] = viewEntry{depth: d, exported: sources[0]}
		}
	}
	return out
}
