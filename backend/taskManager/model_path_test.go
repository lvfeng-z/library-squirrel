package taskManager

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// pathTestStrPtr 测试辅助:返回字符串指针
func pathTestStrPtr(s string) *string { return &s }

// pathTestFormatProvider 测试用文件名模板提供者
type pathTestFormatProvider struct{ format string }

func (p pathTestFormatProvider) GetFileNameFormat() string { return p.format }

// TestFindMainSpec_PrimaryRoles 验证 D2-A:展示主体按资源类型 PrimaryRoles 选取
func TestFindMainSpec_PrimaryRoles(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		specs        []*sdkdto.StoreSpec
		wantRole     string // 期望主体 role;空=期望 nil(由调用方兜底 specs[0])
	}{
		{
			name:         "article 主体为 document(非内嵌 image)",
			resourceType: entity.ResourceTypeArticle,
			specs: []*sdkdto.StoreSpec{
				{Role: entity.StoreTypeDocument, Format: "md"},
				{Role: entity.StoreTypeImage, Format: "png"},
				{Role: entity.StoreTypeImage, Format: "png"},
			},
			wantRole: entity.StoreTypeDocument,
		},
		{
			name:         "image 主体为 image",
			resourceType: entity.ResourceTypeImage,
			specs: []*sdkdto.StoreSpec{
				{Role: entity.StoreTypeImage, Format: "png"},
				{Role: entity.StoreTypeThumbnail, Format: "jpg"},
			},
			wantRole: entity.StoreTypeImage,
		},
		{
			name:         "video 主体优先 merged 次选 videoTrack",
			resourceType: entity.ResourceTypeVideo,
			specs: []*sdkdto.StoreSpec{
				{Role: entity.StoreTypeVideoTrack, Format: "mp4"},
				{Role: entity.StoreTypeAudioTrack, Format: "m4a"},
			},
			wantRole: entity.StoreTypeVideoTrack,
		},
		{
			name:         "unknown 返回 nil 由调用方兜底 specs[0]",
			resourceType: entity.ResourceTypeUnknown,
			specs: []*sdkdto.StoreSpec{
				{Role: entity.StoreTypeImage, Format: "png"},
			},
			wantRole: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMainSpec(tt.specs, tt.resourceType)
			if tt.wantRole == "" {
				if got != nil {
					t.Fatalf("期望 nil, 实际 role=%s", got.Role)
				}
				return
			}
			if got == nil {
				t.Fatalf("期望 role=%s, 实际 nil", tt.wantRole)
			}
			if got.Role != tt.wantRole {
				t.Fatalf("期望 role=%s, 实际=%s", tt.wantRole, got.Role)
			}
		})
	}
}

// TestResolveStorePath_SameRoleDisambiguation 验证 D3-B:同 role 多 store 加序号后缀消歧
func TestResolveStorePath_SameRoleDisambiguation(t *testing.T) {
	m := &ManagedTask{
		task: entity.NewTask(),
		deps: &TaskDeps{
			FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		},
		workResp: &sdkdto.WorkResponse{
			Work: &sdkdto.WorkDTO{
				SiteWorkID:   pathTestStrPtr("123"),
				SiteWorkName: pathTestStrPtr("test"),
			},
			LocalAuthors: []*sdkdto.LocalAuthorDTO{{AuthorName: pathTestStrPtr("author1")}},
		},
	}
	m.task.ResourceType = sql.NullString{String: entity.ResourceTypeArticle, Valid: true}

	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeDocument, Format: "md", Generation: entity.GenerationDerived},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
		{Role: entity.StoreTypeImage, Format: "png", Generation: entity.GenerationDownloaded},
	}

	roleCounts := countSpecRoles(specs)
	nctx := m.namingCtxFromTask(m.workResp)
	counters := map[string]int{}
	seen := make(map[string]string, len(specs))
	var imageNames []string
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := resolveStorePath(spec, "", "", seq, roleCounts[spec.Role] > 1, nctx)
		if other, dup := seen[fileName]; dup {
			t.Fatalf("文件名重复: %s (role=%s 与 %s)", fileName, spec.Role, other)
		}
		seen[fileName] = spec.Role
		if spec.Role == entity.StoreTypeImage {
			imageNames = append(imageNames, fileName)
		}
	}

	// document 单例:无序号后缀
	if _, ok := seen["[author1]_[123]_test.md"]; !ok {
		t.Fatalf("document 单例应无后缀 [author1]_[123]_test.md, 实际=%v", seen)
	}
	// 3 个 image 各带递增序号后缀(_000/_001/_002)
	wantImages := []string{"[author1]_[123]_test_000.png", "[author1]_[123]_test_001.png", "[author1]_[123]_test_002.png"}
	if len(imageNames) != len(wantImages) {
		t.Fatalf("期望 %d 个 image 文件名, 实际 %d (%v)", len(wantImages), len(imageNames), imageNames)
	}
	for i, w := range wantImages {
		if imageNames[i] != w {
			t.Fatalf("image[%d] 期望 %s 实际 %s", i, w, imageNames[i])
		}
	}
}

// TestResolveStorePath_SingleRoleNoSuffix 验证单例 role 不加后缀(R1:零扰动 pixiv/local 现有命名)
func TestResolveStorePath_SingleRoleNoSuffix(t *testing.T) {
	m := &ManagedTask{
		task: entity.NewTask(),
		deps: &TaskDeps{
			FileNameFormatProvider: pathTestFormatProvider{format: "[${author}]_[${siteWorkId}]_${siteWorkName}"},
		},
		workResp: &sdkdto.WorkResponse{
			Work: &sdkdto.WorkDTO{
				SiteWorkID:   pathTestStrPtr("456"),
				SiteWorkName: pathTestStrPtr("single"),
			},
			LocalAuthors: []*sdkdto.LocalAuthorDTO{{AuthorName: pathTestStrPtr("author2")}},
		},
	}
	m.task.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}

	// 单例 image + 单例 thumbnail:均不加序号后缀
	specs := []*sdkdto.StoreSpec{
		{Role: entity.StoreTypeImage, Format: "png"},
		{Role: entity.StoreTypeThumbnail, Format: "jpg"},
	}
	roleCounts := countSpecRoles(specs)
	nctx := m.namingCtxFromTask(m.workResp)
	counters := map[string]int{}
	mainRelPath, mainFileName := "store/resource/author2/[author2]_[456]_single.png", "[author2]_[456]_single.png"
	for _, spec := range specs {
		seq := counters[spec.Role]
		counters[spec.Role]++
		_, fileName := resolveStorePath(spec, mainRelPath, mainFileName, seq, roleCounts[spec.Role] > 1, nctx)
		if spec.Role == entity.StoreTypeImage && fileName != "[author2]_[456]_single.png" {
			t.Fatalf("单例 image 不应有后缀, 实际 %s", fileName)
		}
	}
}
