package authorRole

import (
	"context"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

func newTestRepo(t *testing.T) *AuthorRoleRepository {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	return NewRepository(db)
}

// TestSyncBuiltinsEmptySetNoop 内置集为空：投影无行可建（清单靠用户使用与插件声明自然生长），
// 既有行不被触碰
func TestSyncBuiltinsEmptySetNoop(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	ctx := context.Background()

	userRow := entity.NewAuthorRole()
	userRow.Value = "原画"
	userRow.Origin = constant.ORIGIN_USER
	if err := repo.Create(ctx, userRow); err != nil {
		t.Fatalf("插用户行失败: %v", err)
	}

	if err := svc.SyncBuiltins(ctx); err != nil {
		t.Fatalf("空内置集投影失败: %v", err)
	}

	var rows []*entity.AuthorRole
	if err := repo.GORM().Find(&rows).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != "原画" || rows[0].Origin != constant.ORIGIN_USER {
		t.Fatalf("空内置集投影应不建行不动既有行，实际 %+v", rows)
	}
}

// TestEnsureUsedBatchFindOrCreateAndOriginMerge role 清单登记：空串不产生清单行；find-or-create
// （origin=调用来源）；同值低→高来源升级（plugin→user）、高→低不降级；last_use 刷新、label 不改写
func TestEnsureUsedBatchFindOrCreateAndOriginMerge(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.EnsureUsedBatch(ctx, []string{"", "  "}, constant.ORIGIN_USER); err != nil {
		t.Fatalf("空串登记失败: %v", err)
	}
	var n int64
	if err := repo.GORM().Model(entity.NewAuthorRole()).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 0 {
		t.Fatalf("空串不应产生清单行，实际 %d 行", n)
	}

	// 插件来源 find-or-create（归一化变体归一为同一行）
	if err := svc.EnsureUsedBatch(ctx, []string{"作画监督 ", "作画监督"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("插件登记失败: %v", err)
	}
	var pluginRow entity.AuthorRole
	if err := repo.GORM().Where("value = ?", "作画监督").First(&pluginRow).Error; err != nil {
		t.Fatalf("插件来源行应 find-or-create: %v", err)
	}
	if pluginRow.Origin != constant.ORIGIN_PLUGIN || pluginRow.Label != "" || pluginRow.LastUse == 0 {
		t.Fatalf("插件来源行形态不符: %+v", pluginRow)
	}

	// 用户来源再用同值：升级不降级；时间戳毫秒精度，间隔 2ms 保证 last_use 可区分
	time.Sleep(2 * time.Millisecond)
	if err := svc.EnsureUsedBatch(ctx, []string{"作画监督"}, constant.ORIGIN_USER); err != nil {
		t.Fatalf("用户登记失败: %v", err)
	}
	if err := svc.EnsureUsedBatch(ctx, []string{"作画监督"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("插件再登记失败: %v", err)
	}
	var merged entity.AuthorRole
	if err := repo.GORM().Where("value = ?", "作画监督").First(&merged).Error; err != nil {
		t.Fatalf("回查合并行失败: %v", err)
	}
	if merged.Origin != constant.ORIGIN_USER {
		t.Fatalf("origin 应保持升级后的 user（高→低不降级），实际 %d", merged.Origin)
	}
	if merged.LastUse <= pluginRow.LastUse {
		t.Fatalf("last_use 应刷新，前 %d 后 %d", pluginRow.LastUse, merged.LastUse)
	}

	// 预置 builtin 行：任何来源使用不改写 origin/label，仅刷 last_use（D-19 同族语义）
	builtin := entity.NewAuthorRole()
	builtin.Value = "官方钦定"
	builtin.Label = "权威名"
	builtin.Origin = constant.ORIGIN_BUILTIN
	if err := repo.Create(ctx, builtin); err != nil {
		t.Fatalf("预置 builtin 行失败: %v", err)
	}
	if err := svc.EnsureUsedBatch(ctx, []string{"官方钦定"}, constant.ORIGIN_PLUGIN); err != nil {
		t.Fatalf("插件使用内置行值失败: %v", err)
	}
	var builtinAfter entity.AuthorRole
	if err := repo.GORM().Where("value = ?", "官方钦定").First(&builtinAfter).Error; err != nil {
		t.Fatalf("回查 builtin 行失败: %v", err)
	}
	if builtinAfter.Origin != constant.ORIGIN_BUILTIN || builtinAfter.Label != "权威名" {
		t.Fatalf("builtin 行 origin/label 不应被改写: %+v", builtinAfter)
	}
	if builtinAfter.LastUse == 0 {
		t.Fatal("builtin 行 last_use 应照常刷新")
	}
}
