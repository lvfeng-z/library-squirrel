package authorInfo

// local 侧头像手动导入与删除联动清理锚定：导入链（格式白名单 → 拷入暂存 → 四调用入库 →
// 引用列同事务）、换头像先删旧（同扩展名同路径不做覆写、不撞 file_path 活行唯一索引）、
// 移除头像（置 NULL + 删行 + 删文件）、DeleteAvatarStoreRows 的 FK 调用契约（作者行删除
// 前删 store 行被外键拒绝、删除后放行的对照）。

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
)

// seedLocalAuthor 建本地作者种子行，返回作者行
func (env *fetchTestEnv) seedLocalAuthor(t *testing.T) *entity.LocalAuthor {
	t.Helper()
	row := entity.NewLocalAuthor()
	row.AuthorName = sql.NullString{String: "本地作者甲", Valid: true}
	if err := env.db.Create(row).Error; err != nil {
		t.Fatalf("建本地作者种子失败: %v", err)
	}
	return row
}

// loadLocalAuthor 回查本地作者行
func (env *fetchTestEnv) loadLocalAuthor(t *testing.T, id int64) *entity.LocalAuthor {
	t.Helper()
	var row entity.LocalAuthor
	if err := env.db.First(&row, id).Error; err != nil {
		t.Fatalf("回查本地作者失败: %v", err)
	}
	return &row
}

// writeSourceImage 在工作目录之外的临时目录写源头像文件（源可在任意盘，链路恒 copy），
// 返回绝对路径
func writeSourceImage(t *testing.T, name, content string) string {
	t.Helper()
	srcDir := t.TempDir()
	abs := filepath.Join(srcDir, name)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("写源头像文件失败: %v", err)
	}
	return abs
}

// setLocalAuthorRef 直写引用列（种子态构造，绕开被测链路）
func (env *fetchTestEnv) setLocalAuthorRef(t *testing.T, localAuthorId, storeId int64) {
	t.Helper()
	if err := env.db.Exec("UPDATE local_author SET avatar_store_id = ? WHERE id = ?", storeId, localAuthorId).Error; err != nil {
		t.Fatalf("置本地作者引用列失败: %v", err)
	}
}

// TestSetLocalAuthorAvatarIngestsAndReferences 导入全链：源文件拷入暂存（源保留，非移动）→
// 四调用入库（store 行落 store/avatar/local/、登记行收口、作用域回收）→ 引用列指向新行
func TestSetLocalAuthorAvatarIngestsAndReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	src := writeSourceImage(t, "头像.png", "new-avatar-bytes")

	if err := env.svc.SetLocalAuthorAvatar(context.Background(), author.ID, src); err != nil {
		t.Fatalf("导入本地作者头像失败: %v", err)
	}

	wantRel, err := LocalAvatarRelPath(author.ID, "png")
	if err != nil {
		t.Fatalf("派生期望路径失败: %v", err)
	}
	if n := env.countStoreRows(t, wantRel); n != 1 {
		t.Fatalf("头像 store 行应恰一行，实际 %d", n)
	}
	got, err := os.ReadFile(filepath.Join(env.workDir, filepath.FromSlash(wantRel)))
	if err != nil {
		t.Fatalf("头像文件应已落位: %v", err)
	}
	if string(got) != "new-avatar-bytes" {
		t.Fatalf("头像内容应为拷贝源内容，实际 %q", string(got))
	}
	loaded := env.loadLocalAuthor(t, author.ID)
	if !loaded.AvatarStoreID.Valid {
		t.Fatalf("引用列应指向新行，实际 NULL")
	}
	var refPath string
	if err := env.db.Raw("SELECT file_path FROM persistent_store WHERE id = ?", loaded.AvatarStoreID.Int64).Scan(&refPath).Error; err != nil || refPath != wantRel {
		t.Fatalf("引用列应指向新行（%s），实际 %q (err=%v)", wantRel, refPath, err)
	}
	// 源文件保留（copy 非 move）；登记行收口；作用域回收
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("源头像文件应保留: %v", err)
	}
	if n := env.journalCount(t); n != 0 {
		t.Fatalf("登记行应收口，实际残留 %d", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, "staging", "author-info",
		localAvatarScopeKeyPrefix+strconv.FormatInt(author.ID, 10))); err == nil {
		t.Fatalf("导入作用域应已回收")
	}
}

