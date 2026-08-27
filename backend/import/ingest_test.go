package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/persistentStore"
	"github.com/library-squirrel/backend/util"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 本文件为回灌导入测试（方案阶段2 退出标准）：fixture manifest + 文件源，断言
// ①导入后 DB 状态与 manifest 语义一致（作品/标签含 namespace/作者/作品集层级与排序全保真）
// ②文件落位到 workDir 正确路径且 persistent_store/resource/resource_store 记录就位
// ③重复导入幂等；附：版本锚拒绝、落盘路径冲突消解、ZIP 入口（handler）链路。

// testTransactor 测试事务器：与 app.go 的 dbTransactorAdapter 同形态（事务 DB 经 ctx 传递）。
type testTransactor struct {
	db *gorm.DB
}

func (t *testTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, database.TxKey, tx))
	})
}

// newTestSetup 建内存库（migration.OpenTestDB 全量迁移 + 外键强制）+ 真实 persistentStore
// 服务（workDir 指向测试临时目录），并预置既有站点/同名本地标签（验证 find-or-create 复用
// 与导出库 ID → 本库 ID 重映射）。
func newTestSetup(t *testing.T) (ManifestIngestor, *gorm.DB, *persistentStore.Service, string) {
	t.Helper()
	db, workDir := newTestDB(t)
	psService := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	ing := NewIngestor(duplicate.NewRepository(db), NewRepository(db), &testTransactor{db: db}, psService)
	return ing, db, psService, workDir
}

// newTestDB 建内存库（migration.OpenTestDB 全量迁移 + 外键强制）+ 预置既有站点/同名本地标签
// （验证 find-or-create 复用与导出库 ID → 本库 ID 重映射），并建临时工作目录。
func newTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	logger.Log = zap.NewNop().Sugar() // 测试期 logger 未初始化，置 nop 防日志调用 panic
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	// 预置行：占住自增 id=1，令导入行必然重映射到不同 ID（重映射语义可断言）
	if err := db.Exec("INSERT INTO site (site_name, create_time, update_time) VALUES ('seed-site', 0, 0)").Error; err != nil {
		t.Fatalf("预置站点失败: %v", err)
	}
	if err := db.Exec("INSERT INTO local_tag (local_tag_name, create_time, update_time) VALUES ('同名标签', 0, 0)").Error; err != nil {
		t.Fatalf("预置本地标签失败: %v", err)
	}
	return db, t.TempDir()
}

// mapFileSource 内存文件源（包内路径 → 内容）。
func mapFileSource(files map[string]string) FileSource {
	return func(entryPath string) (io.ReadCloser, error) {
		content, ok := files[entryPath]
		if !ok {
			return nil, fmt.Errorf("包内缺少文件 %s", entryPath)
		}
		return io.NopCloser(strings.NewReader(content)), nil
	}
}

func strPtr(s string) *string   { return &s }
func i64Ptr(i int64) *int64     { return &i }
func sha256Hex(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

// buildFixture 构造 fixture manifest 与包内文件：
// 站点 pixiv（导出库 ID 100）；本地标签 201 根 / 202 子（base=201）/ 203 与预置同名（应复用）；
// 站点标签 301（namespace=character，桥接 201）；本地作者 401；站点作者 501（桥接 401）；
// 作品集 601 父 / 602 子（父边 sort=3/siteSort=4，封面指向作品 701）；
// 作品 701（site_work_id=w-1）：双标签关联（site 带 namespace / local）、双作者关联（带 role/sort）、
// 作品集成员关系（sort=5/siteSort=6）；资源 801（image，image+thumbnail 双挂载）、
// 资源 802（document，挂载指向缺失文件——决策4 挂载缺席）；文件条目 901/902/903（903 missing）。
func buildFixture() (*export.Manifest, map[string]string) {
	files := map[string]string{
		"works/作品一/pic.jpg":       "picture-bytes-701",
		"works/作品一/pic_thumb.jpg": "thumb-bytes-701",
	}
	m := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Meta:          export.Meta{AppVersion: "test", WorkCount: 1, FileCount: 3},
		Sites: []export.SiteRecord{
			{ID: 100, SiteName: strPtr("pixiv"), Homepage: strPtr("https://www.pixiv.net"), CreateTime: 111, UpdateTime: 112},
		},
		LocalTags: []export.TagRecord{
			{ID: 201, Name: strPtr("同人标签"), Description: strPtr("根标签"), CreateTime: 121, UpdateTime: 122},
			{ID: 202, Name: strPtr("子标签"), BaseLocalTagID: i64Ptr(201), CreateTime: 123, UpdateTime: 124},
			{ID: 203, Name: strPtr("同名标签"), CreateTime: 125, UpdateTime: 126},
		},
		SiteTags: []export.TagRecord{
			{ID: 301, SiteID: i64Ptr(100), SiteTagID: strPtr("site-tag-x"), Name: strPtr("原站标签"),
				Namespace: strPtr("character"), LocalTagID: i64Ptr(201), CreateTime: 131, UpdateTime: 132},
		},
		LocalAuthors: []export.AuthorRecord{
			{ID: 401, Name: strPtr("本地作者甲"), Introduce: strPtr("作者介绍"), CreateTime: 141, UpdateTime: 142},
		},
		SiteAuthors: []export.AuthorRecord{
			{ID: 501, SiteID: i64Ptr(100), SiteAuthorID: strPtr("site-author-1"), Name: strPtr("站点作者甲"),
				LocalAuthorID: i64Ptr(401), CreateTime: 151, UpdateTime: 152},
		},
		WorkSets: []export.WorkSetRecord{
			{ID: 601, SiteID: i64Ptr(100), SiteWorkSetID: strPtr("ws-parent"), SiteWorkSetName: strPtr("父集"), CreateTime: 161, UpdateTime: 162},
			{ID: 602, SiteID: i64Ptr(100), SiteWorkSetID: strPtr("ws-child"), SiteWorkSetName: strPtr("子集"),
				CoverWorkID: i64Ptr(701),
				Parents:     []export.WorkSetParentEdge{{ParentWorkSetID: 601, SortOrder: i64Ptr(3), SiteSortOrder: i64Ptr(4)}},
				CreateTime:  163, UpdateTime: 164},
		},
		Works: []export.WorkRecord{
			{
				ID: 701, SiteID: i64Ptr(100), SiteWorkID: strPtr("w-1"), SiteWorkName: strPtr("作品一"),
				SiteWorkDescription: strPtr("站点侧描述"), NickName: strPtr("昵称一"), LocalAuthorID: i64Ptr(401),
				CreateTime: 171, UpdateTime: 172,
				Resources: []export.ResourceRecord{
					{ID: 801, ResourceType: "image", SuggestName: strPtr("主体图"), ResourceComplete: i64Ptr(1),
						CreateTime: 181, UpdateTime: 182,
						Stores: []export.StoreMount{
							{StoreType: "image", Generation: "downloaded", StoreSeq: 0, StoreID: 901},
							{StoreType: "thumbnail", Generation: "derived", StoreSeq: 0, StoreID: 902},
						}},
					{ID: 802, ResourceType: "document", SuggestName: strPtr("图文文档"), ResourceComplete: i64Ptr(1),
						CreateTime: 183, UpdateTime: 184,
						Stores: []export.StoreMount{
							{StoreType: "document", Generation: "downloaded", StoreSeq: 0, StoreID: 903},
						}},
				},
				TagLinks: []export.TagLink{
					{TagType: constant.SITE, TagID: 301, Namespace: strPtr("character")},
					{TagType: constant.LOCAL, TagID: 202},
				},
				AuthorLinks: []export.AuthorLink{
					{AuthorType: constant.LOCAL, AuthorID: 401, RoleName: strPtr("作者"), SortOrder: i64Ptr(1)},
					{AuthorType: constant.SITE, AuthorID: 501, RoleName: strPtr("原作者"), SortOrder: i64Ptr(2)},
				},
				WorkSetLinks: []export.WorkSetLink{
					{WorkSetID: 602, SortOrder: i64Ptr(5), SiteSortOrder: i64Ptr(6)},
				},
			},
		},
		Files: []export.FileEntry{
			{StoreID: 901, StorePath: "store/resource/作者甲/pic.jpg", Path: "works/作品一/pic.jpg",
				Size: int64(len(files["works/作品一/pic.jpg"])), Sha256: sha256Hex(files["works/作品一/pic.jpg"])},
			{StoreID: 902, StorePath: "store/resource/作者甲/pic_thumb.jpg", Path: "works/作品一/pic_thumb.jpg",
				Size: int64(len(files["works/作品一/pic_thumb.jpg"])), Sha256: sha256Hex(files["works/作品一/pic_thumb.jpg"])},
			{StoreID: 903, StorePath: "store/resource/作者甲/doc.md", Missing: true},
		},
	}
	return m, files
}

