package download

// 命名派生测试：桶段（复合键哈希前 2 位 hex，同键恒同桶）与作品目录段（siteKey_siteWorkId
// 派生段——合法 ID 原文直用、净化变更追加消歧、超长截断消歧、空值拒绝、同键恒同路径）及
// store 文件名（恒带 role_seq 三位零填充、扩展名规范化、描述段退役）。替换链「软删移出先于
// rename 写入同路径」的时序锚定见 staging_test.go 的 TestRedownloadSamePath_VictimFileMovedBeforeRename。

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// pathTestStrPtr 测试辅助:返回字符串指针
func pathTestStrPtr(s string) *string { return &s }

// pathTestNullStr 测试辅助:可空字符串
func pathTestNullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// newNamingSession 构造命名派生测试会话（siteId=1，站点键经 resolver 桩映射为 siteKey，
// 领域行携带站点复合键 siteWorkId）
func newNamingSession(siteKey, siteWorkId string) (*execSession, context.CancelFunc) {
	deps := &Deps{
		SiteKeyResolver: &fakeSiteKeyResolver{keys: map[int64]string{1: siteKey}},
	}
	h, cancel := newFakeHandle()
	wt := entity.NewWorkTask(1)
	wt.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	wt.SiteWorkID = sql.NullString{String: siteWorkId, Valid: true}
	sess := newExecSession(deps, h, wt)
	return sess, cancel
}

// hashPrefix siteWorkId 的 sha256 前 digits 位 hex（与 SDK storepath 消歧哈希段同源算法，
// 期望值在测试侧独立计算）
func hashPrefix(siteWorkId string, digits int) string {
	sum := sha256.Sum256([]byte(siteWorkId))
	return hex.EncodeToString(sum[:])[:digits]
}

// bucketOf 复合键（siteKey + "_" + siteWorkId）的 sha256 前 2 位 hex（与 SDK storepath
// 桶段同源算法，期望值在测试侧独立计算）
func bucketOf(siteKey, siteWorkId string) string {
	sum := sha256.Sum256([]byte(siteKey + "_" + siteWorkId))
	return hex.EncodeToString(sum[:])[:2]
}

// TestResolveStoreDir_IdentityForms 目录段形态：store/resource/{桶段}/siteKey 原文 +
// siteWorkId 派生段（合法字符 ID 原文直用——覆盖 pixiv 数字 ID、bilibili 混合 ID、
// local 64 位 hex ID）；桶段随复合键各自不同
func TestResolveStoreDir_IdentityForms(t *testing.T) {
	localId := strings.Repeat("3f2a", 16) // 64 位 hex（local 导入作品的站点侧 ID 形态）
	cases := []struct {
		siteKey    string
		siteWorkId string
		want       string
	}{
		{"pixiv", "128937464", "store/resource/" + bucketOf("pixiv", "128937464") + "/pixiv_128937464"},
		{"bilibili", "BV1xx411c7mD_4538792", "store/resource/" + bucketOf("bilibili", "BV1xx411c7mD_4538792") + "/bilibili_BV1xx411c7mD_4538792"},
		{"local", localId, "store/resource/" + bucketOf("local", localId) + "/local_" + localId},
	}
	for _, c := range cases {
		sess, cancel := newNamingSession(c.siteKey, c.siteWorkId)
		got, err := sess.resolveStoreDir(context.Background())
		cancel()
		if err != nil {
			t.Fatalf("派生失败(%s_%s): %v", c.siteKey, c.siteWorkId, err)
		}
		if got != c.want {
			t.Fatalf("目录期望 %s 实际 %s", c.want, got)
		}
	}
}

// TestResolveStoreDir_SanitizeAppendsDisambiguator 净化发生变更（含 Windows 非法字符）时
// 追加消歧哈希段：派生段 = 净化结果 + "_" + sha256(原始 ID) 前 8 位 hex
func TestResolveStoreDir_SanitizeAppendsDisambiguator(t *testing.T) {
	sess, cancel := newNamingSession("pixiv", `a/b:c`)
	defer cancel()
	got, err := sess.resolveStoreDir(context.Background())
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	want := "store/resource/" + bucketOf("pixiv", `a/b:c`) + "/pixiv_a／b：c_" + hashPrefix(`a/b:c`, 8)
	if got != want {
		t.Fatalf("净化消歧段期望 %s 实际 %s", want, got)
	}
}