// TestSetLocalAuthorAvatarReplacesOldAvatarSameExt 同扩展名换头像：新旧最终路径相同——
// 先删旧保证不做同路径覆写、亦不撞 persistent_store file_path 活行唯一索引；旧行旧文件消亡，
// 引用列指向新行
func TestSetLocalAuthorAvatarReplacesOldAvatarSameExt(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	wantRel, err := LocalAvatarRelPath(author.ID, "jpg")
	if err != nil {
		t.Fatalf("派生期望路径失败: %v", err)
	}
	oldStoreId := env.seedAvatarStoreRow(t, wantRel)
	env.setLocalAuthorRef(t, author.ID, oldStoreId)

	src := writeSourceImage(t, "新头像.JPG", "replacement-bytes")
	if err := env.svc.SetLocalAuthorAvatar(context.Background(), author.ID, src); err != nil {
		t.Fatalf("同扩展名换头像失败: %v", err)
	}

	if n := env.countStoreRows(t, wantRel); n != 1 {
		t.Fatalf("同路径应只有新行一行，实际 %d", n)
	}
	var newStoreId int64
	if err := env.db.Raw("SELECT id FROM persistent_store WHERE file_path = ?", wantRel).Scan(&newStoreId).Error; err != nil {
		t.Fatalf("回查新行失败: %v", err)
	}
	if newStoreId == oldStoreId {
		t.Fatalf("同路径行应为新行（id 应不同于旧行）")
	}
	loaded := env.loadLocalAuthor(t, author.ID)
	if !loaded.AvatarStoreID.Valid || loaded.AvatarStoreID.Int64 != newStoreId {
		t.Fatalf("引用列应指向新行 %d，实际 %+v", newStoreId, loaded.AvatarStoreID)
	}
	got, err := os.ReadFile(filepath.Join(env.workDir, filepath.FromSlash(wantRel)))
	if err != nil || string(got) != "replacement-bytes" {
		t.Fatalf("同路径文件内容应为新内容，实际 %q (err=%v)", string(got), err)
	}
}

// TestSetLocalAuthorAvatarRejectsBadFormat 白名单外格式拒绝：不建行、引用列保留旧值
func TestSetLocalAuthorAvatarRejectsBadFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	src := writeSourceImage(t, "notes.txt", "not an image")

	err := env.svc.SetLocalAuthorAvatar(context.Background(), author.ID, src)
	if err == nil || !strings.Contains(err.Error(), "格式") {
		t.Fatalf("白名单外格式应被拒绝，实际 err=%v", err)
	}
	if n := env.journalCount(t); n != 0 {
		t.Fatalf("不应产生登记行，实际 %d", n)
	}
	loaded := env.loadLocalAuthor(t, author.ID)
	if loaded.AvatarStoreID.Valid {
		t.Fatalf("引用列应保持空，实际 %d", loaded.AvatarStoreID.Int64)
	}
}

// TestSetLocalAuthorAvatarOverSizeCap 超尺寸上限拒绝：不建行、不写引用
func TestSetLocalAuthorAvatarOverSizeCap(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "huge.png")
	big := make([]byte, maxAvatarBytes+1)
	if err := os.WriteFile(src, big, 0o644); err != nil {
		t.Fatalf("写超限源头像失败: %v", err)
	}

	err := env.svc.SetLocalAuthorAvatar(context.Background(), author.ID, src)
	if err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("超限应被拒绝，实际 err=%v", err)
	}
	if n := env.journalCount(t); n != 0 {
		t.Fatalf("不应产生登记行，实际 %d", n)
	}
}

// TestSetLocalAuthorAvatarUnknownAuthor 作者行不存在报 404
func TestSetLocalAuthorAvatarUnknownAuthor(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	src := writeSourceImage(t, "头像.png", "x")
	err := env.svc.SetLocalAuthorAvatar(context.Background(), 9999, src)
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("不存在的作者应报错，实际 err=%v", err)
	}
}

