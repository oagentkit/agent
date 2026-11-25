package roundguard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// ── 导出面计数的包级模型与 AST 解析（口径在 exportbudget_count_test.go）──
//
// 为什么需要**包级模型**（跨文件）：导出面不是「逐个文件数一遍」能得到的——
//  ① 「未导出类型被导出类型嵌入 ⇒ 它的导出方法被提升进导出面」这条关系要跨文件
//     才看得出来（嵌入在 A 文件、被嵌入的类型声明在 B 文件）；
//  ② 「给导出接口加一个方法 ⇒ 宿主实现方被迫多实现一个」这条关系必须按**方法集**
//     而不是按**声明**来数；
//  ③ 「嵌入的接口贡献的方法集」**根本不是声明**——方法集写在类型声明
//     （*ast.InterfaceType）里，**永远不是 FuncDecl** ⇒ 只收 FuncDecl 的话，
//     嵌入接口的方法集对计数器**完全不可见**（声明面宣称覆盖、执行面没实现 ⇒
//     声明/执行漂移）。
// 故先把整个包（生产文件）的类型、**方法声明**与**接口方法集**收成三张表，再计数。

// packageModel 汇总一个包的生产文件里的类型、方法声明与接口方法集。
//
// methods 只收**导出方法名**（方法名导出即可；接收者类型导不导出是另一层判定）：
// 接收者类型是否导出，决定这条方法是不是「宿主 import 得到的名字」；而
// 「被提升进导出面的方法」用的正是「方法名导出」这一半。
//
// ifaces 收**接口的方法集**（导出方法名，且只收**直接声明**的那些）：方法集写在
// 类型声明里，不是声明 ⇒ 缺了这张表，嵌入接口贡献的方法集**永远不可见**。
type packageModel struct {
	types   map[string]*ast.TypeSpec
	methods map[string]map[string]bool // 接收者类型名 → 导出方法名（FuncDecl）
	ifaces  map[string]map[string]bool // 接口类型名 → 其方法集里的导出方法名

	// aliasExposed 本包里被**导出别名**指向的类型名（别名链追到底的最终类型）。
	//
	// 为什么需要这张表：`type Alias = inner`（inner **未导出**）时，宿主 import
	// 得到的是 `Alias`，而 `Alias` 与 `inner` 是**同一个类型** ⇒ inner 的导出字段
	// 与导出方法**对宿主可见** ⇒ 属交付面 ⇒ 必须计入。而 `inner` 这个**名字**
	// 宿主拿不到 ⇒「未导出 ⇒ 不计入」那条规则对它的**成员面**不成立、对它的
	// **名字**仍成立。
	//
	// ⚠️ 这张表只登记**本包内**的目标：跨包别名（`type Alias = pkg.T`）的目标
	// 方法集不在本包 AST 里 ⇒ 不计入本包测量面（与「跨包嵌入类型的提升方法」
	// 同款边界）。
	aliasExposed map[string]bool
}

// newPackageModel 收一张包级表（类型声明 + 方法声明 + 接口方法集）。
func newPackageModel(files []*ast.File) *packageModel {
	m := &packageModel{
		types:   make(map[string]*ast.TypeSpec),
		methods: make(map[string]map[string]bool),
		ifaces:  make(map[string]map[string]bool),
	}

	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil || !d.Name.IsExported() {
					continue
				}
				recv := receiverTypeName(d.Recv)
				if recv == "" {
					continue
				}
				if m.methods[recv] == nil {
					m.methods[recv] = map[string]bool{}
				}
				m.methods[recv][d.Name.Name] = true
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, dup := m.types[ts.Name.Name]; !dup {
						m.types[ts.Name.Name] = ts
					}
					// 接口的方法集：**不是声明** ⇒ 必须单收一张表。
					if it, ok := ts.Type.(*ast.InterfaceType); ok {
						m.ifaces[ts.Name.Name] = interfaceMethodNames(it)
					}
				}
			}
		}
	}
	// 别名必须**先收齐全部类型声明**才能解析（别名链可以指向后面才声明的类型）
	// ⇒ 这张表在收类型的循环**之后**才算。
	m.aliasExposed = collectAliasExposed(m.types)
	return m
}

