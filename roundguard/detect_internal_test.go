package roundguard

import "testing"

// 白盒用例：三个尾部判据的**空序列守卫**是判定面的一部分，但 `Detect` 在空
// 序列上提前收口 ⇒ 正常路径走不到那三个守卫 ⇒ 守卫写错（索引越界 / 空指针）
// 不会有任何用例会红。故白盒直钉。
//
// 合成数据：用中性词面构造事实，不带任何领域味。

// TestTrailingHelpersOnEmptySeq 钉住空输入：三个尾部判据在空序列上各自返回
// 零值（不 panic、不越界）。
func TestTrailingHelpersOnEmptySeq(t *testing.T) {
	if got := trailingSameFingerprint(nil); got != 0 {
		t.Fatalf("空序列的同指纹连续长度应为 0，实得 %d", got)
	}
	if got := trailingSameNormalized(nil); got != 0 {
		t.Fatalf("空序列的归一指纹连续长度应为 0，实得 %d", got)
	}
	if allSameFingerprint(nil) {
		t.Fatal("空序列不该判为原指纹全等")
	}
}

// TestAllSameFingerprint 钉住「原指纹是否全等」这条**守卫**的判定口径：
// 全等 ⇒ true；有一个不等 ⇒ false。
//
// 为什么白盒直钉：它在漂移分支里是**冗余守卫**——判定次序（重复在漂移之前，
// 两者同用阈值 k）保证「原指纹全等」的那串先被重复判走 ⇒ 这个分支在整条判定链
// 上**到不了** ⇒ 黑盒观察不到。直钉它的口径，是为了让「冗余」这件事有据可查，
// 而不是让它悄悄变形。
func TestAllSameFingerprint(t *testing.T) {
	same := []Observation{{Fingerprint: "a"}, {Fingerprint: "a"}}
	if !allSameFingerprint(same) {
		t.Fatal("原指纹全等应判 true")
	}
	diff := []Observation{{Fingerprint: "a"}, {Fingerprint: "b"}}
	if allSameFingerprint(diff) {
		t.Fatal("原指纹不全等应判 false")
	}
}
