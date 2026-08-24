package backupGovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/library-squirrel/backend/util"
)

// TestPageBackupsReferencedFilterAndMarking 分页引用态标注与过滤：软删 store 行引用与
// 已卸载插件行引用均判有主；无主行正确标注；fileSize 逐行落值
func TestPageBackupsReferencedFilterAndMarking(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	storeRef := makeBackup(t, backupSvc, db, workDir, 0)
	pluginRef := makeBackup(t, backupSvc, db, workDir, 0)
	orphan := makeBackup(t, backupSvc, db, workDir, 0)
	store := makeStoreRow(t, db, "store/resource/page-a.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), storeRef.GetID())
	makePluginRow(t, db, pluginRef.GetID(), true) // 已卸载行引用仍判有主

	// 全部：三行，引用态各自正确
	all, err := svc.PageBackups(ctx, 1, 10, nil)
	if err != nil {
		t.Fatalf("查询全部失败: %v", err)
	}
	if all.DataCount != 3 || len(all.Data) != 3 {
		t.Fatalf("全部应 3 行，实际 %d", all.DataCount)
	}
	marked := make(map[int64]bool, 3)
	for _, row := range all.Data {
		marked[row.ID] = row.Referenced
		if row.FileSize != int64(len("backup-content")) {
			t.Fatalf("行 %d fileSize=%d 期望 %d", row.ID, row.FileSize, len("backup-content"))
		}
	}
	if !marked[storeRef.GetID()] || !marked[pluginRef.GetID()] {
		t.Fatalf("软删 store 行/已卸载插件行引用的备份应判有主: %+v", marked)
	}
	if marked[orphan.GetID()] {
		t.Fatalf("无主备份 %d 不应判有主", orphan.GetID())
	}

	// 有主过滤：仅两行有主
	referencedOnly, err := svc.PageBackups(ctx, 1, 10, boolPtr(true))
	if err != nil {
		t.Fatalf("查询有主失败: %v", err)
	}
	if referencedOnly.DataCount != 2 {
		t.Fatalf("有主应 2 行，实际 %d", referencedOnly.DataCount)
	}
	for _, row := range referencedOnly.Data {
		if !row.Referenced {
			t.Fatalf("有主过滤结果含无主行 %d", row.ID)
		}
	}

	// 无主过滤：仅一行
	orphanOnly, err := svc.PageBackups(ctx, 1, 10, boolPtr(false))
	if err != nil {
		t.Fatalf("查询无主失败: %v", err)
	}
	if orphanOnly.DataCount != 1 || orphanOnly.Data[0].ID != orphan.GetID() {
		t.Fatalf("无主应仅 %d 一行，实际 %+v", orphan.GetID(), orphanOnly.Data)
	}
}

// TestPageBackupsReferencedFilterNoReferences 零引用态的有主过滤：引用集为空集时
// 有主过滤应恒空（空集 include ≠ 不过滤）、无主过滤应全量
func TestPageBackupsReferencedFilterNoReferences(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	orphan := makeBackup(t, backupSvc, db, workDir, 0)

	referencedOnly, err := svc.PageBackups(ctx, 1, 10, boolPtr(true))
	if err != nil {
		t.Fatalf("查询有主失败: %v", err)
	}
	if referencedOnly.DataCount != 0 || len(referencedOnly.Data) != 0 {
		t.Fatalf("零引用时有主过滤应恒空，实际 %d 行", referencedOnly.DataCount)
	}
	orphanOnly, err := svc.PageBackups(ctx, 1, 10, boolPtr(false))
	if err != nil {
		t.Fatalf("查询无主失败: %v", err)
	}
	if orphanOnly.DataCount != 1 || orphanOnly.Data[0].ID != orphan.GetID() {
		t.Fatalf("零引用时无主过滤应全量")
	}
}

// TestPageBackupsReferencerFailureFuse 引用集查询失败熔断：引用态数据面报错而非
// 把失败方引用呈现为零（否则无主过滤误标、跟随删除误删）
func TestPageBackupsReferencerFailureFuse(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7, &failReferencer{})
	ctx := context.Background()

	makeBackup(t, backupSvc, db, workDir, 0)

	if _, err := svc.PageBackups(ctx, 1, 10, nil); err == nil {
		t.Fatalf("存在引用集查询失败方时查询应报错")
	}
	if _, err := svc.PageBackups(ctx, 1, 10, boolPtr(false)); err == nil {
		t.Fatalf("存在引用集查询失败方时无主过滤应报错")
	}
}