// queryInt64 单值 int64 查询（无行返回 0，测试断言用）。
func queryInt64(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := db.Raw(query, args...).Scan(&v).Error; err != nil {
		t.Fatalf("查询失败 %s: %v", query, err)
	}
	return v
}

// queryString 单值字符串查询（SQL NULL 落空串）。
func queryString(t *testing.T, db *gorm.DB, query string, args ...any) string {
	t.Helper()
	var ns sql.NullString
	if err := db.Raw(query, args...).Scan(&ns).Error; err != nil {
		t.Fatalf("查询失败 %s: %v", query, err)
	}
	if !ns.Valid {
		return ""
	}
	return ns.String
}

// assertRowCounts 幂等基准：各表行数全量断言（预置行计入）。
func assertRowCounts(t *testing.T, db *gorm.DB, label string) {
	t.Helper()
	expected := map[string]int64{
		"site": 2, "local_tag": 3, "site_tag": 1, "local_author": 1, "site_author": 1,
		"work": 1, "resource": 2, "resource_store": 2, "persistent_store": 2,
		"re_work_tag": 2, "re_work_author": 2, "re_work_work_set": 1, "re_work_set_work_set": 1,
	}
	for table, want := range expected {
		if got := queryInt64(t, db, "SELECT COUNT(*) FROM "+table); got != want {
			t.Fatalf("[%s] 表 %s 行数=%d，期望 %d", label, table, got, want)
		}
	}
}