// collectAliasExposed 找出「被**导出别名**指向」的本包类型名。
//
// 口径三条：
//
//	① 别名的**名字**必须导出（宿主 import 得到的是它）⇒ 别名自己未导出 ⇒ 它的
//	   目标对宿主**不可见** ⇒ 不算；
//	② 目标链要**追到底**（`type A = b` + `type b = c` ⇒ 最终目标是 c）；
//	③ 目标必须**在本包内**（跨包别名 ⇒ 目标方法集不在本包 AST 里 ⇒ 不计入本包
//	   测量面，与「跨包嵌入类型的提升方法」同款边界）。
func collectAliasExposed(types map[string]*ast.TypeSpec) map[string]bool {
	out := map[string]bool{}
	for name, ts := range types {
		if ts.Assign == 0 || !ast.IsExported(name) {
			continue
		}
		if target := resolveAliasTarget(types, ts); target != "" {
			out[target] = true
		}
	}
	return out
}

// resolveAliasTarget 追别名链到底，返回**本包内**的最终目标类型名。
//
// 空串 = 不是本包类型（跨包 / 泛型别名 / 链断了 / 自环）。
func resolveAliasTarget(types map[string]*ast.TypeSpec, ts *ast.TypeSpec) string {
	seen := map[string]bool{}
	cur := ts
	for cur != nil && cur.Assign != 0 {
		id, ok := cur.Type.(*ast.Ident)
		if !ok {
			return "" // 跨包（pkg.T）或泛型实例 ⇒ 不在本包测量面内
		}
		if seen[id.Name] {
			return "" // 自环（非法代码，防御）
		}
		seen[id.Name] = true
		next, ok := types[id.Name]
		if !ok {
			return "" // 本包里没有这个类型
		}
		cur = next
	}
	if cur == nil {
		return ""
	}
	return cur.Name.Name
}

// interfaceMethodNames 取一个接口**直接声明**的导出方法名。
//
// 嵌入的接口（interface { io.Reader }）的方法集不在这里——它们要么在**它自己**
// 的声明处被计过，要么属跨包边界（不可从本包 AST 解析）。
func interfaceMethodNames(it *ast.InterfaceType) map[string]bool {
	out := map[string]bool{}
	if it == nil || it.Methods == nil {
		return out
	}
	for _, field := range it.Methods.List {
		for _, name := range field.Names {
			if name.IsExported() {
				out[name.Name] = true
			}
		}
	}
	return out
}

// embeddedTypes 列出一个类型**直接嵌入**的**本包**类型名。
//
// 跨包嵌入（pkg.Inner）不返回：它的方法集不在本包 AST 里 ⇒ 本包解析不到 ⇒
// 其提升方法**不计入本包测量面**（口径的已知边界，见门禁文件头注）。
func (m *packageModel) embeddedTypes(typeName string) []string {
	ts, ok := m.types[typeName]
	if !ok {
		return nil
	}
	return embeddedLocalNames(ts.Type)
}

// exportedSurface 实测一个目录下的导出面（总数 + 逐文件）。
func exportedSurface(t *testing.T, dir string) (int, map[string]int) {
	t.Helper()
	files, err := productionGoFiles(dir)
	if err != nil {
		t.Fatalf("列生产文件失败：%v", err)
	}
	parsed := make([]*ast.File, 0, len(files))
	for _, path := range files {
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("解析 %s 失败：%v", path, perr)
		}
		parsed = append(parsed, f)
	}
	// 包级模型必须**先建后数**：提升关系与方法集都要跨文件看。
	model := newPackageModel(parsed)
	total := 0
	perFile := make(map[string]int, len(files))
	for i, path := range files {
		n := countExportedSymbols(parsed[i], model)
		perFile[filepath.Base(path)] = n
		total += n
	}
	return total, perFile
}

