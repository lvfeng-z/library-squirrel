package taskManager

import (
	"path/filepath"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// pathTestStrPtr 测试辅助:返回字符串指针
func pathTestStrPtr(s string) *string { return &s }

// pathTestFormatProvider 测试用文件名模板提供者
type pathTestFormatProvider struct{ format string }

func (p pathTestFormatProvider) GetFileNameFormat() string { return p.format }

// newNamingTask 构造命名测试用 ManagedTask(模板 [${author}]_[${siteWorkId}]_${siteWorkName} + 作品元数据)。
// resolveBaseName 算出的 bas = [作者]_[workId]_作品名
func newNamingTask(author, workId, workName string) *ManagedTask {
	return &ManagedTask{
		task: entity.NewTask(),
		deps: &TaskDeps{
			FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		},
		workResp: &sdkdto.WorkResponse{
			Work: &sdkdto.WorkDTO{
				SiteWorkId:   pathTestStrPtr(workId),
				SiteWorkName: pathTestStrPtr(workName),
			},
			LocalAuthors: []*sdkdto.LocalAuthorDTO{{AuthorName: pathTestStrPtr(author)}},
		},
	}
}

// TestResolveStorePath_SingleStoreNoSuffix 验证单 store 资源用 <bas>.<ext>(无 role/seq 后缀)。
// 对应 pixiv 单图:InvolvedRoles=[image],specs 仅一个 image store
func TestResolveStorePath_SingleStoreNoSuffix(t *testing.T) {
	m := newNamingTask("author2", "456", "single")
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Format: "png"},
	}
	baseRelPath, bas := m.resolveBaseName(m.workResp)
	multiStore := len(specs) > 1
	relPath, fileName := m.resolveStorePath(specs[0], baseRelPath, bas, 0, multiStore)

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
	m := newNamingTask("author1", "123", "test")
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeDocument, Format: "md", Generation: entity.GenerationDerived},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
	}
	baseRelPath, bas := m.resolveBaseName(m.workResp)
	multiStore := len(specs) > 1

	counters := map[string]int{}
	seen := make(map[string]string, len(specs))
	var imageNames []string
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := m.resolveStorePath(spec, baseRelPath, bas, seq, multiStore)
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
	m := newNamingTask("author3", "789", "thumb")
	// 多 store 资源含 thumbnail(如 local 视频导入:image + thumbnail)
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Format: "png"},
		{Role: entity.StoreTypeThumbnail, Format: "jpg"},
	}
	baseRelPath, bas := m.resolveBaseName(m.workResp)
	multiStore := len(specs) > 1

	counters := map[string]int{}
	got := map[string]string{}
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := m.resolveStorePath(spec, baseRelPath, bas, seq, multiStore)
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
	m := newNamingTask("author4", "111", "desc")
	baseRelPath, bas := m.resolveBaseName(m.workResp)
	multiStore := true

	withDesc := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "png", Description: "cover"}
	withoutDesc := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Format: "png"}

	_, nameWith := m.resolveStorePath(withDesc, baseRelPath, bas, 0, multiStore)
	_, nameWithout := m.resolveStorePath(withoutDesc, baseRelPath, bas, 1, multiStore)

	// 有描述:<bas>_image_000_cover.png
	if nameWith != "[author4]_[111]_desc_image_000_cover.png" {
		t.Fatalf("有描述期望 [author4]_[111]_desc_image_000_cover.png 实际 %s", nameWith)
	}
	// 无描述:省略描述段
	if nameWithout != "[author4]_[111]_desc_image_001.png" {
		t.Fatalf("无描述期望 [author4]_[111]_desc_image_001.png 实际 %s", nameWithout)
	}
}

// TestResumeSpecSeq 验证 resume 时 spec→全局 store_seq 的正确配对:
// 同 role 部分 store 完成时,downloaded specs 按 streamOffsets 配对全局 seq(非 specs 内 0-based 重计),
// derived 按 role 从 storeRows 未完成行查。否则 findStoreRowByIdentity 会匹配已完成行→续传覆盖。
// resumeSpecSeq 为纯函数,ResourceStore 字面量仅设读取字段(StoreType/StoreSeq/Generation)
func TestResumeSpecSeq(t *testing.T) {
	// storeRows:image seq=0 完成 / seq=1,2 未完成(downloaded);document seq=0 完成;thumbnail seq=0 未完成(derived)
	storeRows := []*entity.ResourceStore{
		{StoreType: entity.StoreTypeImage, StoreSeq: 0, Generation: entity.GenerationDownloaded},
		{StoreType: entity.StoreTypeImage, StoreSeq: 1, Generation: entity.GenerationDownloaded},
		{StoreType: entity.StoreTypeImage, StoreSeq: 2, Generation: entity.GenerationDownloaded},
		{StoreType: entity.StoreTypeDocument, StoreSeq: 0, Generation: entity.GenerationDerived},
		{StoreType: entity.StoreTypeThumbnail, StoreSeq: 0, Generation: entity.GenerationDerived},
	}
	completed := map[storeIdentity]struct{}{
		{role: entity.StoreTypeImage, seq: 0}:    {},
		{role: entity.StoreTypeDocument, seq: 0}: {},
	}
	// streamOffsets:主程序按 storeRows 未完成 downloaded 顺序构造,携带全局 StoreSeq
	streamOffsets := []*sdkdto.StoreResumeOffset{
		{Role: entity.StoreTypeImage, StoreSeq: 1, Offset: 1024},
		{Role: entity.StoreTypeImage, StoreSeq: 2, Offset: 0},
	}
	// resume 返回 specs:2 个 downloaded image(对应 streamOffsets) + 1 个 derived thumbnail
	dlImg1 := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded}
	dlImg2 := &sdkdto.StoreSpec{Role: entity.StoreTypeImage, Generation: entity.GenerationDownloaded}
	derivedThumb := &sdkdto.StoreSpec{Role: entity.StoreTypeThumbnail, Generation: entity.GenerationDerived}
	specs := []*sdkdto.StoreSpec{dlImg1, dlImg2, derivedThumb}

	got := resumeSpecSeq(specs, streamOffsets, storeRows, completed)

	// downloaded 配对全局 seq(1,2),非 specs 内重计(0,1)——否则 dlImg1 会错配到已完成 seq=0
	if got[dlImg1] != 1 {
		t.Fatalf("dlImg1 期望全局 seq=1(streamOffsets[0]), 实际 %d", got[dlImg1])
	}
	if got[dlImg2] != 2 {
		t.Fatalf("dlImg2 期望全局 seq=2(streamOffsets[1]), 实际 %d", got[dlImg2])
	}
	// derived thumbnail 从 storeRows 未完成 derived 行查(seq=0)
	if got[derivedThumb] != 0 {
		t.Fatalf("derivedThumb 期望全局 seq=0(storeRows 未完成 derived), 实际 %d", got[derivedThumb])
	}
}
