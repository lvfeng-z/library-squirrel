package task

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// 本文件为 TaskDTO 组装契约锚点：核心行+作品领域行双源组装与单实体组装（AssembleTaskDTO）
// 输出字段集固定——前端 JSON 契约（除 taskType 透出值显式化外）不变的守卫。

// TestAssembleTaskDTOContract 组装契约锚点：核心行+领域行组装（AssembleTaskDTO）输出字段集
// 固定——前端 JSON 契约（除 taskType 透出值显式化外）不变的守卫
func TestAssembleTaskDTOContract(t *testing.T) {
	task := entity.NewTask()
	task.SetID(7)
	task.SetCreateTime(1000)
	task.SetUpdateTime(2000)
	task.HasChild = sql.NullBool{Bool: false, Valid: true}
	task.TaskName = sql.NullString{String: "任务", Valid: true}
	task.Status = 3
	task.ErrorMessage = sql.NullString{String: "e", Valid: true}
	task.TaskType = sql.NullString{String: entity.TaskTypePluginDownload, Valid: true}
	wt := entity.NewWorkTask(7)
	wt.SiteID = sql.NullInt64{Int64: 100, Valid: true}
	wt.SiteWorkID = sql.NullString{String: "w-1", Valid: true}
	wt.URL = sql.NullString{String: "http://x/1", Valid: true}
	wt.Continuable = sql.NullBool{Bool: true, Valid: true}
	wt.PluginPublicID = sql.NullString{String: "pub-1", Valid: true}
	wt.PluginExtensionID = sql.NullString{String: "ext-1", Valid: true}
	wt.PluginData = sql.NullString{String: `{"a":1}`, Valid: true}
	wt.InvolvedRoles = sql.NullString{String: "image, thumbnail", Valid: true}
	wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}

	got, err := json.Marshal(dto.AssembleTaskDTO(task, wt, nil))
	if err != nil {
		t.Fatalf("序列化组装结果失败: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("反解组装结果失败: %v", err)
	}
	want := map[string]any{
		"id": float64(7), "createTime": float64(1000), "updateTime": float64(2000),
		"hasChild": false, "taskName": "任务",
		"siteId": float64(100), "siteWorkId": "w-1", "url": "http://x/1",
		"status": float64(3), "continuable": true,
		"pluginPublicId": "pub-1", "pluginExtensionId": "ext-1", "pluginData": `{"a":1}`,
		"errorMessage": "e", "involvedRoles": []any{"image", "thumbnail"},
		"resourceType": entity.ResourceTypeImage, "taskType": entity.TaskTypePluginDownload,
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("组装 JSON 契约不符:\n期望 %v\n实际 %v", want, fields)
	}

	// 领域行缺失（内置类型任务）：领域字段整体缺省（指针零值经 omitempty 略去），控制字段照常
	noDomain, err := json.Marshal(dto.AssembleTaskDTO(task, nil, nil))
	if err != nil {
		t.Fatalf("序列化无领域行组装失败: %v", err)
	}
	if strings.Contains(string(noDomain), `"siteId"`) || !strings.Contains(string(noDomain), `"taskName":"任务"`) {
		t.Fatalf("无领域行组装应略去领域字段并保留控制字段: %s", noDomain)
	}
}