// TestRemoveLocalAuthorAvatarClearsRowFileAndRef 移除头像：引用置 NULL、store 行物理删、
// 文件删除
func TestRemoveLocalAuthorAvatarClearsRowFileAndRef(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	wantRel, err := LocalAvatarRelPath(author.ID, "png")
	if err != nil {
		t.Fatalf("派生期望路径失败: %v", err)
	}
	storeId := env.seedAvatarStoreRow(t, wantRel)
	env.setLocalAuthorRef(t, author.ID, storeId)

	if err := env.svc.RemoveLocalAuthorAvatar(context.Background(), author.ID); err != nil {
		t.Fatalf("移除头像失败: %v", err)
	}
	loaded := env.loadLocalAuthor(t, author.ID)
	if loaded.AvatarStoreID.Valid {
		t.Fatalf("引用列应置 NULL，实际 %d", loaded.AvatarStoreID.Int64)
	}
	if n := env.countStoreRows(t, wantRel); n != 0 {
		t.Fatalf("store 行应物理删，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, filepath.FromSlash(wantRel))); !os.IsNotExist(err) {
		t.Fatalf("头像文件应已删除 (err=%v)", err)
	}
}

// TestRemoveLocalAuthorAvatarWithoutReference 无头像时幂等成功
func TestRemoveLocalAuthorAvatarWithoutReference(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedLocalAuthor(t)
	if err := env.svc.RemoveLocalAuthorAvatar(context.Background(), author.ID); err != nil {
		t.Fatalf("无头像移除应幂等成功: %v", err)
	}
}

// TestDeleteAvatarStoreRowsFKGate 删除联动的 FK 调用契约对照：作者行仍在（引用未释放）时在
// 事务内删 store 行被外键拒绝（事务回滚行保留）；作者行删除后同调用放行（行删清、文件路径
// 返回供事务后删文件）
func TestDeleteAvatarStoreRowsFKGate(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	author := env.seedSiteAuthor(t, "pixiv", "sa-fk", nil)
	storeId := env.seedAvatarStoreRow(t, "store/avatar/site/ab/pixiv_sa-fk.jpg")
	if err := env.db.Exec("UPDATE site_author SET avatar_store_id = ? WHERE id = ?", storeId, author.ID).Error; err != nil {
		t.Fatalf("置引用列失败: %v", err)
	}
	tx := &txTransactor{db: env.db}

	// 对照一：作者行未删（引用未释放），事务内删 store 行被 FK 拒绝
	err := tx.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		_, err := env.svc.DeleteAvatarStoreRows(txCtx, []int64{storeId})
		return err
	})
	if err == nil {
		t.Fatalf("引用未释放时删 store 行应被外键拒绝")
	}
	if n := env.countStoreRows(t, "store/avatar/site/ab/pixiv_sa-fk.jpg"); n != 1 {
		t.Fatalf("被拒后事务回滚，store 行应保留，实际 %d", n)
	}
	if err := env.db.First(&entity.SiteAuthor{}, author.ID).Error; err != nil {
		t.Fatalf("被拒后作者行应保留: %v", err)
	}

	// 对照二：作者行删除（引用释放）后同调用放行
	if err := env.db.Exec("DELETE FROM site_author WHERE id = ?", author.ID).Error; err != nil {
		t.Fatalf("删作者行失败: %v", err)
	}
	var paths []string
	if err := tx.ExecInTransaction(context.Background(), func(txCtx context.Context) error {
		var err error
		paths, err = env.svc.DeleteAvatarStoreRows(txCtx, []int64{storeId})
		return err
	}); err != nil {
		t.Fatalf("引用释放后删 store 行应放行: %v", err)
	}
	if len(paths) != 1 || paths[0] != "store/avatar/site/ab/pixiv_sa-fk.jpg" {
		t.Fatalf("应返回被删行文件路径，实际 %v", paths)
	}
	env.svc.RemoveAvatarFiles(paths)
	if _, err := os.Stat(filepath.Join(env.workDir, "store/avatar/site/ab/pixiv_sa-fk.jpg")); !os.IsNotExist(err) {
		t.Fatalf("事务后文件面应删除文件 (err=%v)", err)
	}
	if n := env.countStoreRows(t, "store/avatar/site/ab/pixiv_sa-fk.jpg"); n != 0 {
		t.Fatalf("store 行应已删清，实际 %d", n)
	}
}
