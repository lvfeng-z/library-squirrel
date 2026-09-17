package tagNamespace

import (
	"context"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

func newTestRepo(t *testing.T) *TagNamespaceRepository {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	return NewRepository(db)
}

// TestSyncBuiltinsInsertsRegistryRows 全新库首次投影：内置集全量条目各建一行，
// value/label/origin 与权威值一致
func TestSyncBuiltinsInsertsRegistryRows(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	if err := svc.SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("投影失败: %v", err)
	}
	rows, err := repo.ListByValues(context.Background(), []string{"language", "character", "parody", "female", "male", "misc", "general"})
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if len(rows) != len(constant.BuiltinTagNamespaces) {
		t.Fatalf("投影行数 = %d，期望 %d", len(rows), len(constant.BuiltinTagNamespaces))
	}
	labelByValue := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.Origin != constant.ORIGIN_BUILTIN {
			t.Fatalf("内置投影行 origin 应为 builtin，value=%q 实际 %d", r.Value, r.Origin)
		}
		labelByValue[r.Value] = r.Label
	}
	if labelByValue["language"] != "语言" || labelByValue["character"] != "角色" || labelByValue["parody"] != "原作" ||
		labelByValue["female"] != "女性" || labelByValue["male"] != "男性" || labelByValue["misc"] != "杂项" || labelByValue["general"] != "通用" {
		t.Fatalf("内置 label 应取权威值，实际 %+v", labelByValue)
	}
}

// TestSyncBuiltinsInsertOnly insert-only：既有行一律不动——用户/插件写入的同值行（origin/label
// 已偏离权威值）与被改过 label 的行，重投影后保持原样，仅缺失键补建
func TestSyncBuiltinsInsertOnly(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	ctx := context.Background()
	if err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("首次投影失败: %v", err)
	}

	// 用户/插件语义行：改 builtin 行的 label 与 origin（模拟低优先级来源先写/用户改文案）
	if err := repo.GORM().Model(entity.NewTagNamespace()).Where("value = ?", "character").
		Updates(map[string]interface{}{"label": "角色（改）", "origin": constant.ORIGIN_USER}).Error; err != nil {
		t.Fatalf("改写既有行失败: %v", err)
	}
	// 插件来源自定义值（内置集外）
	pluginRow := entity.NewTagNamespace()
	pluginRow.Value = "artist-group"
	pluginRow.Origin = constant.ORIGIN_PLUGIN
	if err := repo.Create(ctx, pluginRow); err != nil {
		t.Fatalf("插插件来源行失败: %v", err)
	}
	// 删除一个内置行（模拟投影行被清），重投影应补建
	if err := repo.GORM().Where("value = ?", "misc").Delete(entity.NewTagNamespace()).Error; err != nil {
		t.Fatalf("删内置行失败: %v", err)
	}

	if err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("重投影失败: %v", err)
	}

	var changed entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "character").First(&changed).Error; err != nil {
		t.Fatalf("回查 character 失败: %v", err)
	}
	if changed.Label != "角色（改）" || changed.Origin != constant.ORIGIN_USER {
		t.Fatalf("重投影不应重置既有行，实际 label=%q origin=%d", changed.Label, changed.Origin)
	}
	var reback entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "misc").First(&reback).Error; err != nil {
		t.Fatalf("缺失内置键应补建: %v", err)
	}
	if reback.Label != "杂项" || reback.Origin != constant.ORIGIN_BUILTIN {
		t.Fatalf("补建行应取权威值，实际 label=%q origin=%d", reback.Label, reback.Origin)
	}
	var kept entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "artist-group").First(&kept).Error; err != nil {
		t.Fatalf("内置集外的行应保持: %v", err)
	}
	if kept.Origin != constant.ORIGIN_PLUGIN {
		t.Fatalf("内置集外的行 origin 应保持 plugin，实际 %d", kept.Origin)
	}
	var total int64
	if err := repo.GORM().Model(entity.NewTagNamespace()).Count(&total).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if total != int64(len(constant.BuiltinTagNamespaces))+1 {
		t.Fatalf("总数 = %d，期望 %d（内置全量 + 插件行）", total, len(constant.BuiltinTagNamespaces)+1)
	}
}