// TestIngestRoundTripThenIdempotent 退出标准①②③：首次导入全保真落库落盘，二次导入幂等。
func TestIngestRoundTripThenIdempotent(t *testing.T) {
	ing, db, _, workDir := newTestSetup(t)
	ctx := context.Background()
	manifest, files := buildFixture()

	r1, err := ing.Ingest(ctx, manifest, mapFileSource(files), nil)
	if err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}

	// ===== 结果摘要 =====
	if r1.CreatedWorks != 1 || r1.SkippedWorks != 0 || r1.CreatedWorkSets != 2 || r1.SkippedWorkSets != 0 {
		t.Fatalf("首次导入作品/作品集计数不符: %+v", r1)
	}
	if r1.CreatedSites != 1 || r1.CreatedLocalTags != 2 || r1.CreatedSiteTags != 1 ||
		r1.CreatedLocalAuthors != 1 || r1.CreatedSiteAuthors != 1 {
		t.Fatalf("首次导入主数据计数不符: %+v", r1)
	}
	if r1.ExtractedFiles != 2 || r1.AbsentStores != 1 {
		t.Fatalf("首次导入文件计数不符: %+v", r1)
	}

	// ===== ① DB 语义保真 =====
	// 站点：按名 find-or-create 新建，ID 与导出库（100）不同（重映射生效）
	siteID := queryInt64(t, db, "SELECT id FROM site WHERE site_name = 'pixiv'")
	if siteID == 0 || siteID == 100 {
		t.Fatalf("站点应按名新建且重映射，实际 id=%d", siteID)
	}
	// 本地标签：根 + 子（层级重映射）；同名标签复用预置行（id=1，不新建）
	tagRoot := queryInt64(t, db, "SELECT id FROM local_tag WHERE local_tag_name = '同人标签'")
	tagChild := queryInt64(t, db, "SELECT id FROM local_tag WHERE local_tag_name = '子标签'")
	if tagRoot == 0 || tagChild == 0 {
		t.Fatalf("本地标签未落库")
	}
	if got := queryInt64(t, db, "SELECT base_local_tag_id FROM local_tag WHERE id = ?", tagChild); got != tagRoot {
		t.Fatalf("子标签层级未保真: base=%d 期望 %d", got, tagRoot)
	}
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM local_tag WHERE local_tag_name = '同名标签'"); got != 1 {
		t.Fatalf("同名标签应复用既有行，行数=%d", got)
	}
	if got := queryInt64(t, db, "SELECT id FROM local_tag WHERE local_tag_name = '同名标签'"); got != 1 {
		t.Fatalf("同名标签应复用预置行 id=1，实际 %d", got)
	}
	if got := queryInt64(t, db, "SELECT create_time FROM local_tag WHERE id = ?", tagRoot); got != 121 {
		t.Fatalf("本地标签源库时间戳未保真: create_time=%d 期望 121", got)
	}
	// 站点标签：复合身份落库、namespace 与 site→local 桥接保真
	siteTagID := queryInt64(t, db, "SELECT id FROM site_tag WHERE site_id = ? AND site_tag_id = 'site-tag-x'", siteID)
	if siteTagID == 0 {
		t.Fatalf("站点标签未落库")
	}
	if got := queryString(t, db, "SELECT namespace FROM site_tag WHERE id = ?", siteTagID); got != "character" {
		t.Fatalf("站点标签 namespace 未保真: %q", got)
	}
	if got := queryInt64(t, db, "SELECT local_tag_id FROM site_tag WHERE id = ?", siteTagID); got != tagRoot {
		t.Fatalf("站点标签桥接未重映射: local_tag_id=%d 期望 %d", got, tagRoot)
	}
	// 本地作者 + 站点作者（桥接重映射）
	localAuthorID := queryInt64(t, db, "SELECT id FROM local_author WHERE author_name = '本地作者甲'")
	siteAuthorID := queryInt64(t, db, "SELECT id FROM site_author WHERE site_id = ? AND site_author_id = 'site-author-1'", siteID)
	if localAuthorID == 0 || siteAuthorID == 0 {
		t.Fatalf("作者未落库")
	}
	if got := queryInt64(t, db, "SELECT local_author_id FROM site_author WHERE id = ?", siteAuthorID); got != localAuthorID {
		t.Fatalf("站点作者桥接未重映射: %d 期望 %d", got, localAuthorID)
	}
	// 作品：全字段与时间戳保真、site/local_author 引用重映射
	workID := queryInt64(t, db, "SELECT id FROM work WHERE site_id = ? AND site_work_id = 'w-1'", siteID)
	if workID == 0 || workID == 701 {
		t.Fatalf("作品应新建且重映射，实际 id=%d", workID)
	}
	if got := queryString(t, db, "SELECT site_work_name FROM work WHERE id = ?", workID); got != "作品一" {
		t.Fatalf("作品名未保真: %q", got)
	}
	if got := queryString(t, db, "SELECT nick_name FROM work WHERE id = ?", workID); got != "昵称一" {
		t.Fatalf("作品昵称未保真: %q", got)
	}
	if got := queryInt64(t, db, "SELECT local_author_id FROM work WHERE id = ?", workID); got != localAuthorID {
		t.Fatalf("作品主作者未重映射: %d 期望 %d", got, localAuthorID)
	}
	if got := queryInt64(t, db, "SELECT create_time FROM work WHERE id = ?", workID); got != 171 {
		t.Fatalf("作品源库时间戳未保真: create_time=%d 期望 171", got)
	}
	// 标签关联：site 关联带 namespace 镜像、local 关联 namespace 落 NULL、tag_type 分轨
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE work_id = ?", workID); got != 2 {
		t.Fatalf("标签关联行数=%d 期望 2", got)
	}
	if got := queryString(t, db,
		"SELECT namespace FROM re_work_tag WHERE work_id = ? AND site_tag_id = ?", workID, siteTagID); got != "character" {
		t.Fatalf("site 标签关联 namespace 未保真: %q", got)
	}
	if got := queryInt64(t, db, "SELECT tag_type FROM re_work_tag WHERE work_id = ? AND site_tag_id = ?", workID, siteTagID); got != constant.SITE {
		t.Fatalf("site 标签关联 tag_type=%d 期望 %d", got, constant.SITE)
	}
	if got := queryInt64(t, db,
		"SELECT local_tag_id FROM re_work_tag WHERE work_id = ? AND local_tag_id = ?", workID, tagChild); got != tagChild {
		t.Fatalf("local 标签关联未重映射")
	}
	if got := queryString(t, db,
		"SELECT namespace FROM re_work_tag WHERE work_id = ? AND local_tag_id = ?", workID, tagChild); got != "" {
		t.Fatalf("local 标签关联 namespace 应为 NULL，实际 %q", got)
	}
	// 作者关联：role/sort 保真
	if got := queryString(t, db,
		"SELECT role_name FROM re_work_author WHERE work_id = ? AND local_author_id = ?", workID, localAuthorID); got != "作者" {
		t.Fatalf("local 作者关联 role 未保真: %q", got)
	}
	if got := queryInt64(t, db,
		"SELECT sort_order FROM re_work_author WHERE work_id = ? AND site_author_id = ?", workID, siteAuthorID); got != 2 {
		t.Fatalf("site 作者关联 sort 未保真: %d", got)
	}
	// 作品集：层级父边双轨排序 + 封面重映射 + 成员关系双轨排序
	wsParent := queryInt64(t, db, "SELECT id FROM work_set WHERE site_id = ? AND site_work_set_id = 'ws-parent'", siteID)
	wsChild := queryInt64(t, db, "SELECT id FROM work_set WHERE site_id = ? AND site_work_set_id = 'ws-child'", siteID)
	if wsParent == 0 || wsChild == 0 {
		t.Fatalf("作品集未落库")
	}
	if got := queryInt64(t, db, "SELECT sort_order FROM re_work_set_work_set WHERE parent_work_set_id = ? AND child_work_set_id = ?", wsParent, wsChild); got != 3 {
		t.Fatalf("作品集父边 sort_order 未保真: %d", got)
	}
	if got := queryInt64(t, db, "SELECT site_sort_order FROM re_work_set_work_set WHERE parent_work_set_id = ? AND child_work_set_id = ?", wsParent, wsChild); got != 4 {
		t.Fatalf("作品集父边 site_sort_order 未保真: %d", got)
	}
	if got := queryInt64(t, db, "SELECT cover_work_id FROM work_set WHERE id = ?", wsChild); got != workID {
		t.Fatalf("作品集封面未重映射: %d 期望 %d", got, workID)
	}
	if got := queryInt64(t, db, "SELECT sort_order FROM re_work_work_set WHERE work_id = ? AND work_set_id = ?", workID, wsChild); got != 5 {
		t.Fatalf("成员关系 sort_order 未保真: %d", got)
	}
	if got := queryInt64(t, db, "SELECT site_sort_order FROM re_work_work_set WHERE work_id = ? AND work_set_id = ?", workID, wsChild); got != 6 {
		t.Fatalf("成员关系 site_sort_order 未保真: %d", got)
	}

	// ===== ② 文件落位与记录就位 =====
	for _, rel := range []string{"store/resource/作者甲/pic.jpg", "store/resource/作者甲/pic_thumb.jpg"} {
		content, err := os.ReadFile(filepath.Join(workDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("文件未落位 %s: %v", rel, err)
		}
		key := "works/作品一/" + filepath.Base(rel)
		if string(content) != files[key] {
			t.Fatalf("文件内容不符 %s", rel)
		}
		psID := queryInt64(t, db, "SELECT id FROM persistent_store WHERE file_path = ?", rel)
		if psID == 0 {
			t.Fatalf("persistent_store 记录缺失 %s", rel)
		}
		if got := queryInt64(t, db, "SELECT completed_at FROM persistent_store WHERE id = ?", psID); got <= 0 {
			t.Fatalf("persistent_store 未落完成态 %s", rel)
		}
	}
	psPic := queryInt64(t, db, "SELECT id FROM persistent_store WHERE file_path = 'store/resource/作者甲/pic.jpg'")
	// resource_store：image 挂载指向重映射后的 persistent_store 行
	rsID := queryInt64(t, db,
		"SELECT rs.id FROM resource_store rs JOIN resource r ON r.id = rs.resource_id WHERE r.work_id = ? AND rs.store_type = 'image'", workID)
	if rsID == 0 {
		t.Fatalf("resource_store image 挂载缺失")
	}
	if got := queryInt64(t, db, "SELECT store_id FROM resource_store WHERE id = ?", rsID); got != psPic {
		t.Fatalf("挂载文件引用未重映射: %d 期望 %d", got, psPic)
	}
	if got := queryInt64(t, db, "SELECT store_seq FROM resource_store WHERE id = ?", rsID); got != 0 {
		t.Fatalf("挂载 store_seq 未保真: %d", got)
	}
	if got := queryString(t, db, "SELECT generation FROM resource_store WHERE id = ?", rsID); got != "downloaded" {
		t.Fatalf("挂载 generation 未保真: %q", got)
	}
	// 缺席挂载（决策4）：document 资源无任何挂载行
	if got := queryInt64(t, db,
		"SELECT COUNT(*) FROM resource_store rs JOIN resource r ON r.id = rs.resource_id WHERE r.work_id = ? AND rs.store_type = 'document'", workID); got != 0 {
		t.Fatalf("缺失文件的挂载应缺席，实际 %d 行", got)
	}

	// ===== ③ 幂等：重复导入不重复建 =====
	assertRowCounts(t, db, "首次导入后")
	r2, err := ing.Ingest(ctx, manifest, mapFileSource(files), nil)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if r2.CreatedWorks != 0 || r2.SkippedWorks != 1 || r2.CreatedWorkSets != 0 || r2.SkippedWorkSets != 2 {
		t.Fatalf("二次导入应全量查重跳过: %+v", r2)
	}
	if r2.CreatedSites != 0 || r2.CreatedLocalTags != 0 || r2.CreatedSiteTags != 0 ||
		r2.CreatedLocalAuthors != 0 || r2.CreatedSiteAuthors != 0 {
		t.Fatalf("二次导入主数据应全量复用: %+v", r2)
	}
	if r2.ExtractedFiles != 0 || r2.AbsentStores != 0 {
		t.Fatalf("二次导入不应再落盘: %+v", r2)
	}
	assertRowCounts(t, db, "二次导入后")
}

