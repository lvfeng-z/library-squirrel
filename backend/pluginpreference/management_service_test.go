package pluginpreference

import (
	"context"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// TestManagementListAllWithPluginInfo 全量列表带归属插件显示信息（id/公开 ID/显示名），
// 供记忆管理页按插件分组；条目含信封展示字段所需的原始值
func TestManagementListAllWithPluginInfo(t *testing.T) {
	svc, mgmt, repo := newTestServices(t)
	ctx := context.Background()
	pluginA := plantPluginRow(t, repo, "com.example.mgmt-a", "管理面插件甲")
	pluginB := plantPluginRow(t, repo, "com.example.mgmt-b", "管理面插件乙")

	if err := svc.Set(ctx, pluginA, "illust.form", &PreferenceValue{SchemaVersion: 1, Title: "图文形态", Description: "多图"}); err != nil {
		t.Fatalf("插件甲写入失败: %v", err)
	}
	if err := svc.Set(ctx, pluginB, "video.form", &PreferenceValue{SchemaVersion: 1, Title: "视频形态"}); err != nil {
		t.Fatalf("插件乙写入失败: %v", err)
	}

	entries, err := mgmt.ListAll(ctx)
	if err != nil {
		t.Fatalf("全量列表失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("全量条目 = %d，期望 2", len(entries))
	}
	byKey := make(map[string]EntryWithPlugin, len(entries))
	for _, e := range entries {
		byKey[e.PrefKey] = e
	}
	a := byKey["illust.form"]
	if a.PluginID != pluginA || a.PluginPublicId != "com.example.mgmt-a" || a.PluginName != "管理面插件甲" {
		t.Fatalf("插件甲条目归属信息不符: %+v", a)
	}
	b := byKey["video.form"]
	if b.PluginID != pluginB || b.PluginPublicId != "com.example.mgmt-b" || b.PluginName != "管理面插件乙" {
		t.Fatalf("插件乙条目归属信息不符: %+v", b)
	}
	if a.Value == "" || a.UpdateTime == 0 || a.ID == 0 {
		t.Fatalf("条目展示字段不全: %+v", a)
	}
}

// TestManagementListByPluginPublicId 按插件公开 ID 列表：仅该插件条目（插件设置区
// 只读列表）；未知公开 ID 返回空清单不报错
func TestManagementListByPluginPublicId(t *testing.T) {
	svc, mgmt, repo := newTestServices(t)
	ctx := context.Background()
	pluginA := plantPluginRow(t, repo, "com.example.only-a", "单查插件甲")
	pluginB := plantPluginRow(t, repo, "com.example.only-b", "单查插件乙")

	if err := svc.Set(ctx, pluginA, "k1", &PreferenceValue{SchemaVersion: 1, Title: "一"}); err != nil {
		t.Fatalf("插件甲写入 k1 失败: %v", err)
	}
	if err := svc.Set(ctx, pluginA, "k2", &PreferenceValue{SchemaVersion: 1, Title: "二"}); err != nil {
		t.Fatalf("插件甲写入 k2 失败: %v", err)
	}
	if err := svc.Set(ctx, pluginB, "k3", &PreferenceValue{SchemaVersion: 1, Title: "三"}); err != nil {
		t.Fatalf("插件乙写入 k3 失败: %v", err)
	}

	entries, err := mgmt.ListByPlugin(ctx, "com.example.only-a")
	if err != nil {
		t.Fatalf("按公开 ID 列表失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("插件甲条目 = %d，期望 2（他插件条目不可见）", len(entries))
	}
	for _, e := range entries {
		if e.PluginPublicId != "com.example.only-a" {
			t.Fatalf("串入他插件条目: %+v", e)
		}
	}

	unknown, err := mgmt.ListByPlugin(ctx, "com.example.none")
	if err != nil {
		t.Fatalf("未知公开 ID 应返回空清单不报错: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("未知公开 ID 条目 = %d，期望 0", len(unknown))
	}
}

// TestManagementDeleteThenRuntimeMiss 管理面删除闭环：按条目 id 删除后运行时读取即无
// 记录（下次问答重新发起的前提），同插件他条目不受影响
func TestManagementDeleteThenRuntimeMiss(t *testing.T) {
	svc, mgmt, repo := newTestServices(t)
	ctx := context.Background()
	pluginID := plantPluginRow(t, repo, "com.example.del", "删除闭环插件")

	if err := svc.Set(ctx, pluginID, "illust.form", &PreferenceValue{SchemaVersion: 1, Title: "图文形态"}); err != nil {
		t.Fatalf("写入待删条目失败: %v", err)
	}
	if err := svc.Set(ctx, pluginID, "video.form", &PreferenceValue{SchemaVersion: 1, Title: "视频形态"}); err != nil {
		t.Fatalf("写入留存条目失败: %v", err)
	}

	entries, err := mgmt.ListAll(ctx)
	if err != nil {
		t.Fatalf("删除前列表失败: %v", err)
	}
	var victimID int64
	for _, e := range entries {
		if e.PrefKey == "illust.form" {
			victimID = e.ID
		}
	}
	if victimID == 0 {
		t.Fatal("未找到待删条目 id")
	}

	if err := mgmt.Delete(ctx, victimID); err != nil {
		t.Fatalf("管理面删除失败: %v", err)
	}

	if v, err := svc.Get(ctx, pluginID, "illust.form"); err != nil || v != nil {
		t.Fatalf("删除后运行时读取应无记录: v=%+v err=%v", v, err)
	}
	keys, err := svc.ListKeys(ctx, pluginID)
	if err != nil {
		t.Fatalf("删除后列键失败: %v", err)
	}
	if len(keys) != 1 || keys[0] != "video.form" {
		t.Fatalf("删除后键清单 = %v，期望仅 [video.form]", keys)
	}
	if v, err := svc.Get(ctx, pluginID, "video.form"); err != nil || v == nil || v.Title != "视频形态" {
		t.Fatalf("他条目不应受删除影响: v=%+v err=%v", v, err)
	}
}

// TestHandlerEntryDTOAssembly Handler 展示组装：信封标题/描述拆出，标题空值回落偏好键；
// 值 JSON 非法时按空信封降级且标题仍回落偏好键，单行异常不阻塞列表
func TestHandlerEntryDTOAssembly(t *testing.T) {
	svc, mgmt, repo := newTestServices(t)
	ctx := context.Background()
	pluginID := plantPluginRow(t, repo, "com.example.dto", "组装插件")
	handler := NewHandler(mgmt)

	if err := svc.Set(ctx, pluginID, "titled", &PreferenceValue{SchemaVersion: 1, Title: "有标题", Description: "带描述"}); err != nil {
		t.Fatalf("写入有标题条目失败: %v", err)
	}
	if err := svc.Set(ctx, pluginID, "untitled", &PreferenceValue{SchemaVersion: 1}); err != nil {
		t.Fatalf("写入无标题条目失败: %v", err)
	}
	// 直写非法 JSON 行（绕过服务面序列化），锚定列表降级行为
	broken := entity.NewPluginPreference()
	broken.PluginID = pluginID
	broken.PrefKey = "broken"
	broken.Value = `{not-json`
	if err := repo.GORM().Create(broken).Error; err != nil {
		t.Fatalf("写入非法值行失败: %v", err)
	}

	resp := handler.ListAllPreferences(ctx)
	if !resp.Success {
		t.Fatalf("全量查询失败: %s", resp.Msg)
	}
	items := resp.Data
	if len(items) != 3 {
		t.Fatalf("条目 = %d，期望 3", len(items))
	}
	byKey := make(map[string]*dto.PluginPreferenceEntryDTO, len(items))
	for _, item := range items {
		byKey[item.PrefKey] = item
	}
	if item := byKey["titled"]; item.Title != "有标题" || item.Description != "带描述" {
		t.Fatalf("有标题条目拆解不符: %+v", item)
	}
	if item := byKey["untitled"]; item.Title != "untitled" {
		t.Fatalf("无标题条目应回落偏好键，实际 %q", item.Title)
	}
	if item := byKey["broken"]; item.Title != "broken" {
		t.Fatalf("非法值条目应降级空信封并回落偏好键，实际 %q", item.Title)
	}
	if item := byKey["titled"]; item.PluginPublicId != "com.example.dto" || item.PluginName != "组装插件" || item.PluginID != pluginID {
		t.Fatalf("条目归属插件信息不符: %+v", item)
	}
}

// TestHandlerListByPluginAndDelete Handler 按插件查询与删除入口：响应信封成功态携带
// 条目清单；删除后条目消失
func TestHandlerListByPluginAndDelete(t *testing.T) {
	svc, mgmt, repo := newTestServices(t)
	ctx := context.Background()
	pluginID := plantPluginRow(t, repo, "com.example.hdlr", "入口插件")
	handler := NewHandler(mgmt)

	if err := svc.Set(ctx, pluginID, "video.form", &PreferenceValue{SchemaVersion: 1, Title: "视频形态"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	resp := handler.ListPreferencesByPlugin(ctx, "com.example.hdlr")
	if !resp.Success {
		t.Fatalf("按插件查询失败: %s", resp.Msg)
	}
	if len(resp.Data) != 1 || resp.Data[0].PrefKey != "video.form" {
		t.Fatalf("按插件查询条目不符: %+v", resp.Data)
	}

	del := handler.DeletePreference(ctx, resp.Data[0].ID)
	if !del.Success {
		t.Fatalf("删除失败: %s", del.Msg)
	}
	after := handler.ListPreferencesByPlugin(ctx, "com.example.hdlr")
	if !after.Success || len(after.Data) != 0 {
		t.Fatalf("删除后条目应消失: success=%t data=%+v", after.Success, after.Data)
	}
}
