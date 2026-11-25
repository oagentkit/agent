package roundguard

import "testing"

// 白盒用例：本包内部的缺省口径（effectivePolicy）是**判定语义**的一部分，
// 外部用例看不到它，但它必须被钉住——缺省口径错了，处置分流整条跟着错。
//
// 合成数据：用中性词面构造事实，不带任何领域味。

// TestEffectivePolicyDefaultsToForceFinal 钉住缺省：零值口径 = 强制收尾
// （腰斩最接近答案的那一刻，代价最大）。
//
// 这条是「把缺省改成 fail_fast（或恒返回 force_final）」会被抓住的反向验证。
func TestEffectivePolicyDefaultsToForceFinal(t *testing.T) {
	if got := (Budget{}).effectivePolicy(); got != PolicyForceFinal {
		t.Fatalf("零值处置口径应缺省为 %q，实得 %q", PolicyForceFinal, got)
	}
}

// TestEffectivePolicyKeepsExplicitChoice 钉住：显式口径不被缺省覆盖
// （两种口径必须分流到不同结论，否则「可配处置」名存实亡）。
func TestEffectivePolicyKeepsExplicitChoice(t *testing.T) {
	if got := (Budget{Policy: PolicyFailFast}).effectivePolicy(); got != PolicyFailFast {
		t.Fatalf("显式口径应原样保留 %q，实得 %q", PolicyFailFast, got)
	}
	if got := (Budget{Policy: PolicyForceFinal}).effectivePolicy(); got != PolicyForceFinal {
		t.Fatalf("显式口径应原样保留 %q，实得 %q", PolicyForceFinal, got)
	}
}