// TestResolveStoreDir_TruncationBranch 派生段超长（>96 字符）截断消歧：净化结果前 88 字符
// + "_" + sha256(原始 ID) 前 8 位 hex
func TestResolveStoreDir_TruncationBranch(t *testing.T) {
	id := strings.Repeat("a", 120)
	sess, cancel := newNamingSession("pixiv", id)
	defer cancel()
	got, err := sess.resolveStoreDir(context.Background())
	if err != nil {
		t.Fatalf("派生失败: %v", err)
	}
	want := "store/resource/" + bucketOf("pixiv", id) + "/pixiv_" + strings.Repeat("a", 88) + "_" + hashPrefix(id, 8)
	if got != want {
		t.Fatalf("截断消歧段期望 %s 实际 %s", want, got)
	}
}

// TestResolveStoreDir_SameIdentitySamePath 同一站点复合键恒派生同一目录（ID 名下重下同作品
// 命中同一路径的身份前提）；净化后同形的不同 ID 由消歧哈希段区分（单射——半角非法字符与
// 其全角等价字符净化后同形，消歧按原始 ID 计算承担区分）
func TestResolveStoreDir_SameIdentitySamePath(t *testing.T) {
	s1, cancel1 := newNamingSession("pixiv", "123456")
	a, errA := s1.resolveStoreDir(context.Background())
	cancel1()
	s2, cancel2 := newNamingSession("pixiv", "123456")
	b, errB := s2.resolveStoreDir(context.Background())
	cancel2()
	if errA != nil || errB != nil || a != b {
		t.Fatalf("同一复合键应恒派生同一路径: %q/%q err=%v/%v", a, b, errA, errB)
	}

	s3, cancel3 := newNamingSession("pixiv", "ab/cd") // 净化为全角 → 追加消歧段
	c, errC := s3.resolveStoreDir(context.Background())
	cancel3()
	s4, cancel4 := newNamingSession("pixiv", "ab／cd") // 原文合法（全角本就安全）→ 原文直用
	d, errD := s4.resolveStoreDir(context.Background())
	cancel4()
	if errC != nil || errD != nil {
		t.Fatalf("派生失败: %v/%v", errC, errD)
	}
	if c == d {
		t.Fatalf("净化后同形的不同 ID 应得不同目录（消歧单射）: %q", c)
	}
}

// TestResolveStoreDir_BucketSegmentStable 桶段显式锚定：派生路径的第三段恒为复合键哈希
// 前 2 位 hex——同 siteKey+siteWorkId 两次派生路径一致（同键恒同桶→恒同路径，ID 名下
// 重下命中同路径的桶层前提）；不同复合键的桶段按各自键哈希独立得出（可以不同）
func TestResolveStoreDir_BucketSegmentStable(t *testing.T) {
	keys := [][2]string{
		{"pixiv", "128937464"},
		{"bilibili", "BV1xx411c7mD_4538792"},
	}
	for _, k := range keys {
		s1, cancel1 := newNamingSession(k[0], k[1])
		a, errA := s1.resolveStoreDir(context.Background())
		cancel1()
		s2, cancel2 := newNamingSession(k[0], k[1])
		b, errB := s2.resolveStoreDir(context.Background())
		cancel2()
		if errA != nil || errB != nil || a != b {
			t.Fatalf("同一复合键(%s_%s)两次派生应恒同路径: %q/%q err=%v/%v", k[0], k[1], a, b, errA, errB)
		}
		parts := strings.Split(a, "/")
		if len(parts) != 4 || parts[0] != "store" || parts[1] != "resource" || parts[2] != bucketOf(k[0], k[1]) {
			t.Fatalf("路径桶段应为复合键哈希前 2 位 hex(%s_%s → %s): %q", k[0], k[1], bucketOf(k[0], k[1]), a)
		}
	}
}

