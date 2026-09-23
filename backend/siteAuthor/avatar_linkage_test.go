package siteAuthor

// 删除联动的头像清理锚定：删除站点作者在同一事务内连带物理删被引用的头像 store 行（含
// 外部裁决失效的软删行——作者行消亡后头像行即无主死行），事务提交后删头像文件。

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/authorInfo"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/reWorkAuthor"
	"github.com/library-squirrel/backend/settings"

	"gorm.io/gorm"
)

// noopDisambiguationMemory 消歧记忆 no-op 替身：本链只走删除联动的头像清理轨道，不触
// 手动拉取面；authorInfo 生产代码对 memory 依赖无 nil 守卫，传 nil 留静默 panic 风险，
// 故以恒未命中替身占位（Recall 恒未命中即回落冲突询问语义，Remember 零行为）
type noopDisambiguationMemory struct{}

func (noopDisambiguationMemory) Remember(context.Context, string, string, string) error {
	return nil
}

func (noopDisambiguationMemory) Recall(context.Context, string, string) (string, bool) {
	return "", false
}

// newAvatarLinkageEnv 内存库（外键强制 + 完整迁移）+ 临时工作目录 + 真实 authorInfo 清理
// 提供方接线（authorInfo 的 local 轨道不被本链触及，传 nil）
func newAvatarLinkageEnv(t *testing.T) (*Service, *gorm.DB, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	settingsSvc := settings.NewService(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsSvc.SaveSettings([]settings.SettingChange{{Path: "workdir", Value: workDir}}); err != nil {
		t.Fatalf("配置工作目录失败: %v", err)
	}
	reWorkAuthorSvc := reWorkAuthor.NewService(reWorkAuthor.NewRepository(db), nil, nil)
	svc := NewService(
		NewRepository(db),
		nil, // LocalAuthorOperator（删除编排不触及）
		nil, // SiteOperator（删除编排不触及）
		&txTransactor{db: db},
		reWorkAuthorSvc,
	)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	aiSvc := authorInfo.NewService(svc, nil, psSvc, psSvc, settingsSvc, settingsSvc, &txTransactor{db: db}, noopDisambiguationMemory{})
	svc.SetAvatarFileCleaner(aiSvc)
	return svc, db, workDir
}

// seedAvatarStoreRowWithFile 建已落盘的头像 store 行（软删标记 softDead 时置 deleted_at），返回行 ID
func seedAvatarStoreRowWithFile(t *testing.T, db *gorm.DB, workDir, relPath string, softDead bool) int64 {
	t.Helper()
	abs := filepath.Join(workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("建头像目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("avatar"), 0o644); err != nil {
		t.Fatalf("写头像文件失败: %v", err)
	}
	row := entity.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("建头像 store 行失败: %v", err)
	}
	if softDead {
		if err := db.Exec("UPDATE persistent_store SET deleted_at = 1000 WHERE id = ?", row.GetID()).Error; err != nil {
			t.Fatalf("软删头像行失败: %v", err)
		}
	}
	return row.GetID()
}

// countStoreRowsIncludeDeleted 含软删行统计（原生 SQL 直查，不受软删 scope 影响）
func countStoreRowsIncludeDeleted(t *testing.T, db *gorm.DB, relPath string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM persistent_store WHERE file_path = ?", relPath).Scan(&n).Error; err != nil {
		t.Fatalf("统计 store 行失败: %v", err)
	}
	return n
}

// TestDeleteSiteAuthorCleansAvatarRowAndFile 删除站点作者：事务内连带物理删被引用头像行，
// 事务后删头像文件；「删除成功」本身即作者行先删（引用释放）后删 store 行的 FK 顺序证明
func TestDeleteSiteAuthorCleansAvatarRowAndFile(t *testing.T) {
	svc, db, workDir := newAvatarLinkageEnv(t)

	if err := db.Exec("INSERT INTO site (id, site_key, site_name, create_time, update_time) VALUES (1, 'pixiv', 'pixiv', 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	relPath := "store/avatar/site/ab/pixiv_sa-1.jpg"
	storeId := seedAvatarStoreRowWithFile(t, db, workDir, relPath, false)
	author := entity.NewSiteAuthor()
	author.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	author.SiteAuthorID = sql.NullString{String: "sa-1", Valid: true}
	author.AvatarStoreID = sql.NullInt64{Int64: storeId, Valid: true}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建站点作者失败: %v", err)
	}

	if err := svc.Delete(context.Background(), author.GetID()); err != nil {
		t.Fatalf("删除站点作者失败: %v", err)
	}
	var authorCount int64
	if err := db.Raw("SELECT COUNT(*) FROM site_author WHERE id = ?", author.GetID()).Scan(&authorCount).Error; err != nil {
		t.Fatalf("统计作者行失败: %v", err)
	}
	if authorCount != 0 {
		t.Fatalf("作者行应消亡，实际 %d", authorCount)
	}
	if n := countStoreRowsIncludeDeleted(t, db, relPath); n != 0 {
		t.Fatalf("头像 store 行应物理删，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(workDir, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
		t.Fatalf("头像文件应已删除 (err=%v)", err)
	}
}

// TestDeleteSiteAuthorPurgesSoftDeletedAvatarRow 引用指向外部裁决失效的软删行时同样连带
// 物理删清（不留无主死行与回收站孤儿条目），软删行对应的文件不存在为容忍态
func TestDeleteSiteAuthorPurgesSoftDeletedAvatarRow(t *testing.T) {
	svc, db, workDir := newAvatarLinkageEnv(t)

	if err := db.Exec("INSERT INTO site (id, site_key, site_name, create_time, update_time) VALUES (1, 'pixiv', 'pixiv', 0, 0)").Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	relPath := "store/avatar/site/cd/pixiv_sa-2.png"
	storeId := seedAvatarStoreRowWithFile(t, db, workDir, relPath, true)
	author := entity.NewSiteAuthor()
	author.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	author.SiteAuthorID = sql.NullString{String: "sa-2", Valid: true}
	author.AvatarStoreID = sql.NullInt64{Int64: storeId, Valid: true}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建站点作者失败: %v", err)
	}

	if err := svc.Delete(context.Background(), author.GetID()); err != nil {
		t.Fatalf("删除引用软删头像行的站点作者失败: %v", err)
	}
	if n := countStoreRowsIncludeDeleted(t, db, relPath); n != 0 {
		t.Fatalf("软删头像行应被一并物理删清，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(workDir, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
		t.Fatalf("软删行对应文件应已删除 (err=%v)", err)
	}
}
