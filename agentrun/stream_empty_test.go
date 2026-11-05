package agentrun

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// 流式零分片成因区分（agent#46）：零分片 = 传输/网关断流，上抛走重试——
// 不再合成 (空消息, nil) 成功形态（此前瞬断被下游「空内容」判定放大成整程
// 重试重放工具采集）；有分片但内容空（真空答复）仍走原形态。
func TestStreamDeltasZeroChunkCause(t *testing.T) {
	h := &reactHandler{ctx: context.Background()}

	// ① 零分片（干净 EOF 即断）：必须报错非空成功
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close() // 关流即断，一个分片都没有
	mv := &adk.MessageVariant{IsStreaming: true, MessageStream: sr, Role: schema.Assistant}
	_, err := h.streamDeltas(mv)
	if err == nil {
		t.Fatal("零分片应报错（传输断流）")
	}
	if !strings.Contains(err.Error(), "零分片") {
		t.Fatalf("错误应说明成因: %v", err)
	}

	// ② 有分片但内容空（真空答复形态）：保持空消息成功（走原「空答复」判定面）
	sr2, sw2 := schema.Pipe[*schema.Message](1)
	_ = sw2.Send(&schema.Message{Role: schema.Assistant, Content: ""}, nil)
	sw2.Close()
	mv2 := &adk.MessageVariant{IsStreaming: true, MessageStream: sr2, Role: schema.Assistant}
	msg, err := h.streamDeltas(mv2)
	if err != nil {
		t.Fatalf("真空答复不应报错: %v", err)
	}
	if msg.Content != "" {
		t.Fatalf("真空答复内容应空: %q", msg.Content)
	}
}
