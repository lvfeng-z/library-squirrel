package pluginpreference

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

// newTestServices 测试装配：内存库（完整迁移 + 外键强制）+ 共享同一仓储的运行时/管理两面服务
func newTestServices(t *testing.T) (*Service, *ManagementService, *PluginPreferenceRepository) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), NewManagementService(repo), repo
}

// plantPluginRow 预置插件行（偏好行 plugin_id 外键的父行）
func plantPluginRow(t *testing.T, repo *PluginPreferenceRepository, publicId, name string) int64 {
	t.Helper()
	row := entity.NewPlugin()
	row.PublicID = sql.NullString{String: publicId, Valid: true}
	row.Name = sql.NullString{String: name, Valid: true}
	if err := repo.GORM().Create(row).Error; err != nil {
		t.Fatalf("预置插件行失败: %v", err)
	}
	return row.GetID()
}

// TestGetMissReturnsNilNil 无记录读取返回 (nil, nil) 不报错——无记录是合法状态
//（用户已删除或从未写入），调用方据此回落重新发起问答
func TestGetMissReturnsNilNil(t *testing.T) {
	svc, _, repo := newTestServices(t)
	pluginID := plantPluginRow(t, repo, "com.example.miss", "未写插件")

	v, err := svc.Get(context.Background(), pluginID, "any.key")
	if err != nil {
		t.Fatalf("无记录读取不应报错: %v", err)
	}
	if v != nil {
		t.Fatalf("无记录应返回 nil，实际 %+v", v)
	}
}

// TestSetNewAndWholeValueOverwrite 新增写入回读完整信封；同键再写为整值覆写——旧值
// 字段全部消失（非合并），行数保持 1、id 与 create_time 不变、update_time 刷新
func TestSetNewAndWholeValueOverwrite(t *testing.T) {
	svc, _, repo := newTestServices(t)
	ctx := context.Background()
	pluginID := plantPluginRow(t, repo, "com.example.overwrite", "覆写插件")

	first := &PreferenceValue{
		SchemaVersion: 1,
		Title:         "图文形态",
		Description:   "多图",
		Data:          map[string]any{"mode": "multi"},
	}
	if err := svc.Set(ctx, pluginID, "illust.form", first); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	got, err := svc.Get(ctx, pluginID, "illust.form")
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Title != "图文形态" || got.Description != "多图" || got.SchemaVersion != 1 {
		t.Fatalf("回读信封展示字段不符: %+v", got)
	}
	data, _ := got.Data.(map[string]any)
	if data["mode"] != "multi" {
		t.Fatalf("回读负载不符: %+v", got.Data)
	}

	before, err := repo.GetByKey(ctx, pluginID, "illust.form")
	if err != nil {
		t.Fatalf("首写行查询失败: %v", err)
	}

	// 时间戳毫秒精度：同毫秒内两次写入无法区分 update_time 变化，间隔 2ms 再覆写
	time.Sleep(2 * time.Millisecond)
	second := &PreferenceValue{
		SchemaVersion: 2,
		Title:         "图文形态·改",
		Data:          map[string]any{"mode": "article"},
	}
	if err := svc.Set(ctx, pluginID, "illust.form", second); err != nil {
		t.Fatalf("覆写失败: %v", err)
	}

	after, err := repo.GetByKey(ctx, pluginID, "illust.form")
	if err != nil {
		t.Fatalf("覆写行查询失败: %v", err)
	}
	if after.GetID() != before.GetID() {
		t.Fatalf("同键覆写不应换行: 前 id=%d 后 id=%d", before.GetID(), after.GetID())
	}
	if after.GetCreateTime() != before.GetCreateTime() {
		t.Fatalf("create_time 应保持首记时刻: 前 %d 后 %d", before.GetCreateTime(), after.GetCreateTime())
	}
	if after.GetUpdateTime() <= before.GetUpdateTime() {
		t.Fatalf("update_time 应刷新: 前 %d 后 %d", before.GetUpdateTime(), after.GetUpdateTime())
	}

	var total int64
	if err := repo.GORM().Model(entity.NewPluginPreference()).Count(&total).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("同键覆写应保持单行，实际 %d 行", total)
	}

	got2, err := svc.Get(ctx, pluginID, "illust.form")
	if err != nil {
		t.Fatalf("覆写后回读失败: %v", err)
	}
	if got2.Title != "图文形态·改" || got2.SchemaVersion != 2 {
		t.Fatalf("覆写后应读到新信封: %+v", got2)
	}
	data2, _ := got2.Data.(map[string]any)
	if data2["mode"] != "article" {
		t.Fatalf("覆写后负载应为新值: %+v", got2.Data)
	}
	// 整值覆写：旧信封独有字段（Description）不得残留
	if got2.Description != "" {
		t.Fatalf("整值覆写后旧信封字段应消失，Description=%q", got2.Description)
	}
}

