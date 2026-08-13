package fsmonitor

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
)

// scanner 基于 workDir 遍历 + DB 全量比对的 ReconciliationScanner 实现
type scanner struct {
	storeReader   StoreReader
	workDirGetter func() string
}

// NewScanner 创建离线对账扫描器
func NewScanner(storeReader StoreReader, workDirGetter func() string) ReconciliationScanner {
	return &scanner{storeReader: storeReader, workDirGetter: workDirGetter}
}

// Scan 全量对账：比对 workDir 实际文件与 persistent_store 记录，产出差异集
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
	return diff, nil
}

// collectDiskFiles 遍历白名单子目录，收集所有文件的相对 workDir 正斜杠路径集合
func (s *scanner) collectDiskFiles(workDir string) (map[string]bool, error) {
	files := make(map[string]bool)
	for _, dir := range storeRegistry.RegisteredDirs {
		absSub := filepath.Join(workDir, filepath.FromSlash(dir.Path))
		info, err := os.Stat(absSub)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 子目录不存在，跳过
			}
			logger.Log.Warnf("[fsmonitor] 对账扫描：访问目录失败 %s: %v", dir.Path, err)
			continue
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.Walk(absSub, func(path string, info os.FileInfo, err error) error {
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
		if err != nil {
			logger.Log.Warnf("[fsmonitor] 对账扫描：遍历目录失败 %s: %v", dir.Path, err)
		}
	}
	return files, nil
}