// TestIngestSchemaVersionRejected 版本锚不匹配的产物拒绝导入（契约破坏性变更防护）。
func TestIngestSchemaVersionRejected(t *testing.T) {
	ing, _, _, _ := newTestSetup(t)
	manifest, files := buildFixture()
	manifest.SchemaVersion = export.SchemaVersion + 1
	_, err := ing.Ingest(context.Background(), manifest, mapFileSource(files), nil)
	if !errors.Is(err, ErrSchemaVersionUnsupported) {
		t.Fatalf("应拒绝不支持的版本，实际 err=%v", err)
	}
}

// TestIngestChecksumMismatch 校验失败时报错并补偿清理（不留半成品文件）。
func TestIngestChecksumMismatch(t *testing.T) {
	ing, db, _, workDir := newTestSetup(t)
	manifest, files := buildFixture()
	files["works/作品一/pic.jpg"] = "tampered-content"
	_, err := ing.Ingest(context.Background(), manifest, mapFileSource(files), nil)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("应报校验失败，实际 err=%v", err)
	}
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM work"); got != 0 {
		t.Fatalf("校验失败不应入库作品，实际 %d 行", got)
	}
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM persistent_store"); got != 0 {
		t.Fatalf("校验失败应补偿清理文件记录，实际 %d 行", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, "store/resource/作者甲/pic.jpg")); !os.IsNotExist(err) {
		t.Fatalf("校验失败应补偿清理半成品文件")
	}
}

