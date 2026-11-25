package roundguard_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ── 终审 I1：spec §11 的宿主接线示例**不得**另开一套宿主侧额度判定分支 ──
//
// 为什么要有这道闸（2026-11-21 终审 I1）：spec §11 原来保留着「额度判定分支 +
// forceFinal 本地布尔」，与 docs/USAGE.md §3.18 的「额度判定只留 Decide 一处」
// **正面冲突**——本地布尔绕过 Budget.EffectivePolicy() 与 Decide 的三段优先级，
// 宿主照抄 spec 就会把「零值缺省 = 强制收尾」误判成「没设」，且两套额度口径
// 各判各的、漂了没人知道。
//
// 判定形态（**§11 的代码块内**——围栏块与缩进块**同等**是代码面）：
//
//	① CheckRounds( / CheckToolCalls( 的**调用形态**（func 声明形态除外）；
//	② 宿主侧**本地布尔兜底**（forceFinal := 这类）。
//
// 为什么限定代码块：散文中「不要再另开一套 CheckRounds 分支」是**正面**表述，
// 全文出现即红会把正确的说明也判成违规 ⇒ 误报的门禁等于没有门禁。
//
// 为什么排除 func 声明形态：§6.3 的 func CheckRounds(...) 是本包的**公开面**
// （Decide 内部调用它），不是宿主接线 ⇒ 判据只卡**调用形态**，不卡声明。

// readSpecText 读 spec 全文（判定面与登记面共用同一份真相）。
func readSpecText(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("读不到 spec（%s）：%v", specPath, err)
	}
	return string(raw)
}

// specSectionBody 返回 spec 里某个 "## N." 二级小节的正文（不含标题行，
// 不含下一个 "## " 之后的内容）。判定面用它把范围钉在小节级。
func specSectionBody(specText, sec string) string {
	lines := strings.Split(specText, "\n")
	start := -1
	for i, ln := range lines {
		if strings.HasPrefix(ln, "## "+sec+".") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// codeBlocks 抽出小节正文里的**代码面**：围栏代码块（三个反引号之间的行）
// + 缩进代码块（4 空格 / Tab 起头的连续行）。
//
// 为什么也要收缩进块（终审复审 F3）：markdown 里缩进代码块与围栏块**同等**是
// 代码面。只判围栏块 ⇒ 宿主把 §11 的接线示例改写成缩进写法时，这道闸**静默
// 失明**（规则在、报告照常、就是判不到）——那正是本仓反复强调的最贵失效形态。
//
// 为什么缩进块要排除列表项续行：markdown 语义里「4 空格缩进的列表项」**不是**
// 代码块；不排除会把普通列表文本扫进判定面 ⇒ 误报的门禁等于没有门禁。
func codeBlocks(body string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	inFence := false
	for _, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			if inFence {
				flush()
				inFence = false
				continue
			}
			flush() // 围栏之前的缩进块先结算
			inFence = true
			continue
		}
		if inFence {
			cur = append(cur, ln)
			continue
		}
		if isIndentedCodeLine(ln) {
			cur = append(cur, ln)
			continue
		}
		flush()
	}
	flush()
	return out
}

// isIndentedCodeLine 判断一行是不是 markdown 的**缩进代码块**内容。
//
// 判据：4 空格 / Tab 起头，且去掉缩进后**不是**列表项起头
// （markdown 语义：列表项的续行不算代码块）。
func isIndentedCodeLine(ln string) bool {
	if strings.TrimSpace(ln) == "" {
		return false
	}
	if strings.HasPrefix(ln, "\t") {
		return true
	}
	if !strings.HasPrefix(ln, "    ") {
		return false
	}
	rest := strings.TrimLeft(ln, " ")
	for _, mark := range []string{"- ", "* ", "+ ", "> "} {
		if strings.HasPrefix(rest, mark) {
			return false
		}
	}
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(rest) && (rest[digits] == '.' || rest[digits] == ')') {
		return false // 有序列表项（「1. 」/「1) 」起头）
	}
	return true
}

