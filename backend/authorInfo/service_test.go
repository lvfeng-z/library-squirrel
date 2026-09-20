package authorInfo

// site 侧拉取主链编排锚定：元数据回写白名单（用户域/引用域列保留）、头像变更检测与换头像
// 先删旧、入库失败无痕迹（meta 与资源各自独立成功）、引用列同事务（事务失败行不建）、
// 在途去重（两触发面共用）与自动触发面开关判定。拉取能力以脚本化替身注入（广播路由归属
// 判定在 plugin/extension 侧单测锚定）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/siteAuthor"
	"github.com/library-squirrel/backend/staging"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// txTransactor 真实事务适配器（与生产 dbTransactorAdapter 同款：事务开启后把 tx 放进 ctx，
// 仓储方法经 DBFromContext 取事务连接）
type txTransactor struct {
	db *gorm.DB
}

func (t *txTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// fetchScript 单作者拉取脚本：meta 恒发；errAfterMeta 非 nil 时模拟 meta 之后流中断（字节未发完）
type fetchScript struct {
	meta         *pluginsdkdto.AuthorInfoMeta
	chunks       [][]byte
	errAfterMeta error
}

// scriptedFetcher 脚本化拉取替身：按站点侧作者 id 取脚本回放，记录调用与字节下发次数
type scriptedFetcher struct {
	mu         sync.Mutex
	calls      []string
	dataChunks int
	plan       map[string]fetchScript
}

func (f *scriptedFetcher) FetchSiteAuthorInfo(_ context.Context, siteKey, siteAuthorId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	f.mu.Lock()
	f.calls = append(f.calls, siteKey+"/"+siteAuthorId)
	script, ok := f.plan[siteAuthorId]
	f.mu.Unlock()
	if !ok {
		return fmt.Errorf("替身无脚本: %s", siteAuthorId)
	}
	wantAvatar, err := onMeta(script.meta)
	if err != nil {
		return err
	}
	if script.errAfterMeta != nil {
		return script.errAfterMeta
	}
	if !wantAvatar {
		return nil
	}
	for _, c := range script.chunks {
		f.mu.Lock()
		f.dataChunks++
		f.mu.Unlock()
		if err := onData(c); err != nil {
			return err
		}
	}
	return nil
}

func (f *scriptedFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *scriptedFetcher) dataChunkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dataChunks
}

// blockingFetcher 阻塞式拉取替身（在途去重测试）：进入即发信号，收到放行信号前不返回
type blockingFetcher struct {
	entered chan struct{}
	release chan struct{}
	callsMu sync.Mutex
	callN   int
}

func (f *blockingFetcher) FetchSiteAuthorInfo(_ context.Context, _, _ string,
	_ func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	_ func(data []byte) error) error {
	f.callsMu.Lock()
	f.callN++
	f.callsMu.Unlock()
	f.entered <- struct{}{}
	<-f.release
	return nil
}

func (f *blockingFetcher) callCount() int {
	f.callsMu.Lock()
	defer f.callsMu.Unlock()
	return f.callN
}

// refFailingSiteAuthorStore 包装真实 siteAuthor.Service，按需令 UpdateAvatarStoreId 失败
// （引用列同事务测试：写引用失败 → 建行随事务回滚）
type refFailingSiteAuthorStore struct {
	*siteAuthor.Service
	failRefUpdate bool
}

func (w *refFailingSiteAuthorStore) UpdateAvatarStoreId(ctx context.Context, siteAuthorId int64, storeId sql.NullInt64) error {
	if w.failRefUpdate {
		return errors.New("注入失败：引用列更新受阻")
	}
	return w.Service.UpdateAvatarStoreId(ctx, siteAuthorId, storeId)
}

// fetchTestEnv 拉取编排测试环境：内存库（外键强制 + 完整迁移）+ 临时工作目录 + 真实
// persistentStore/siteAuthor/settings 参与件 + 脚本化拉取替身
type fetchTestEnv struct {
	svc      *Service
	db       *gorm.DB
	workDir  string
	settings *settings.Service
	fetcher  *scriptedFetcher
	saSvc    *siteAuthor.Service
}