// TestResolveStoreDir_RejectsMissingIdentity 空值严格拒绝：领域行缺站点复合键或站点行
// 查不到时显式报错（写入路径严格识别，不回落）
func TestResolveStoreDir_RejectsMissingIdentity(t *testing.T) {
	h, cancel := newFakeHandle()
	defer cancel()
	deps := &Deps{SiteKeyResolver: &fakeSiteKeyResolver{keys: map[int64]string{1: "pixiv"}}}

	wtNoWorkId := entity.NewWorkTask(1)
	wtNoWorkId.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	if _, err := newExecSession(deps, h, wtNoWorkId).resolveStoreDir(context.Background()); err == nil {
		t.Fatal("缺 siteWorkId 应显式报错")
	}

	wtNoSiteId := entity.NewWorkTask(1)
	wtNoSiteId.SiteWorkID = sql.NullString{String: "123", Valid: true}
	if _, err := newExecSession(deps, h, wtNoSiteId).resolveStoreDir(context.Background()); err == nil {
		t.Fatal("缺 siteId 应显式报错")
	}

	// 站点行缺失（resolver 无该 id 映射）
	wtNoSiteRow := entity.NewWorkTask(1)
	wtNoSiteRow.SiteID = sql.NullInt64{Int64: 9, Valid: true}
	wtNoSiteRow.SiteWorkID = sql.NullString{String: "123", Valid: true}
	if _, err := newExecSession(deps, h, wtNoSiteRow).resolveStoreDir(context.Background()); err == nil {
		t.Fatal("站点行缺失应显式报错")
	}
}

// TestResolveStorePath_Form 文件名恒带 role_seq（单 store 资源不省略段）；ext 经 normalizeExt
// 补前导点；spec.Description 不参与最终名（描述段退役）
func TestResolveStorePath_Form(t *testing.T) {
	base := "store/resource/pixiv_123"
	// 单 store 资源（旧命名在此场景省略 role_seq）
	rel, name, err := resolveStorePath(&sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "jpg"}, base, 0)
	if err != nil || name != "image_000.jpg" || rel != "store/resource/pixiv_123/image_000.jpg" {
		t.Fatalf("单 store 应恒带 role_seq: name=%q rel=%q err=%v", name, rel, err)
	}
	// 同 role 多轨递增 seq（三位零填充）、ext 已带点直用
	rel2, name2, err2 := resolveStorePath(&sdkdto.StoreSpec{Role: entity.StoreTypeVideoTrack, Format: ".mp4"}, base, 12)
	if err2 != nil || name2 != "videoTrack_012.mp4" || rel2 != "store/resource/pixiv_123/videoTrack_012.mp4" {
		t.Fatalf("多轨 seq 递增失败: name=%q rel=%q err=%v", name2, rel2, err2)
	}
	// 描述段退役：Description 不影响最终名
	_, withDesc, err3 := resolveStorePath(&sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "jpg", Description: "cover"}, base, 0)
	if err3 != nil || withDesc != "image_000.jpg" {
		t.Fatalf("Description 不应参与文件名: %q err=%v", withDesc, err3)
	}
	// 非法 role 显式报错（契约输入严格识别）
	if _, _, err4 := resolveStorePath(&sdkdto.StoreSpec{Role: "a/b", Format: "jpg"}, base, 0); err4 == nil {
		t.Fatal("非法 role 应显式报错")
	}
}

// TestRunModeFromTask 板块模式派生：StoreRoles NULL→All（universe 透传）、Valid 空串→None、
// 非空→Selected；workInfo 统一取 IncludeWorkInfo
func TestRunModeFromTask(t *testing.T) {
	wt := entity.NewWorkTask(1)
	wt.IncludeWorkInfo = true
	wt.InvolvedRoles = pathTestNullStr("image,thumbnail")
	got := runModeFromTask(wt)
	if got.storeScope.kind != scopeAll || len(got.storeScope.roles) != 2 || !got.workInfo {
		t.Fatalf("NULL StoreRoles 应派生 All+universe, 实际 %+v", got)
	}

	wt2 := entity.NewWorkTask(2)
	wt2.StoreRoles = pathTestNullStr("")
	wt2.IncludeWorkInfo = true
	got2 := runModeFromTask(wt2)
	if got2.storeScope.kind != scopeNone || got2.storeScope.coversStores() {
		t.Fatalf("Valid 空串应派生 None, 实际 %+v", got2)
	}

	wt3 := entity.NewWorkTask(3)
	wt3.StoreRoles = pathTestNullStr("image")
	wt3.IncludeWorkInfo = false
	got3 := runModeFromTask(wt3)
	if got3.storeScope.kind != scopeSelected || got3.workInfo {
		t.Fatalf("非空应派生 Selected 且 workInfo 取行值, 实际 %+v", got3)
	}
}
