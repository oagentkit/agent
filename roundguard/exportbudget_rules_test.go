package roundguard_test

import (
	"path/filepath"
	"testing"
)

// ── 计数规则的自检（证明口径没被放松）──
//
// 这些用例与门禁本体（exportbudget_test.go）分开放，理由与本包既有惯例同款：
// 「正常用例」与「反向验证用例」分开成文件，红的时候信号不被大批绿灯淹没。
//
// 每一条都钉住**计数规则的一句话**。规则被放松时（方法计数去掉、接口方法集不
// 数、_test.go 放回计数面、歧义的名字被计入……），对应的用例当场变红 ⇒ 「精确
// 计数」钉住的才是**当前这条口径**下的数字，而不是某个被放松过的口径下的数字。

// TestCountingRulesCountExportedMethods：**导出方法被计入**。
//
// 做法是「同一段源码，加方法前后各数一次」：差值必须**恰好**是导出方法的个数。
// 若有人把方法计数从口径里去掉，两个数字就相等 ⇒ 差值 0 ≠ 2 ⇒ 用例变红。
// 未导出的方法（hidden）不得计入 ⇒ 差值是 2 而不是 3。
func TestCountingRulesCountExportedMethods(t *testing.T) {
	const withMethods = `package sample

type Widget struct {
	ID   string
	size int
}

func (w Widget) Describe() string { return w.ID }
func (w *Widget) Reset()            { w.size = 0 }
func (w Widget) hidden()            {}

func Build() Widget { return Widget{} }
func helper() int   { return 0 }
`
	const withoutMethods = `package sample

type Widget struct {
	ID   string
	size int
}

func Build() Widget { return Widget{} }
func helper() int   { return 0 }
`
	withN := countSource(t, withMethods)
	withoutN := countSource(t, withoutMethods)
	// 基线：type Widget(1) + 导出字段 ID(1) + 包级导出函数 Build(1) = 3。
	if withoutN != 3 {
		t.Fatalf("计数基线不对：未加方法的样例数出 %d（应为 3）——计数规则被改了？", withoutN)
	}
	// 差值 = 两个导出方法（Describe / Reset）；未导出方法（hidden）不计入。
	if withN-withoutN != 2 {
		t.Fatalf("导出方法未被计入：加方法的样例数出 %d，未加的 %d，差值 %d（应为 2）——"+
			"给已有类型加一个导出方法是最容易悄悄扩大 API 面的动作，它必须在计数面里",
			withN, withoutN, withN-withoutN)
	}
}

// TestCountingRulesExcludeTestFiles：**_test.go 不计入**。
//
// 门禁数的是**交付面**（宿主 import 得到的符号），测试函数不是交付面。
// 若 _test.go 被放回计数面，同一个包的导出面会被测试代码撑大一倍，而门禁从此
// 变成「测试代码量」的看门狗 ⇒ 真正的导出面变化被淹没。
func TestCountingRulesExcludeTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "prod.go"), "package sample\n\nfunc Exported() {}\n")
	writeFile(t, filepath.Join(dir, "prod_test.go"),
		"package sample\n\nfunc ExportedToo() {}\n\nfunc TestSomething(t *testing.T) {}\n")
	total, _ := exportedSurface(t, dir)
	if total != 1 {
		t.Fatalf("导出面计数 %d（应为 1）：_test.go 不属于交付面，"+
			"测试函数名首字母大写也不该计入", total)
	}
}

// TestCountingRulesCountInterfaceMethods：**导出接口的方法集被计入**。
//
// 给导出接口**加一个方法**，宿主侧的实现方就被迫多实现一个 ⇒ 破坏性变更。
// 而「加方法」只多一条**方法声明**——按「声明」数的口径对这类改动**完全失明**
// ⇒ 「精确计数」会在一条增长路径上彻底失效。
func TestCountingRulesCountInterfaceMethods(t *testing.T) {
	const oneMethod = `package sample

type Probe interface {
	Alpha() int
}
`
	const twoMethods = `package sample

type Probe interface {
	Alpha() int
	Beta() string
}
`
	one := countSource(t, oneMethod)
	two := countSource(t, twoMethods)
	// 基线：type Probe(1) + 方法 Alpha(1) = 2。
	if one != 2 {
		t.Fatalf("计数基线不对：一个导出方法的接口数出 %d（应为 2）——接口的方法集没被计入？", one)
	}
	if two-one != 1 {
		t.Fatalf("给导出接口加一个方法，计数只变了 %d（应为 1）——"+
			"给接口加方法是破坏性变更（宿主实现方被迫多实现一个），它必须在计数面里",
			two-one)
	}
}

