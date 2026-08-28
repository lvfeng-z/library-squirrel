package recycleBin

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// mockWorkRestorer WorkRestorer 测试替身：按 workId 返回预置的 store 行集合
type mockWorkRestorer struct {
	storesByWork map[int64][]*domain.PersistentStore
	restoredIds  []int64
}

func (m *mockWorkRestorer) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	return nil, nil
}
func (m *mockWorkRestorer) SoftDeleteWork(ctx context.Context, workId int64) error { return nil }
func (m *mockWorkRestorer) GetDeletedWork(ctx context.Context, id int64) (*domain.Work, error) {
	w := domain.NewWork()
	w.SetID(id)
	return w, nil
}
func (m *mockWorkRestorer) RestoreDeletedWork(ctx context.Context, id int64) error { return nil }
func (m *mockWorkRestorer) RestoreWorkStores(ctx context.Context, workId int64) error {
	m.restoredIds = append(m.restoredIds, workId)
	return nil
}
func (m *mockWorkRestorer) ListWorkStoresIncludeDeleted(ctx context.Context, workId int64) []*domain.PersistentStore {
	return m.storesByWork[workId]
}
func (m *mockWorkRestorer) ListRevivableWorkStores(ctx context.Context, workId int64) []*domain.PersistentStore {
	return m.storesByWork[workId]
}
func (m *mockWorkRestorer) ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.Work, error) {
	return nil, nil
}
func (m *mockWorkRestorer) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
	return nil
}

// recordingBackupReader BackupReader 测试替身：记录被取回与被删除的清单行 ID
type recordingBackupReader struct {
	restoredIds []int64
	deletedIds  []int64
}

func (r *recordingBackupReader) GetById(ctx context.Context, id int64) (*domain.Backup, error) {
	b := domain.NewBackup()
	b.SetID(id)
	return b, nil
}
func (r *recordingBackupReader) GetBackupPath(backup *domain.Backup) string {
	return filepath.Join(os.TempDir(), "mock_backup", fmt.Sprintf("%d.mp4", backup.GetID()))
}
func (r *recordingBackupReader) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	return nil
}
func (r *recordingBackupReader) DeleteBackup(ctx context.Context, id int64) error {
	r.deletedIds = append(r.deletedIds, id)
	return nil
}
func (r *recordingBackupReader) DeleteBackupFile(ctx context.Context, id int64) error {
	r.deletedIds = append(r.deletedIds, id)
	return nil
}
func (r *recordingBackupReader) DeleteBackupRecord(ctx context.Context, id int64) error {
	r.deletedIds = append(r.deletedIds, id)
	return nil
}

// TestRestoreWorkFilesGenerationIsolation 多代同路径已删作品复原隔离：
// 复原作品 A 只消耗 A 各行 backup_id 指向的备份，作品 B 的备份原封不动。
// 回归锚定：曾按 original_file_path 反查命中全部代次、复原 A 误删 B 的全部备份
func TestRestoreWorkFilesGenerationIsolation(t *testing.T) {
	const sharedPath = "store/resource/作者/同路径文件.mp4"
	newRow := func(backupId int64) *domain.PersistentStore {
		row := domain.NewPersistentStore()
		row.BackupID = sql.NullInt64{Int64: backupId, Valid: true}
		row.FilePath = sql.NullString{String: sharedPath, Valid: true}
		return row
	}
	// 作品 A（workId=1）与作品 B（workId=2）的同路径已删行，各指各的备份清单行
	restorer := &mockWorkRestorer{storesByWork: map[int64][]*domain.PersistentStore{
		1: {newRow(101)},
		2: {newRow(202)},
	}}
	reader := &recordingBackupReader{}
	svc := NewService(restorer, reader, nil, nil, nil, nil, nil, nil, func() string { return t.TempDir() }, nil, nil, nil, nil, nil, nil)

	if err := svc.restoreWorkFiles(context.Background(), 1); err != nil {
		t.Fatalf("复原作品 1 失败: %v", err)
	}
	if len(reader.deletedIds) != 1 || reader.deletedIds[0] != 101 {
		t.Fatalf("复原作品 1 须仅删其行内引用的备份 101，实际删除 %v（作品 2 的备份 202 被波及即为回归）", reader.deletedIds)
	}
}
