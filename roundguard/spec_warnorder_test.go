package roundguard_test

import (
	"strconv"
	"strings"
	"testing"
)

// ── 终审 M1：警告回灌必须排在「被评判的那一回合」之后 ──
//
// 为什么要有这道闸（2026-11-21 终审 M1）：§11 的宿主接线示例原来把
// msgs = append(msgs, hostWarnMessage(d.Shape)) 写在
// msgs = append(msgs, assistantMsg(resp), toolResultsMsg(results)) **之前**
// ⇒ 模型先看到警告、后看到触发警告的那一回合 ⇒ **语义倒置**。
//
// 而这类错**配对不破**：没有编译错误、没有断言会红，宿主照抄也不报错
// ⇒ 只有把「顺序」本身变成判据，才不会下一次又被挪回去。
//
// 判定形态：§11 的**代码面**（围栏块 + 缩进块）内，hostWarnMessage( 所在行
// 必须**晚于** assistantMsg( 所在行。判的是**相对顺序**、不判绝对行号
// （宿主可自由排版；换个写法也不该被判红）。
//
// ⚠️ 为什么不把 USAGE 的接线示例也纳入本判据面：USAGE §3.18 是**宿主手册**，
// spec §11 是**真相源**——真相源被钉住、手册照抄即可；反过来做（以手册为准）
// 会让真相源反过来看手册的脸色。

// warnOrderViolations 返回 spec 宿主接线小节里「警告回灌顺序」违规（空 = 通过）。
//
// 纯函数：吃 spec 文本 ⇒ 返回违规列表 ⇒ 判据自身可被「注入文本 ⇒ 变红」自证。
func warnOrderViolations(specText string) []string {
	body := specSectionBody(specText, "11")
	if body == "" {
		return []string{"spec 里找不到 §11 宿主接线小节（登记面与 spec 漂了）"}
	}
	var out []string
	for bi, blk := range codeBlocks(body) {
		warnLine, turnLine := -1, -1
		for i, ln := range strings.Split(blk, "\n") {
			code := stripLineComment(ln)
			if warnLine < 0 && strings.Contains(code, "hostWarnMessage(") {
				warnLine = i
			}
			if turnLine < 0 && strings.Contains(code, "assistantMsg(") {
				turnLine = i
			}
		}
		if warnLine < 0 || turnLine < 0 {
			continue // 块里没有这两句 ⇒ 不归本判据管（不是每个代码块都是接线示例）
		}
		if warnLine < turnLine {
			out = append(out, "§11 第 "+strconv.Itoa(bi+1)+" 个代码块第 "+
				strconv.Itoa(warnLine+1)+" 行：警告回灌（hostWarnMessage）排在被评判回合"+
				"（assistantMsg，第 "+strconv.Itoa(turnLine+1)+" 行）**之前** ⇒ 语义倒置"+
				"（模型先看到警告、后看到触发警告的那一回合）")
		}
	}
	return out
}

// TestSpecHostWiringWarnsAfterJudgedRound 断言当前 spec §11 **通过**该判据。
//
// 正面锚：判据若对合规 spec 也报红，它就没人信了。
func TestSpecHostWiringWarnsAfterJudgedRound(t *testing.T) {
	if v := warnOrderViolations(readSpecText(t)); len(v) > 0 {
		t.Fatalf("spec §11 的警告回灌顺序倒了（应排在被评判回合之后）：\n  %s",
			strings.Join(v, "\n  "))
	}
}

// TestSpecHostWiringRejectsWarnBeforeJudgedRound 反向验证：注入一段「警告回灌排在
// 被评判回合之前」的接线 ⇒ 判据必须变红（否则这道闸是空转的）。
//
// 只写「好实现能过」的用例，一个把顺序判反的错实现照样全过 ⇒ 这条是
// 「坏实现会红」的证据位。
func TestSpecHostWiringRejectsWarnBeforeJudgedRound(t *testing.T) {
	injected := "\n" + "```go\n" +
		"if d.Warn {\n" +
		"    msgs = append(msgs, hostWarnMessage(d.Shape))\n" +
		"}\n" +
		"msgs = append(msgs, assistantMsg(resp), toolResultsMsg(results))\n" +
		"```\n"
	inj := injectSectionBlock(readSpecText(t), "11", injected)
	v := warnOrderViolations(inj)
	if len(v) == 0 {
		t.Fatal("注入「警告回灌排在被评判回合之前」后判据仍判绿 ⇒ 这道闸是空转的")
	}
	found := false
	for _, s := range v {
		if strings.Contains(s, "语义倒置") {
			found = true
		}
	}
	if !found {
		t.Errorf("未报出「语义倒置」，实际报出：%v", v)
	}
}