// TestCountingRulesCountPromotedMethods：**嵌入提升上来的导出方法被计入**。
//
// 未导出类型被导出类型嵌入时，它的导出方法被**提升**进导出类型 ⇒ 宿主能通过
// 导出类型调用它（Outer.Promoted()）⇒ 属交付面。只数「嵌入字段名」的话，这类
// 方法对计数**完全不可见**。
//
// 同时钉住**不重复计**：嵌入的若是**导出**类型，它的方法已在它自己的声明处计过
// ⇒ 不能再在嵌入处计一次。
func TestCountingRulesCountPromotedMethods(t *testing.T) {
	const withPromoted = `package sample

type inner struct{}

func (inner) Promoted() int { return 0 }

type Outer struct {
	inner
}
`
	const withoutPromoted = `package sample

type inner struct{}

type Outer struct {
	inner
}
`
	withN := countSource(t, withPromoted)
	withoutN := countSource(t, withoutPromoted)
	// 基线：type Outer(1) + 嵌入的 inner（未导出 ⇒ 0）= 1。
	if withoutN != 1 {
		t.Fatalf("计数基线不对：未加方法的样例数出 %d（应为 1）", withoutN)
	}
	if withN-withoutN != 1 {
		t.Fatalf("嵌入提升上来的导出方法未被计入：加方法的样例 %d，未加的 %d，差值 %d（应为 1）——"+
			"宿主能通过 Outer.Promoted() 调用它，它属交付面", withN, withoutN, withN-withoutN)
	}
	// 不重复计：嵌入**导出**类型时，它的方法已在它自己的声明处计过。
	const exportedInner = `package sample

type Inner struct{}

func (Inner) Promoted() int { return 0 }

type Outer struct {
	Inner
}
`
	// Inner(1) + Promoted(1) + Outer(1) + 嵌入的 Inner(1) = 4。
	if got := countSource(t, exportedInner); got != 4 {
		t.Fatalf("嵌入导出类型时计数 %d（应为 4）——提升上来的方法被**重复计**了", got)
	}
}

// TestCountingRulesCountInterfaceEmbeddedMethods：**导出接口嵌入未导出接口**时，
// 提升上来的方法必须计入。
//
// ⚠️ 这一条是「声明/执行漂移」的钉子：接口的方法集写在类型声明里
// （*ast.InterfaceType），**永远不是 FuncDecl** ⇒ 若计数器只收方法声明，嵌入
// 接口贡献的方法集对它**完全不可见** ⇒ 声明面写的规则没有兑现。
//
// 嵌入未导出接口时，宿主实现 Probe 仍被迫实现 Alpha ⇒ Alpha 属交付面。
func TestCountingRulesCountInterfaceEmbeddedMethods(t *testing.T) {
	const withEmbed = `package sample

type inner interface {
	Alpha() int
}

type Probe interface {
	inner
}
`
	const withoutEmbed = `package sample

type inner interface {
	Alpha() int
}

type Probe interface{}
`
	withN := countSource(t, withEmbed)
	withoutN := countSource(t, withoutEmbed)
	// 基线：type Probe(1)（inner 未导出且未被嵌入 ⇒ 它的方法集不算）= 1。
	if withoutN != 1 {
		t.Fatalf("计数基线不对：未嵌入的样例数出 %d（应为 1）", withoutN)
	}
	// 差值 = 提升上来的 Alpha（宿主实现 Probe 时被迫实现它）。
	if withN-withoutN != 1 {
		t.Fatalf("嵌入未导出接口的方法集未被计入：嵌入的样例 %d，未嵌入的 %d，差值 %d（应为 1）——"+
			"方法集不是声明，只数声明就看不见它", withN, withoutN, withN-withoutN)
	}
}

// TestCountingRulesCountStructEmbeddedInterfaceMethods：**导出结构体嵌入未导出
// 接口**时，提升上来的方法必须计入。
//
// Go 1.14 起结构体可以嵌入接口，宿主能直接调 Outer.Alpha() ⇒ 属交付面。
func TestCountingRulesCountStructEmbeddedInterfaceMethods(t *testing.T) {
	const withEmbed = `package sample

type inner interface {
	Alpha() int
}

type Outer struct {
	inner
}
`
	const withoutEmbed = `package sample

type inner interface {
	Alpha() int
}

type Outer struct{}
`
	withN := countSource(t, withEmbed)
	withoutN := countSource(t, withoutEmbed)
	// 基线：type Outer(1) = 1。
	if withoutN != 1 {
		t.Fatalf("计数基线不对：未嵌入的样例数出 %d（应为 1）", withoutN)
	}
	if withN-withoutN != 1 {
		t.Fatalf("结构体嵌入未导出接口的方法集未被计入：嵌入的样例 %d，未嵌入的 %d，差值 %d（应为 1）——"+
			"宿主能直接调 Outer.Alpha()，它属交付面", withN, withoutN, withN-withoutN)
	}
}