// TestDeleteBackupsReferencedGuard 有主守卫：混入任一引用行即整体拒绝（一个不删）；
// 软删 store 行引用与已卸载插件行引用均触发守卫；纯无主批量成功；引用集查询失败同样拒绝
func TestDeleteBackupsReferencedGuard(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	storeRef := makeBackup(t, backupSvc, db, workDir, 0)
	pluginRef := makeBackup(t, backupSvc, db, workDir, 0)
	orphan := makeBackup(t, backupSvc, db, workDir, 0)
	store := makeStoreRow(t, db, "store/resource/del-a.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), storeRef.GetID())
	makePluginRow(t, db, pluginRef.GetID(), true)

	// 混批：整体拒绝，两行原样保留
	err := svc.DeleteBackups(ctx, []int64{orphan.GetID(), storeRef.GetID()})
	if !errors.Is(err, ErrBackupReferenced) {
		t.Fatalf("混入有主行应整体拒绝，实际 err=%v", err)
	}
	if !backupExists(t, db, orphan.GetID()) || !backupExists(t, db, storeRef.GetID()) {
		t.Fatalf("整体拒绝后任何行不应被删除")
	}

	// 已卸载插件行引用：单独删同样拒绝
	if err := svc.DeleteBackups(ctx, []int64{pluginRef.GetID()}); !errors.Is(err, ErrBackupReferenced) {
		t.Fatalf("已卸载插件行引用的备份删除应拒绝，实际 err=%v", err)
	}

	// 纯无主：行与文件皆删
	if err := svc.DeleteBackups(ctx, []int64{orphan.GetID()}); err != nil {
		t.Fatalf("删除无主备份失败: %v", err)
	}
	if backupExists(t, db, orphan.GetID()) {
		t.Fatalf("无主备份 %d 应已删除", orphan.GetID())
	}
	if util.FileExists(backupSvc.GetBackupPath(orphan)) {
		t.Fatalf("无主备份文件应已删除")
	}
}

// TestDeleteBackupsReferencerFailureFuse 引用集查询失败时删除守卫失据——整体拒绝
func TestDeleteBackupsReferencerFailureFuse(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7, &failReferencer{})
	ctx := context.Background()

	orphan := makeBackup(t, backupSvc, db, workDir, 0)

	if err := svc.DeleteBackups(ctx, []int64{orphan.GetID()}); err == nil {
		t.Fatalf("引用集查询失败时删除应拒绝")
	}
	if !backupExists(t, db, orphan.GetID()) {
		t.Fatalf("拒绝后备份 %d 不应被删除", orphan.GetID())
	}
}