func newFetchTestEnv(t *testing.T) *fetchTestEnv {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	settingsSvc := settings.NewService(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsSvc.SaveSettings([]settings.SettingChange{
		{Path: "workdir", Value: workDir},
	}); err != nil {
		t.Fatalf("配置工作目录失败: %v", err)
	}
	saSvc := siteAuthor.NewService(siteAuthor.NewRepository(db), nil, nil, &txTransactor{db: db}, nil)
	psSvc := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	fetcher := &scriptedFetcher{plan: map[string]fetchScript{}}
	svc := NewService(saSvc, psSvc, psSvc, settingsSvc, settingsSvc, &txTransactor{db: db})
	svc.SetSiteAuthorFetcher(fetcher)
	return &fetchTestEnv{svc: svc, db: db, workDir: workDir, settings: settingsSvc, fetcher: fetcher, saSvc: saSvc}
}

// seedSiteAuthor 建站点+站点作者种子行，返回作者行
func (env *fetchTestEnv) seedSiteAuthor(t *testing.T, siteKey, siteAuthorId string, mutate func(row *entity.SiteAuthor)) *entity.SiteAuthor {
	t.Helper()
	if err := env.db.Exec("INSERT INTO site (site_key, site_name, create_time, update_time) VALUES (?, ?, 0, 0)",
		siteKey, siteKey).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	var siteId int64
	if err := env.db.Raw("SELECT id FROM site WHERE site_key = ?", siteKey).Scan(&siteId).Error; err != nil || siteId == 0 {
		t.Fatalf("回查站点种子失败: %v (id=%d)", err, siteId)
	}
	row := entity.NewSiteAuthor()
	row.SiteID = sql.NullInt64{Int64: siteId, Valid: true}
	row.SiteAuthorID = sql.NullString{String: siteAuthorId, Valid: true}
	row.AuthorName = sql.NullString{String: "旧名", Valid: true}
	if mutate != nil {
		mutate(row)
	}
	if err := env.db.Create(row).Error; err != nil {
		t.Fatalf("建站点作者种子失败: %v", err)
	}
	return row
}

// seedAvatarStoreRow 建一个已落盘的旧头像 store 行（含物理文件），返回行 ID
func (env *fetchTestEnv) seedAvatarStoreRow(t *testing.T, relPath string) int64 {
	t.Helper()
	abs := filepath.Join(env.workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("建旧头像目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("old-avatar"), 0o644); err != nil {
		t.Fatalf("写旧头像文件失败: %v", err)
	}
	row := entity.NewPersistentStore()
	row.FilePath = sql.NullString{String: relPath, Valid: true}
	row.FileName = sql.NullString{String: filepath.Base(relPath), Valid: true}
	row.FilenameExtension = sql.NullString{String: strings.ToLower(filepath.Ext(relPath)), Valid: true}
	row.CompletedAt = time.Now().UnixMilli()
	if err := env.db.Create(row).Error; err != nil {
		t.Fatalf("建旧头像 store 行失败: %v", err)
	}
	return row.GetID()
}

func (env *fetchTestEnv) loadSiteAuthor(t *testing.T, id int64) *entity.SiteAuthor {
	t.Helper()
	var row entity.SiteAuthor
	if err := env.db.First(&row, id).Error; err != nil {
		t.Fatalf("回查站点作者失败: %v", err)
	}
	return &row
}

func (env *fetchTestEnv) countStoreRows(t *testing.T, relPath string) int64 {
	t.Helper()
	var n int64
	if err := env.db.Model(&entity.PersistentStore{}).Where("file_path = ?", relPath).Count(&n).Error; err != nil {
		t.Fatalf("统计 store 行失败: %v", err)
	}
	return n
}

func (env *fetchTestEnv) journalCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := env.db.Raw("SELECT COUNT(*) FROM store_ingest_journal").Scan(&n).Error; err != nil {
		t.Fatalf("统计登记行失败: %v", err)
	}
	return n
}

func metaOf(name, introduce, homepage, avatarUrl, format string) *pluginsdkdto.AuthorInfoMeta {
	return &pluginsdkdto.AuthorInfoMeta{
		AuthorName:   name,
		Introduce:    introduce,
		Homepage:     homepage,
		AvatarUrl:    avatarUrl,
		AvatarFormat: format,
	}
}

// TestManualFetchWritesMetaWhitelistAndIngestsAvatar 手动拉取全链：元数据回写走插件权威域
// 白名单（用户域 local_author_id/last_use 与引用域 avatar_store_id 之外的原值保留、
// fixed_author_name 与任务链不声明时同款落 NULL），头像走四调用入库（store 行建成、文件落
// store/avatar/site/、引用列指向新行、登记行收口、作用域回收）
func TestManualFetchWritesMetaWhitelistAndIngestsAvatar(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", func(r *entity.SiteAuthor) {
		r.FixedAuthorName = sql.NullString{String: "用户固定名", Valid: true}
		r.LocalAuthorID = sql.NullInt64{Int64: 888, Valid: true}
		r.LastUse = sql.NullInt64{Int64: 123456, Valid: true}
	})
	avatarBytes := []byte("avatar-bytes-新")
	env.fetcher.plan["12345"] = fetchScript{
		meta:   metaOf("新名", "新签名", "https://home.example", "https://img.example/1.jpg", "jpg"),
		chunks: [][]byte{avatarBytes[:6], avatarBytes[6:]},
	}

	if err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID()); err != nil {
		t.Fatalf("手动拉取失败: %v", err)
	}

	got := env.loadSiteAuthor(t, row.GetID())
	if got.AuthorName.String != "新名" || !got.AuthorName.Valid {
		t.Fatalf("author_name 应回写为 %q, 实际 %v", "新名", got.AuthorName)
	}
	if got.FixedAuthorName.Valid {
		t.Fatalf("fixed_author_name 无拉取契约来源，应与任务链不声明时同款落 NULL, 实际 %v", got.FixedAuthorName)
	}
	if got.Introduce.String != "新签名" || got.Homepage.String != "https://home.example" {
		t.Fatalf("introduce/homepage 应回写, 实际 %v / %v", got.Introduce, got.Homepage)
	}
	if !got.AvatarSourceURL.Valid || got.AvatarSourceURL.String != "https://img.example/1.jpg" {
		t.Fatalf("avatar_source_url 应回写, 实际 %v", got.AvatarSourceURL)
	}
	if !got.LocalAuthorID.Valid || got.LocalAuthorID.Int64 != 888 {
		t.Fatalf("local_author_id 用户域应保留, 实际 %v", got.LocalAuthorID)
	}
	if !got.LastUse.Valid || got.LastUse.Int64 != 123456 {
		t.Fatalf("last_use 用户域应保留, 实际 %v", got.LastUse)
	}

	wantRel := "store/avatar/site/2e/pixiv_12345.jpg"
	if n := env.countStoreRows(t, wantRel); n != 1 {
		t.Fatalf("新头像 store 行应建成于 %s, 实际 %d 行", wantRel, n)
	}
	if !got.AvatarStoreID.Valid || got.AvatarStoreID.Int64 == 0 {
		t.Fatalf("avatar_store_id 应指向新行, 实际 %v", got.AvatarStoreID)
	}
	data, err := os.ReadFile(filepath.Join(env.workDir, filepath.FromSlash(wantRel)))
	if err != nil || string(data) != string(avatarBytes) {
		t.Fatalf("头像文件内容不符: err=%v content=%q", err, data)
	}
	if n := env.journalCount(t); n != 0 {
		t.Fatalf("入库登记行应已收口, 实际 %d 行", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, "staging", string(staging.OwnerAuthorInfo), fmt.Sprint(row.GetID()))); !os.IsNotExist(err) {
		t.Fatalf("拉取暂存作用域应已回收, stat err=%v", err)
	}
}

