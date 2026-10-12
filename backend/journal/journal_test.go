package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/util"

	"go.uber.org/zap"
)

// 服务路径记日志，测试进程无 logger.Init——注入 Nop 防全局 logger.Log 为 nil
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

func newTestService(t *testing.T) (*Service, *JournalRepository) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

func countRows(t *testing.T, repo *JournalRepository) int64 {
	t.Helper()
	var total int64
	if err := repo.GORM().Model(entity.NewJournalEntry()).Count(&total).Error; err != nil {
		t.Fatalf("账行计数失败: %v", err)
	}
	return total
}

// TestAppendThenListSincePaging Append→ListSince 全链：空表形态、游标分批翻页（limit 边界、
// 到底判断、游标推进与驻留）、行字段原样往返、id 升序、迁移建表核验
func TestAppendThenListSincePaging(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	// 迁移注册核验：journal_entry 表随 OpenTestDB 全量迁移建出
	if !repo.GORM().Migrator().HasTable(&entity.JournalEntry{}) {
		t.Fatalf("迁移后应存在 journal_entry 表")
	}

	// 空表：cursor=0 引导读不判过期；非 0 游标指向不可见位置判过期
	empty, err := svc.ListSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("空表读取失败: %v", err)
	}
	if len(empty.Entries) != 0 || empty.NextCursor != 0 || empty.HasMore || empty.CursorExpired {
		t.Fatalf("空表 cursor=0 形态不符: %+v", empty)
	}
	emptyNonZero, err := svc.ListSince(ctx, 3, 10)
	if err != nil {
		t.Fatalf("空表读取失败: %v", err)
	}
	if !emptyNonZero.CursorExpired {
		t.Fatalf("空表非 0 游标应判过期")
	}

	const total = 7
	for i := 1; i <= total; i++ {
		if err := svc.Append(ctx, entity.JournalDomainTask, "taskTerminal", int64(i), fmt.Sprintf(`{"state":"done","seq":%d}`, i)); err != nil {
			t.Fatalf("追加第 %d 行失败: %v", i, err)
		}
	}

	// limit=总量：一次读尽，HasMore=false
	full, err := svc.ListSince(ctx, 0, total)
	if err != nil {
		t.Fatalf("全量读取失败: %v", err)
	}
	if len(full.Entries) != total || full.HasMore || full.NextCursor != total || full.CursorExpired {
		t.Fatalf("limit=总量读取形态不符: len=%d hasMore=%t next=%d expired=%t",
			len(full.Entries), full.HasMore, full.NextCursor, full.CursorExpired)
	}
	// 行字段原样往返 + id 严格升序
	for i, e := range full.Entries {
		if e.GetID() != int64(i+1) {
			t.Fatalf("行序非 id 升序: 第 %d 行 id=%d", i, e.GetID())
		}
		if e.Domain != entity.JournalDomainTask || e.EventType != "taskTerminal" || e.SubjectID != int64(i+1) {
			t.Fatalf("行字段往返不符: %+v", e)
		}
		if want := fmt.Sprintf(`{"state":"done","seq":%d}`, i+1); e.Payload != want {
			t.Fatalf("载荷往返不符: got %q want %q", e.Payload, want)
		}
		if e.GetCreateTime() == 0 {
			t.Fatalf("create_time 未回填: id=%d", e.GetID())
		}
	}

	// limit=总量+1（探测行不存在）：同一次读尽
	over, err := svc.ListSince(ctx, 0, total+1)
	if err != nil {
		t.Fatalf("超量读取失败: %v", err)
	}
	if len(over.Entries) != total || over.HasMore {
		t.Fatalf("limit=总量+1 读取形态不符: len=%d hasMore=%t", len(over.Entries), over.HasMore)
	}

	// limit=0：取缺省批量（100 > 7，一次读尽）
	def, err := svc.ListSince(ctx, 0, 0)
	if err != nil {
		t.Fatalf("缺省批量读取失败: %v", err)
	}
	if len(def.Entries) != total || def.HasMore {
		t.Fatalf("缺省批量读取形态不符: len=%d hasMore=%t", len(def.Entries), def.HasMore)
	}

	// 三行一批翻页：NextCursor 逐批推进，HasMore 恰在余量耗尽时翻转
	type step struct {
		wantIDs   []int64
		wantMore  bool
		wantNext  int64
		wantEmpty bool
	}
	steps := []step{
		{wantIDs: []int64{1, 2, 3}, wantMore: true, wantNext: 3},
		{wantIDs: []int64{4, 5, 6}, wantMore: true, wantNext: 6},
		{wantIDs: []int64{7}, wantMore: false, wantNext: 7},
		{wantEmpty: true, wantMore: false, wantNext: 7},
	}
	cursor := int64(0)
	for i, st := range steps {
		res, err := svc.ListSince(ctx, cursor, 3)
		if err != nil {
			t.Fatalf("第 %d 批读取失败: %v", i+1, err)
		}
		if res.CursorExpired {
			t.Fatalf("第 %d 批不应判过期", i+1)
		}
		if st.wantEmpty {
			if len(res.Entries) != 0 || res.NextCursor != st.wantNext {
				t.Fatalf("追平批次形态不符: len=%d next=%d", len(res.Entries), res.NextCursor)
			}
		} else {
			if len(res.Entries) != len(st.wantIDs) || res.HasMore != st.wantMore || res.NextCursor != st.wantNext {
				t.Fatalf("第 %d 批形态不符: len=%d hasMore=%t next=%d", i+1, len(res.Entries), res.HasMore, res.NextCursor)
			}
			for j, e := range res.Entries {
				if e.GetID() != st.wantIDs[j] {
					t.Fatalf("第 %d 批行集不符: 位置 %d got id=%d want id=%d", i+1, j, e.GetID(), st.wantIDs[j])
				}
			}
		}
		cursor = res.NextCursor
	}

	// 游标越过末尾：空集、游标驻留、不判过期
	past, err := svc.ListSince(ctx, 999, 3)
	if err != nil {
		t.Fatalf("越尾读取失败: %v", err)
	}
	if len(past.Entries) != 0 || past.NextCursor != 999 || past.HasMore || past.CursorExpired {
		t.Fatalf("越尾读取形态不符: %+v", past)
	}
}

