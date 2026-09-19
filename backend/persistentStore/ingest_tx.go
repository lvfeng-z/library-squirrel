package persistentStore

// 入库事务 API（四调用拆分）：登记（PrepareIngest）→ 落位（PlaceIngest）→ 调用方业务
// 事务内建行并删登记行（CommitIngest）；异常路径撤回（AbortIngest）按登记时声明的处置执行。
// 登记表 store_ingest_journal 是跨崩溃的入库原子性账本：登记先于文件落位持久化、建行与
// 删登记同事务，故「文件已落位、行未建」与「行已建、引用未写」的崩溃现场在重启后都能被
// 启动恢复（RecoverIngest）精确识别并按声明收口。调用方保持对自己业务事务的完全掌控，
// 编排归发起方。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/storeRegistry"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// IngestItem 单个文件的入库登记意图
type IngestItem struct {
	// FilePath 最终落位路径（relPath 域：workDir 相对、正斜杠；须命中 store/ 白名单）
	FilePath string
	// StagingPath 内容当前所在暂存位置（relPath 域；处置为退回暂存时的退回目标）
	StagingPath string
	// AbortAction 撤回处置声明（必填无默认：该轨能否续传、是否可重产只有调用方知道）
	AbortAction domain.IngestAbortAction
}

// ErrCommitIngestOutsideTransaction CommitIngest 收到的 ctx 未携带调用方事务。
// 建行与删登记行必须同事务，脱离事务调用会静默失去判据精确性，故入口显式拒绝
var ErrCommitIngestOutsideTransaction = errors.New("CommitIngest 须在调用方事务内调用（ctx 未携带事务）")

// PrepareIngest 登记入库意图：每文件一行 store_ingest_journal，随语句立即提交持久化。
// 处置声明必填（空值或未定义取值拒绝整批）、落位路径须命中 store/ 白名单，任一项不合法
// 即整批拒绝（不落任何一行）。返回登记行 ID 清单（与入参顺序一致），供后续 PlaceIngest /
// CommitIngest / AbortIngest 寻址
func (s *Service) PrepareIngest(ctx context.Context, items []IngestItem) ([]int64, error) {
	if err := settings.RefuseIfUnconfigured(s.getWorkDir(), "persistentStore"); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	workDir := s.getWorkDir()
	rows := make([]*domain.StoreIngestJournal, 0, len(items))
	for i, item := range items {
		if err := item.AbortAction.Validate(); err != nil {
			return nil, fmt.Errorf("第 %d 项撤回处置声明被拒绝: %w", i+1, err)
		}
		filePath := filepath.ToSlash(item.FilePath)
		if err := storeRegistry.ValidatePath(filePath); err != nil {
			return nil, fmt.Errorf("第 %d 项最终落位路径被拒绝: %w", i+1, err)
		}
		row := domain.NewStoreIngestJournal()
		row.FilePath = filePath
		row.StagingPath = filepath.ToSlash(item.StagingPath)
		row.Workdir = workDir
		row.AbortAction = item.AbortAction
		rows = append(rows, row)
	}
	if err := s.repo.CreateIngestJournals(ctx, rows); err != nil {
		return nil, fmt.Errorf("登记入库意图失败: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.GetID())
	}
	return ids, nil
}

// PlaceIngest 文件落位：逐登记行把文件从暂存位置同卷 rename 到最终路径（目录按需创建）。
// rename 前对最终路径登记操作抑制（store/ 白名单内文件的出现/移离瞬间不落入 fsmonitor
// 外部变更裁决），方法返回时统一宽限释放。任一文件落位失败即中止返回错误——已落位文件
// 保持不动，由调用方决定撤回（AbortIngest）或整体重试；未收口的登记行在下次启动时由
// 启动恢复收口
func (s *Service) PlaceIngest(ctx context.Context, intentIds []int64) error {
	if err := settings.RefuseIfUnconfigured(s.getWorkDir(), "persistentStore"); err != nil {
		return err
	}
	rows, err := s.repo.ListIngestJournalsByIds(ctx, intentIds)
	if err != nil {
		return fmt.Errorf("查询入库登记行失败: %w", err)
	}
	workDir := s.getWorkDir()
	suppressed := make([]string, 0, len(rows))
	defer func() {
		for _, key := range suppressed {
			storeRegistry.Release(key)
		}
	}()
	for _, row := range rows {
		// 抑制先于目录创建：对最终文件路径的登记同时命中其父目录的 Create 事件
		storeRegistry.Suppress(row.FilePath)
		suppressed = append(suppressed, row.FilePath)
		finalAbs := filepath.Join(workDir, row.FilePath)
		if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
			return fmt.Errorf("创建最终目录失败(%s): %w", row.FilePath, err)
		}
		if err := os.Rename(filepath.Join(workDir, row.StagingPath), finalAbs); err != nil {
			return fmt.Errorf("暂存移入最终路径失败(%s): %w", row.FilePath, err)
		}
	}
	return nil
}

