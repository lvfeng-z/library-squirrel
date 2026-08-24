package fsmonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
)

// scanner 基于 workDir 遍历 + DB 全量比对的 ReconciliationScanner 实现
type scanner struct {
	storeReader   StoreReader
	backupReader  BackupReader // nil = 跳过 backup 段（能力降级）
	workDirGetter func() string
}

// NewScanner 创建离线对账扫描器
func NewScanner(storeReader StoreReader, backupReader BackupReader, workDirGetter func() string) ReconciliationScanner {
	return &scanner{storeReader: storeReader, backupReader: backupReader, workDirGetter: workDirGetter}
}

// Scan 全量对账：比对 workDir 实际文件与 DB 记录，产出差异集。
// store 域：persistent_store 记录 × 白名单子树磁盘文件（Missing/Untracked）；
// backup 域：backup 清单行 × backup/ 磁盘文件（仅 Missing 方向，孤儿文件不产出）
func (s *scanner) Scan(ctx context.Context) (DiffSet, error) {
	workDir := s.workDirGetter()
	// 1. 遍历磁盘收集实际文件（相对 workDir 正斜杠路径）
	diskFiles, err := s.collectDiskFiles(workDir)
	if err != nil {
		return DiffSet{}, err
	}
	// 2. 全量查 DB 有效已完成记录
	dbRecords, err := s.storeReader.ListValidComplete(ctx)
	if err != nil {
		return DiffSet{}, err
	}
	// 3. 比对：构建 DB 路径集合
	dbPathSet := make(map[string]StoreRecord, len(dbRecords))
	for _, r := range dbRecords {
		dbPathSet[r.FilePath] = r
	}
	// 4. 产出差异
	var diff DiffSet
	// Missing: DB 有、磁盘无
	for path, rec := range dbPathSet {
		if !diskFiles[path] {
			diff.Missing = append(diff.Missing, MissingRecord{StoreID: rec.ID, FilePath: path})
		}
	}
	// Untracked: 磁盘有、DB 无
	for path := range diskFiles {
		if _, ok := dbPathSet[path]; !ok {
			diff.Untracked = append(diff.Untracked, UntrackedFile{FilePath: path})
		}
	}
	// 5. backup 段：清单行 × backup/ 磁盘文件（reader 未注入时跳过）
	if s.backupReader != nil {
		if err := s.scanBackup(ctx, workDir, &diff); err != nil {
			return diff, err
		}
	}
	return diff, nil
}

// scanBackup backup 域对账：当前工作目录的清单行 × backup/ 磁盘文件比对，
// 行的保管路径不在磁盘即 BackupMissing；磁盘文件无行（孤儿）不产出——外部文件落入
// backup/ 不构成清单行变更
func (s *scanner) scanBackup(ctx context.Context, workDir string, diff *DiffSet) error {
	rows, err := s.backupReader.ListAllInWorkDir(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	backupFiles := make(map[string]bool)
	if err := walkSubtree(workDir, storeRegistry.BackupDirPath, backupFiles); err != nil {
		return err
	}
	for _, row := range rows {
		if !backupFiles[row.FilePath] {
			diff.BackupMissing = append(diff.BackupMissing, BackupMissingRecord{BackupID: row.ID, FilePath: row.FilePath})
		}
	}
	return nil
}

// collectDiskFiles 遍历白名单子目录，收集所有文件的相对 workDir 正斜杠路径集合
func (s *scanner) collectDiskFiles(workDir string) (map[string]bool, error) {
	files := make(map[string]bool)
	for _, dir := range storeRegistry.RegisteredDirs {
		if err := walkSubtree(workDir, dir.Path, files); err != nil {
			logger.Log.Warnf("[fsmonitor] 对账扫描：遍历目录失败 %s: %v", dir.Path, err)
		}
	}
	return files, nil
}

// walkSubtree 遍历 workDir 下一个子树，收集文件的相对 workDir 正斜杠路径。
// 子目录不存在时静默跳过；访问/遍历失败返回 error 由调用方决定降级粒度
func walkSubtree(workDir string, relRoot string, files map[string]bool) error {
	absSub := filepath.Join(workDir, filepath.FromSlash(relRoot))
	info, err := os.Stat(absSub)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 子目录不存在，跳过
		}
		return fmt.Errorf("访问目录失败 %s: %w", relRoot, err)
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.Walk(absSub, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}
		files[strings.ReplaceAll(rel, string(filepath.Separator), "/")] = true
		return nil
	})
}
