package download

// 命名解析测试（自任务管理器迁移，断言不变）：bas 基准名（模板占位符替换+净化）、
// 单/多 store 文件名消歧（role+seq+描述段）、resume 的 spec→全局 store_seq 配对。

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// pathTestStrPtr 测试辅助:返回字符串指针
func pathTestStrPtr(s string) *string { return &s }

// pathTestNullStr 测试辅助:可空字符串
func pathTestNullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// pathTestFormatProvider 测试用文件名模板提供者
type pathTestFormatProvider struct{ format string }

func (p pathTestFormatProvider) GetFileNameFormat() string { return p.format }

// newNamingSession 构造命名测试执行会话(模板 [${author}]_[${siteWorkId}]_${siteWorkName} + 作品元数据)。
// resolveBaseName 算出的 bas = [作者]_[workId]_作品名
func newNamingSession(author, workId, workName string) (*execSession, context.CancelFunc) {
	deps := &Deps{
		FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
	}
	h, cancel := newFakeHandle()
	sess := newExecSession(deps, h, nil)
	sess.workResp = &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkId:   pathTestStrPtr(workId),
			SiteWorkName: pathTestStrPtr(workName),
		},
		LocalAuthors: []*sdkdto.LocalAuthorDTO{{AuthorName: pathTestStrPtr(author)}},
	}
	return sess, cancel
}

// TestResolveStorePath_SingleStoreNoSuffix 验证单 store 资源用 <bas>.<ext>(无 role/seq 后缀)。
// 对应 pixiv 单图:InvolvedRoles=[image],specs 仅一个 image store
func TestResolveStorePath_SingleStoreNoSuffix(t *testing.T) {
	sess, cancel := newNamingSession("author2", "456", "single")
	defer cancel()
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Format: "png"},
	}
	baseRelPath, bas := sess.resolveBaseName(sess.workResp)
	multiStore := len(specs) > 1
	relPath, fileName := sess.resolveStorePath(specs[0], baseRelPath, bas, 0, multiStore)

	if bas != "[author2]_[456]_single" {
		t.Fatalf("bas 期望 [author2]_[456]_single 实际 %s", bas)
	}
	if fileName != "[author2]_[456]_single.png" {
		t.Fatalf("单 store 文件名期望 [author2]_[456]_single.png 实际 %s", fileName)
	}
	wantRel := filepath.ToSlash(filepath.Join("store", "resource", "author2", "[author2]_[456]_single.png"))
	if relPath != wantRel {
		t.Fatalf("单 store 路径期望 %s 实际 %s", wantRel, relPath)
	}
}

// TestResolveStorePath_MultiStoreRoleSeq 验证多 store 资源全部带 role+seq(article:document + N image)。
// 多 store 判定为资源级:store 总数>1 即全部带后缀,不论各 role 单例与否
func TestResolveStorePath_MultiStoreRoleSeq(t *testing.T) {
	sess, cancel := newNamingSession("author1", "123", "test")
	defer cancel()
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeDocument, Format: "md", Generation: entity.GenerationDerived},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
	}
	baseRelPath, bas := sess.resolveBaseName(sess.workResp)
	multiStore := len(specs) > 1

	counters := map[string]int{}
	seen := make(map[string]string, len(specs))
	var imageNames []string
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := sess.resolveStorePath(spec, baseRelPath, bas, seq, multiStore)
		if other, dup := seen[fileName]; dup {
			t.Fatalf("文件名重复: %s (role=%s 与 %s)", fileName, spec.Role, other)
		}
		seen[fileName] = spec.Role
		if spec.Role == entity.StoreTypeImage {
			imageNames = append(imageNames, fileName)
		}
	}

	// document 虽是单例,但资源多 store → 仍带 role+seq
	if _, ok := seen["[author1]_[123]_test_document_000.md"]; !ok {
		t.Fatalf("document 应带 role+seq [author1]_[123]_test_document_000.md, 实际=%v", seen)
	}
	// 3 个 image 各带递增 seq(同 role 内 0-based)
	wantImages := []string{
		"[author1]_[123]_test_image_000.png",
		"[author1]_[123]_test_image_001.png",
		"[author1]_[123]_test_image_002.png",
	}
	if len(imageNames) != len(wantImages) {
		t.Fatalf("期望 %d 个 image 文件名, 实际 %d (%v)", len(wantImages), len(imageNames), imageNames)
	}
	for i, w := range wantImages {
		if imageNames[i] != w {
			t.Fatalf("image[%d] 期望 %s 实际 %s", i, w, imageNames[i])
		}
	}
}

// TestResolveStorePath_ThumbnailOrdinaryRole 验证 thumbnail 作为普通 role:
// 多 store 资源含 thumbnail 时(thumbnail 与主资源共享 bas,需 role 区分),thumbnail 带 _thumbnail_000
func TestResolveStorePath_ThumbnailOrdinaryRole(t *testing.T) {
	sess, cancel := newNamingSession("author3", "789", "thumb")
	defer cancel()
	// 多 store 资源含 thumbnail(如本地视频导入:image + thumbnail)
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Format: "png"},
		{Role: entity.StoreTypeThumbnail, Format: "jpg"},
	}
	baseRelPath, bas := sess.resolveBaseName(sess.workResp)
	multiStore := len(specs) > 1

	counters := map[string]int{}
	got := map[string]string{}
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := sess.resolveStorePath(spec, baseRelPath, bas, seq, multiStore)
		got[spec.Role] = fileName
	}
	if got[entity.StoreTypeImage] != "[author3]_[789]_thumb_image_000.png" {
		t.Fatalf("image 期望 [author3]_[789]_thumb_image_000.png 实际 %s", got[entity.StoreTypeImage])
	}
	if got[entity.StoreTypeThumbnail] != "[author3]_[789]_thumb_thumbnail_000.jpg" {
		t.Fatalf("thumbnail 普通 role 期望 [author3]_[789]_thumb_thumbnail_000.jpg 实际 %s", got[entity.StoreTypeThumbnail])
	}
}

// TestResolveStorePath_Description 验证 spec.Description 作为多 store 文件名的可选拼段:有则拼接,空则省略
func TestResolveStorePath_Description(t *testing.T) {
	sess, cancel := newNamingSession("author4", "111", "desc")
	defer cancel()
	baseRelPath, bas := sess.resolveBaseName(sess.workResp)
	multiStore := true

	withDesc := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "png", Description: "cover"}
	withoutDesc := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "png"}

	_, nameWith := sess.resolveStorePath(withDesc, baseRelPath, bas, 0, multiStore)
	_, nameWithout := sess.resolveStorePath(withoutDesc, baseRelPath, bas, 1, multiStore)

	// 有描述:<bas>_image_000_cover.png
	if nameWith != "[author4]_[111]_desc_image_000_cover.png" {
		t.Fatalf("有描述期望 [author4]_[111]_desc_image_000_cover.png 实际 %s", nameWith)
	}
	// 无描述:省略描述段
	if nameWithout != "[author4]_[111]_desc_image_001.png" {
		t.Fatalf("无描述期望 [author4]_[111]_desc_image_001.png 实际 %s", nameWithout)
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
