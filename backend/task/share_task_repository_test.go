package task

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

// 本文件为 share_task 领域行仓储守卫测试：共享主键（id=所属 task.id）写入收口三防线——
// 工厂非正 id panic、CreateForTask 覆写调用方 id、外键强制库下挂不存在任务行被拒。

// TestNewShareTaskPanicsOnNonPositiveID 共享主键非正 id 在工厂口 fail-fast
func TestNewShareTaskPanicsOnNonPositiveID(t *testing.T) {
	for _, id := range []int64{0, -1} {
		panicked := func() (p bool) {
			defer func() { p = recover() != nil }()
			entity.NewShareTask(id)
			return
		}()
		if !panicked {
			t.Errorf("NewShareTask(%d) 应 panic（零值主键会被 SQLite 静默按 rowid 分配）", id)
		}
	}
}

// TestShareTaskCreateForTask 创建收口：入参 id 被覆写为 taskID（落库行与核心行同主键）、
// 非正任务 id 拒绝、指向不存在任务行的领域行被 id→task 外键拒绝
func TestShareTaskCreateForTask(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewShareTaskRepository(db)
	ctx := context.Background()

	seed := entity.NewTask()
	seed.TaskName = sql.NullString{String: "收件任务", Valid: true}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("建任务核心行失败: %v", err)
	}

	st := entity.NewShareTask(seed.GetID())
	st.SetID(seed.GetID() + 999999)
	st.RelayDial = "127.0.0.1:9000"
	st.RelayHost = "https://relay.example.com"
	st.Token = "tok-1"
	st.KeyB64 = "a2V5"
	st.PasswordHash = "5f4dcc3b5aa765d61d8327deb882cf99"
	st.ManifestPath = "share/inbox/m-1.json"
	st.ManifestID = 42
	if err := repo.CreateForTask(ctx, seed.GetID(), st); err != nil {
		t.Fatalf("创建领域行失败: %v", err)
	}
	if st.GetID() != seed.GetID() {
		t.Fatalf("落库后领域行主键应为任务 id %d，实际 %d", seed.GetID(), st.GetID())
	}

	got, err := repo.GetById(ctx, seed.GetID())
	if err != nil {
		t.Fatalf("按任务 id 查领域行失败: %v", err)
	}
	if got.RelayDial != "127.0.0.1:9000" || got.RelayHost != "https://relay.example.com" ||
		got.Token != "tok-1" || got.KeyB64 != "a2V5" || got.PasswordHash != "5f4dcc3b5aa765d61d8327deb882cf99" ||
		got.ManifestPath != "share/inbox/m-1.json" || got.ManifestID != 42 {
		t.Fatalf("领域字段往返不符: %+v", got)
	}
	if got.GetCreateTime() == 0 || got.GetUpdateTime() == 0 {
		t.Fatalf("领域行时间戳应填充: create=%d update=%d", got.GetCreateTime(), got.GetUpdateTime())
	}

	// 非正任务 id：拒绝
	bad := entity.NewShareTask(seed.GetID())
	if err := repo.CreateForTask(ctx, 0, bad); err == nil {
		t.Fatal("非正任务 id 应拒绝")
	}

	// 指向不存在任务行：id→task 外键拒绝
	ghostID := seed.GetID() + 424242
	ghost := entity.NewShareTask(ghostID)
	if err := repo.CreateForTask(ctx, ghostID, ghost); err == nil {
		t.Fatal("挂不存在任务行的领域行应被外键拒绝")
	}

	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM share_task").Scan(&n).Error; err != nil {
		t.Fatalf("计数 share_task 行失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("share_task 应恰 1 行（仅首次创建成功），实际 %d 行", n)
	}
}