// TestIngestPathCollisionVariant 目标路径被既有记录占用时派生变体路径，
// 不改写既有作品文件（对persistentStore.Store 同路径覆盖语义的规避）。
func TestIngestPathCollisionVariant(t *testing.T) {
	ing, db, psService, workDir := newTestSetup(t)
	ctx := context.Background()
	// 预占目标路径：既有记录 + 既有文件内容
	if _, err := psService.Store(ctx, "store/resource/作者甲/pic.jpg", "pic.jpg",
		strings.NewReader("existing-work-file")); err != nil {
		t.Fatalf("预置占用记录失败: %v", err)
	}
	manifest, files := buildFixture()
	r1, err := ing.Ingest(ctx, manifest, mapFileSource(files), nil)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if r1.CreatedWorks != 1 {
		t.Fatalf("路径冲突不应阻断导入: %+v", r1)
	}
	// 既有文件原样保留
	content, err := os.ReadFile(filepath.Join(workDir, "store/resource/作者甲/pic.jpg"))
	if err != nil || string(content) != "existing-work-file" {
		t.Fatalf("既有作品文件被改写: %q err=%v", string(content), err)
	}
	// 导入文件落在变体路径，内容正确
	variant, err := os.ReadFile(filepath.Join(workDir, "store/resource/作者甲/pic_import1.jpg"))
	if err != nil {
		t.Fatalf("导入文件未落到变体路径: %v", err)
	}
	if string(variant) != files["works/作品一/pic.jpg"] {
		t.Fatalf("变体路径文件内容不符")
	}
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM persistent_store WHERE file_path LIKE 'store/resource/作者甲/%'"); got != 3 {
		t.Fatalf("占用+导入两文件应三行记录，实际 %d", got)
	}
}

// TestHandlerImportFromZip ZIP 入口链路：manifest.json + 文件经 handler 导入成功；
// 缺 manifest 的包报错。
func TestHandlerImportFromZip(t *testing.T) {
	_, db, _, workDir := newTestSetup(t)
	manifest, files := buildFixture()
	manifestData, err := manifest.Serialize()
	if err != nil {
		t.Fatalf("序列化 manifest 失败: %v", err)
	}
	entries := map[string][]byte{"manifest.json": manifestData}
	for name, content := range files {
		entries[name] = []byte(content)
	}
	zipPath := writeZip(t, entries)

	handler := NewHandler(NewIngestor(duplicate.NewRepository(db), NewRepository(db), &testTransactor{db: db},
		persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })))
	resp := handler.ImportFromZip(context.Background(), zipPath)
	if !resp.Success {
		t.Fatalf("ZIP 导入失败: %s", resp.Msg)
	}
	if resp.Data.CreatedWorks != 1 || resp.Data.ExtractedFiles != 2 {
		t.Fatalf("ZIP 导入结果不符: %+v", resp.Data)
	}
	// 缺 manifest 的包：报错且无数据落库
	emptyZip := writeZip(t, map[string][]byte{"other.txt": []byte("x")})
	resp2 := handler.ImportFromZip(context.Background(), emptyZip)
	if resp2.Success {
		t.Fatalf("缺 manifest 的包应报错")
	}
}

// writeZip 构建内存 zip 并落临时文件，返回路径。
func writeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("写 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	p := filepath.Join(t.TempDir(), "import-pkg.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写临时 zip 失败: %v", err)
	}
	return p
}

// ===== 替换分支（IngestOptions.ReplaceWorks / AutoMergeWorks）=====