// TestListKeysScopedToPlugin 列键限本插件域：他插件同键不可见；同键跨插件共存可达
//（复合唯一键含 plugin_id，不同插件同键各自成行）
func TestListKeysScopedToPlugin(t *testing.T) {
	svc, _, repo := newTestServices(t)
	ctx := context.Background()
	pluginA := plantPluginRow(t, repo, "com.example.a", "插件甲")
	pluginB := plantPluginRow(t, repo, "com.example.b", "插件乙")

	for _, key := range []string{"illust.form", "video.form"} {
		if err := svc.Set(ctx, pluginA, key, &PreferenceValue{SchemaVersion: 1, Title: key + "@甲"}); err != nil {
			t.Fatalf("插件甲写入 %s 失败: %v", key, err)
		}
	}
	if err := svc.Set(ctx, pluginB, "illust.form", &PreferenceValue{SchemaVersion: 1, Title: "同键@乙"}); err != nil {
		t.Fatalf("插件乙同键写入失败: %v", err)
	}

	keysA, err := svc.ListKeys(ctx, pluginA)
	if err != nil {
		t.Fatalf("插件甲列键失败: %v", err)
	}
	if len(keysA) != 2 || keysA[0] != "illust.form" || keysA[1] != "video.form" {
		t.Fatalf("插件甲键清单 = %v，期望 [illust.form video.form]（pref_key 升序）", keysA)
	}
	keysB, err := svc.ListKeys(ctx, pluginB)
	if err != nil {
		t.Fatalf("插件乙列键失败: %v", err)
	}
	if len(keysB) != 1 || keysB[0] != "illust.form" {
		t.Fatalf("插件乙键清单 = %v，期望仅自身域 [illust.form]", keysB)
	}

	// 跨插件同键共存：两插件各自读到各自的值，互不串域
	vA, err := svc.Get(ctx, pluginA, "illust.form")
	if err != nil || vA == nil {
		t.Fatalf("插件甲同键读取失败: v=%+v err=%v", vA, err)
	}
	vB, err := svc.Get(ctx, pluginB, "illust.form")
	if err != nil || vB == nil {
		t.Fatalf("插件乙同键读取失败: v=%+v err=%v", vB, err)
	}
	if vA.Title == vB.Title {
		t.Fatalf("同键跨插件应各自成行，实际读到同值 %q", vA.Title)
	}
}

// TestCompositeUniqueConstraintAtDBLevel 复合唯一约束库级锚定：同插件同键直接建二行被
// 唯一索引拒绝（服务面 upsert 之外的第二写入路径同样受约束）
func TestCompositeUniqueConstraintAtDBLevel(t *testing.T) {
	_, _, repo := newTestServices(t)
	pluginID := plantPluginRow(t, repo, "com.example.unique", "唯一约束插件")

	row1 := entity.NewPluginPreference()
	row1.PluginID = pluginID
	row1.PrefKey = "same.key"
	row1.Value = `{"schemaVersion":1}`
	if err := repo.GORM().Create(row1).Error; err != nil {
		t.Fatalf("首行建行失败: %v", err)
	}
	row2 := entity.NewPluginPreference()
	row2.PluginID = pluginID
	row2.PrefKey = "same.key"
	row2.Value = `{"schemaVersion":2}`
	if err := repo.GORM().Create(row2).Error; err == nil {
		t.Fatal("同插件同键第二行应被复合唯一索引拒绝，实际写入成功")
	}
}

// TestSetValidatesInput 写入入参校验：插件 id 非正数、空键（含纯空白）、空信封各自报
// 对应错误
func TestSetValidatesInput(t *testing.T) {
	svc, _, _ := newTestServices(t)
	ctx := context.Background()

	if err := svc.Set(ctx, 0, "k", &PreferenceValue{}); !errors.Is(err, ErrInvalidPluginID) {
		t.Fatalf("插件 id 无效应报 ErrInvalidPluginID，实际 %v", err)
	}
	if err := svc.Set(ctx, 1, "  ", &PreferenceValue{}); !errors.Is(err, ErrEmptyPrefKey) {
		t.Fatalf("空键应报 ErrEmptyPrefKey，实际 %v", err)
	}
	if err := svc.Set(ctx, 1, "k", nil); !errors.Is(err, ErrNilPreferenceValue) {
		t.Fatalf("空信封应报 ErrNilPreferenceValue，实际 %v", err)
	}
}

// TestSetRejectsUnknownPlugin 外键防线：plugin_id 指向不存在的插件行时写入被库级外键
// 拒绝（偏好行恒有归属插件）
func TestSetRejectsUnknownPlugin(t *testing.T) {
	svc, _, _ := newTestServices(t)
	err := svc.Set(context.Background(), 999999, "orphan.key", &PreferenceValue{SchemaVersion: 1})
	if err == nil {
		t.Fatal("悬空 plugin_id 写入应被外键拒绝，实际成功")
	}
}

// TestRuntimeServiceExposesNoDelete 固化「删除仅管理面」：插件运行时服务结构不得暴露
// 删除类方法——「忘掉」是用户权利，删除能力只住 ManagementService（经管理面 Handler
// 触达）。后续给 Service 增删类方法的改动会被本断言拦下
func TestRuntimeServiceExposesNoDelete(t *testing.T) {
	typ := reflect.TypeOf(&Service{})
	if typ.NumMethod() == 0 {
		t.Fatal("运行时服务应暴露读写列方法，实际无方法")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		lower := strings.ToLower(name)
		for _, banned := range []string{"delete", "forget", "remove", "clear", "drop", "erase"} {
			if strings.Contains(lower, banned) {
				t.Fatalf("运行时服务面不得暴露删除类方法 %s（删除仅经 ManagementService）", name)
			}
		}
	}
}