// TestFetchSkipsAvatarWhenUrlUnchanged 变更检测：来源 URL 未变且引用在 → 不落字节（仅刷
// 元数据），存量头像行与文件原样保留
func TestFetchSkipsAvatarWhenUrlUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	oldRel := "store/avatar/site/2e/pixiv_12345.png"
	row := env.seedSiteAuthor(t, "pixiv", "12345", func(r *entity.SiteAuthor) {
		r.AvatarSourceURL = sql.NullString{String: "https://img.example/1.png", Valid: true}
	})
	oldStoreId := env.seedAvatarStoreRow(t, oldRel)
	if err := env.db.Exec("UPDATE site_author SET avatar_store_id = ? WHERE id = ?", oldStoreId, row.GetID()).Error; err != nil {
		t.Fatalf("挂旧头像引用失败: %v", err)
	}
	env.fetcher.plan["12345"] = fetchScript{
		meta:   metaOf("名字v2", "", "", "https://img.example/1.png", "png"),
		chunks: [][]byte{[]byte("should-not-be-consumed")},
	}

	if err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID()); err != nil {
		t.Fatalf("拉取失败: %v", err)
	}
	if got := env.fetcher.dataChunkCount(); got != 0 {
		t.Fatalf("URL 未变且引用在，不应下发头像字节, 实际 %d 块", got)
	}
	got := env.loadSiteAuthor(t, row.GetID())
	if !got.AvatarStoreID.Valid || got.AvatarStoreID.Int64 != oldStoreId {
		t.Fatalf("存量头像引用应保留, 实际 %v", got.AvatarStoreID)
	}
	if n := env.countStoreRows(t, oldRel); n != 1 {
		t.Fatalf("存量头像行应保留, 实际 %d 行", n)
	}
	if got.AuthorName.String != "名字v2" {
		t.Fatalf("元数据仍应回写, 实际 %v", got.AuthorName)
	}
}

