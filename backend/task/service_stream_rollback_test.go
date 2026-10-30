package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/site"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm"
)

// 本文件为未收批统一回滚（决策4=甲）与落库批量（乙4）的真库锚定：
//   - 桩流「父块（含内联子行）+ 无 last 批块 + 关流」后 task/work_task 零残留——回滚复用
//     批量删除链（清引用 → 领域行 → 核心行，事务感知），真库断言覆盖外键强制下的删除序。
//   - ctx 取消（=通道断，决策4 明列形态）中断消费：回滚仍执行且零残留（消费循环不裸退、
//     回滚事务以剥取消信号的 persistCtx 落地——模拟插件死亡竞态）。
//   - 收批（last=true 已到）为原子提交点：其后流结束不回滚（同形状对照组）。
//   - 落库批量 2000 行/事务：2000 行单条多值 INSERT 在外键强制真库上成功本身即 SQLite 参数
//     上限内的证明（work_task 14 列 × 2000 = 28000 绑定参数 < 32766）。

// newRealDBStreamService 在外键强制测试库（migration.OpenTestDB）上经生产构造函数组装
// Service（真实仓储 + 真事务执行器 + 真实站点服务）并播种站点。
func newRealDBStreamService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	seedSite := entity.NewSite()
	seedSite.SiteKey = testSiteKey
	seedSite.SiteName = sql.NullString{String: testSiteName, Valid: true}
	if err := db.Create(seedSite).Error; err != nil {
		t.Fatalf("建站点种子失败: %v", err)
	}
	siteSvc := site.NewService(site.NewRepository(db))
	wtStore := newTestWorkTaskStore(db)
	svc := NewService(NewRepository(db, wtStore, wtStore), &testTransactor{db: db}, nil, nil, siteSvc, nil, nil)
	return svc, db
}

// assertTableCount 断言真库表行数。
func assertTableCount(t *testing.T, db *gorm.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&got).Error; err != nil {
		t.Fatalf("统计 %s 行数失败: %v", table, err)
	}
	if got != want {
		t.Fatalf("期望 %s 表 %d 行，得到 %d", table, want, got)
	}
}

// TestStreamMissingLastRollsBackZeroResidue 桩流「父块 + 无 last 批块 + 关流」：未收批统一
// 回滚——父任务与全部已落库子行删除，task/work_task 零残留（决策4=甲，阶段1 退出标准①）；
// 缺末批错误项随回滚折减数（=已透出子行数）透出。
func TestStreamMissingLastRollsBackZeroResidue(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", false, childOf("batch-1", "c-2"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), createStreamFlushBatchSize)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	var retracted int
	var errs []error
	for item := range out {
		if item.Error != nil {
			errs = append(errs, item.Error)
			retracted += item.RetractedTaskCount
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchMissingLastBatch) {
		t.Fatalf("期望恰 1 个缺末批错误项，得到 %v", errs)
	}
	if retracted != 2 {
		t.Fatalf("期望回滚折减数 = 已透出子行数 2（内联 1 + 批次 1），得到 %d", retracted)
	}
	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}

// TestStreamCtxCancelMidBatchStillRollsBack ctx 取消（=通道断，决策4 明列形态）中断消费：
// 已开批未收批父任务的统一回滚仍执行且 task/work_task 零残留——消费循环察觉取消后不裸退、
// 跳出走流结束回滚分支，回滚事务以剥取消信号的 persistCtx 落地（源 ctx 已取消不致事务失败），
// 模拟插件死亡竞态。先等批次子行透出（父行与 2 子已随 batchSize=1 落库）再取消，保证取消时点
// 落在已处理之后；唤醒块为空 items 开批块（被处理与否皆零子行），计数断言与调度交错无关。
func TestStreamCtxCancelMidBatchStillRollsBack(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan *sdkdto.CreateStreamItem, 4)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", false, childOf("batch-1", "c-2"))

	out, err := svc.handleCreateTaskStream(ctx, in, testListener(), 1)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}

	var tasks, retracted int
	var errs []error
	batchChildSeen := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		seen := 0
		for item := range out {
			switch {
			case item.Task != nil:
				tasks++
				seen++
				if seen == 2 {
					close(batchChildSeen) // 内联 1 + 批次 1 都已透出（emit 为处理尾步）
				}
			case item.Error != nil:
				errs = append(errs, item.Error)
				retracted += item.RetractedTaskCount
			}
		}
	}()

	select {
	case <-batchChildSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("等待父块与批次块处理完成超时")
	}
	cancel()                         // 通道断：此后消费循环察觉 ctx.Done 即跳出
	in <- batchItem("work-X", false) // 唤醒块（空开批块，零子行）
	close(in)

	select {
	case <-drained:
	case <-time.After(3 * time.Second):
		t.Fatal("取消后结果 channel 未关闭（回滚收尾未执行）")
	}

	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchMissingLastBatch) {
		t.Fatalf("期望恰 1 个缺末批错误项，得到 %v", errs)
	}
	if tasks != 2 || retracted != 2 {
		t.Fatalf("期望 2 子透出且全数折减（唤醒块零子行，tasks=retracted=2），得到 tasks=%d retracted=%d", tasks, retracted)
	}
	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}