// TestEnsureUsedBatchFindOrCreate 三类登记语义：空串不产生清单行；未知值 find-or-create
// （label 空、origin=调用来源、last_use 落写入时刻）；既有行只刷 origin 升级与 last_use、
// label 永不改写；批内重复值折叠为单行操作
func TestEnsureUsedBatchFindOrCreate(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	ctx := context.Background()

	// 空串与归一化后为空串的值：不产生清单行
	if err := svc.EnsureUsedBatch(ctx, []string{"", "  "}, constant.ORIGIN_USER); err != nil {
		t.Fatalf("空串登记失败: %v", err)
	}
	var n int64
	if err := repo.GORM().Model(entity.NewTagNamespace()).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 0 {
		t.Fatalf("空串不应产生清单行，实际 %d 行", n)
	}

	// 插件来源 find-or-create：批内重复折叠为一行
	if err := svc.EnsureUsedBatch(ctx, []string{"ns-from-plugin", "ns-from-plugin"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("插件登记失败: %v", err)
	}
	var pluginRow entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "ns-from-plugin").First(&pluginRow).Error; err != nil {
		t.Fatalf("插件来源行应 find-or-create: %v", err)
	}
	if pluginRow.Origin != constant.ORIGIN_PLUGIN || pluginRow.Label != "" || pluginRow.LastUse == 0 {
		t.Fatalf("插件来源行形态不符: %+v", pluginRow)
	}

	// 用户来源再用同值：origin 升级 plugin→user、last_use 刷新、label 保持空
	//（间隔 2ms：时间戳毫秒精度，同毫秒内两次写入无法区分 last_use 变化）
	time.Sleep(2 * time.Millisecond)
	if err := svc.EnsureUsedBatch(ctx, []string{"NS-From-Plugin "}, constant.ORIGIN_USER); err != nil {
		t.Fatalf("用户登记失败: %v", err)
	}
	var upgraded entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "ns-from-plugin").First(&upgraded).Error; err != nil {
		t.Fatalf("回查升级行失败: %v", err)
	}
	if upgraded.Origin != constant.ORIGIN_USER {
		t.Fatalf("同值后由高优先级来源使用应升级 origin=user，实际 %d", upgraded.Origin)
	}
	if upgraded.LastUse <= pluginRow.LastUse {
		t.Fatalf("last_use 应刷新，前 %d 后 %d", pluginRow.LastUse, upgraded.LastUse)
	}

	// 归一化变体归一为同一清单行（不分裂）
	if err := svc.EnsureUsedBatch(ctx, []string{"Ns-From-Plugin", "ns-from-plugin"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("变体登记失败: %v", err)
	}
	if err := repo.GORM().Model(entity.NewTagNamespace()).Where("value = ?", "ns-from-plugin").Count(&n).Error; err != nil {
		t.Fatalf("变体回查失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("归一化变体应归一为同一清单行，实际 %d 行", n)
	}
}

// TestEnsureUsedBatchBuiltinRowNotRewritten 内置行不被降级：插件/用户来源使用内置值时
// origin/label 保持内置权威值，仅 last_use 刷新（D-19）
func TestEnsureUsedBatchBuiltinRowNotRewritten(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	ctx := context.Background()
	if err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("投影失败: %v", err)
	}

	if err := svc.EnsureUsedBatch(ctx, []string{"character"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("插件使用内置值失败: %v", err)
	}
	if err := svc.EnsureUsedBatch(ctx, []string{"character"}, constant.ORIGIN_USER); err != nil {
		t.Fatalf("用户使用内置值失败: %v", err)
	}

	var row entity.TagNamespace
	if err := repo.GORM().Where("value = ?", "character").First(&row).Error; err != nil {
		t.Fatalf("回查内置行失败: %v", err)
	}
	if row.Origin != constant.ORIGIN_BUILTIN {
		t.Fatalf("内置行 origin 不应被降级，实际 %d", row.Origin)
	}
	if row.Label != "角色" {
		t.Fatalf("内置行 label 应保持权威值，实际 %q", row.Label)
	}
	if row.LastUse == 0 {
		t.Fatal("内置行 last_use 应照常刷新")
	}
}