// TestAvatarUrlChangeReplacesOldAvatar 换头像：来源 URL 变化 → 先删旧（引用清空+旧行物理删、
// 事务外旧文件删除）再走新序列（新行新路径建成，ext 不同即路径不同）
func TestAvatarUrlChangeReplacesOldAvatar(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	oldRel := "store/avatar/site/2e/pixiv_12345.png"
	newRel := "store/avatar/site/2e/pixiv_12345.jpg"
	row := env.seedSiteAuthor(t, "pixiv", "12345", func(r *entity.SiteAuthor) {
		r.AvatarSourceURL = sql.NullString{String: "https://img.example/1.png", Valid: true}
	})
	oldStoreId := env.seedAvatarStoreRow(t, oldRel)
	if err := env.db.Exec("UPDATE site_author SET avatar_store_id = ? WHERE id = ?", oldStoreId, row.GetID()).Error; err != nil {
		t.Fatalf("挂旧头像引用失败: %v", err)
	}
	env.fetcher.plan["12345"] = fetchScript{
		meta:   metaOf("名", "", "", "https://img.example/2.jpg", "jpg"),
		chunks: [][]byte{[]byte("new-avatar")},
	}

	if err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID()); err != nil {
		t.Fatalf("拉取失败: %v", err)
	}
	if n := env.countStoreRows(t, oldRel); n != 0 {
		t.Fatalf("旧头像行应已删除, 实际 %d 行", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, filepath.FromSlash(oldRel))); !os.IsNotExist(err) {
		t.Fatalf("旧头像文件应已删除, stat err=%v", err)
	}
	if n := env.countStoreRows(t, newRel); n != 1 {
		t.Fatalf("新头像行应建成于 %s, 实际 %d 行", newRel, n)
	}
	got := env.loadSiteAuthor(t, row.GetID())
	if !got.AvatarStoreID.Valid || got.AvatarStoreID.Int64 == oldStoreId {
		t.Fatalf("引用应指向新行, 实际 %v (旧行 %d)", got.AvatarStoreID, oldStoreId)
	}
	if got.AvatarSourceURL.String != "https://img.example/2.jpg" {
		t.Fatalf("avatar_source_url 应更新为新 URL, 实际 %v", got.AvatarSourceURL)
	}
}

