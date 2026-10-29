package extension

import (
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"
)

// 子行批次块（children_batch，契约 21）的代理侧行为锚定：
//   - 流式 mode：接收泵按到达序把批次块透传为条目形态（指称父 ID、子行清单、末批标志原样）。
//   - 批量 mode：批次块仅流式合法，批量流上出现即插件协议违规，按流错误收流。

// childrenBatchChunk 构造承载子行批次块的块。
func childrenBatchChunk(pluginTaskId string, last bool, siteWorkIds ...string) *gen.CreateChunk {
	items := make([]*gen.TaskCreateChildResponse, 0, len(siteWorkIds))
	for _, id := range siteWorkIds {
		items = append(items, &gen.TaskCreateChildResponse{TaskName: "c-" + id, SiteWorkId: id, Url: "http://x/" + id})
	}
	return &gen.CreateChunk{Payload: &gen.CreateChunk_ChildrenBatch{
		ChildrenBatch: &gen.TaskChildrenBatch{PluginTaskId: pluginTaskId, Items: items, Last: last},
	}}
}

// TestProxyCreateStream_ForwardsChildrenBatch 流式 mode：task 块与 children_batch 块按到达序
// 透传为条目（task 形态/批次形态字段完整转换，last 透传）。
func TestProxyCreateStream_ForwardsChildrenBatch(t *testing.T) {
	proxy := newCreateProxy(
		&gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: true}}},
		taskChunk("w-1"),
		childrenBatchChunk("work-X", false, "c-1", "c-2"),
		childrenBatchChunk("work-X", true, "c-3"),
	)

	result, err := proxy.Create("http://x")
	if err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if !result.IsStream() {
		t.Fatal("期望流式模式，得到批量")
	}

	var taskSeen, batchSeen int
	for item := range result.Items() {
		if resp := item.Task(); resp != nil {
			taskSeen++
			if resp.SiteWorkId != "w-1" {
				t.Fatalf("task 块条目应透传任务声明，得到 SiteWorkId=%q", resp.SiteWorkId)
			}
			continue
		}
		batch := item.ChildrenBatch()
		if batch == nil {
			t.Fatalf("条目应恰居 task/children_batch 一形态，得到两者皆空: %+v", item)
		}
		batchSeen++
		switch batchSeen {
		case 1:
			if batch.GetPluginTaskId() != "work-X" || batch.GetLast() {
				t.Fatalf("首批次块应为 work-X 非末批，得到 %+v", batch)
			}
			if ids := len(batch.GetItems()); ids != 2 {
				t.Fatalf("首批次块应含 2 行子行，得到 %d", ids)
			}
			if first := batch.GetItems()[0]; first.TaskName != "c-c-1" || first.Url != "http://x/c-1" {
				t.Fatalf("批次子行字段转换不符，得到 %+v", first)
			}
		case 2:
			if !batch.GetLast() || len(batch.GetItems()) != 1 {
				t.Fatalf("末批次块应为 last=true 且 1 行子行，得到 %+v", batch)
			}
		default:
			t.Fatalf("期望恰 2 个批次块条目，第 %d 个到达", batchSeen)
		}
	}
	if taskSeen != 1 || batchSeen != 2 {
		t.Fatalf("期望 1 个 task 条目 + 2 个批次条目，得到 task=%d batch=%d", taskSeen, batchSeen)
	}
}

// TestProxyCreate_BatchModeChildrenBatchViolates 批量 mode 流上出现 children_batch 块：
// 仅流式 mode 合法的分帧载荷出现在批量流即插件协议违规，按流错误收流（不落成截断的批量结果）。
func TestProxyCreate_BatchModeChildrenBatchViolates(t *testing.T) {
	proxy := newCreateProxy(
		&gen.CreateChunk{Payload: &gen.CreateChunk_Mode{Mode: &gen.CreateMode{IsStream: false}}},
		taskChunk("w-1"),
		childrenBatchChunk("work-X", true, "c-1"),
	)

	if result, err := proxy.Create("http://x"); err == nil {
		t.Fatalf("批量 mode 出现 children_batch 应按流错误收流，得到结果 %+v", result)
	}
}
