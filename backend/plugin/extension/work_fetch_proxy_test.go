package extension

import (
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/gen"
)

// TestProtoToTaskCreateResponseFieldFidelity 转换字段保真回归：
// dto.TaskCreateResponse 是 gen.TaskCreateResponse 的类型别名，转换函数手工逐字段复制，
// proto 新增字段漏复制时消费侧拿到零值（事故：siteKey 接线后漏复制致所有插件任务创建
// 被 SiteKey 必填校验静默拒绝）。全字段填非零值 + DeepEqual，任何字段漏复制即失败。
func TestProtoToTaskCreateResponseFieldFidelity(t *testing.T) {
	src := &gen.TaskCreateResponse{
		PluginTaskId:  "ptid-1",
		TaskName:      "任务名",
		SiteWorkId:    "swid-1",
		Url:           "https://example.com/1",
		PluginData:    `{"schemaVersion":1}`,
		SiteName:      "pixiv",
		SiteKey:       "pixiv",
		InvolvedRoles: []string{"image"},
		ResourceType:  "image",
		Children: []*gen.TaskCreateChildResponse{{
			TaskName:      "子任务",
			SiteWorkId:    "swid-1-0",
			Url:           "https://example.com/1_0",
			PluginData:    `{"schemaVersion":1,"page":0}`,
			SiteName:      "pixiv",
			InvolvedRoles: []string{"image"},
			ResourceType:  "image",
		}},
	}

	got := protoToTaskCreateResponse(src)
	if got == nil {
		t.Fatal("转换返回 nil")
	}
	if got.PluginTaskId != src.PluginTaskId || got.SiteKey != src.SiteKey || got.ResourceType != src.ResourceType {
		t.Fatalf("关键字段丢失: PluginTaskId=%q SiteKey=%q ResourceType=%q", got.PluginTaskId, got.SiteKey, got.ResourceType)
	}
	if len(got.Children) != len(src.Children) || got.Children[0].ResourceType != src.Children[0].ResourceType {
		t.Fatalf("children 转换丢失: %+v", got.Children)
	}
}

// TestProtoToTaskCreateResponseChildSiteKeyPassthrough 子行归属键透传回归：
// 契约 19 子行 siteKey 由插件侧填入 proto、宿主 RPC 重建层拷回 DTO、下游
// childToResponse 非空用子行键/空回退父键（事故：子行拷贝循环漏拷 SiteKey，
// 两层各自单测全绿、缺口恰在中间——插件侧填 pixiv 落库仍为 local）。
// 透传语义=不默认化/不校验/不在本层回退：非空子键逐字保留，空子键保持空
// （父键非空也不得被错误灌入空子行，回退是下游 childToResponse 的职责）。
func TestProtoToTaskCreateResponseChildSiteKeyPassthrough(t *testing.T) {
	src := &gen.TaskCreateResponse{
		PluginTaskId: "ptid-1",
		SiteKey:      "parent-key",
		Children: []*gen.TaskCreateChildResponse{
			{TaskName: "子任务-带键", SiteKey: "pixiv"},
			{TaskName: "子任务-空键"},
		},
	}

	got := protoToTaskCreateResponse(src)
	if got == nil {
		t.Fatal("转换返回 nil")
	}
	if len(got.Children) != len(src.Children) {
		t.Fatalf("children 条目数不符: %+v", got.Children)
	}
	if got.Children[0].SiteKey != src.Children[0].SiteKey {
		t.Fatalf("子行非空 SiteKey 未逐字保留: got %q want %q", got.Children[0].SiteKey, src.Children[0].SiteKey)
	}
	if got.Children[1].SiteKey != "" {
		t.Fatalf("子行空 SiteKey 应保持空（不得回退父键）: got %q", got.Children[1].SiteKey)
	}
}

// TestProtoToWorkResponseLocalAuthorRoleName 作品声明面本地作者 RoleName 保真回归：
// LocalAuthorDTO.RoleName 是 proto optional（*string），携带本地作者时随条目声明关联级角色
// （事故：转换漏拷 RoleName 致插件声明的关联级角色在代理层丢失、落库 role_name 恒空）。
// 断言指针值原样保留：有声明的条目拷出同一角色串，未声明的条目保持 nil。
func TestProtoToWorkResponseLocalAuthorRoleName(t *testing.T) {
	role := "原画"
	src := &gen.WorkResponse{
		LocalAuthors: []*gen.LocalAuthorDTO{
			{AuthorName: ptrOf("作者甲"), RoleName: &role},
			{AuthorName: ptrOf("作者乙")},
		},
	}

	got := protoToWorkResponse(src)
	if got == nil {
		t.Fatal("转换返回 nil")
	}
	if len(got.LocalAuthors) != len(src.LocalAuthors) {
		t.Fatalf("localAuthors 条目数不符: %+v", got.LocalAuthors)
	}
	if got.LocalAuthors[0].RoleName == nil || *got.LocalAuthors[0].RoleName != role {
		t.Fatalf("声明的 RoleName 丢失: %+v", got.LocalAuthors[0])
	}
	if got.LocalAuthors[1].RoleName != nil {
		t.Fatalf("未声明的 RoleName 应保持 nil: %+v", got.LocalAuthors[1])
	}
}

// ptrOf 测试辅助：取字符串指针（proto optional 字段赋值用）
func ptrOf(s string) *string { return &s }