// buildReplaceFixture 构造替换导入的 fixture manifest 与包内文件：复用 base 的站点/标签/作者/
// 作品集（find-or-create 复用既有行），作品 701 内容替换——站点侧名称更新、资源改为单 image
// 挂载（新文件 911，StorePath 与 base 的 pic.jpg 同路径，验证软删后同路径可复用）、
// 新增本地标签 204 关联；作者/作品集关联保持原样（验证关联合并不重复挂）。
func buildReplaceFixture() (*export.Manifest, map[string]string) {
	files := map[string]string{
		"works/作品一/pic_new.jpg": "picture-bytes-replace-701",
	}
	m := &export.Manifest{
		SchemaVersion: export.SchemaVersion,
		Meta:          export.Meta{AppVersion: "test", WorkCount: 1, FileCount: 1},
		Sites: []export.SiteRecord{
			{ID: 100, SiteName: strPtr("pixiv"), Homepage: strPtr("https://www.pixiv.net"), CreateTime: 111, UpdateTime: 112},
		},
		LocalTags: []export.TagRecord{
			{ID: 201, Name: strPtr("同人标签"), Description: strPtr("根标签"), CreateTime: 121, UpdateTime: 122},
			{ID: 202, Name: strPtr("子标签"), BaseLocalTagID: i64Ptr(201), CreateTime: 123, UpdateTime: 124},
			{ID: 204, Name: strPtr("新增标签"), CreateTime: 125, UpdateTime: 126},
		},
		SiteTags: []export.TagRecord{
			{ID: 301, SiteID: i64Ptr(100), SiteTagID: strPtr("site-tag-x"), Name: strPtr("原站标签"),
				Namespace: strPtr("character"), LocalTagID: i64Ptr(201), CreateTime: 131, UpdateTime: 132},
		},
		LocalAuthors: []export.AuthorRecord{
			{ID: 401, Name: strPtr("本地作者甲"), Introduce: strPtr("作者介绍"), CreateTime: 141, UpdateTime: 142},
		},
		SiteAuthors: []export.AuthorRecord{
			{ID: 501, SiteID: i64Ptr(100), SiteAuthorID: strPtr("site-author-1"), Name: strPtr("站点作者甲"),
				LocalAuthorID: i64Ptr(401), CreateTime: 151, UpdateTime: 152},
		},
		WorkSets: []export.WorkSetRecord{
			{ID: 601, SiteID: i64Ptr(100), SiteWorkSetID: strPtr("ws-parent"), SiteWorkSetName: strPtr("父集"), CreateTime: 161, UpdateTime: 162},
			{ID: 602, SiteID: i64Ptr(100), SiteWorkSetID: strPtr("ws-child"), SiteWorkSetName: strPtr("子集"),
				CoverWorkID: i64Ptr(701),
				Parents:     []export.WorkSetParentEdge{{ParentWorkSetID: 601, SortOrder: i64Ptr(3), SiteSortOrder: i64Ptr(4)}},
				CreateTime:  163, UpdateTime: 164},
		},
		Works: []export.WorkRecord{
			{
				ID: 701, SiteID: i64Ptr(100), SiteWorkID: strPtr("w-1"), SiteWorkName: strPtr("作品一·替换版"),
				SiteWorkDescription: strPtr("站点侧描述·新版"), NickName: strPtr("昵称一·新版"), LocalAuthorID: i64Ptr(401),
				CreateTime: 171, UpdateTime: 173,
				Resources: []export.ResourceRecord{
					{ID: 811, ResourceType: "image", SuggestName: strPtr("主体图·新版"), ResourceComplete: i64Ptr(1),
						CreateTime: 181, UpdateTime: 183,
						Stores: []export.StoreMount{
							{StoreType: "image", Generation: "downloaded", StoreSeq: 0, StoreID: 911},
						}},
				},
				TagLinks: []export.TagLink{
					{TagType: constant.SITE, TagID: 301, Namespace: strPtr("character")},
					{TagType: constant.LOCAL, TagID: 204},
				},
				AuthorLinks: []export.AuthorLink{
					{AuthorType: constant.LOCAL, AuthorID: 401, RoleName: strPtr("作者"), SortOrder: i64Ptr(1)},
					{AuthorType: constant.SITE, AuthorID: 501, RoleName: strPtr("原作者"), SortOrder: i64Ptr(2)},
				},
				WorkSetLinks: []export.WorkSetLink{
					{WorkSetID: 602, SortOrder: i64Ptr(5), SiteSortOrder: i64Ptr(6)},
				},
			},
		},
		Files: []export.FileEntry{
			{StoreID: 911, StorePath: "store/resource/作者甲/pic.jpg", Path: "works/作品一/pic_new.jpg",
				Size: int64(len(files["works/作品一/pic_new.jpg"])), Sha256: sha256Hex(files["works/作品一/pic_new.jpg"])},
		},
	}
	return m, files
}

// softDeleteWorkStores 模拟替换调用方（阶段5 替换前置软删）在 ingest 前的动作：软删作品下
// 所选角色的活行 store 行。文件留原位、resource_store 关联保留——本测试只关心 ingest 侧对
// 软删状态的消费（空壳净化/同路径复用），不涉及备份移动与回滚。
func softDeleteWorkStores(t *testing.T, db *gorm.DB, workID int64, roles ...string) {
	t.Helper()
	for _, role := range roles {
		var storeIDs []int64
		if err := db.Raw(`
			SELECT rs.store_id FROM resource_store rs
			JOIN resource r ON r.id = rs.resource_id
			WHERE r.work_id = ? AND rs.store_type = ?
			  AND rs.store_id IN (SELECT id FROM persistent_store WHERE deleted_at = 0)`,
			workID, role).Scan(&storeIDs).Error; err != nil {
			t.Fatalf("查询待软删 store 失败: %v", err)
		}
		if len(storeIDs) == 0 {
			continue
		}
		ts := util.GetCurrentTimestamp()
		if err := db.Exec(`UPDATE persistent_store SET deleted_at = ? WHERE id IN ?`, ts, storeIDs).Error; err != nil {
			t.Fatalf("软删 store 失败: %v", err)
		}
	}
}