// CommitIngest 提交入库：在调用方业务事务内建 persistent_store 行并删该文件的登记行。
// 两个动作同事务——事务回滚则建行与删登记一并撤销（登记行保留，下一轮撤回/恢复可收口），
// 事务提交则行与登记行同时落定，「登记行存在 ⟺ 入库未提交」的判据由此精确。ctx 须携带
// 事务（database.TxKey），脱离事务调用返回 ErrCommitIngestOutsideTransaction。行构建复用
// CommitStore 语义（completed_at 即时置位、宽高/头指纹按最终路径提取、哈希双列由调用方
// 传入、同路径活行复用全字段覆写）。返回 persistent_store 行 ID（调用方挂载关联用）
func (s *Service) CommitIngest(ctx context.Context, intentId int64, relPath string, fileName string,
	expectedSha, actualSha sql.NullString) (int64, error) {
	if _, ok := ctx.Value(database.TxKey).(*gorm.DB); !ok {
		return 0, ErrCommitIngestOutsideTransaction
	}
	storeId, err := s.CommitStore(ctx, relPath, fileName, expectedSha, actualSha)
	if err != nil {
		return 0, err
	}
	if err := s.repo.DeleteIngestJournalsByIds(ctx, []int64{intentId}); err != nil {
		return 0, fmt.Errorf("删除入库登记行失败: %w", err)
	}
	return storeId, nil
}