// TestCountingRulesSkipAmbiguousPromotedMethods：**歧义的提升方法不得计入**。
//
// Go 的选择器规则：同一深度上多个嵌入类型带同名方法 ⇒ 那个名字是**歧义选择器**、
// **不可调用** ⇒ 不属交付面 ⇒ 不得计入（计了就是多计，方向是假红）。
//
// 浅层赢仍成立：浅层出现过的名字，深层同名只是被**遮蔽**，不是歧义。
func TestCountingRulesSkipAmbiguousPromotedMethods(t *testing.T) {
	const ambiguous = `package sample

type a struct{}

func (a) Same() int { return 0 }

type b struct{}

func (b) Same() int { return 0 }

type Outer struct {
	a
	b
}
`
	const single = `package sample

type a struct{}

func (a) Same() int { return 0 }

type Outer struct {
	a
}
`
	ambN := countSource(t, ambiguous)
	singleN := countSource(t, single)
	// 单嵌入：type Outer(1) + 提升的 Same(1) = 2。
	if singleN != 2 {
		t.Fatalf("计数基线不对：单嵌入的样例数出 %d（应为 2）", singleN)
	}
	// 同深度两个嵌入类型同名 ⇒ 歧义 ⇒ Same 不被提升 ⇒ 只有 type Outer(1) = 1。
	if ambN != 1 {
		t.Fatalf("歧义的提升方法被计入了：同深度同名嵌入的样例数出 %d（应为 1）——"+
			"Go 里那个名字是歧义选择器、不可调用，不属交付面", ambN)
	}
}

// TestCountingRulesCountAliasExposedMembers：**导出别名指向本包未导出类型**时，
// 被别名暴露出去的那一面必须计入。
//
// 为什么必须钉这条（终审 Minor M2）：`type Alias = inner`（inner 未导出）时，
// `Alias` 与 `inner` 是**同一个类型** ⇒ 宿主 import 得到 `Alias` 就拿到了 inner 的
// 字段与方法 ⇒ 它们属交付面。而计数器原先只按「接收者类型名导出」判方法、只按
// 「类型名导出」判成员面 ⇒ inner 的成员面对这道门禁**完全不可见** ⇒ 给 inner 加一个
// 导出方法或导出字段，门禁**一声不响**（规则在、报告照常、就是判不到）。
//
// 判据写法：同一段源码，**加别名 / 不加别名**各数一次 ⇒ 差值必须恰好是
// 「别名名字 1 + 目标的导出字段 1 + 目标自己的导出方法 1」。
func TestCountingRulesCountAliasExposedMembers(t *testing.T) {
	withAlias := `package sample

type inner struct {
	Field int
}

func (i inner) Method() {}

type Alias = inner
`
	withoutAlias := `package sample

type inner struct {
	Field int
}

func (i inner) Method() {}
`
	got := countSource(t, withAlias) - countSource(t, withoutAlias)
	if got != 3 {
		t.Fatalf("导出别名暴露出的成员面必须计入：差值 = %d，实期 3"+
			"（别名名字 1 + 目标的导出字段 1 + 目标自己的导出方法 1）", got)
	}
}

// TestCountingRulesDoesNotDoubleCountAliasToExportedType：别名指向**导出**类型时，
// 目标的成员面**不得**再计一次（`Alias` 与 `Inner` 是同一个类型 ⇒ 同一批成员）。
//
// 为什么必须钉这条：修「别名暴露未导出类型」的那条规则时，很容易顺手把别名目标
// 的成员面**无条件**再计一遍 ⇒ 同一个方法被数两次 ⇒ 计数虚高 ⇒ 天花板被提前撞线
// （假红方向）。
func TestCountingRulesDoesNotDoubleCountAliasToExportedType(t *testing.T) {
	src := `package sample

type Inner struct {
	Field int
}

func (i Inner) Method() {}

type Alias = Inner
`
	if got := countSource(t, src); got != 4 {
		t.Fatalf("别名指向导出类型时不得重复计：实得 %d，实期 4"+
			"（Inner 名字 1 + 导出字段 1 + 导出方法 1 + 别名名字 1）", got)
	}
}

// TestCountingRulesIgnoresUnexportedAlias：**未导出**的别名不暴露任何东西。
//
// 为什么必须钉这条：别名自己未导出 ⇒ 宿主 import 不到它 ⇒ 它的目标对宿主
// **不可见** ⇒ 目标的成员面不该计入。反过来（把它也当成暴露）⇒ 计数虚高。
func TestCountingRulesIgnoresUnexportedAlias(t *testing.T) {
	src := `package sample

type inner struct {
	Field int
}

func (i inner) Method() {}

type alias = inner
`
	if got := countSource(t, src); got != 0 {
		t.Fatalf("未导出的别名不暴露任何东西：实得 %d，实期 0", got)
	}
}