// TestIngestReplaceWorks 替换分支退出标准：确认替换作品——元数据覆盖（workID 不变）、
// manifest 资源新建挂载、本地独有角色保留、空壳净化、关联合并去重、计数拆分。
func TestIngestReplaceWorks(t *testing.T) {
	ing, db, _, workDir := newTestSetup(t)
	ctx := context.Background()
	baseManifest, baseFiles := buildFixture()
	if _, err := ing.Ingest(ctx, baseManifest, mapFileSource(baseFiles), nil); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	workID := queryInt64(t, db, "SELECT id FROM work WHERE site_work_id = 'w-1'")
	if workID == 0 {
		t.Fatalf("已有作品未建立")
	}
	// 模拟调用方替换前置软删：软删 image 角色活行 store（resource 801 的 image 行）
	softDeleteWorkStores(t, db, workID, "image")

	repManifest, repFiles := buildReplaceFixture()
	r, err := ing.Ingest(ctx, repManifest, mapFileSource(repFiles), &IngestOptions{
		ReplaceWorks: map[int64]struct{}{701: {}},
	})
	if err != nil {
		t.Fatalf("替换导入失败: %v", err)
	}

	// ===== 结果摘要 =====
	if r.CreatedWorks != 0 || r.ReplacedWorks != 1 || r.ReplacedConfirmed != 1 || r.ReplacedAuto != 0 || r.SkippedWorks != 0 {
		t.Fatalf("替换计数不符: %+v", r)
	}
	if r.ExtractedFiles != 1 || r.AbsentStores != 0 {
		t.Fatalf("替换文件计数不符: %+v", r)
	}

	// ===== 元数据覆盖（workID 不变）=====
	if got := queryString(t, db, "SELECT site_work_name FROM work WHERE id = ?", workID); got != "作品一·替换版" {
		t.Fatalf("作品站点名未覆盖: %q", got)
	}
	if got := queryString(t, db, "SELECT nick_name FROM work WHERE id = ?", workID); got != "昵称一·新版" {
		t.Fatalf("作品昵称未覆盖: %q", got)
	}
	if got := queryString(t, db, "SELECT site_work_description FROM work WHERE id = ?", workID); got != "站点侧描述·新版" {
		t.Fatalf("作品站点描述未覆盖: %q", got)
	}

	// ===== 资源面：manifest 资源新建挂载 + 本地独有角色保留 + 空壳净化 =====
	// 作品资源：旧 resource 801（保 thumbnail 活行）保留 + manifest 新资源（image）新建；
	// 旧 resource 802（document，无任何活行 store）空壳净化（资源行物理删）
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM resource WHERE work_id = ?", workID); got != 2 {
		t.Fatalf("替换后资源行数=%d 期望 2（旧 801 保留 + 新资源）", got)
	}
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM resource WHERE work_id = ? AND resource_type = 'document'", workID); got != 0 {
		t.Fatalf("空壳 document 资源应被净化，实际 %d 行", got)
	}
	// 本地独有角色保留：旧 resource 801 的 thumbnail 活行仍在
	if got := queryInt64(t, db, `
		SELECT COUNT(*) FROM resource_store rs
		JOIN resource r ON r.id = rs.resource_id
		JOIN persistent_store ps ON ps.id = rs.store_id
		WHERE r.work_id = ? AND rs.store_type = 'thumbnail' AND ps.deleted_at = 0`, workID); got != 1 {
		t.Fatalf("本地独有 thumbnail 角色应保留活行，实际 %d", got)
	}
	// 交集角色替换：新 image 活行 + 旧 image 软删行关联保留（替换链语义）
	if got := queryInt64(t, db, `
		SELECT COUNT(*) FROM resource_store rs
		JOIN resource r ON r.id = rs.resource_id
		JOIN persistent_store ps ON ps.id = rs.store_id
		WHERE r.work_id = ? AND rs.store_type = 'image' AND ps.deleted_at = 0`, workID); got != 1 {
		t.Fatalf("替换后应有 1 个活行 image 挂载，实际 %d", got)
	}
	if got := queryInt64(t, db, `
		SELECT COUNT(*) FROM resource_store rs
		JOIN resource r ON r.id = rs.resource_id
		JOIN persistent_store ps ON ps.id = rs.store_id
		WHERE r.work_id = ? AND rs.store_type = 'image' AND ps.deleted_at <> 0`, workID); got != 1 {
		t.Fatalf("旧 image 软删行关联应保留（替换链语义），实际 %d", got)
	}
	// 新文件落位到与旧 image 同路径（软删释放路径后复用），内容为替换版
	content, err := os.ReadFile(filepath.Join(workDir, "store/resource/作者甲/pic.jpg"))
	if err != nil || string(content) != "picture-bytes-replace-701" {
		t.Fatalf("新文件未落位或内容不符: %q err=%v", string(content), err)
	}
	// 本地独有角色文件保留（thumbnail 未被替换）
	thumb, err := os.ReadFile(filepath.Join(workDir, "store/resource/作者甲/pic_thumb.jpg"))
	if err != nil || string(thumb) != "thumb-bytes-701" {
		t.Fatalf("本地独有 thumbnail 文件被改写: %q err=%v", string(thumb), err)
	}

	// ===== 关联合并：manifest 关联增量挂载、本地已有关联去重不重复 =====
	// 标签：base 已有 site 301 + local 202；替换新增 local 204，site 301 去重
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE work_id = ?", workID); got != 3 {
		t.Fatalf("替换后标签关联行数=%d 期望 3（site 301 + local 202 + local 204）", got)
	}
	if got := queryInt64(t, db, `
		SELECT COUNT(*) FROM re_work_tag rwt
		JOIN local_tag lt ON lt.id = rwt.local_tag_id
		WHERE rwt.work_id = ? AND lt.local_tag_name = '新增标签'`, workID); got != 1 {
		t.Fatalf("新增本地标签关联应挂载，实际 %d", got)
	}
	// 作者：base 已有 local 401 + site 501；替换去重不重复挂
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_author WHERE work_id = ?", workID); got != 2 {
		t.Fatalf("替换后作者关联行数=%d 期望 2（去重）", got)
	}
	// 作品集成员：base 已有 602；替换去重不重复挂
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_work_set WHERE work_id = ?", workID); got != 1 {
		t.Fatalf("替换后作品集成员行数=%d 期望 1（去重）", got)
	}
}

// TestIngestReplaceAutoMerge 零交集自动增补：AutoMergeWorks 命中作品走替换分支，计数按
// 「自动增补」拆分（ReplacedConfirmed=0 / ReplacedAuto=1）。
func TestIngestReplaceAutoMerge(t *testing.T) {
	ing, db, _, _ := newTestSetup(t)
	ctx := context.Background()
	baseManifest, baseFiles := buildFixture()
	if _, err := ing.Ingest(ctx, baseManifest, mapFileSource(baseFiles), nil); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	workID := queryInt64(t, db, "SELECT id FROM work WHERE site_work_id = 'w-1'")
	softDeleteWorkStores(t, db, workID, "image")

	repManifest, repFiles := buildReplaceFixture()
	r, err := ing.Ingest(ctx, repManifest, mapFileSource(repFiles), &IngestOptions{
		AutoMergeWorks: map[int64]struct{}{701: {}},
	})
	if err != nil {
		t.Fatalf("自动增补导入失败: %v", err)
	}
	if r.ReplacedWorks != 1 || r.ReplacedConfirmed != 0 || r.ReplacedAuto != 1 || r.SkippedWorks != 0 {
		t.Fatalf("自动增补计数不符: %+v", r)
	}
	if got := queryString(t, db, "SELECT site_work_name FROM work WHERE id = ?", workID); got != "作品一·替换版" {
		t.Fatalf("自动增补作品应执行替换，实际名 %q", got)
	}
}