// stripLineComment 去掉行尾注释（字符串字面量内的双斜杠不算）。
func stripLineComment(ln string) string {
	inStr := false
	for i := 0; i < len(ln); i++ {
		switch c := ln[i]; {
		case c == '"':
			inStr = !inStr
		case c == '/' && i+1 < len(ln) && ln[i+1] == '/' && !inStr:
			return ln[:i]
		}
	}
	return ln
}

var (
	// hostQuotaCallRe 宿主侧额度判定**调用形态**：CheckRounds( / CheckToolCalls(。
	// 第 1 组捕获 func —— 捕获到 ⇒ **声明**形态（§6.3 的公开面）⇒ 不算违规。
	hostQuotaCallRe = regexp.MustCompile(`(?:(func)\s+)?\b(CheckRounds|CheckToolCalls)\s*\(`)
	// hostLocalBoolRe 宿主侧**本地布尔兜底**（forceFinal := 这类）：本地布尔绕过
	// Budget.EffectivePolicy() 与 Decide 的三段优先级 ⇒ 处置口径分叉。
	hostLocalBoolRe = regexp.MustCompile(`\bforceFinal\b\s*:?=`)
)

// hostWiringViolations 返回 spec 宿主接线小节里的违规列表（空 = 通过）。
//
// 纯函数：吃 spec 文本 ⇒ 返回违规列表 ⇒ 判据自身可被「注入文本 ⇒ 变红」自证。
func hostWiringViolations(specText string) []string {
	var out []string
	body := specSectionBody(specText, "11")
	if body == "" {
		return []string{"spec 里找不到 §11 宿主接线小节（登记面与 spec 漂了）"}
	}
	for bi, blk := range codeBlocks(body) {
		for i, ln := range strings.Split(blk, "\n") {
			code := stripLineComment(ln)
			if strings.TrimSpace(code) == "" {
				continue
			}
			where := "§11 第 " + strconv.Itoa(bi+1) + " 个代码块第 " + strconv.Itoa(i+1) + " 行"
			for _, m := range hostQuotaCallRe.FindAllStringSubmatch(code, -1) {
				if m[1] != "" {
					continue // 声明形态：不是宿主接线
				}
				out = append(out, where+
					"出现宿主侧额度判定**调用形态**："+m[2]+
					"( —— 额度判定只留 Decide 一处，宿主侧不得另开分支")
			}
			if hostLocalBoolRe.MatchString(code) {
				out = append(out, where+
					"出现宿主侧**本地布尔兜底** forceFinal（绕过 Budget.EffectivePolicy()）")
			}
		}
	}
	return out
}

