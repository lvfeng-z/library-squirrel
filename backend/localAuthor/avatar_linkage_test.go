package localAuthor

// 删除联动的头像清理锚定：删除本地作者在同一事务内（三类引用清理 + 作者行删除之后）连带
// 物理删被引用的头像 store 行，事务提交后删头像文件——作者行先删释放引用为 FK 强制顺序。

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
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/work"

	"gorm.io/gorm"
)

// newAvatarLinkageEnv 内存库（外键强制 + 完整迁移）+ 临时工作目录 + 真实 authorInfo 清理
// 提供方接线（本链不触 local 头像导入轨道，authorInfo 的 site 轨道以完整 siteAuthor 服务参与）
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
		&txTransactor{db: db},
		siteAuthor.NewRepository(db),
		reWorkAuthorSvc,
		work.NewRepository(db),
	)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	saSvc := siteAuthor.NewService(siteAuthor.NewRepository(db), nil, nil, &txTransactor{db: db}, reWorkAuthorSvc)
	aiSvc := authorInfo.NewService(saSvc, svc, psSvc, psSvc, settingsSvc, settingsSvc, &txTransactor{db: db})
	svc.SetAvatarFileCleaner(aiSvc)
	return svc, db, workDir
}

// TestDeleteLocalAuthorCleansAvatarRowAndFile 删除本地作者：事务内连带物理删被引用头像行，
// 事务后删头像文件。删除成功本身即三类引用清理 + 作者行先删（引用释放）后删 store 行的
// FK 顺序证明（任一步顺序颠倒，外键强制库直接违约报错）
func TestDeleteLocalAuthorCleansAvatarRowAndFile(t *testing.T) {
	svc, db, workDir := newAvatarLinkageEnv(t)

	author := entity.NewLocalAuthor()
	author.AuthorName = sql.NullString{String: "作者乙", Valid: true}
	relPath := "store/avatar/local/ef/local_1.png"
	row := entity.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("建头像 store 行失败: %v", err)
	}
	abs := filepath.Join(workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("建头像目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("avatar"), 0o644); err != nil {
		t.Fatalf("写头像文件失败: %v", err)
	}
	author.AvatarStoreID = sql.NullInt64{Int64: row.GetID(), Valid: true}
	if err := db.Create(author).Error; err != nil {
		t.Fatalf("建本地作者失败: %v", err)
	}

	if err := svc.Delete(context.Background(), author.ID); err != nil {
		t.Fatalf("删除本地作者失败: %v", err)
	}
	var authorCount int64
	if err := db.Raw("SELECT COUNT(*) FROM local_author WHERE id = ?", author.ID).Scan(&authorCount).Error; err != nil {
		t.Fatalf("统计作者行失败: %v", err)
	}
	if authorCount != 0 {
		t.Fatalf("作者行应消亡，实际 %d", authorCount)
	}
	var storeCount int64
	if err := db.Raw("SELECT COUNT(*) FROM persistent_store WHERE file_path = ?", relPath).Scan(&storeCount).Error; err != nil {
		t.Fatalf("统计 store 行失败: %v", err)
	}
	if storeCount != 0 {
		t.Fatalf("头像 store 行应物理删，实际 %d", storeCount)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("头像文件应已删除 (err=%v)", err)
	}
}