// TestRunReconciliationNowReturnsStats 手动巡检返回清理统计：悬空清列数与无主清理数如实回传
func TestRunReconciliationNowReturnsStats(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	// 悬空引用两条：store 行引用不存在的 999、plugin 行引用不存在的 888
	store := makeStoreRow(t, db, "store/resource/stats-dangling.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), 999)
	plantDanglingPluginRef(t, db, 888)
	// 无主超期一份（保留期 7 天）
	makeBackup(t, backupSvc, db, workDir, 8)

	result := svc.RunReconciliationNow(ctx)

	if result.DanglingRefsCleared != 2 {
		t.Fatalf("悬空清列数应 2，实际 %d", result.DanglingRefsCleared)
	}
	if result.OrphansCleaned != 1 {
		t.Fatalf("无主清理数应 1，实际 %d", result.OrphansCleaned)
	}
	if result.IllegalRefsCleared != 0 {
		t.Fatalf("非法活行引用应 0，实际 %d", result.IllegalRefsCleared)
	}
}

// TestGetBackupStats 统计拆分：总/有主/无主数量与占用、无主超期圈定、引用方分组同源计算
func TestGetBackupStats(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	fileBytes := int64(len("backup-content"))
	referenced := makeBackup(t, backupSvc, db, workDir, 100) // 有主（超龄但被引用，不入超期圈定）
	store := makeStoreRow(t, db, "store/resource/stats-old.mp4")
	softDeleteStoreWithRef(t, db, store.GetID(), referenced.GetID())
	expiredOrphan := makeBackup(t, backupSvc, db, workDir, 8) // 无主超期 → 入圈定
	freshOrphan := makeBackup(t, backupSvc, db, workDir, 0)   // 无主未超期 → 不入圈定
	// 统计的行圈定按 create_time < now 严格比较：未回拨时间的 freshOrphan 与查询同毫秒时会被
	// 误排除（内存库操作快于 1ms 时偶发总量少一行），垫 2ms 保证其落入圈定窗口
	time.Sleep(2 * time.Millisecond)

	stats, err := svc.GetBackupStats(ctx)
	if err != nil {
		t.Fatalf("查询统计失败: %v", err)
	}
	if stats.TotalCount != 3 || stats.TotalBytes != 3*fileBytes {
		t.Fatalf("总量应 3 行/%d 字节，实际 %d/%d", 3*fileBytes, stats.TotalCount, stats.TotalBytes)
	}
	if stats.ReferencedCount != 1 || stats.ReferencedBytes != fileBytes {
		t.Fatalf("有主应 1 行/%d 字节，实际 %d/%d", fileBytes, stats.ReferencedCount, stats.ReferencedBytes)
	}
	if stats.OrphanedCount != 2 || stats.OrphanedBytes != 2*fileBytes {
		t.Fatalf("无主应 2 行/%d 字节，实际 %d/%d", 2*fileBytes, stats.OrphanedCount, stats.OrphanedBytes)
	}
	if len(stats.ExpiredOrphanIDs) != 1 || stats.ExpiredOrphanIDs[0] != expiredOrphan.GetID() {
		t.Fatalf("无主超期圈定应仅 %d，实际 %v", expiredOrphan.GetID(), stats.ExpiredOrphanIDs)
	}
	if stats.OrphanedCount != 2 {
		t.Fatalf("无主未超期行 %d 不应被圈定但应计入无主统计", freshOrphan.GetID())
	}
	byName := make(map[string]ReferencerStats, len(stats.Referencers))
	for _, st := range stats.Referencers {
		byName[st.Name] = st
	}
	if psStat, ok := byName["作品存储"]; !ok || psStat.Count != 1 || psStat.OldestAgeDays < 100 {
		t.Fatalf("作品存储分组统计错误: %+v", psStat)
	}
	if plStat, ok := byName["插件"]; !ok || plStat.Count != 0 {
		t.Fatalf("插件分组统计错误: %+v", plStat)
	}
}

// TestGetBackupStatsRetentionChangeRecompute 保留期变化后超期圈定即时重算：
// 圈定不进缓存（缓存只覆盖逐行 Stat 字节数）——保留期调大后立即重查圈定即清空，
// 「清理全部无主」不会拿旧圈定误删新保留期下的受保护行
func TestGetBackupStatsRetentionChangeRecompute(t *testing.T) {
	svc, backupSvc, _, db, workDir := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	makeBackup(t, backupSvc, db, workDir, 8) // 8 天 > 默认保留期 7

	stats, err := svc.GetBackupStats(ctx)
	if err != nil {
		t.Fatalf("查询统计失败: %v", err)
	}
	if len(stats.ExpiredOrphanIDs) != 1 {
		t.Fatalf("保留期 7 天时 8 天无主应被圈定，实际 %v", stats.ExpiredOrphanIDs)
	}

	svc.retention = &stubRetention{days: 30}
	stats2, err := svc.GetBackupStats(ctx)
	if err != nil {
		t.Fatalf("保留期调整后查询统计失败: %v", err)
	}
	if len(stats2.ExpiredOrphanIDs) != 0 {
		t.Fatalf("保留期调大后圈定应即时清空（圈定不进缓存），实际 %v", stats2.ExpiredOrphanIDs)
	}
	if stats2.OrphanedCount != 1 {
		t.Fatalf("保留期调整只影响圈定，不影响无主计数: %+v", stats2)
	}
}

// TestRunOnceMutualExclusion 对账轮次互斥：runMu 被占（定时巡检在跑）时
// RunOnce/RunReconciliationNow 阻塞排队而非并发双跑
func TestRunOnceMutualExclusion(t *testing.T) {
	svc, _, _, _, _ := newGovernanceTestEnv(t, 7)
	ctx := context.Background()

	svc.runMu.Lock() // 模拟定时巡检持有轮次锁
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.RunReconciliationNow(ctx)
	}()
	select {
	case <-done:
		t.Fatalf("runMu 被占时对账不应立即完成（互斥失效，双跑风险）")
	case <-time.After(200 * time.Millisecond):
		// 仍在阻塞排队——互斥生效
	}
	svc.runMu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("锁释放后对账应完成")
	}
}

// boolPtr bool 指针便捷构造（引用态过滤参数 nil=全部 的非 nil 分支）
func boolPtr(v bool) *bool { return &v }