// injectSectionBlock 把一段代码块注入到 spec 指定小节的末尾（返回新文本）。
// 只在测试里用：用于「注入即红」自证，不碰磁盘上的 spec。
func injectSectionBlock(specText, sec, block string) string {
	lines := strings.Split(specText, "\n")
	start, end := -1, len(lines)
	for i, ln := range lines {
		if start < 0 {
			if strings.HasPrefix(ln, "## "+sec+".") {
				start = i
			}
			continue
		}
		if strings.HasPrefix(ln, "## ") {
			end = i
			break
		}
	}
	if start < 0 {
		return specText
	}
	out := append([]string{}, lines[:end]...)
	out = append(out, strings.Split(strings.TrimRight(block, "\n"), "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// TestSpecHostWiringHasNoHostSideQuotaBranch 断言当前 spec §11 **通过**该判据。
//
// 这条是「判据不误报」的正面锚：判据若对合规 spec 也报红，它就没人信了。
func TestSpecHostWiringHasNoHostSideQuotaBranch(t *testing.T) {
	v := hostWiringViolations(readSpecText(t))
	if len(v) > 0 {
		t.Fatalf("spec §11 出现宿主侧额度判定分支（应只留 Decide 一处）：\n  %s",
			strings.Join(v, "\n  "))
	}
}

// TestSpecHostWiringRejectsInjectedHostSideQuotaBranch 反向验证：注入一段宿主侧
// 额度判定分支 ⇒ 判据必须变红（否则这道闸是空转的）。
func TestSpecHostWiringRejectsInjectedHostSideQuotaBranch(t *testing.T) {
	injected := "```go\n" +
		"for {\n" +
		"    if v := roundguard.CheckRounds(b, meter.Used()); !v.Allowed {\n" +
		"        return finish(v)\n" +
		"    }\n" +
		"    forceFinal := !roundguard.CheckToolCalls(b, meter.Used()).Allowed\n" +
		"    _ = forceFinal\n" +
		"}\n" +
		"```"
	v := hostWiringViolations(injectSectionBlock(readSpecText(t), "11", injected))
	if len(v) == 0 {
		t.Fatal("注入宿主侧额度判定分支后判据仍判绿 ⇒ 这道判据是空转的")
	}
	// 两条形态都要命中：调用形态 + 本地布尔兜底
	var sawCall, sawBool bool
	for _, s := range v {
		if strings.Contains(s, "调用形态") {
			sawCall = true
		}
		if strings.Contains(s, "本地布尔兜底") {
			sawBool = true
		}
	}
	if !sawBool {
		t.Errorf("注入后未命中「本地布尔兜底」这一条，实际命中：%v", v)
	}
	if !sawCall {
		t.Errorf("注入后未命中「调用形态」这一条，实际命中：%v", v)
	}
}

// TestSpecHostWiringAllowsDeclaredCheckSurface 断言 §6.3 的**声明形态**不误报：
// 本包的公开面必须继续声明这两个 Check*（判据若把它们也判红，它就误伤了契约）。
func TestSpecHostWiringAllowsDeclaredCheckSurface(t *testing.T) {
	specText := readSpecText(t)
	if !strings.Contains(specText, "func CheckRounds(b Budget, used Used) Verdict") {
		t.Fatal("spec 里找不到 CheckRounds 的声明形态（本包公开面被改掉了？）")
	}
	if !strings.Contains(specText, "func CheckToolCalls(b Budget, used Used) Verdict") {
		t.Fatal("spec 里找不到 CheckToolCalls 的声明形态（本包公开面被改掉了？）")
	}
	// 声明形态放在 §6.3，不在 §11 ⇒ §11 判据不该命中它
	if v := hostWiringViolations(specText); len(v) > 0 {
		t.Fatalf("§6.3 的声明形态被判成违规（误伤契约）：%v", v)
	}
}

// TestSpecHostWiringRejectsInjectedIndentedHostSideQuotaBranch 反向验证：
// **缩进**代码块（markdown 的另一种代码写法）里出现宿主侧额度判定 ⇒ 判据必须
// 变红。
//
// 为什么必须钉这条（终审复审 F3）：原判据只扫围栏代码块 ⇒ 宿主把 §11 的接线
// 示例改写成缩进写法时，这道闸**静默失明**——规则在、报告照常、就是判不到。
// 只写「好实现能过」的用例，一个把判定面缩窄到围栏块的错实现照样全过 ⇒
// 这条是「坏实现会红」的证据位。
func TestSpecHostWiringRejectsInjectedIndentedHostSideQuotaBranch(t *testing.T) {
	injected := "" +
		"    if v := roundguard.CheckRounds(b, meter.Used()); !v.Allowed {\n" +
		"        return finish(v)\n" +
		"    }"
	v := hostWiringViolations(injectSectionBlock(readSpecText(t), "11", injected))
	if len(v) == 0 {
		t.Fatal("注入缩进代码块形态的宿主侧额度判定后判据仍判绿 ⇒ 这道判据只扫了围栏块")
	}
}