// TestStreamFailureAfterMetaLeavesNoResourceTrace 流中断失败无痕迹：meta 回写是独立成功（保留），
// 资源轨道无 store 行、无引用、暂存作用域回收
func TestStreamFailureAfterMetaLeavesNoResourceTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.plan["12345"] = fetchScript{
		meta:         metaOf("中断前的新名", "", "", "https://img.example/1.jpg", "jpg"),
		errAfterMeta: errors.New("模拟连接中断"),
	}

	err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID())
	if err == nil || !strings.Contains(err.Error(), "模拟连接中断") {
		t.Fatalf("应上抛流中断错误, 实际 %v", err)
	}
	got := env.loadSiteAuthor(t, row.GetID())
	if got.AuthorName.String != "中断前的新名" {
		t.Fatalf("meta 回写应独立成功保留, 实际 %v", got.AuthorName)
	}
	if got.AvatarStoreID.Valid {
		t.Fatalf("失败不应留引用, 实际 %v", got.AvatarStoreID)
	}
	if n := env.countStoreRows(t, "store/avatar/site/2e/pixiv_12345.jpg"); n != 0 {
		t.Fatalf("失败不应建 store 行, 实际 %d 行", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, "staging", string(staging.OwnerAuthorInfo), fmt.Sprint(row.GetID()))); !os.IsNotExist(err) {
		t.Fatalf("失败后暂存作用域应回收, stat err=%v", err)
	}
}

// TestCommitIngestRollsBackStoreRowWhenRefUpdateFails 引用列同事务：业务事务内写引用列失败 →
// 建 store 行随事务回滚（行不建）、已落位文件按丢弃声明撤回、引用保持原值——「行建而引用
// 未写」的中间态不存在
func TestCommitIngestRollsBackStoreRowWhenRefUpdateFails(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.plan["12345"] = fetchScript{
		meta:   metaOf("名", "", "", "https://img.example/1.jpg", "jpg"),
		chunks: [][]byte{[]byte("avatar")},
	}
	// 以失败注入的仓储重建服务（其余参与件沿用测试环境）
	wrapped := &refFailingSiteAuthorStore{Service: env.saSvc, failRefUpdate: true}
	svc := NewService(wrapped, persistentStore.NewService(persistentStore.NewRepository(env.db), nil, func() string { return env.workDir }),
		persistentStore.NewService(persistentStore.NewRepository(env.db), nil, func() string { return env.workDir }),
		env.settings, env.settings, &txTransactor{db: env.db})
	svc.SetSiteAuthorFetcher(env.fetcher)

	newRel := "store/avatar/site/2e/pixiv_12345.jpg"
	if err := svc.FetchSiteAuthorInfoById(context.Background(), row.GetID()); err == nil {
		t.Fatal("写引用列失败应上抛")
	}
	if n := env.countStoreRows(t, newRel); n != 0 {
		t.Fatalf("事务失败建行应回滚, 实际 %d 行", n)
	}
	if _, err := os.Stat(filepath.Join(env.workDir, filepath.FromSlash(newRel))); !os.IsNotExist(err) {
		t.Fatalf("已落位文件应按丢弃声明撤回, stat err=%v", err)
	}
	if got := env.loadSiteAuthor(t, row.GetID()); got.AvatarStoreID.Valid {
		t.Fatalf("引用应保持原值, 实际 %v", got.AvatarStoreID)
	}
}