// stubStreamOutcome 流终态面替身：模拟泵侧结果对象的意图/失败声明（真泵在 close(ch) 前写入，
// 消费侧在 itemChan 关闭后读取——happens-before 由 channel close 建立）。
type stubStreamOutcome struct {
	kind   sdkdto.CreateIntentKind
	reason string
}

func (o *stubStreamOutcome) Intent() (sdkdto.CreateIntentKind, string) { return o.kind, "" }
func (o *stubStreamOutcome) Reason() string                            { return o.reason }

// TestStreamInlineChildrenCancelledIntentRollsBack 内联首批面（B3 缺陷 1 形态）：父块内联子行、
// children_batch 批次块从未到达（开批标志恒 false），流以「已取消」意图收尾——分帧插件取消
// 落在首个批次块之前时仅此面可达，须按未收批统一回滚（决策4）：父行与全部内联子行删除、
// task/work_task 零残留，缺末批错误项折减数=已透出子行数。
func TestStreamInlineChildrenCancelledIntentRollsBack(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	in := make(chan *sdkdto.CreateStreamItem, 1)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1"), childOf("inline-2", "c-2")))
	close(in)

	out, err := svc.handleCreateTaskStreamWithOutcome(context.Background(), in, testListener(), createStreamFlushBatchSize,
		&stubStreamOutcome{kind: sdkdto.CreateIntentCancelled})
	if err != nil {
		t.Fatalf("handleCreateTaskStreamWithOutcome 返回错误: %v", err)
	}
	var retracted int
	var errs []error
	for item := range out {
		if item.Error != nil {
			errs = append(errs, item.Error)
			retracted += item.RetractedTaskCount
		}
	}
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchMissingLastBatch) {
		t.Fatalf("期望恰 1 个缺末批错误项（内联面回滚），得到 %v", errs)
	}
	if retracted != 2 {
		t.Fatalf("期望回滚折减数 = 已透出内联子行数 2，得到 %d", retracted)
	}
	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}

// TestStreamInlineChildrenErrorReasonRollsBack 内联首批面 + 失败收尾（error 块/流异常 reason）：
// 同属未收批统一回滚（决策4：error 收流一律同路径）——父行与内联子行删除、零残留。
func TestStreamInlineChildrenErrorReasonRollsBack(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	in := make(chan *sdkdto.CreateStreamItem, 1)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	close(in)

	out, err := svc.handleCreateTaskStreamWithOutcome(context.Background(), in, testListener(), createStreamFlushBatchSize,
		&stubStreamOutcome{reason: "所选文件已不存在（磁盘对账未通过）"})
	if err != nil {
		t.Fatalf("handleCreateTaskStreamWithOutcome 返回错误: %v", err)
	}
	for range out {
	}
	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}