// TestAppendValidatesInput 写入校验：空域/空事件类型/非正主体引用各自报显式错误，且不落行
func TestAppendValidatesInput(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		domain    string
		eventType string
		subjectID int64
		wantErr   error
	}{
		{"空域", "", "taskTerminal", 1, ErrDomainEmpty},
		{"空事件类型", entity.JournalDomainTask, "", 1, ErrEventTypeEmpty},
		{"零主体", entity.JournalDomainTask, "taskTerminal", 0, ErrSubjectIDInvalid},
		{"负主体", entity.JournalDomainTask, "taskTerminal", -5, ErrSubjectIDInvalid},
	}
	for _, c := range cases {
		err := svc.Append(ctx, c.domain, c.eventType, c.subjectID, "")
		if !errors.Is(err, c.wantErr) {
			t.Fatalf("%s: 错误不符: got %v want %v", c.name, err, c.wantErr)
		}
	}
	if got := countRows(t, repo); got != 0 {
		t.Fatalf("校验失败不应落行，实际 %d 行", got)
	}
}

// ageRows 把指定 id 段的账行 create_time 改到指定时刻（直改库模拟历史行老化）
func ageRows(t *testing.T, repo *JournalRepository, maxID int64, agedAt int64) {
	t.Helper()
	if err := repo.GORM().Exec("UPDATE journal_entry SET create_time = ? WHERE id <= ?", agedAt, maxID).Error; err != nil {
		t.Fatalf("老化账行失败: %v", err)
	}
}

// TestRotateTTLTrimsOldestAndExpiresCursor TTL 轨：超龄最旧行被裁；裁后旧游标读取返回
// 过期标记而非空集静默，cursor=0 引导与最旧存活行游标不判过期
func TestRotateTTLTrimsOldestAndExpiresCursor(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		if err := svc.Append(ctx, entity.JournalDomainIngest, "ingestComplete", int64(i), `{"title":"t"}`); err != nil {
			t.Fatalf("追加第 %d 行失败: %v", i, err)
		}
	}
	aged := util.GetCurrentTimestamp() - RotationTTL.Milliseconds() - 1000
	ageRows(t, repo, 2, aged)

	deleted, err := svc.Rotate(ctx)
	if err != nil {
		t.Fatalf("轮转失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("TTL 轨应裁 2 行，实际 %d", deleted)
	}
	if got := countRows(t, repo); got != 3 {
		t.Fatalf("TTL 轨后应余 3 行，实际 %d", got)
	}

	// cursor=0 引导：从最旧存活行起读，不判过期
	fromHead, err := svc.ListSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("引导读取失败: %v", err)
	}
	if fromHead.CursorExpired || len(fromHead.Entries) != 3 || fromHead.Entries[0].GetID() != 3 {
		t.Fatalf("裁剪后引导读取形态不符: expired=%t len=%d 首行 id=%d",
			fromHead.CursorExpired, len(fromHead.Entries), fromHead.Entries[0].GetID())
	}
	// 游标早于最旧存活行（指向行已被裁）：显式过期标记
	stale, err := svc.ListSince(ctx, 2, 10)
	if err != nil {
		t.Fatalf("过期游标读取失败: %v", err)
	}
	if !stale.CursorExpired || len(stale.Entries) != 3 {
		t.Fatalf("过期游标应携带标记并返回存活行集: expired=%t len=%d", stale.CursorExpired, len(stale.Entries))
	}
	// 游标=最旧存活行：位置可见，不判过期
	current, err := svc.ListSince(ctx, 3, 10)
	if err != nil {
		t.Fatalf("存活游标读取失败: %v", err)
	}
	if current.CursorExpired || len(current.Entries) != 2 {
		t.Fatalf("最旧存活行游标不应判过期: expired=%t len=%d", current.CursorExpired, len(current.Entries))
	}
}

