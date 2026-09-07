package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

// 本文件为 work_task 领域行仓储守卫测试：共享主键（id=所属 task.id）写入收口三防线——
// 工厂非正 id panic、CreateForTask 覆写调用方 id、外键强制库下挂不存在任务行被拒；
// 附双源组装与单实体组装的 TaskDTO 等价锚点。

// TestNewWorkTaskPanicsOnNonPositiveID 共享主键非正 id 在工厂口 fail-fast
func TestNewWorkTaskPanicsOnNonPositiveID(t *testing.T) {
	for _, id := range []int64{0, -1} {
		panicked := func() (p bool) {
			defer func() { p = recover() != nil }()
			entity.NewWorkTask(id)
			return
		}()
		if !panicked {
			t.Errorf("NewWorkTask(%d) 应 panic（零值主键会被 SQLite 静默按 rowid 分配）", id)
		}
	}
}

// TestWorkTaskCreateForTask 创建收口：入参 id 被覆写为 taskID（落库行与核心行同主键）、
// 非正任务 id 拒绝、指向不存在任务行的领域行被 id→task 外键拒绝
func TestWorkTaskCreateForTask(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewWorkTaskRepository(db)
	ctx := context.Background()

	seed := entity.NewTask()
	seed.TaskName = sql.NullString{String: "插件任务", Valid: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("建任务核心行失败: %v", err)
	}

	// 入参 id 错挂也被覆写为 taskID，落库行与核心行同主键
	wt := entity.NewWorkTask(seed.GetID())
	wt.SetID(seed.GetID() + 999999)
	wt.SiteWorkID = sql.NullString{String: "w-1", Valid: true}
	wt.URL = sql.NullString{String: "http://x/1", Valid: true}
	wt.PluginPublicID = sql.NullString{String: "pub-1", Valid: true}
	wt.PluginExtensionID = sql.NullString{String: "ext-1", Valid: true}
	wt.PluginData = sql.NullString{String: `{"schemaVersion":1}`, Valid: true}
	wt.StoreRoles = sql.NullString{String: "image,thumbnail", Valid: true}
	wt.InvolvedRoles = sql.NullString{String: "image", Valid: true}
	wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}
	wt.IncludeWorkInfo = true
	if err := repo.CreateForTask(ctx, seed.GetID(), wt); err != nil {
		t.Fatalf("创建领域行失败: %v", err)
	}
	if wt.GetID() != seed.GetID() {
		t.Fatalf("落库后领域行主键应为任务 id %d，实际 %d", seed.GetID(), wt.GetID())
	}

	got, err := repo.GetById(ctx, seed.GetID())
	if err != nil {
		t.Fatalf("按任务 id 查领域行失败: %v", err)
	}
	if got.SiteWorkID.String != "w-1" || got.PluginPublicID.String != "pub-1" ||
		got.ResourceType.String != entity.ResourceTypeImage || !got.IncludeWorkInfo {
		t.Fatalf("领域字段往返不符: %+v", got)
	}
	if got.GetCreateTime() == 0 || got.GetUpdateTime() == 0 {
		t.Fatalf("领域行时间戳应填充: create=%d update=%d", got.GetCreateTime(), got.GetUpdateTime())
	}

	// 非正任务 id：拒绝
	bad := entity.NewWorkTask(seed.GetID())
	if err := repo.CreateForTask(ctx, 0, bad); err == nil {
		t.Fatal("非正任务 id 应拒绝")
	}

	// 指向不存在任务行：id→task 外键拒绝
	ghostID := seed.GetID() + 424242
	ghost := entity.NewWorkTask(ghostID)
	if err := repo.CreateForTask(ctx, ghostID, ghost); err == nil {
		t.Fatal("挂不存在任务行的领域行应被外键拒绝")
	}

	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM work_task").Scan(&n).Error; err != nil {
		t.Fatalf("计数 work_task 行失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("work_task 应恰 1 行（仅首次创建成功），实际 %d 行", n)
	}
}

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
	wt.PendingResourceID = sql.NullInt64{Int64: 55, Valid: true}
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
		"status": float64(3), "pendingResourceId": float64(55), "continuable": true,
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