// productionGoFiles 列目录下的**生产** .go 文件（排除 _test.go）。
//
// 排除 _test.go 是口径的一部分，不是实现细节：门禁数的是**交付面**（宿主
// import 得到的符号），测试函数宿主 import 不到。
//
// ⚠️ **只列本目录**（不递归子目录）：测量面 = 本包目录下的生产文件。将来若
// 出现 roundguard/xxx 子包，它的导出符号对这道门禁**不可见**（那时要另设门禁
// 或把子包纳入）——这是口径的已知边界，不是隐含假设。
func productionGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	slices.Sort(out)
	return out, nil
}

// receiverTypeName 取接收者类型名（剥指针、剥泛型索引）。
//
// 跨包接收者（a.T）取不到本包的类型名 ⇒ 返回空串 ⇒ 不计入（那种方法不可能声明
// 在本包文件里，但口径上要认得出来）。
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return baseTypeName(recv.List[0].Type)
}

// baseTypeName 取类型表达式的名字（剥指针与泛型索引）。
func baseTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return baseTypeName(e.X)
	case *ast.IndexExpr:
		return baseTypeName(e.X)
	case *ast.IndexListExpr:
		return baseTypeName(e.X)
	}
	return ""
}

// embeddedFieldName 取嵌入字段的类型名（它同时是本包暴露出去的那个名字）。
//
// 跨包嵌入（pkg.Inner）也返回名字——那个名字宿主调得到 ⇒ 计入；只是它的**提升
// 方法**不在本包测量面内（见 embeddedLocalName 的边界说明）。
func embeddedFieldName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return embeddedFieldName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.IndexExpr:
		return embeddedFieldName(e.X)
	case *ast.IndexListExpr:
		return embeddedFieldName(e.X)
	}
	return ""
}

// embeddedLocalNames 列出一个类型（结构体或接口）里嵌入的**本包**类型名。
//
// 跨包嵌入（*ast.SelectorExpr）不返回：它的方法集不在本包 AST 里 ⇒ 其提升方法
// 无法在本包解析 ⇒ 不计入本包测量面（口径的已知边界）。
func embeddedLocalNames(expr ast.Expr) []string {
	var fields *ast.FieldList
	switch t := expr.(type) {
	case *ast.StructType:
		fields = t.Fields
	case *ast.InterfaceType:
		fields = t.Methods
	default:
		return nil
	}
	if fields == nil {
		return nil
	}
	var out []string
	for _, f := range fields.List {
		if len(f.Names) != 0 {
			continue // 具名字段不是嵌入
		}
		if name := embeddedLocalName(f.Type); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// embeddedLocalName 取嵌入字段的**本包**类型名（剥指针、剥泛型索引）。
//
// 跨包嵌入（pkg.T）返回空串 ⇒ 不参与提升解析（边界：跨包类型的方法集归它自己
// 的包计）。
func embeddedLocalName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return embeddedLocalName(e.X)
	case *ast.IndexExpr:
		return embeddedLocalName(e.X)
	case *ast.IndexListExpr:
		return embeddedLocalName(e.X)
	}
	return ""
}

// countSource 解析一段源码并数它的导出符号数（自检夹具的入口）。
func countSource(t *testing.T, src string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("解析夹具源码失败：%v", err)
	}
	return countExportedSymbols(f, newPackageModel([]*ast.File{f}))
}

// writeFile 写一个夹具文件。
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写 %s 失败：%v", path, err)
	}
}

// formatPerFile 把逐文件计数排成一行（失败信息要能一眼看出是哪个文件变了）。
func formatPerFile(perFile map[string]int) string {
	names := make([]string, 0, len(perFile))
	for name := range perFile {
		names = append(names, name)
	}
	slices.Sort(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+strconv.Itoa(perFile[name]))
	}
	return strings.Join(parts, " ")
}
