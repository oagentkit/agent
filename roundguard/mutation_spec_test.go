package roundguard_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ── spec ↔ 矩阵的机器判定：矩阵必须覆盖 spec 的**每一条**判据 ──
//
// 为什么要有这道闸（2026-11-21 第 1 轮评审 Minor 2）：矩阵表与 spec 的判据面是
// **两张表**——两张表会漂 ⇒ 漂了没人知道 ⇒ 当天即红。上一轮的实际缺口：spec §7.3
// 加了一条判据、矩阵表没加 ⇒ **没有任何门禁变红**；矩阵里写一条 mutant 写错 / 写
// 空泛的行（活样本：白盒那一行的「两处耦合」压根没钉住）也不会红。
//
// 判据：**从 spec 派生判据锚点**，矩阵每一行必须登记它钉住的那条锚点；spec 的每条
// 锚点必须被至少一行矩阵钉住。⇒ spec 新增判据 ⇒ 矩阵当天必须跟上；矩阵新增一行
// ⇒ 它登记的锚点必须**真的存在于 spec**（登记面与 spec 漂了 ⇒ 当天即红）。

// specPath spec 文件（相对本包目录）。
const specPath = "../docs/superpowers/specs/2026-11-21-roundguard-agent-loop-design.md"

// specCriterionSections spec 里的**判据面小节**（登记面）。
//
// 为什么登记而不是全量扫：spec 里还有设计说明 / 接线示例 / 未决点等小节，它们不是
// 「判据」，不该被矩阵逐条钉。登记面写死「哪些小节是判据面」⇒ spec 里的小节被
// 改名 / 删除 ⇒ 本用例当场红（维护者必须回来把登记面与矩阵一起改）。
var specCriterionSections = []string{
	"4.2",   // 组合结论的形状（判定优先级）
	"5.1",   // 回合结局词表（失败型 / 温和型、零值语义）
	"5.2",   // 结局分类的输入面
	"5.3",   // 结局分类的判定次序
	"6.1",   // 预算面
	"6.1.1", // 额度维度 vs 处置口径
	"6.2",   // 用量面（回合数 ≠ 模型请求数）
	"6.3",   // 额度判定面（三个 Check* + 两维次序）
	"6.4",   // 「不得续跑」判据
	"7.1",   // 无进展检测的输入面
	"7.2",   // 形态与判定（形态、档位、窗口、零值缺省）
	"7.3",   // 判定顺序（形态次序 + RC 口径 + 重复优先于漂移）
	"7.4",   // 失败事实与进展事实从哪来
	"7.5",   // 处置分三档
}

// specMarkerClauses 散文条款（没有编号 ⇒ 只能用标记串钉住）。
var specMarkerClauses = map[string]string{
	"重复优先于漂移":   "§7.3 重复优先于漂移",
	"额度两维的判定次序": "§6.3 额度两维次序",
}

var (
	// specListItemRe 编号列表条目（§4.2 的判定优先级、§5.3 的判定次序、§7.5 的三档）。
	specListItemRe = regexp.MustCompile(`^\s*(\d+)\.\s`)
	// specTableRowRe 判定表的行（§5.3 的八臂、§7.3 的五形态）。
	specTableRowRe = regexp.MustCompile(`^\|\s*(\d+)\s*\|`)
)

// specAnchors 从 spec 文件派生「判据锚点」集合。
//
// 锚点有三类：① 小节级（§7.3）；② 条目级（§7.3 行 2 / §4.2 项 1）；③ 散文条款
// （§7.3 重复优先于漂移，用标记串钉住）。三者都由 spec **派生** ⇒ spec 改了就变。
func specAnchors(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("读不到 spec（%s）：%v", specPath, err)
	}
	text := string(raw)

	// 按「### x.y」标题把小节切开（登记面按小节号索引）。
	bodies := map[string][]string{}
	cur := ""
	for _, ln := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(ln, "### "):
			head := strings.TrimSpace(strings.TrimPrefix(ln, "### "))
			cur = head
			if i := strings.Index(head, " "); i > 0 {
				cur = head[:i]
			}
		case strings.HasPrefix(ln, "## "):
			cur = ""
		default:
			if cur != "" {
				bodies[cur] = append(bodies[cur], ln)
			}
		}
	}

	out := map[string]bool{}
	for _, sec := range specCriterionSections {
		body, ok := bodies[sec]
		if !ok {
			t.Fatalf("spec 里找不到判据面小节 §%s（登记面与 spec 漂了 ⇒ 当天即红）", sec)
		}
		out["§"+sec] = true
		for _, ln := range body {
			if m := specListItemRe.FindStringSubmatch(ln); m != nil {
				out["§"+sec+" 项 "+m[1]] = true
			}
			if m := specTableRowRe.FindStringSubmatch(ln); m != nil {
				out["§"+sec+" 行 "+m[1]] = true
			}
		}
	}
	for marker, key := range specMarkerClauses {
		if !strings.Contains(text, marker) {
			t.Fatalf("spec 里找不到条款标记 %q（登记面与 spec 漂了 ⇒ 当天即红）", marker)
		}
		out[key] = true
	}
	if len(out) == 0 {
		t.Fatal("从 spec 派生出的判据锚点是空集（解析面坏了 ⇒ 当天即红）")
	}
	return out
}

// TestMutationMatrixCoversEverySpecCriterion 棘轮：矩阵必须覆盖 spec 的**每一条**
// 判据。
//
// 两条判据一起判：
//  1. 矩阵每一行都得登记它钉住的 spec 判据，且登记的锚点**必须真的存在于 spec**
//     （登记面与 spec 漂了 ⇒ 当天即红）；
//  2. spec 的每一条判据都必须被**至少一行**矩阵钉住（spec 新增判据而矩阵没跟上
//     ⇒ 当天即红）。
//
// ⚠ 这条棘轮盯的是「两张表会漂」那个病：上一轮矩阵里的白盒行 W2 声称钉住「两处
// 耦合」，实际一条也没钉住（评审 Important 1）——**行写得再漂亮，没有机器判定，
// 照样可能是自说自话的凭据**。
func TestMutationMatrixCoversEverySpecCriterion(t *testing.T) {
	anchors := specAnchors(t)
	cited := map[string]int{}
	for _, row := range mutationMatrix {
		if len(row.anchors) == 0 {
			t.Fatalf("矩阵行没登记它钉住的 spec 判据：%s", row.criterion)
		}
		for _, a := range row.anchors {
			if !anchors[a] {
				t.Fatalf("矩阵行 %q 登记的判据 %q 在 spec 里不存在（登记面与 spec 漂了 ⇒ 当天即红）",
					row.criterion, a)
			}
			cited[a]++
		}
	}
	for a := range anchors {
		if cited[a] == 0 {
			t.Fatalf("spec 的判据 %q 没有任何矩阵行钉住（spec 新增判据 ⇒ 矩阵必须当天跟上）", a)
		}
	}
}
