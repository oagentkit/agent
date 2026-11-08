package dispatch

import (
	"errors"
	"testing"
)

type fakeEdges map[string][]string

func (f fakeEdges) Slugs() []string {
	out := make([]string, 0, len(f))
	for slug := range f {
		out = append(out, slug)
	}
	return out
}
func (f fakeEdges) DispatchAllow(slug string) []string { return f[slug] }

func TestGuardAssert(t *testing.T) {
	g := NewGuard(fakeEdges{"a": {"b"}, "b": {}}, 3)
	if err := g.Assert("a", "b", 0); err != nil {
		t.Fatalf("allow 边应放行: %v", err)
	}
	var deny *DenyError
	if err := g.Assert("a", "c", 0); !errors.As(err, &deny) || deny.Reason != "not_allowed" {
		t.Fatalf("未登记边应 not_allowed: %v", err)
	}
	if err := g.Assert("a", "a", 0); !errors.As(err, &deny) || deny.Reason != "self_dispatch" {
		t.Fatalf("自派发应 self_dispatch: %v", err)
	}
	if err := g.Assert("a", "b", 3); !errors.As(err, &deny) || deny.Reason != "depth_exceeded" {
		t.Fatalf("深度触顶应 depth_exceeded: %v", err)
	}
	if err := g.Assert("x", "b", 0); !errors.As(err, &deny) || deny.Reason != "not_allowed" {
		t.Fatalf("未登记 slug 应 not_allowed: %v", err)
	}
	if err := g.AssertDepth(2); err != nil {
		t.Fatalf("深度内应放行: %v", err)
	}
	if !g.Allowed("a", "b") || g.Allowed("b", "a") || g.Allowed("a", "x") {
		t.Fatal("Allowed 单向：登记边为 true、反向与未登记为 false")
	}
}

// TestNewGuardRejectsNonPositiveDepth 零值/负值 maxDepth 构造期 panic（agent#89）。
//
// 此前 maxDepth<=0 静默构造出「一切派发必拒」的守卫：运行期症状是「整条引擎派发
// 全被拒但原因文案指向深度」，宿主顺着文案排查限深配置永远找不到病根（真实病因
// 是漏配），审计侧还把每次拒绝当真实限深事件记录。同函数对矩阵环已 fail-fast
// panic——深度漏配同为装配错误，风格必须一致。零值语义不做「不设限」回退：
// 白名单默认拒绝的守卫不该有无界深度这条暗门。
func TestNewGuardRejectsNonPositiveDepth(t *testing.T) {
	for _, depth := range []int{0, -1, -7} {
		d := depth
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewGuard(src, %d) 应 panic——漏配深度必须在装配期暴露而非运行期静默全拒", d)
				}
			}()
			NewGuard(fakeEdges{"a": {"b"}}, d)
		}()
	}
	// 合法边界仍照常构造：maxDepth=1 允许一层派发（depth=0 放行、depth=1 拒）。
	g := NewGuard(fakeEdges{"a": {"b"}}, 1)
	if err := g.Assert("a", "b", 0); err != nil {
		t.Fatalf("maxDepth=1 时 depth=0 应放行: %v", err)
	}
	var deny *DenyError
	if err := g.Assert("a", "b", 1); !errors.As(err, &deny) || deny.Reason != ReasonDepthExceeded {
		t.Fatalf("maxDepth=1 时 depth=1 应 depth_exceeded: %v", err)
	}
}