// TestInFlightDedupSharedAcrossFaces 在途去重：手动拉取在途时——再次手动直接拒绝、批量结果
// 记跳过（同一去重集合为两触发面共用的单一 acquire/release 通路，自动触发面走同一通路）
func TestInFlightDedupSharedAcrossFaces(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	blocking := &blockingFetcher{entered: make(chan struct{}, 1), release: make(chan struct{})}
	env.svc.SetSiteAuthorFetcher(blocking)

	done := make(chan error, 1)
	go func() {
		done <- env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID())
	}()
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("首次拉取未进入替身")
	}

	// 再次手动：在途拒绝
	if err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID()); err == nil || !strings.Contains(err.Error(), "正在拉取") {
		t.Fatalf("在途作者手动拉取应拒绝, 实际 %v", err)
	}
	// 批量：在途记跳过
	results, err := env.svc.FetchSiteAuthorsInfoByIds(context.Background(), []int64{row.GetID()})
	if err != nil {
		t.Fatalf("批量拉取失败: %v", err)
	}
	if len(results) != 1 || results[0].Success || !strings.Contains(results[0].Message, "正在拉取") {
		t.Fatalf("批量在途结果应为跳过, 实际 %+v", results)
	}
	close(blocking.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("首次拉取应成功: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("首次拉取未收尾")
	}
	if got := blocking.callCount(); got != 1 {
		t.Fatalf("在途窗口内替身只被真实调用一次, 实际 %d 次", got)
	}
}

// TestAutoFetchGateOnAndOff 自动触发面开关判定：开→异步触发拉取；关→不触发（手动面不受限
// 由同一开关只挂在 OnSiteAuthorsUpserted 的入口判定保证）
func TestAutoFetchGateOnAndOff(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.plan["12345"] = fetchScript{meta: metaOf("自动名", "", "", "", "")}

	// 关：不触发
	if err := env.settings.SaveSettings([]settings.SettingChange{{Path: "authorSettings.autoFetchInfo", Value: false}}); err != nil {
		t.Fatalf("关闭自动拉取开关失败: %v", err)
	}
	env.svc.OnSiteAuthorsUpserted([]int64{row.GetID()})
	time.Sleep(200 * time.Millisecond)
	if got := env.fetcher.callCount(); got != 0 {
		t.Fatalf("开关关闭时自动触发面不应拉取, 实际 %d 次", got)
	}

	// 开：触发
	if err := env.settings.SaveSettings([]settings.SettingChange{{Path: "authorSettings.autoFetchInfo", Value: true}}); err != nil {
		t.Fatalf("开启自动拉取开关失败: %v", err)
	}
	env.svc.OnSiteAuthorsUpserted([]int64{row.GetID()})
	deadline := time.Now().Add(2 * time.Second)
	for env.fetcher.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := env.fetcher.callCount(); got != 1 {
		t.Fatalf("开关开启时自动触发面应拉取一次, 实际 %d 次", got)
	}
}

// TestFetchRejectsBadAvatarFormat 格式白名单：插件声明白名单外格式 → 资源轨道中止（元数据
// 已回写属独立成功），无 store 行
func TestFetchRejectsBadAvatarFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.plan["12345"] = fetchScript{
		meta:   metaOf("名", "", "", "https://img.example/1.exe", "exe"),
		chunks: [][]byte{[]byte("MZ")},
	}
	err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID())
	if err == nil || !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("白名单外格式应中止, 实际 %v", err)
	}
	if n := env.countStoreRows(t, "store/avatar/site/2e/pixiv_12345.exe"); n != 0 {
		t.Fatalf("不应落白名单外文件, 实际 %d 行", n)
	}
	if got := env.loadSiteAuthor(t, row.GetID()); got.AuthorName.String != "名" {
		t.Fatalf("meta 回写应独立成功, 实际 %v", got.AuthorName)
	}
}

// TestManualFetchUnknownAuthor 手动拉取不存在的作者：显式业务错误
func TestManualFetchUnknownAuthor(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	if err := env.svc.FetchSiteAuthorInfoById(context.Background(), 999); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("不存在作者应显式报错, 实际 %v", err)
	}
	results, err := env.svc.FetchSiteAuthorsInfoByIds(context.Background(), []int64{999})
	if err != nil {
		t.Fatalf("批量入口不应整体失败: %v", err)
	}
	if len(results) != 1 || results[0].Success || results[0].Message != "站点作者不存在" {
		t.Fatalf("批量逐条结果应为不存在, 实际 %+v", results)
	}
}
