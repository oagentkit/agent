package roundguard

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// ── 白盒矩阵：判据 × 变异 × 用例（内部包那一半）──
//
// 为什么矩阵分成两半：Go 的包边界——外部测试包（roundguard_test）看不见未导出
// 符号，而**承重守卫**（漂移分支的「原指纹不全等」）只在未导出面上 ⇒ 它只能由
// 内部测试包直钉。两半合起来才是完整的矩阵（外部半张表在 mutation_test.go）。
//
// 合成数据：夹具只用中性词面（指纹用字母串）。

// specPath spec 文件（相对本包目录）。
const specPath = "../docs/superpowers/specs/2026-11-21-roundguard-agent-loop-design.md"

// specMarker 白盒矩阵钉住的那条 spec 条款的标记（**必须真的存在于 spec**——
// spec 改了条款而白盒用例没跟上 ⇒ 当天即红）。
const specMarker = "重复优先于漂移"

// whiteBoxRow 白盒矩阵的一行。
type whiteBoxRow struct {
	criterion string           // 判据（一句）
	mutant    string           // 变异类（把它改坏是什么样）
	pin       func(*testing.T) // 钉住它的用例
}

// whiteBoxMatrix 白盒矩阵（内部包那一半）。
var whiteBoxMatrix = []whiteBoxRow{
	{
		criterion: "漂移分支的承重守卫：原指纹全等 ⇒ 漂移分支不可达",
		mutant:    "删掉 driftCandidate 里的 allSameFingerprint 守卫 ⇒ 该子情形被误判成漂移（假阳）",
		pin:       TestDriftCandidateGuardIsLoadBearing,
	},
	{
		criterion: "守卫惰性所依赖的两处耦合：重复分支先于漂移分支 + 两者同用阈值 k",
		mutant:    "M4：漂移分支的阈值被拆开（driftCandidate(seq, k-1)）⇒ 漂移提前一轮命中；M8：漂移分支被挪到重复分支之前 ⇒ 结论落点从重复变漂移",
		pin:       TestDriftGuardCouplingHolds,
	},
}

// TestMutationMatrixWhiteBox 跑白盒矩阵的每一行。
func TestMutationMatrixWhiteBox(t *testing.T) {
	if len(whiteBoxMatrix) == 0 {
		t.Fatal("白盒矩阵为空：承重守卫就没有钉它的用例")
	}
	for _, row := range whiteBoxMatrix {
		t.Run(row.criterion, row.pin)
	}
}

// TestMutationMatrixWhiteBoxIsComplete 棘轮：每一行都得写清「判据」与「钉住哪个
// 变异」——只写判据不写变异的那行等于没钉（读者无法从表里看出它挡的是哪种改坏）。
func TestMutationMatrixWhiteBoxIsComplete(t *testing.T) {
	for _, row := range whiteBoxMatrix {
		if row.criterion == "" || row.mutant == "" || row.pin == nil {
			t.Fatalf("白盒矩阵行不完整（判据/变异/用例三缺一）：%+v", row)
		}
	}
}

// TestWhiteBoxMatrixAnchorExistsInSpec 棘轮：白盒矩阵钉住的 spec 条款必须**真的
// 存在**——spec 改了条款（挪走 / 改写 / 删除）而白盒用例没跟上 ⇒ 当天即红。
//
// 为什么白盒那一半也要有这道闸：白盒用例是**手写**的（不参与穷举），它与 spec
// 的耦合只能靠这道标记检查兜住，否则它会悄悄变成「自说自话的凭据」
// （2026-11-21 第 1 轮评审 Important 1 的活样本）。
func TestWhiteBoxMatrixAnchorExistsInSpec(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("读不到 spec（%s）：%v", specPath, err)
	}
	if !strings.Contains(string(raw), specMarker) {
		t.Fatalf("spec 里找不到白盒矩阵钉住的条款标记 %q（spec 改了条款 ⇒ 白盒用例必须当天跟上）", specMarker)
	}
}