// TestRotateCapacityKeepsNewest 容量轨（真实常量）：超容量时裁最旧保留最新 RotationMaxRows 行；
// 裁后旧游标过期、最旧存活行游标不过期
func TestRotateCapacityKeepsNewest(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	// 递归 CTE 单语句播种超容量行数（create_time 取当前时刻，仅触发容量轨）
	now := util.GetCurrentTimestamp()
	if err := repo.GORM().Exec(`
		WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM cnt WHERE x < ?)
		INSERT INTO journal_entry (domain, event_type, subject_id, payload, create_time, update_time)
		SELECT 'task', 'taskTerminal', x, '', ?, ? FROM cnt
	`, RotationMaxRows+1, now, now).Error; err != nil {
		t.Fatalf("播种账行失败: %v", err)
	}

	deleted, err := svc.Rotate(ctx)
	if err != nil {
		t.Fatalf("轮转失败: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("容量轨应裁 %d 行，实际 %d", RotationMaxRows+1-RotationMaxRows, deleted)
	}
	if got := countRows(t, repo); got != RotationMaxRows {
		t.Fatalf("容量轨后应余 %d 行，实际 %d", RotationMaxRows, got)
	}

	// 被裁首行（id=1）的游标判过期；最旧存活行（id=2）游标不判过期
	if res, _ := svc.ListSince(ctx, 1, 2); !res.CursorExpired {
		t.Fatalf("被裁行游标应判过期")
	}
	if res, _ := svc.ListSince(ctx, 2, 2); res.CursorExpired || res.Entries[0].GetID() != 3 {
		t.Fatalf("最旧存活行游标不应判过期且续读 id=3: expired=%t 首行 id=%d", res.CursorExpired, res.Entries[0].GetID())
	}
}

// TestRotateOldestCombinedConditions 仓储轮转双条件 OR 语义（小参数）：TTL 命中超龄行、
// 容量轨保留最新 keepRows 行，一次调用两轨叠加；不命中时零裁剪（幂等）、空表安全
func TestRotateOldestCombinedConditions(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	// 空表安全
	if deleted, err := repo.RotateOldest(ctx, 0, 1); err != nil || deleted != 0 {
		t.Fatalf("空表轮转应零裁剪: deleted=%d err=%v", deleted, err)
	}

	for i := 1; i <= 4; i++ {
		if err := svc.Append(ctx, entity.JournalDomainTask, "taskTerminal", int64(i), ""); err != nil {
			t.Fatalf("追加第 %d 行失败: %v", i, err)
		}
	}
	// id 1-2 超龄（2 秒前），id 3-4 新鲜；cutoff=1 秒前，keepRows=1 只保留最新行
	now := util.GetCurrentTimestamp()
	ageRows(t, repo, 2, now-2000)

	deleted, err := repo.RotateOldest(ctx, now-1000, 1)
	if err != nil {
		t.Fatalf("轮转失败: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("双条件叠加应裁 3 行（超龄 2 + 容量 1），实际 %d", deleted)
	}
	var rows []*entity.JournalEntry
	if err := repo.GORM().Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if len(rows) != 1 || rows[0].GetID() != 4 {
		t.Fatalf("应仅余最新行 id=4，实际 %+v", rows)
	}

	// 幂等：再次调用零命中
	if deleted, err := repo.RotateOldest(ctx, now-1000, 1); err != nil || deleted != 0 {
		t.Fatalf("二次轮转应零裁剪: deleted=%d err=%v", deleted, err)
	}
}

// TestConcurrentAppendRace 并发写安全：多 goroutine 并发 Append（每次写顺带轮转）全部
// 成功、行数守恒、id 严格升序无重复（自增游标保序）
func TestConcurrentAppendRace(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	const workers, perWorker = 8, 25

	var wg sync.WaitGroup
	errCh := make(chan error, workers*perWorker)
	for w := 1; w <= workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := svc.Append(ctx, entity.JournalDomainIngest, "ingestComplete", int64(w*1000+i), `{"title":"t"}`); err != nil {
					errCh <- fmt.Errorf("worker %d: %w", w, err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发写账失败: %v", err)
	}

	if got := countRows(t, repo); got != workers*perWorker {
		t.Fatalf("并发写后应 %d 行，实际 %d", workers*perWorker, got)
	}
	res, err := svc.ListSince(ctx, 0, workers*perWorker)
	if err != nil {
		t.Fatalf("并发后全量读取失败: %v", err)
	}
	if len(res.Entries) != workers*perWorker || res.HasMore {
		t.Fatalf("并发后全量读取形态不符: len=%d hasMore=%t", len(res.Entries), res.HasMore)
	}
	prev := int64(0)
	seen := make(map[int64]struct{}, len(res.Entries))
	for _, e := range res.Entries {
		id := e.GetID()
		if id <= prev {
			t.Fatalf("id 应严格升序: prev=%d id=%d", prev, id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id 重复: %d", id)
		}
		seen[id] = struct{}{}
		prev = id
	}
}