// TestIngestReplaceNilOptsSkips nil opts 行为等价旧签名：替换 manifest 不带 opts（nil 或空
// 选项）时命中作品维持现状全跳过语义——文件/记录/关联一概不动。
func TestIngestReplaceNilOptsSkips(t *testing.T) {
	ing, db, _, _ := newTestSetup(t)
	ctx := context.Background()
	baseManifest, baseFiles := buildFixture()
	if _, err := ing.Ingest(ctx, baseManifest, mapFileSource(baseFiles), nil); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	workID := queryInt64(t, db, "SELECT id FROM work WHERE site_work_id = 'w-1'")

	repManifest, repFiles := buildReplaceFixture()
	for _, label := range []string{"nil", "empty"} {
		var opts *IngestOptions
		if label == "empty" {
			opts = &IngestOptions{}
		}
		r, err := ing.Ingest(ctx, repManifest, mapFileSource(repFiles), opts)
		if err != nil {
			t.Fatalf("[%s] 无替换选项导入失败: %v", label, err)
		}
		if r.CreatedWorks != 0 || r.ReplacedWorks != 0 || r.SkippedWorks != 1 {
			t.Fatalf("[%s] 应全量查重跳过: %+v", label, r)
		}
		if got := queryString(t, db, "SELECT site_work_name FROM work WHERE id = ?", workID); got != "作品一" {
			t.Fatalf("[%s] 跳过语义下作品元数据不应改动: %q", label, got)
		}
		if got := queryInt64(t, db, "SELECT COUNT(*) FROM resource WHERE work_id = ?", workID); got != 2 {
			t.Fatalf("[%s] 跳过语义下资源不应增删: %d", label, got)
		}
		if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE work_id = ?", workID); got != 2 {
			t.Fatalf("[%s] 跳过语义下关联不应增挂: %d", label, got)
		}
	}
}

// failCreateResourcesRepo 包装 Repository：开启 fail 后 CreateResources 报错
// （替换分支事务回滚测试用）。
type failCreateResourcesRepo struct {
	Repository
	fail bool
}

func (r *failCreateResourcesRepo) CreateResources(ctx context.Context, rows []*entity.Resource) error {
	if r.fail {
		return errors.New("注入的建资源失败")
	}
	return r.Repository.CreateResources(ctx, rows)
}

// TestIngestReplaceTransactionRollback 替换分支相位三单事务失败 → 事务回滚不动库：
// 元数据未覆盖、空壳未净化、新资源未挂、新增标签关联未落、相位二落盘文件补偿清理。
func TestIngestReplaceTransactionRollback(t *testing.T) {
	db, workDir := newTestDB(t)
	ctx := context.Background()
	psService := persistentStore.NewService(persistentStore.NewRepository(db), nil, func() string { return workDir })
	wrapped := &failCreateResourcesRepo{Repository: NewRepository(db)}
	ing := &ingestor{repo: wrapped, dupRepo: duplicate.NewRepository(db), transactor: &testTransactor{db: db}, fileStore: psService}

	baseManifest, baseFiles := buildFixture()
	if _, err := ing.Ingest(ctx, baseManifest, mapFileSource(baseFiles), nil); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	workID := queryInt64(t, db, "SELECT id FROM work WHERE site_work_id = 'w-1'")
	softDeleteWorkStores(t, db, workID, "image")

	repManifest, repFiles := buildReplaceFixture()
	wrapped.fail = true
	if _, err := ing.Ingest(ctx, repManifest, mapFileSource(repFiles), &IngestOptions{
		ReplaceWorks: map[int64]struct{}{701: {}},
	}); err == nil {
		t.Fatalf("注入失败后替换导入应报错")
	}
	wrapped.fail = false

	// 事务回滚：元数据未覆盖
	if got := queryString(t, db, "SELECT site_work_name FROM work WHERE id = ?", workID); got != "作品一" {
		t.Fatalf("事务失败后作品元数据不应覆盖: %q", got)
	}
	// 空壳未净化：resource 802（document）仍在
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM resource WHERE work_id = ?", workID); got != 2 {
		t.Fatalf("事务失败后资源行数应保持 2，实际 %d", got)
	}
	// 新增本地标签未落（事务内 ensureLocalTags 创建被回滚）
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM local_tag WHERE local_tag_name = '新增标签'"); got != 0 {
		t.Fatalf("事务失败后新增标签应被回滚，实际 %d 行", got)
	}
	// 标签关联数保持 base（site 301 + local 202）
	if got := queryInt64(t, db, "SELECT COUNT(*) FROM re_work_tag WHERE work_id = ?", workID); got != 2 {
		t.Fatalf("事务失败后标签关联应保持 2，实际 %d", got)
	}
	// 相位二落盘的新 persistent_store 行被补偿清理（work 资源下仅剩 2 行：旧 image 软删 + thumbnail 活行）
	if got := queryInt64(t, db, `
		SELECT COUNT(*) FROM persistent_store ps
		JOIN resource_store rs ON rs.store_id = ps.id
		JOIN resource r ON r.id = rs.resource_id
		WHERE r.work_id = ?`, workID); got != 2 {
		t.Fatalf("事务失败后持久化 store 行应保持 2（软删 image + 活 thumbnail），实际 %d", got)
	}
}
