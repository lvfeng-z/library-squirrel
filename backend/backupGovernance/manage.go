package backupGovernance

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/util"
)

// statsCacheTTL 统计缓存有效期：GetBackupStats 对全量清单行逐个 os.Stat，大库（HDD 数千文件）
// 可达秒级，面板进入与筛选联动高频触发——短 TTL 内复用；删除/巡检后即时失效
const statsCacheTTL = 30 * time.Second

// ErrBackupReferenced 批量删除中存在被业务行引用的备份（整体拒绝，一个不删）
var ErrBackupReferenced = fmt.Errorf("存在被业务行引用的备份，已拒绝删除")

// PageBackups 分页查询保管清单（create_time 倒序），标注每行引用态并按引用态过滤。
// referenced：nil=全部 / true=有主 / false=无主；引用集与治理对账同源，任一引用方查询失败即报错
// （失败方引用呈现为零会让无主过滤误标误删，与治理正向熔断同一约束）
func (s *Service) PageBackups(ctx context.Context, pageNumber, pageSize int, referenced *bool) (*model.Page[BackupDTO], error) {
	_, referencedSet, err := s.collectReferencedStrict(ctx)
	if err != nil {
		return nil, err
	}
	var includeIDs, excludeIDs []int64
	if referenced != nil {
		set := make([]int64, 0, len(referencedSet))
		for id := range referencedSet {
			set = append(set, id)
		}
		if *referenced {
			includeIDs = set
		} else {
			excludeIDs = set
		}
	}
	result, err := s.catalog.PageBackups(ctx, pageNumber, pageSize, includeIDs, excludeIDs)
	if err != nil {
		return nil, err
	}
	dtoPage := &model.Page[BackupDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: len(result.Data),
		Data:         make([]*BackupDTO, 0, len(result.Data)),
	}
	for _, row := range result.Data {
		_, isReferenced := referencedSet[row.GetID()]
		dtoPage.Data = append(dtoPage.Data, &BackupDTO{
			ID:         row.GetID(),
			FileName:   nullStringValue(row.FileName),
			FilePath:   nullStringValue(row.FilePath),
			Workdir:    nullStringValue(row.Workdir),
			CreateTime: row.GetCreateTime(),
			Referenced: isReferenced,
			FileSize:   backupFileBytes(row),
		})
	}
	return dtoPage, nil
}

// DeleteBackups 批量删除备份（磁盘文件与清单行）。有主守卫：任一 id ∈ 当前引用集即整体拒绝
// （错误消息指明有主行）——引用集与治理对账同源（含软删 store 行/已卸载插件行引用），
// 引用集查询失败同样整体拒绝。单行删除不限年龄（页面有保管时间列可判）；
// 「清理全部无主」的批量圈定走 GetBackupStats 的 ExpiredOrphanIDs（超保留期）
func (s *Service) DeleteBackups(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, referencedSet, err := s.collectReferencedStrict(ctx)
	if err != nil {
		return err
	}
	blocked := make([]int64, 0)
	for _, id := range ids {
		if _, ok := referencedSet[id]; ok {
			blocked = append(blocked, id)
		}
	}
	if len(blocked) > 0 {
		return fmt.Errorf("%w: %v（有主备份由回收站/插件流程管理）", ErrBackupReferenced, blocked)
	}
	for _, id := range ids {
		if err := s.catalog.DeleteBackup(ctx, id); err != nil {
			return fmt.Errorf("删除备份 %d 失败: %w", id, err)
		}
	}
	s.invalidateStatsCache()
	return nil
}

// RunReconciliationNow 手动触发一轮双向对账（复用 RunOnce，与定时巡检经 runMu 串行），
// 返回清理统计；统计缓存随清理失效
func (s *Service) RunReconciliationNow(ctx context.Context) ReconciliationResult {
	result := s.RunOnce(ctx)
	s.invalidateStatsCache()
	return result
}

// GetBackupStats 备份占用统计：总占用、有主/无主拆分、按引用方分组（监视哨同源计算，
// 复用 computeReferencerStats）、无主超期圈定。逐行 os.Stat 大库可达秒级——TTL 内复用
// 逐行字节缓存；引用态计数与超期圈定每次现算（保留期调整/引用变化即时生效）
func (s *Service) GetBackupStats(ctx context.Context) (*BackupStatsDTO, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	refsByReferencer, referencedSet, err := s.collectReferencedStrict(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.catalog.ListCreatedBefore(ctx, util.GetCurrentTimestamp())
	if err != nil {
		return nil, err
	}

	// 逐行字节缓存（过期=整缓存作废重 Stat；新行 miss 就地补 Stat 登记）
	bytesById := map[int64]int64(nil)
	if s.statsBytesById != nil && time.Since(s.statsCachedAt) < statsCacheTTL {
		bytesById = s.statsBytesById
	} else {
		bytesById = make(map[int64]int64, len(rows))
	}

	dto := &BackupStatsDTO{ExpiredOrphanIDs: make([]int64, 0)}
	retentionDays := s.retention.GetBackupGovernanceRetentionDays()
	expireBefore := util.GetCurrentTimestamp() - int64(retentionDays)*24*60*60*1000
	dto.TotalCount = len(rows)
	for _, row := range rows {
		id := row.GetID()
		bytes, cached := bytesById[id]
		if !cached {
			bytes = backupFileBytes(row)
			bytesById[id] = bytes
		}
		dto.TotalBytes += bytes
		if _, ok := referencedSet[id]; ok {
			dto.ReferencedCount++
			dto.ReferencedBytes += bytes
			continue
		}
		dto.OrphanedCount++
		dto.OrphanedBytes += bytes
		// 无主超期圈定与治理正向判据一致（保留期垫住替换在途还原点/崩溃窗口新孤儿）
		if create := row.GetCreateTime(); create > 0 && create < expireBefore {
			dto.ExpiredOrphanIDs = append(dto.ExpiredOrphanIDs, id)
		}
	}
	stats, err := s.computeReferencerStats(ctx, refsByReferencer)
	if err != nil {
		return nil, err
	}
	dto.Referencers = stats

	s.statsBytesById = bytesById
	s.statsCachedAt = time.Now()
	return dto, nil
}

// collectReferencedStrict 汇总各引用方引用集与并集；任一引用方查询失败即整体报错。
// 引用集来源与治理对账完全同源（同一 BackupReferencer 枚举与 ListReferencedBackupIDs 调用），
// 管理面板的引用态标注与删除守卫同受熔断约束——失败方的引用不可呈现为零
func (s *Service) collectReferencedStrict(ctx context.Context) (refsByReferencer map[string][]int64, referenced map[int64]struct{}, err error) {
	refsByReferencer = make(map[string][]int64, len(s.referencers))
	referenced = make(map[int64]struct{})
	for _, ref := range s.referencers {
		ids, err := ref.ListReferencedBackupIDs(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("查询引用方 %s 的引用集失败: %w", ref.Name(), err)
		}
		refsByReferencer[ref.Name()] = ids
		for _, id := range ids {
			referenced[id] = struct{}{}
		}
	}
	return refsByReferencer, referenced, nil
}

// invalidateStatsCache 失效统计缓存（删除备份/巡检清理后文件已变，逐行字节数作废；
// 计算锁内等待在途计算结束后置空，不与统计计算并发）
func (s *Service) invalidateStatsCache() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	s.statsBytesById = nil
}

// nullStringValue sql.NullString 取值（NULL 归空串）
func nullStringValue(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