// TestDriftCandidateGuardIsLoadBearing 直钉漂移分支上那道**承重守卫**。
//
// 为什么必须白盒：这道守卫在整条判定链上**不可达**——「原指纹全等」的那串必被
// **先判的重复形态**判走（两处耦合：① 重复分支先于漂移分支；② 两者同用阈值
// k，见 spec §7.3）⇒ 黑盒观察不到 ⇒ 2026-11-21 实测：**删掉守卫后整套
// roundguard 用例仍全绿** ⇒ 守卫是一句惰性断言（谁也证明不了它还在起作用）。
//
// 收成 driftCandidate 之后，守卫的契约可直钉：删掉守卫 ⇒ 本用例当场红 ⇒
// 守卫从「惰性断言」变成「承重判据」。
func TestDriftCandidateGuardIsLoadBearing(t *testing.T) {
	const k = 3
	// 构造：归一指纹相同、**原指纹全等**、长度达阈值的那串。
	same := []Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "a", Normalized: "n", Tools: 1},
	}
	// 前置一：漂移候选长度确实达到阈值（否则守卫无从承重）。
	if L := trailingSameNormalized(same); L < k {
		t.Fatalf("构造的序列必须让漂移候选长度达到阈值（守卫才承重），实得 %d", L)
	}
	// 前置二：原指纹全等（守卫的触发条件成立）。
	if !allSameFingerprint(same) {
		t.Fatal("构造的序列必须原指纹全等")
	}
	// 守卫：该进不进。
	if L, ok := driftCandidate(same, k); ok {
		t.Fatalf("原指纹全等时漂移分支必须不可达（守卫承重），实得 L=%d", L)
	}
	// 守卫不是一刀切：原指纹不全等 ⇒ 漂移分支成立，且长度就是阈值。
	diff := []Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
		{Fingerprint: "c", Normalized: "n", Tools: 1},
	}
	if L, ok := driftCandidate(diff, k); !ok || L != k {
		t.Fatalf("原指纹不全等时漂移分支必须成立且长度达阈值，实得 L=%d ok=%v", L, ok)
	}
}

// TestDriftGuardCouplingHolds 直钉守卫惰性所依赖的**两处耦合**——这次**真的读
// 调用点**（经由 Detect 的真实路径），不是只在 helper 层比两个长度。
//
// 耦合① **重复分支先于漂移分支**；耦合② 两者用**同一个阈值 k**。
//
// ⚠ 2026-11-21 第 1 轮评审 Important 1：先前这条用例**只**断言两个 helper 在一个
// 夹具上返回相等长度——它既不读 Detect 里**传给漂移分支的阈值**，也不读**分支
// 次序** ⇒ 注入 M4（阈值拆开）/ M8（次序对调）时它**仍绿** ⇒ 矩阵行 W2 当时是一张
// 自说自话的凭据。本轮改成读真实路径：
//
//   - 耦合①：造一串「重复与漂移**同时**成立」的夹具 ⇒ 结论必须落在重复上
//     （M8 把漂移挪到前面 ⇒ 结论变漂移 ⇒ 红）；
//   - 耦合②：漂移分支**只在**长度达 k 时成立（k-1 时不成立）⇒ 它的阈值与重复
//     分支同源（M4 把阈值拆成 k-1 ⇒ k-1 时就成立 ⇒ 红）。
func TestDriftGuardCouplingHolds(t *testing.T) {
	const k = 3

	// ── 耦合①：重复分支先于漂移分支 ──
	// 夹具：末 k 轮同指纹（重复成立），整段归一指纹相同但**原指纹不全等**
	//（漂移也成立）⇒ 两条分支同时成立 ⇒ 结论由**次序**决定。
	both := []Observation{
		{Fingerprint: "a", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
		{Fingerprint: "b", Normalized: "n", Tools: 1},
	}
	f := Detect(both, Config{})
	if f.Shape != ShapeRepeat {
		t.Fatalf("重复与漂移同时成立时必须判 %q（重复分支先判走），实得 %q", ShapeRepeat, f.Shape)
	}

	// ── 耦合②：漂移分支的阈值与重复分支同源（都是 k）──
	// 只有漂移成立的夹具（归一同、原指纹各异 ⇒ 重复不成立）：k-1 轮不成立、k 轮成立。
	driftOnly := func(n int) []Observation {
		out := make([]Observation, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, Observation{
				Fingerprint: "p" + strconv.Itoa(i),
				Normalized:  "n",
				Tools:       1,
			})
		}
		return out
	}
	if g := Detect(driftOnly(k-1), Config{}); g.Shape != ShapeNone {
		t.Fatalf("漂移的阈值应恰为 k：差一轮不该命中（耦合②），实得 %q", g.Shape)
	}
	if g := Detect(driftOnly(k), Config{}); g.Shape != ShapeDrift {
		t.Fatalf("漂移的阈值应恰为 k：刚够阈值应命中，实得 %q", g.Shape)
	}
	// 重复分支那侧同样：k-1 轮同指纹 ⇒ 不成立（两条分支的阈值同源）。
	repeatOnly := func(n int) []Observation {
		out := make([]Observation, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, Observation{Fingerprint: "a", Tools: 1})
		}
		return out
	}
	if g := Detect(repeatOnly(k-1), Config{}); g.Shape != ShapeNone {
		t.Fatalf("重复的阈值应恰为 k：差一轮不该命中，实得 %q", g.Shape)
	}
}
