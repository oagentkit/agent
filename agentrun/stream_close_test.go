package agentrun

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// streamClosed 验证子流已被 streamDeltas 关闭：已 Close 的子流再 Recv 返回
// ErrRecvAfterClosed；未 Close 的子流（缺陷形态）恒可重复 Recv 出缓存 io.EOF。
// Copy(2) 扇出桩复刻 eino ADK 对模型流的分发形态（llm#80 同款探针）。
func streamClosed(t *testing.T, kid *schema.StreamReader[*schema.Message]) bool {
	t.Helper()
	_, err := kid.Recv()
	return errors.Is(err, schema.ErrRecvAfterClosed)
}

// 回归（oagentkit/agent#77）：eino 契约「reader 必须恰好 Close 一次——无论
// 循环正常结束还是 break/return 提前退出」（schema/stream.go，与 llm#80 同源
// 纪律）。streamDeltas 此前只有 Recv 错误路径关流：干净 EOF break、零分片、
// concat 错误三条路径全漏——ADK 事件流是 Copy 扇出的子副本，子流不 Close 则
// 父流 closedNum 永不收口，上游生产者 goroutine 挂死在 Send 上。四条退出路径
// 逐一钉住。
func TestStreamDeltasClosesOnEveryExitPath(t *testing.T) {
	h := &reactHandler{ctx: context.Background()}
	newVariant := func(kid *schema.StreamReader[*schema.Message]) *adk.MessageVariant {
		return &adk.MessageVariant{IsStreaming: true, MessageStream: kid, Role: schema.Assistant}
	}

	// ① 干净 EOF（正常完成路径）：两分片收满 EOF 后 concat 成功
	base, sw := schema.Pipe[*schema.Message](2)
	_ = sw.Send(&schema.Message{Role: schema.Assistant, Content: "a"}, nil)
	_ = sw.Send(&schema.Message{Role: schema.Assistant, Content: "b"}, nil)
	sw.Close()
	kids := base.Copy(2)
	full, err := h.streamDeltas(newVariant(kids[0]))
	if err != nil {
		t.Fatalf("EOF 路径不应报错: %v", err)
	}
	if full == nil || full.Content != "ab" {
		t.Fatalf("concat 结果不符: %v", full)
	}
	if !streamClosed(t, kids[0]) {
		t.Fatal("EOF 后必须 Close 流（eino 契约 exactly-once，agent#77）")
	}
	kids[1].Close()

	// ② 零分片错误路径（agent#46 形态：连接开即断，干净 EOF 零分片）
	base2, sw2 := schema.Pipe[*schema.Message](1)
	sw2.Close()
	kids2 := base2.Copy(2)
	_, err = h.streamDeltas(newVariant(kids2[0]))
	if err == nil || !strings.Contains(err.Error(), "零分片") {
		t.Fatalf("零分片应报错: %v", err)
	}
	if !streamClosed(t, kids2[0]) {
		t.Fatal("零分片错误 return 前必须 Close 流")
	}
	kids2[1].Close()

	// ③ Recv 错误路径（修复前唯一的关流路径——回归钉住不退化）
	base3, sw3 := schema.Pipe[*schema.Message](1)
	_ = sw3.Send(nil, errors.New("boom"))
	kids3 := base3.Copy(2)
	_, err = h.streamDeltas(newVariant(kids3[0]))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Recv 错误应上抛: %v", err)
	}
	if !streamClosed(t, kids3[0]) {
		t.Fatal("Recv 错误路径必须 Close 流")
	}
	kids3[1].Close()

	// ④ concat 错误路径（分片角色不一致 → ConcatMessages 失败）
	base4, sw4 := schema.Pipe[*schema.Message](2)
	_ = sw4.Send(&schema.Message{Role: schema.Assistant, Content: "a"}, nil)
	_ = sw4.Send(&schema.Message{Role: schema.Tool, Content: "r"}, nil)
	sw4.Close()
	kids4 := base4.Copy(2)
	if _, err := h.streamDeltas(newVariant(kids4[0])); err == nil {
		t.Fatal("分片角色不一致应 concat 报错")
	}
	if !streamClosed(t, kids4[0]) {
		t.Fatal("concat 错误 return 前必须 Close 流")
	}
	kids4[1].Close()
}