// TestStreamInlineChildrenCleanOutcomeCommits 内联首批面 + 干净收尾（无意图/无失败声明，或
// 「转入解析」意图）：单批全内联的正常完成形态 = 隐式原子提交点（无批次块=无回滚面）——
// 父行与内联子行全部落盘（过度回滚对照组）。
func TestStreamInlineChildrenCleanOutcomeCommits(t *testing.T) {
	svc, db := newRealDBStreamService(t)

	// 干净 EOF（无终态面）与转入解析意图两形态都不得回滚（每次消费用新建通道——
	// 同一通道 close 后不可重复投递）
	inlineOnly := func() chan *sdkdto.CreateStreamItem {
		ch := make(chan *sdkdto.CreateStreamItem, 1)
		ch <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1"), childOf("inline-2", "c-2")))
		close(ch)
		return ch
	}
	out, err := svc.handleCreateTaskStreamWithOutcome(context.Background(), inlineOnly(), testListener(), createStreamFlushBatchSize, nil)
	if err != nil {
		t.Fatalf("handleCreateTaskStreamWithOutcome 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	out, err = svc.handleCreateTaskStreamWithOutcome(context.Background(), inlineOnly(), testListener(), createStreamFlushBatchSize,
		&stubStreamOutcome{kind: sdkdto.CreateIntentToParse})
	if err != nil {
		t.Fatalf("handleCreateTaskStreamWithOutcome 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	assertTableCount(t, db, "task", 6) // 两次消费 ×（父 1+内联 2）
	assertTableCount(t, db, "work_task", 6)
}

// TestStreamClosedBatchCommitsNotRolledBack 收批（last=true 已到）为原子提交点：同形状流仅
// 末批 last 差异，流正常结束不回滚——父 + 内联 1 + 批次 1 全部落盘（对照组）。
func TestStreamClosedBatchCommitsNotRolledBack(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", true, childOf("batch-1", "c-2"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), createStreamFlushBatchSize)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	assertTableCount(t, db, "task", 3)
	assertTableCount(t, db, "work_task", 3)
}

// TestStreamFlushBatchSize2000OnRealDB 落库批量 2000 行/事务（阶段1 退出标准④）：2005 子行
// 以生产批量参数（createStreamFlushBatchSize）在外键强制真库落库——2000 行单事务单条多值
// INSERT 成功本身即参数上限内（work_task 28000 绑定参数 < 32766）与 GORM 单语句落库语义
// 正确的证明，余量行次事务收尾；父 + 内联 1 + 批次 2005 全部落盘。
func TestStreamFlushBatchSize2000OnRealDB(t *testing.T) {
	svc, db := newRealDBStreamService(t)
	children := make([]*sdkdto.TaskCreateChildResponse, 2005)
	for i := range children {
		children[i] = childOf(fmt.Sprintf("c-%04d", i), fmt.Sprintf("sw-%04d", i))
	}
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-big", "work-big", childOf("inline-0", "sw-init")))
	in <- batchItem("work-big", true, children...)
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), createStreamFlushBatchSize)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	assertTableCount(t, db, "task", 1+1+2005)
	assertTableCount(t, db, "work_task", 1+1+2005)
}

// TestStreamFlushBoundariesAtProductionBatchSize 生产批量参数下的事务边界（fake 仓储锚定
// 精确分片）：2005 子行（内联 1 + 批次 2004）分两次落库，行集恰为 [2000, 5]——首事务满批
// 2000 行、余量 5 行次事务收尾，无第三事务。
func TestStreamFlushBoundariesAtProductionBatchSize(t *testing.T) {
	svc, repo := newTestService(t)
	children := make([]*sdkdto.TaskCreateChildResponse, 2004)
	for i := range children {
		children[i] = childOf(fmt.Sprintf("c-%04d", i), fmt.Sprintf("sw-%04d", i))
	}
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-big", "work-big", childOf("inline-0", "sw-init")))
	in <- batchItem("work-big", true, children...)
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), createStreamFlushBatchSize)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	want := []int{2000, 5}
	if len(repo.createBatchSizes) != len(want) {
		t.Fatalf("期望 %d 次批量落库，得到 %d 次: %v", len(want), len(repo.createBatchSizes), repo.createBatchSizes)
	}
	for i, n := range want {
		if repo.createBatchSizes[i] != n {
			t.Fatalf("第 %d 次批量落库期望 %d 行，得到 %d（全序列 %v）", i+1, n, repo.createBatchSizes[i], repo.createBatchSizes)
		}
	}
}
