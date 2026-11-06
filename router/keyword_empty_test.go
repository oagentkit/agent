package router

import (
	"testing"
	"time"
)

// withWatchdog 在界内执行 fn；超时即判红（agent#59：
// 空关键词会让 matcher 原地踏步，直接写在测试体里会变成整包 10 分钟超时，
// 那只是「跑挂了」，说不清是哪条入口挂的）。
//
// ⚠️ 用法约束：fn 里只放**纯判定**。超时分支失败后遗留的 goroutine 仍在空转，
// 这是刻意接受的代价——测试进程本就以失败收场。
func withWatchdog(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s：空关键词使扫描原地踏步（永不返回）——命中/作废循环必须被空词守门挡在进入之前", name)
	}
}

// TestEmptyKeywordMatchesNothing 空关键词**不构成命中**，且不得让扫描挂死。
//
// 五个入口分两族失效形态（agent#59 实测）：
//   - 恒真族：KeywordHit 的 boundary=false 路径首轮即 return true
//     ⇒ 词表里一个空项让该意图吞掉所有输入；
//   - 挂死族：boundary=true 的变体（含 Except）与被作废后「继续找下一处」的
//     KeywordPostNegated 都不 return，而前进量 pos+len(kw) 恒 0
//     ⇒ goroutine 永久空转（这一族没有报错，只有 CPU 与永不返回的调用）。
//
// 反向验证锚点：去掉 hitScan / KeywordPostNegated 的 `kw == ""` 守门后，
// 恒真族的断言打印「得到 true」，挂死族由看门狗打印「永不返回」。
func TestEmptyKeywordMatchesNothing(t *testing.T) {
	const input = "正常评论内容"

	// —— 恒真族：必须判 false（不是「碰巧返回 false」，见下方逐入口命名）
	t.Run(" KeywordHit", func(t *testing.T) {
		if KeywordHit(input, "") {
			t.Fatal("空关键词不得命中（否则一条空词吞掉全部输入）")
		}
	})
	t.Run("KeywordHitExcept", func(t *testing.T) {
		if KeywordHitExcept(input, "", []string{"排除"}) {
			t.Fatal("空关键词不得命中")
		}
	})
	// 空输入 + 空词：两侧都退化时也不得判命中
	t.Run("KeywordHit/empty-input", func(t *testing.T) {
		if KeywordHit("", "") {
			t.Fatal("空关键词不得命中空输入")
		}
	})

	// —— 挂死族：必须立即返回 false
	t.Run("KeywordHitBoundary", func(t *testing.T) {
		withWatchdog(t, "KeywordHitBoundary", func() {
			if KeywordHitBoundary("abc 正常内容", "") {
				t.Errorf("空关键词不得命中")
			}
		})
	})
	t.Run("KeywordHitBoundaryExcept", func(t *testing.T) {
		withWatchdog(t, "KeywordHitBoundaryExcept", func() {
			if KeywordHitBoundaryExcept("abc 正常内容", "", []string{"排除"}) {
				t.Errorf("空关键词不得命中")
			}
		})
	})
	t.Run("KeywordPostNegated", func(t *testing.T) {
		withWatchdog(t, "KeywordPostNegated", func() {
			if KeywordPostNegated("正常内容", "") {
				t.Errorf("空关键词无「被否定」可言，应判否")
			}
		})
	})
}

// TestEmptyMaskIsSkippedNotAllMasking 空 mask 是同一类装配错误的另一面：
// 它容不下任何非空命中，故不得把「一切都被排除」写进语义（那会让
// KeywordHitExcept 恒返回 false，即整张排除表失效）。此处钉的是**成对**结论：
// 空 mask 不影响判定，非空 mask 照常生效。
func TestEmptyMaskIsSkippedNotAllMasking(t *testing.T) {
	// 空 mask 放行⇒命中成立（若把空 mask 当成「包含一切」，这里会是 false）
	if !KeywordHitExcept("怎么执行", "执行", []string{""}) {
		t.Fatal("空 mask 不得排除命中（空区间容不下非空命中）")
	}
	// 成对负对照：真 mask 仍然生效
	if KeywordHitExcept("怎么执行", "执行", []string{"怎么执行"}) {
		t.Fatal("非空 mask 应排除该命中")
	}
	// 词边界变体同样不被空 mask 干扰
	if !KeywordHitBoundaryExcept("kill me", "kill", []string{""}) {
		t.Fatal("空 mask 不得排除词边界命中")
	}
}