// AbortIngest 撤回入库：按各行登记时声明的处置执行——退回暂存（rename 回 staging_path，
// 目标已被同身份文件占用时先移除再落）或丢弃（删除文件）——随后删登记行；文件不在最终
// 路径（撤回先于落位）时仅删登记行。逐行独立收口：单行失败不阻断其余行，失败行登记行
// 保留（可重试撤回，或下次启动经恢复收口），存在任一失败即返回首个错误。幂等——重复
// 调用对已收口的登记行无副作用
func (s *Service) AbortIngest(ctx context.Context, intentIds []int64) error {
	if err := settings.RefuseIfUnconfigured(s.getWorkDir(), "persistentStore"); err != nil {
		return err
	}
	rows, err := s.repo.ListIngestJournalsByIds(ctx, intentIds)
	if err != nil {
		return fmt.Errorf("查询入库登记行失败: %w", err)
	}
	workDir := s.getWorkDir()
	suppressed := make([]string, 0, len(rows))
	defer func() {
		for _, key := range suppressed {
			storeRegistry.Release(key)
		}
	}()
	done := make([]int64, 0, len(rows))
	var firstErr error
	for _, row := range rows {
		if err := revertIngestFile(row, workDir, &suppressed); err != nil {
			logger.Log.Warn("撤回入库单文件失败，登记行保留待重试",
				zap.String("filePath", row.FilePath), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		done = append(done, row.GetID())
	}
	if err := s.repo.DeleteIngestJournalsByIds(ctx, done); err != nil {
		return fmt.Errorf("删除入库登记行失败: %w", err)
	}
	return firstErr
}

// revertIngestFile 单文件撤回：按登记的处置把已落位文件移出最终路径（未落位则为空操作）。
// 返回 nil 表示该登记行可删
func revertIngestFile(row *domain.StoreIngestJournal, workDir string, suppressed *[]string) error {
	action, err := domain.ParseIngestAbortAction(string(row.AbortAction))
	if err != nil {
		return fmt.Errorf("登记行处置声明非法: %w", err)
	}
	finalAbs := filepath.Join(workDir, row.FilePath)
	switch action {
	case domain.AbortActionReturnToStaging:
		exists, err := fileExistsAt(finalAbs)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		storeRegistry.Suppress(row.FilePath)
		*suppressed = append(*suppressed, row.FilePath)
		return returnFileToStaging(finalAbs, workDir, row.StagingPath)
	case domain.AbortActionDiscard:
		storeRegistry.Suppress(row.FilePath)
		*suppressed = append(*suppressed, row.FilePath)
		if err := os.Remove(finalAbs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("丢弃文件失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("登记行处置声明非法(%s)", row.AbortAction)
}

// RecoverIngest 启动恢复：逐登记行按「文件在最终路径与否 × 登记处置」收口——退回暂存
// （退回不可达时删除文件兜底）、丢弃删文件、未落位仅删登记行；登记行所属库根与当前工作
// 目录不匹配时保留不动（对应库根恢复时再收口）。单行失败不阻断其余行（登记行保留，下次
// 启动重试），整体错误仅来自登记行查询失败。工作目录未配置（空串）时短路返回。返回收口
// 行数。恢复动作发生在 fsmonitor 启动之前，无须操作抑制
func (s *Service) RecoverIngest(ctx context.Context) (int, error) {
	workDir := s.getWorkDir()
	if workDir == "" {
		return 0, nil
	}
	rows, err := s.repo.ListIngestJournals(ctx)
	if err != nil {
		return 0, fmt.Errorf("查询入库登记行失败: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	logger.Log.Infof("[persistentStore] 发现 %d 条未收口入库登记行，开始恢复", len(rows))
	recovered := 0
	for _, row := range rows {
		if row.Workdir != workDir {
			logger.Log.Warn("入库登记行所属库根与当前工作目录不匹配，保留待对应库根恢复",
				zap.String("filePath", row.FilePath), zap.String("journalWorkdir", row.Workdir))
			continue
		}
		if err := s.recoverIngestRow(ctx, row, workDir); err != nil {
			logger.Log.Warn("入库登记行恢复失败，登记行保留待下次启动重试",
				zap.String("filePath", row.FilePath), zap.Error(err))
			continue
		}
		recovered++
	}
	return recovered, nil
}

// recoverIngestRow 单登记行收口：执行处置动作并删登记行，返回 nil 即已收口
func (s *Service) recoverIngestRow(ctx context.Context, row *domain.StoreIngestJournal, workDir string) error {
	finalAbs := filepath.Join(workDir, row.FilePath)
	exists, err := fileExistsAt(finalAbs)
	if err != nil {
		return err
	}
	if !exists {
		// 未落位：文件仍在暂存（或已不存在），登记行直接收口
		logger.Log.Infof("[persistentStore] 入库恢复：文件未落位，登记行收口", zap.String("filePath", row.FilePath))
		return s.repo.DeleteIngestJournalsByIds(ctx, []int64{row.GetID()})
	}
	action, err := domain.ParseIngestAbortAction(string(row.AbortAction))
	if err != nil {
		return fmt.Errorf("登记行处置声明非法: %w", err)
	}
	switch action {
	case domain.AbortActionReturnToStaging:
		if err := returnFileToStaging(finalAbs, workDir, row.StagingPath); err != nil {
			// 退回不可达兜底：删除最终路径文件并记 Warn——调用方暂存目录可能已被回收，
			// 兜底保证文件不滞留最终路径冒充已入库；登记行照常收口
			if rerr := os.Remove(finalAbs); rerr != nil && !os.IsNotExist(rerr) {
				return fmt.Errorf("退回暂存失败且兜底删除失败: 退回=%v, 删除=%v", err, rerr)
			}
			logger.Log.Warn("入库恢复：退回暂存不可达，已删除最终路径文件兜底",
				zap.String("filePath", row.FilePath), zap.String("stagingPath", row.StagingPath), zap.Error(err))
		} else {
			logger.Log.Infof("[persistentStore] 入库恢复：文件已退回暂存",
				zap.String("filePath", row.FilePath), zap.String("stagingPath", row.StagingPath))
		}
	case domain.AbortActionDiscard:
		if err := os.Remove(finalAbs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("丢弃文件失败: %w", err)
		}
		logger.Log.Infof("[persistentStore] 入库恢复：已按丢弃声明删除文件", zap.String("filePath", row.FilePath))
	}
	return s.repo.DeleteIngestJournalsByIds(ctx, []int64{row.GetID()})
}

// returnFileToStaging 把已落位文件退回暂存位置：按需创建暂存目录，目标已被同身份文件
// 占用时先移除再落（崩溃态下不存在并发写同一暂存位的情形）
func returnFileToStaging(finalAbs, workDir, stagingRel string) error {
	stagingAbs := filepath.Join(workDir, stagingRel)
	if err := os.MkdirAll(filepath.Dir(stagingAbs), 0o755); err != nil {
		return fmt.Errorf("创建暂存目录失败: %w", err)
	}
	if err := os.Remove(stagingAbs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理暂存目标失败: %w", err)
	}
	if err := os.Rename(finalAbs, stagingAbs); err != nil {
		return fmt.Errorf("退回暂存失败: %w", err)
	}
	return nil
}

// fileExistsAt 查询路径文件是否在位；查询自身失败（权限等）返回错误
func fileExistsAt(absPath string) (bool, error) {
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("查询文件状态失败: %w", err)
	}
	return true, nil
}
