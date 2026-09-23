package stickymemory

import (
	"context"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
)

func newTestService(t *testing.T) (*Service, *StickyMemoryRepository) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("内存 SQLite 不可用: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

// TestRememberUpsertIdempotent upsert 幂等：同 (domain, context_key) 二次 Remember 重写
// value、刷 update_time，行数/id/create_time 保持不变；List 返回字段齐全的条目
func TestRememberUpsertIdempotent(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	key := BuildContextKey("pixiv", []string{CandidateFullKey("plug-a", "ext-1"), CandidateFullKey("plug-b", "ext-2")})

	if err := svc.Remember(ctx, entity.DomainTaskURLDisambiguation, key, CandidateFullKey("plug-a", "ext-1")); err != nil {
		t.Fatalf("首记失败: %v", err)
	}
	first, err := repo.GetByKey(ctx, entity.DomainTaskURLDisambiguation, key)
	if err != nil {
		t.Fatalf("首记回查失败: %v", err)
	}

	// 时间戳毫秒精度：同毫秒内两次写入无法区分 update_time 变化，间隔 2ms 再重记
	time.Sleep(2 * time.Millisecond)
	if err := svc.Remember(ctx, entity.DomainTaskURLDisambiguation, key, CandidateFullKey("plug-b", "ext-2")); err != nil {
		t.Fatalf("重记失败: %v", err)
	}
	second, err := repo.GetByKey(ctx, entity.DomainTaskURLDisambiguation, key)
	if err != nil {
		t.Fatalf("重记回查失败: %v", err)
	}

	if second.Value != CandidateFullKey("plug-b", "ext-2") {
		t.Fatalf("重记应改写 value，实际 %q", second.Value)
	}
	if second.GetID() != first.GetID() {
		t.Fatalf("幂等重记不应换行: 前 id=%d 后 id=%d", first.GetID(), second.GetID())
	}
	if second.GetCreateTime() != first.GetCreateTime() {
		t.Fatalf("create_time 应保持首记时刻: 前 %d 后 %d", first.GetCreateTime(), second.GetCreateTime())
	}
	if second.GetUpdateTime() <= first.GetUpdateTime() {
		t.Fatalf("update_time 应刷新: 前 %d 后 %d", first.GetUpdateTime(), second.GetUpdateTime())
	}

	var total int64
	if err := repo.GORM().Model(entity.NewStickyMemory()).Count(&total).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("同键重记应保持单行，实际 %d 行", total)
	}

	// List 字段齐全（id/domain/context_key/value + 时间戳），供管理列表消费
	rows, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("List 应返回 1 行，实际 %d 行", len(rows))
	}
	row := rows[0]
	if row.Domain != entity.DomainTaskURLDisambiguation || row.ContextKey != key ||
		row.Value != CandidateFullKey("plug-b", "ext-2") || row.GetID() == 0 ||
		row.GetCreateTime() == 0 || row.GetUpdateTime() == 0 {
		t.Fatalf("List 条目字段不全: %+v", row)
	}
}

// TestRecallHitAndMiss 命中返回显选值；未命中 ok=false；同 context_key 不同 domain 互不串域
func TestRecallHitAndMiss(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	key := BuildContextKey("pixiv", []string{CandidateFullKey("plug-a", "ext-1"), CandidateFullKey("plug-b", "ext-2")})

	if v, ok := svc.Recall(ctx, entity.DomainTaskURLDisambiguation, key); ok {
		t.Fatalf("未写入前不应命中，实际得到 %q", v)
	}
	if err := svc.Remember(ctx, entity.DomainTaskURLDisambiguation, key, CandidateFullKey("plug-a", "ext-1")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if v, ok := svc.Recall(ctx, entity.DomainTaskURLDisambiguation, key); !ok || v != CandidateFullKey("plug-a", "ext-1") {
		t.Fatalf("写入后应命中并返回显选值: v=%q ok=%t", v, ok)
	}
	// 记忆域隔离：作者面同键不读任务面的记忆
	if v, ok := svc.Recall(ctx, entity.DomainSiteAuthorFetchDisambiguation, key); ok {
		t.Fatalf("不同 domain 不应串域命中，实际得到 %q", v)
	}
}

// TestForgetDeletes 删除后未命中；他行不受影响
func TestForgetDeletes(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	domain := entity.DomainSiteAuthorFetchDisambiguation
	key1 := BuildContextKey("pixiv", []string{CandidateFullKey("plug-a", "ext-1"), CandidateFullKey("plug-b", "ext-2")})
	key2 := BuildContextKey("bilibili", []string{CandidateFullKey("plug-a", "ext-1"), CandidateFullKey("plug-c", "ext-3")})
	if err := svc.Remember(ctx, domain, key1, CandidateFullKey("plug-a", "ext-1")); err != nil {
		t.Fatalf("写入 key1 失败: %v", err)
	}
	if err := svc.Remember(ctx, domain, key2, CandidateFullKey("plug-c", "ext-3")); err != nil {
		t.Fatalf("写入 key2 失败: %v", err)
	}

	row1, err := repo.GetByKey(ctx, domain, key1)
	if err != nil {
		t.Fatalf("回查 key1 失败: %v", err)
	}
	if err := svc.Forget(ctx, row1.GetID()); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	if v, ok := svc.Recall(ctx, domain, key1); ok {
		t.Fatalf("删除后不应命中，实际得到 %q", v)
	}
	if v, ok := svc.Recall(ctx, domain, key2); !ok || v != CandidateFullKey("plug-c", "ext-3") {
		t.Fatalf("他行不应受删除影响: v=%q ok=%t", v, ok)
	}
}
