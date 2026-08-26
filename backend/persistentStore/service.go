package persistentStore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"
	"github.com/library-squirrel/backend/util/filename"
	"github.com/library-squirrel/backend/util/fingerprint"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 文件持久存储仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建记录（Create 后通过指针回填 ID）
	Create(ctx context.Context, store *domain.PersistentStore) error
	// Updates 更新记录
	Updates(ctx context.Context, store *domain.PersistentStore) error
	// GetById 根据 ID 获取记录
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	// GetByIdUnscoped 按 ID 获取记录（含已软删行；回收站文件条目清理链读取）
	GetByIdUnscoped(ctx context.Context, id int64) (*domain.PersistentStore, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.PersistentStore, error)
	// GetByFilePath 根据路径获取记录
	GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error)
	// ResetCompleted 显式重置 completed_at=0（未完成零值是合法业务值，GORM Updates 跳零值故单列更新）
	ResetCompleted(ctx context.Context, id int64) error
	// DeleteUnscoped 物理删除记录（绕过软删改写；软删仅作品软删链经 DeleteWithBackup/MarkInvalid 使用）
	DeleteUnscoped(ctx context.Context, id int64) error
	// DeleteUnscopedByIds 批量物理删除记录（单条 SQL；目标为已软删行的物理删除通路——
	// HardDelete 的 GetById 受软删 scope 保护会静默跳过已删行，作品彻底删除链的目标行全为软删行故走此直删）
	DeleteUnscopedByIds(ctx context.Context, ids []int64) error
	// RestoreByIds 批量清软删标志（复原链复活记录）
	RestoreByIds(ctx context.Context, ids []int64) error
	// Delete 删除记录
	Delete(ctx context.Context, id int64) error
	// SoftDeleteWithBackup 软删记录并写入备份清单行引用（backup_id 与 deleted_at 单条 UPDATE 同生共死）
	SoftDeleteWithBackup(ctx context.Context, id int64, backupId int64) error
	// ExistsByFilePath 检查文件路径是否已存在记录
	ExistsByFilePath(ctx context.Context, filePath string) bool
	// NormalizeFilePaths 将 file_path 反斜杠统一为正斜杠（数据规范化迁移，幂等）
	NormalizeFilePaths(ctx context.Context) (int64, error)
	// RenameDirectoryPrefix 批量替换 file_path 的目录前缀（目录改名同步）
	RenameDirectoryPrefix(ctx context.Context, oldPrefix string, newPrefix string) (int64, error)
	// ListReferencedBackupIds 全量投影行内引用的备份清单行 ID（含已删行——软删行是合法引用者；
	// 供备份治理引用集对账）
	ListReferencedBackupIds(ctx context.Context) ([]int64, error)
	// ClearBackupRefsByBackupIds 按引用目标清 backup_id（悬空引用清列；含已删行）
	ClearBackupRefsByBackupIds(ctx context.Context, ids []int64) error
	// ClearIllegalAliveBackupRefs 清活行（deleted_at=0）携带备份引用的非法态列，返回受影响行数
	ClearIllegalAliveBackupRefs(ctx context.Context) (int64, error)
}

// StoreWriter 封装文件句柄和 DB 记录，实现完整的写入生命周期管理
//
// 生命周期：
//
//	写入中 → Write() + Sync()
//	暂停   → Close()          关闭文件句柄，保留未完成 DB 记录
//	成功   → Complete()       同步+关闭+更新 DB 为已完成
//	失败   → Abort()          关闭+删除文件+删除 DB 记录
type StoreWriter interface {
	io.Writer
	// Sync 同步文件到磁盘
	Sync() error
	// Close 关闭文件句柄（暂停），DB 记录保持未完成
	Close() error
	// Complete 完成写入：同步+关闭+更新 DB 状态为已完成
	Complete() error
	// Abort 放弃写入：关闭+删除文件+删除 DB 记录
	Abort() error
}

// storeWriter StoreWriter 的内部实现
type storeWriter struct {
	file          *os.File
	storeId       int64
	repo          Repository
	closed        bool
	workDirGetter func() string        // 每次调用获取最新的 workDir
	filePath      sql.NullString       // 落盘相对路径（Complete 算宽高 / Abort 删文件用，免读回 DB）
	ext           sql.NullString       // 文件扩展名（Complete 判断是否图片用）
	fingerprinter fingerprint.Computer // 可选，nil 时 Complete 不算指纹
}

func (w *storeWriter) Write(p []byte) (n int, err error) {
	return w.file.Write(p)
}

func (w *storeWriter) Sync() error {
	if w.closed {
		return nil
	}
	return w.file.Sync()
}

func (w *storeWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

func (w *storeWriter) Complete() error {
	// 同步+关闭文件
	if !w.closed {
		if err := w.file.Sync(); err != nil {
			w.file.Close()
			w.closed = true
			return fmt.Errorf("同步文件失败: %w", err)
		}
		w.closed = true
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("关闭文件失败: %w", err)
		}
	}

	// 提取图片宽高（用构造时缓存的路径，无需读回 DB 记录）
	width, height := tryDecodeImageDimensions(w.filePath, w.ext, w.workDirGetter())
	// 仅写 completed_at + 宽高 + 指纹，其余字段靠 Updates 跳零值保留
	record := domain.NewPersistentStore()
	record.SetID(w.storeId)
	record.CompletedAt = util.GetCurrentTimestamp()
	record.Width = width
	record.Height = height
	if w.fingerprinter != nil && w.filePath.Valid {
		absPath := filepath.Join(w.workDirGetter(), w.filePath.String)
		if fp, err := w.fingerprinter.Fingerprint(context.Background(), absPath); err == nil {
			record.ContentFingerprint = sql.NullString{String: fp.Digest, Valid: true}
		} else {
			logger.Log.Warn("计算内容指纹失败，留空", zap.String("path", absPath), zap.Error(err))
		}
	}
	if err := w.repo.Updates(context.Background(), record); err != nil {
		return fmt.Errorf("更新记录状态失败: %w", err)
	}
	return nil
}

func (w *storeWriter) Abort() error {
	// 关闭文件
	if !w.closed {
		w.file.Close()
		w.closed = true
	}

	// 用构造时缓存的路径删除磁盘文件（无需读回 DB 记录）
	if w.filePath.Valid {
		storeRegistry.Suppress(w.filePath.String)
		defer storeRegistry.Release(w.filePath.String)
		absPath := filepath.Join(w.workDirGetter(), w.filePath.String)
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Log.Warn("Abort 时删除文件失败", zap.String("path", absPath), zap.Error(err))
		}
	}

	// 删除 DB 记录
	if err := w.repo.DeleteUnscoped(context.Background(), w.storeId); err != nil {
		logger.Log.Error("Abort 时删除记录失败", zap.Int64("storeId", w.storeId), zap.Error(err))
		return err
	}
	return nil
}

// tryDecodeImageDimensions 若为图片则读取文件头部解码返回宽高，否则返回无效值
func tryDecodeImageDimensions(filePath, ext sql.NullString, workDir string) (width, height sql.NullInt64) {
	if !ext.Valid || !filePath.Valid {
		return
	}
	if !util.IsImageExt(ext.String) {
		return
	}
	absPath := filepath.Join(workDir, filePath.String)
	w, h, err := util.DecodeImageDimensions(absPath)
	if err != nil {
		logger.Log.Warn("提取图片宽高失败，留空", zap.String("path", absPath), zap.Error(err))
		return
	}
	return sql.NullInt64{Int64: int64(w), Valid: true}, sql.NullInt64{Int64: int64(h), Valid: true}
}

// fillImageDimensions 若记录为图片，读取文件头部解码填入 Width/Height。
// 解码失败时仅记日志、留 0，不阻断入库主流程。
func fillImageDimensions(store *domain.PersistentStore, workDir string) {
	if store == nil {
		return
	}
	store.Width, store.Height = tryDecodeImageDimensions(store.FilePath, store.FilenameExtension, workDir)
}

// FileMover 文件移动备份接口（由 persistentStore 定义，backup.Service 实现）
type FileMover interface {
	// MoveToBackup 将文件移动到备份目录并建保管清单行，返回清单行 ID（供行内嵌 backup_id 引用）
	MoveToBackup(ctx context.Context, absFilePath string) (int64, error)
}

// Service 文件存取服务
type Service struct {
	repo          Repository
	fileMover     FileMover            // 可选依赖，nil 时不备份
	fingerprinter fingerprint.Computer // 可选依赖，nil 时不计算内容指纹
	workDirGetter func() string        // 每次调用获取最新的 workDir（从设置管理器读取）
}

// NewService 创建文件存取服务
func NewService(repo Repository, fileMover FileMover, workDirGetter func() string) *Service {
	return &Service{
		repo:          repo,
		fileMover:     fileMover,
		workDirGetter: workDirGetter,
	}
}

// SetFingerprinter 注入内容指纹计算器（可选依赖，外部变更监控启用时注入）
func (s *Service) SetFingerprinter(fp fingerprint.Computer) {
	s.fingerprinter = fp
}

// NormalizeFilePaths 将 file_path 反斜杠统一为正斜杠（数据规范化，启动时调用）
func (s *Service) NormalizeFilePaths(ctx context.Context) (int64, error) {
	return s.repo.NormalizeFilePaths(ctx)
}

// RenameDirectoryPrefix 批量替换 file_path 的目录前缀（目录改名同步：下级文件路径批量更新）
func (s *Service) RenameDirectoryPrefix(ctx context.Context, oldPrefix string, newPrefix string) (int64, error) {
	return s.repo.RenameDirectoryPrefix(ctx, oldPrefix, newPrefix)
}

// UpdateFilePath 更新记录的 file_path（移动/重命名修复：DB 路径同步到文件新位置）
// 仅改 file_path 字段，其余保留。记录须未失效。
func (s *Service) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	// 路径须在 store/ 白名单内：file_path 指向 backup/ 等白名单外路径（如误报的
	// "移动到备份"被确认同步）会令记录指向非受管文件，后续读取/对账全部失效
	if err := storeRegistry.ValidatePath(newFilePath); err != nil {
		return fmt.Errorf("目标路径不在 store/ 白名单内: %w", err)
	}
	record := domain.NewPersistentStore()
	record.SetID(id)
	record.FilePath = sql.NullString{String: newFilePath, Valid: true}
	return s.repo.Updates(ctx, record)
}

// MarkInvalid 置记录失效（外部删除且用户不复原 / 移出 workDir 的用户裁决）
// 经记录级软删（deleted_at 打时间戳）实现，行保留可追溯；invalid_at 列已退役并入 deleted_at
func (s *Service) MarkInvalid(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// BackfillFingerprints 存量指纹回填：扫描所有 Status=Complete 但 ContentFingerprint 缺失的记录，
// 对磁盘仍存在的文件算指纹填入。磁盘已无的记录留空（交对账处理）。
// 未注入 Fingerprinter 时直接返回。异步调用，不阻塞主流程。
func (s *Service) BackfillFingerprints(ctx context.Context) {
	if s.fingerprinter == nil {
		return
	}
	go s.runBackfillFingerprints(ctx)
}

func (s *Service) runBackfillFingerprints(ctx context.Context) {
	records, err := s.repo.List(ctx, &database.QueryOption{})
	if err != nil {
		logger.Log.Warn("[persistentStore] 回填指纹：查询记录失败", zap.Error(err))
		return
	}
	workDir := s.getWorkDir()
	filled := 0
	for _, r := range records {
		// 仅回填已完成且指纹缺失的记录
		if r.CompletedAt == 0 {
			continue
		}
		if r.ContentFingerprint.Valid {
			continue
		}
		if !r.FilePath.Valid {
			continue
		}
		absPath := filepath.Join(workDir, r.FilePath.String)
		if _, err := os.Stat(absPath); err != nil {
			continue // 磁盘已无，留空交对账
		}
		fp, err := s.fingerprinter.Fingerprint(ctx, absPath)
		if err != nil {
			logger.Log.Warn("[persistentStore] 回填指纹：计算失败", zap.String("path", absPath), zap.Error(err))
			continue
		}
		r.ContentFingerprint = sql.NullString{String: fp.Digest, Valid: true}
		if err := s.repo.Updates(ctx, r); err != nil {
			logger.Log.Warn("[persistentStore] 回填指纹：更新失败", zap.Int64("id", r.GetID()), zap.Error(err))
			continue
		}
		filled++
	}
	if filled > 0 {
		logger.Log.Infof("[persistentStore] 指纹回填完成，共 %d 条", filled)
	}
}

// computeFingerprint 若注入了 Fingerprinter，计算绝对路径文件指纹并返回可落库的 NullString；否则返回无效值
func (s *Service) computeFingerprint(absPath string) sql.NullString {
	if s.fingerprinter == nil {
		return sql.NullString{}
	}
	fp, err := s.fingerprinter.Fingerprint(context.Background(), absPath)
	if err != nil {
		logger.Log.Warn("计算内容指纹失败，留空", zap.String("path", absPath), zap.Error(err))
		return sql.NullString{}
	}
	return sql.NullString{String: fp.Digest, Valid: true}
}

// fillFingerprint 若注入了 Fingerprinter，计算绝对路径文件指纹填入 store.ContentFingerprint
func (s *Service) fillFingerprint(store *domain.PersistentStore, absPath string) {
	if store == nil || s.fingerprinter == nil {
		return
	}
	store.ContentFingerprint = s.computeFingerprint(absPath)
}

// getWorkDir 获取当前 workDir（每次从设置管理器读取最新值）
func (s *Service) getWorkDir() string {
	return s.workDirGetter()
}

// CleanupFileResult 删除指定相对路径的磁盘文件并返回真实失败（文件缺失容忍返回 nil）。
// 供删除流「先文件后记录」两阶段的 Phase A——文件删不动即中止（记录未动），由调用方决定仅删记录或放弃
func (s *Service) CleanupFileResult(relPath string) error {
	storeRegistry.Suppress(relPath)
	defer storeRegistry.Release(relPath)
	absPath := filepath.Join(s.getWorkDir(), relPath)
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CleanupFile 清理指定相对路径的磁盘文件（用于事务回滚后的文件清理）
func (s *Service) CleanupFile(relPath string) {
	if err := s.CleanupFileResult(relPath); err != nil {
		logger.Log.Warn("清理文件失败", zap.String("path", filepath.Join(s.getWorkDir(), relPath)), zap.Error(err))
	}
}

// StoreStream 创建 DB 记录（未完成）+ 目录 + 文件，返回 storeId 和 StoreWriter
// relPath: 相对于 {workDir} 的路径
// fileName: 原始文件名
func (s *Service) StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer StoreWriter, err error) {
	// 1. 校验 relPath
	if err := storeRegistry.ValidatePath(relPath); err != nil {
		return 0, nil, err
	}
	storeRegistry.Suppress(relPath)
	defer storeRegistry.Release(relPath)

	// 入口规范化为正斜杠（PATH_SEPARATOR_DISCIPLINE）：查旧/抑制登记/落库全程与 DB 基准一致
	relPath = filepath.ToSlash(relPath)

	workDir := s.getWorkDir()
	absPath := filepath.Join(workDir, relPath)

	// 2. 检查 relPath 是否已存在记录
	existing, err := s.repo.GetByFilePath(ctx, relPath)
	if err != nil {
		return 0, nil, fmt.Errorf("查询已有记录失败: %w", err)
	}

	if existing != nil {
		// 已存在 → 删除旧文件
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Log.Warn("删除旧文件失败", zap.String("path", absPath), zap.Error(err))
		}
	}

	// 3. 确保目录存在
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return 0, nil, fmt.Errorf("创建目录失败: %w", err)
	}

	// 4. 创建文件
	file, err := os.Create(absPath)
	if err != nil {
		return 0, nil, fmt.Errorf("创建文件失败: %w", err)
	}

	// 5. 提取扩展名
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = filepath.Ext(relPath)
	}

	// 6. 创建或更新 DB 记录（未完成状态）
	if existing != nil {
		existing.FileName.Valid = true
		existing.FileName.String = fileName
		existing.FilenameExtension.Valid = true
		existing.FilenameExtension.String = ext
		// completed_at 置 0（续传重置为未完成）是合法业务零值，GORM Updates 会跳过，经显式列更新补写
		if err := s.repo.Updates(ctx, existing); err != nil {
			file.Close()
			os.Remove(absPath)
			return 0, nil, fmt.Errorf("更新记录失败: %w", err)
		}
		if err := s.repo.ResetCompleted(ctx, existing.GetID()); err != nil {
			file.Close()
			os.Remove(absPath)
			return 0, nil, fmt.Errorf("重置未完成状态失败: %w", err)
		}
		sw := &storeWriter{file: file, storeId: existing.GetID(), repo: s.repo, workDirGetter: s.workDirGetter, filePath: existing.FilePath, ext: existing.FilenameExtension, fingerprinter: s.fingerprinter}
		return existing.GetID(), sw, nil
	}

	// 创建新记录
	store := domain.NewPersistentStore()
	store.FilePath.Valid = true
	store.FilePath.String = filepath.ToSlash(relPath)
	store.FileName.Valid = true
	store.FileName.String = fileName
	store.FilenameExtension.Valid = true
	store.FilenameExtension.String = ext

	if err := s.repo.Create(ctx, store); err != nil {
		file.Close()
		os.Remove(absPath)
		return 0, nil, fmt.Errorf("保存记录失败: %w", err)
	}

	sw := &storeWriter{file: file, storeId: store.GetID(), repo: s.repo, workDirGetter: s.workDirGetter, filePath: store.FilePath, ext: store.FilenameExtension, fingerprinter: s.fingerprinter}
	return store.GetID(), sw, nil
}

// ResumeStream 恢复存储:Truncate(offset) + O_WRONLY 打开文件,从 offset 位置写入。
// offset 为续写起始偏移;文件会被截断到 offset(丢弃 offset 之后的多余数据),消除 TOCTOU 竞态。
// storeId: StoreStream 返回的未完成记录 ID
func (s *Service) ResumeStream(ctx context.Context, storeId int64, offset int64) (StoreWriter, error) {
	// 1. 查询记录，确认状态为未完成
	record, err := s.repo.GetById(ctx, storeId)
	if err != nil {
		return nil, fmt.Errorf("查询记录失败: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("记录不存在: storeId=%d", storeId)
	}
	if record.CompletedAt != 0 {
		return nil, fmt.Errorf("记录已完成，无法恢复: storeId=%d, completed_at=%d", storeId, record.CompletedAt)
	}

	// 2. 获取绝对路径
	absPath := s.GetAbsPath(record)
	if absPath == "" {
		return nil, fmt.Errorf("记录文件路径为空: storeId=%d", storeId)
	}

	// 3. O_WRONLY 打开(不用 O_APPEND),Truncate 到 offset 后 Seek 到 offset 写入。
	// Truncate 消除 os.Stat 与文件打开之间的 TOCTOU:即使文件在 stat 后变大,多余部分被截断。
	file, err := os.OpenFile(absPath, os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	if err := file.Truncate(offset); err != nil {
		file.Close()
		return nil, fmt.Errorf("截断文件到偏移 %d 失败: %w", offset, err)
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return nil, fmt.Errorf("定位到偏移 %d 失败: %w", offset, err)
	}

	return &storeWriter{file: file, storeId: storeId, repo: s.repo, workDirGetter: s.workDirGetter, filePath: record.FilePath, ext: record.FilenameExtension}, nil
}

// Store 存入文件
// relPath: 相对于 {workDir}/store/ 的路径（由调用方指定，必须匹配已注册子目录）
// fileName: 原始文件名
// reader: 文件内容
// 返回 persistent_store 记录 ID
func (s *Service) Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error) {
	// 1. 校验 relPath 是否匹配已注册子目录
	if err := storeRegistry.ValidatePath(relPath); err != nil {
		return 0, err
	}
	storeRegistry.Suppress(relPath)
	defer storeRegistry.Release(relPath)

	// 入口规范化为正斜杠（PATH_SEPARATOR_DISCIPLINE）：查旧/抑制登记/落库全程与 DB 基准一致
	relPath = filepath.ToSlash(relPath)

	workDir := s.getWorkDir()
	absPath := filepath.Join(workDir, relPath)

	// 2. 检查 relPath 是否已存在记录
	existing, err := s.repo.GetByFilePath(ctx, relPath)
	if err != nil {
		return 0, fmt.Errorf("查询已有记录失败: %w", err)
	}

	if existing != nil {
		// 已存在 → 删除旧文件 + 更新记录
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Log.Warn("删除旧文件失败", zap.String("path", absPath), zap.Error(err))
		}
	}

	// 3. 确保目录存在
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return 0, fmt.Errorf("创建目录失败: %w", err)
	}

	// 4. 写入文件
	file, err := os.Create(absPath)
	if err != nil {
		return 0, fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		// 写入失败时清理已创建的文件
		os.Remove(absPath)
		return 0, fmt.Errorf("写入文件失败: %w", err)
	}

	// 5. 提取扩展名
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = filepath.Ext(relPath)
	}

	if existing != nil {
		// 更新已有记录
		existing.FileName.Valid = true
		existing.FileName.String = fileName
		existing.FilenameExtension.Valid = true
		existing.FilenameExtension.String = ext
		existing.CompletedAt = util.GetCurrentTimestamp()
		fillImageDimensions(existing, workDir)
		s.fillFingerprint(existing, absPath)
		if err := s.repo.Updates(ctx, existing); err != nil {
			return 0, fmt.Errorf("更新记录失败: %w", err)
		}
		return existing.GetID(), nil
	}

	// 6. 创建记录（已完成）
	store := domain.NewPersistentStore()
	store.FilePath.Valid = true
	store.FilePath.String = filepath.ToSlash(relPath)
	store.FileName.Valid = true
	store.FileName.String = fileName
	store.FilenameExtension.Valid = true
	store.FilenameExtension.String = ext
	store.CompletedAt = util.GetCurrentTimestamp()
	fillImageDimensions(store, workDir)
	s.fillFingerprint(store, absPath)

	if err := s.repo.Create(ctx, store); err != nil {
		// 记录创建失败时清理文件
		os.Remove(absPath)
		return 0, fmt.Errorf("保存记录失败: %w", err)
	}

	return store.GetID(), nil
}

// StoreFromFile 从本地文件存入
// relPath: 相对于 {workDir}/store/ 的路径
// fileName: 原始文件名
// srcAbsPath: 源文件的绝对路径
func (s *Service) StoreFromFile(ctx context.Context, relPath string, fileName string, srcAbsPath string) (int64, error) {
	srcFile, err := os.Open(srcAbsPath)
	if err != nil {
		return 0, fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	return s.Store(ctx, relPath, fileName, srcFile)
}

// GetById 根据 ID 获取记录
func (s *Service) GetById(ctx context.Context, id int64) (*domain.PersistentStore, error) {
	return s.repo.GetById(ctx, id)
}

// GetByFingerprint 按内容指纹查询已完成记录（排除指定路径），供文件移动关联匹配。
// 指纹匹配=内容相同→判定为同一文件的移动；返回首条命中（理论上指纹唯一，多命中取首条）。
// 已删行（外部删除裁决/作品软删）经 GORM 软删 scope 自动排除。无命中返回 (nil, nil)。
func (s *Service) GetByFingerprint(ctx context.Context, fingerprint string, excludePath string) (*domain.PersistentStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "content_fingerprint", Value: fingerprint},
			clause.Neq{Column: "file_path", Value: excludePath},
			clause.Expr{SQL: "completed_at > 0"},
		},
		Limit: 1,
	}
	records, err := s.repo.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}

// GetByFilePathComplete 按路径查询已完成记录（移动场景：旧路径记录存在且已完成）
// 已删行经 GORM 软删 scope 自动排除。无命中返回 (nil, nil)
func (s *Service) GetByFilePathComplete(ctx context.Context, filePath string) (*domain.PersistentStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "file_path", Value: filePath},
			clause.Expr{SQL: "completed_at > 0"},
		},
		Limit: 1,
	}
	records, err := s.repo.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}

// ListValidComplete 全量查询已完成（completed_at>0）且在位的记录，供 fsmonitor 离线对账
// （已删行——外部删除裁决/作品软删——经 GORM 软删 scope 自动排除，不进对账基线）
func (s *Service) ListValidComplete(ctx context.Context) ([]*domain.PersistentStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Expr{SQL: "completed_at > 0"},
		},
	}
	return s.repo.List(ctx, opt)
}

// GetByIds 根据 ID 列表批量查询记录
func (s *Service) GetByIds(ctx context.Context, ids []int64) ([]*domain.PersistentStore, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.IN{Column: "id", Values: func() []interface{} {
				v := make([]interface{}, len(ids))
				for i, id := range ids {
					v[i] = id
				}
				return v
			}()},
		},
	}
	return s.repo.List(ctx, opt)
}

// GetByFilePath 根据路径获取记录
func (s *Service) GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error) {
	return s.repo.GetByFilePath(ctx, filePath)
}

// GetDeletedStore 按 ID 获取已软删记录行（含行内 backup_id 与 file_path；回收站文件条目清理链入口校验，
// nil = 行不存在或非已删态）
func (s *Service) GetDeletedStore(ctx context.Context, id int64) (*domain.PersistentStore, error) {
	record, err := s.repo.GetByIdUnscoped(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if record.DeletedAt == 0 {
		return nil, nil
	}
	return record, nil
}

// DeleteUnscopedByIds 批量物理删除记录（目标为已软删行的物理删除通路，见仓储方法注释）
func (s *Service) DeleteUnscopedByIds(ctx context.Context, ids []int64) error {
	return s.repo.DeleteUnscopedByIds(ctx, ids)
}

// HardDelete 删除记录及对应文件（物理删记录；软删语义的 Delete 归作品软删链经 DeleteWithBackup/MarkInvalid）
// backup: 是否对已完成文件进行移动备份，返回备份记录 ID（0 表示未备份）
func (s *Service) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	// 1. 根据 ID 查询记录
	record, err := s.repo.GetById(ctx, id)
	if err != nil {
		// 记录不存在视为已删除：返回 nil 而非错误，使备份调用方判定“无需备份”而非“备份失败”
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if record == nil {
		return 0, nil
	}

	var backupId int64
	if record.FilePath.Valid {
		storeRegistry.Suppress(record.FilePath.String)
		defer storeRegistry.Release(record.FilePath.String)
		workDir := s.getWorkDir()
		absPath := filepath.Join(workDir, record.FilePath.String)

		// 2. 对已完成的文件进行移动备份（可选）
		if backup && record.CompletedAt > 0 && s.fileMover != nil {
			backupId, err = s.fileMover.MoveToBackup(ctx, absPath)
			if err != nil {
				logger.Log.Warn("备份文件失败，降级为直接删除", zap.String("path", absPath), zap.Error(err))
				_ = os.Remove(absPath)
			}
		} else {
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				logger.Log.Warn("删除文件失败（将仅删除记录）", zap.String("path", absPath), zap.Error(err))
			}
		}
	}

	// 3. 物理删除数据库记录（本方法为物理删语义；软删仅作品软删链经 DeleteWithBackup/MarkInvalid 内部使用）
	if err := s.repo.DeleteUnscoped(ctx, id); err != nil {
		return backupId, err
	}
	return backupId, nil
}

// DeleteWithBackup 删除 store 文件——移入 backup 目录建保管清单行，同时软删记录并写入行内 backup_id
// （backup_id 与 deleted_at 单条 UPDATE 同生共死：记录状态如实反映文件去向，复原/彻底删除按行内引用定位备份）。
// 供作品软删除链使用，均在调用方事务外执行。
// 与 HardDelete(id, backup=true) 的契约区别：后者仅备份已完成文件、失败降级为直接删除、物理删记录；
// 本方法不看文件完成状态、失败返回错误中断调用方流程（保全优先——降级删除会销毁待复原文件）。
// 返回备份清单行 ID；0 = 无备份（记录不存在、路径无效、源文件缺失或未注入 FileMover，均不阻断）。
func (s *Service) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	// 查询失败（含脏数据 record not found）不阻断删除：返回 0，调用方按"未备份"跳过该条
	record, err := s.repo.GetById(ctx, id)
	if err != nil {
		logger.Log.Warn("备份前查询 store 记录失败，跳过", zap.Int64("storeId", id), zap.Error(err))
		return 0, nil
	}
	if record == nil || !record.FilePath.Valid {
		return 0, nil
	}

	// 文件操作前登记操作抑制：文件离开 store/ 会触发旧路径 Remove 事件（MoveBackup 汇点亦登记，此处同层加固）
	storeRegistry.Suppress(record.FilePath.String)
	defer storeRegistry.Release(record.FilePath.String)

	if s.fileMover == nil {
		logger.Log.Warn("未注入 FileMover，跳过备份（文件留存原地）", zap.Int64("storeId", id))
		return 0, s.repo.SoftDeleteWithBackup(ctx, id, 0)
	}
	workDir := s.getWorkDir()
	absPath := filepath.Join(workDir, record.FilePath.String)
	if !util.FileExists(absPath) {
		logger.Log.Warn("源文件不存在，跳过备份", zap.Int64("storeId", id), zap.String("path", absPath))
		return 0, s.repo.SoftDeleteWithBackup(ctx, id, 0)
	}

	backupId, err := s.fileMover.MoveToBackup(ctx, absPath)
	if err != nil {
		return 0, fmt.Errorf("移动 store 文件到备份失败: %w", err)
	}
	return backupId, s.repo.SoftDeleteWithBackup(ctx, id, backupId)
}

// SoftDeleteAndDiscardFile 软删记录并废弃其文件（未完成行进入软删产道的分支：partial 文件无复原价值，
// 移入 backup/ 只会膨胀备份目录）。文件尽力删（扑空容忍+操作抑制登记），软删经 SoftDeleteWithBackup
// 单点写入（backup_id=NULL）。完成后行复活时无文件，交文件监控对账裁决
func (s *Service) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error {
	record, err := s.repo.GetById(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if record == nil {
		return nil
	}
	if record.FilePath.Valid && record.FilePath.String != "" {
		s.CleanupFile(record.FilePath.String)
	}
	return s.repo.SoftDeleteWithBackup(ctx, id, 0)
}

// RestoreByIds 批量复活记录（清软删标志与 backup_id，复原链：文件还原回 store/ 后调用）
func (s *Service) RestoreByIds(ctx context.Context, ids []int64) error {
	return s.repo.RestoreByIds(ctx, ids)
}

// ListByIdsIncludeDeleted 按 ID 集合查记录行（含已删行；复原/彻底删除链取行内 backup_id 与 file_path）
func (s *Service) ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*domain.PersistentStore {
	if len(ids) == 0 {
		return []*domain.PersistentStore{}
	}
	idVals := make([]interface{}, len(ids))
	for i, id := range ids {
		idVals[i] = id
	}
	records, err := s.repo.List(ctx, &database.QueryOption{
		Conditions:     []clause.Expression{clause.IN{Column: "id", Values: idVals}},
		IncludeDeleted: true,
	})
	if err != nil {
		logger.Log.Warn("按 ID 批量查询 store 记录（含删）失败", zap.Error(err))
		return []*domain.PersistentStore{}
	}
	return records
}

// StoreFromExternal 将外部文件导入到 store 目录并创建 DB 记录
// PersistentStore 全权负责文件移动和 DB 记录的创建
// srcAbsPath: 外部源文件绝对路径（移动后源文件消失）
// relPath: 目标相对路径（相对于 {workDir}）
// fileName: 原始文件名
func (s *Service) StoreFromExternal(ctx context.Context, srcAbsPath string, relPath string, fileName string) (int64, error) {
	// 入口规范化为正斜杠（PATH_SEPARATOR_DISCIPLINE）：清旧/抑制登记/落库全程与 DB 基准一致
	relPath = filepath.ToSlash(relPath)
	// 1. 校验 relPath
	if err := storeRegistry.ValidatePath(relPath); err != nil {
		return 0, err
	}
	storeRegistry.Suppress(relPath)
	defer storeRegistry.Release(relPath)

	// 2. 确认源文件存在
	if _, err := os.Stat(srcAbsPath); err != nil {
		return 0, fmt.Errorf("源文件不存在: %s: %w", srcAbsPath, err)
	}

	workDir := s.getWorkDir()
	targetAbsPath := filepath.Join(workDir, relPath)

	// 3. 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(targetAbsPath), 0755); err != nil {
		return 0, fmt.Errorf("创建目录失败: %w", err)
	}

	// 4. 清理目标路径旧 store：先删同 file_path 的既有记录(含其磁盘文件)，再兜底删残留磁盘文件
	//    避免导入路径被既有 store 占用时 INSERT 触发 file_path UNIQUE 冲突
	if _, err := s.HardDeleteByFilePath(ctx, relPath, false); err != nil {
		return 0, fmt.Errorf("清理目标路径旧 store 记录失败: %w", err)
	}
	if _, err := os.Stat(targetAbsPath); err == nil {
		if err := os.Remove(targetAbsPath); err != nil {
			return 0, fmt.Errorf("删除已有文件失败: %w", err)
		}
	}

	// 5. 移动源文件到 store 目录（同文件系统 O(1)）
	if err := os.Rename(srcAbsPath, targetAbsPath); err != nil {
		// 跨文件系统时回退为复制
		logger.Log.Warn("移动文件失败（回退为复制）", zap.String("src", srcAbsPath), zap.String("dst", targetAbsPath), zap.Error(err))
		if copyErr := util.CopyFile(srcAbsPath, targetAbsPath); copyErr != nil {
			return 0, fmt.Errorf("移动文件失败，回退复制也失败: %w（原始移动错误: %v）", copyErr, err)
		}
		_ = os.Remove(srcAbsPath)
	}

	// 6. 提取扩展名
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = filepath.Ext(relPath)
	}

	// 7. 创建 PersistentStore 记录（已完成状态）
	store := domain.NewPersistentStore()
	store.FilePath.Valid = true
	store.FilePath.String = filepath.ToSlash(relPath)
	store.FileName.Valid = true
	store.FileName.String = fileName
	store.FilenameExtension.Valid = true
	store.FilenameExtension.String = ext
	store.CompletedAt = util.GetCurrentTimestamp()
	fillImageDimensions(store, workDir)
	s.fillFingerprint(store, targetAbsPath)

	if err := s.repo.Create(ctx, store); err != nil {
		return 0, fmt.Errorf("注册 PersistentStore 记录失败: %w", err)
	}

	return store.GetID(), nil
}

// HardDeleteByFilePath 根据路径删除记录及文件（物理删）
func (s *Service) HardDeleteByFilePath(ctx context.Context, filePath string, backup bool) (int64, error) {
	record, err := s.repo.GetByFilePath(ctx, filePath)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, nil
	}
	return s.HardDelete(ctx, record.GetID(), backup)
}

// Exists 检查文件是否存在（记录存在且磁盘文件存在）
func (s *Service) Exists(ctx context.Context, id int64) bool {
	record, err := s.repo.GetById(ctx, id)
	if err != nil || record == nil {
		return false
	}
	if !record.FilePath.Valid {
		return false
	}
	workDir := s.getWorkDir()
	absPath := filepath.Join(workDir, record.FilePath.String)
	_, err = os.Stat(absPath)
	return err == nil
}

// GetAbsPath 获取记录对应文件的绝对路径
func (s *Service) GetAbsPath(store *domain.PersistentStore) string {
	if store == nil || !store.FilePath.Valid {
		return ""
	}
	workDir := s.getWorkDir()
	return filepath.Join(workDir, store.FilePath.String)
}

// ResolveFileState 按相对路径解析记录状态（含已删行；活行优先，全删时取最新删代）
// completed = 文件曾完整落盘；deleted = 记录已软删（文件移 backup 或外部裁决失效）；
// backupId = 行内嵌的备份清单行 ID（0 = 无备份，行内存储 NULL=无备份），供已删分支定位备份文件
// 无记录时 completed=true 兜底（向后兼容：按磁盘文件 fallback，如 store/ 白名单内的非受管文件）
func (s *Service) ResolveFileState(ctx context.Context, relPath string) (completed bool, deleted bool, backupId int64) {
	activeOpt := &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "file_path", Value: relPath}},
		Limit:      1,
	}
	if records, err := s.repo.List(ctx, activeOpt); err == nil && len(records) > 0 {
		return records[0].CompletedAt > 0, false, records[0].BackupID.Int64
	}
	deletedOpt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "file_path", Value: relPath},
			clause.Expr{SQL: "deleted_at > 0"},
		},
		OrderBy:        []clause.Expression{clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "deleted_at"}, Desc: true}}}},
		IncludeDeleted: true,
		Limit:          1,
	}
	if records, err := s.repo.List(ctx, deletedOpt); err == nil && len(records) > 0 {
		return records[0].CompletedAt > 0, true, records[0].BackupID.Int64
	}
	return true, false, 0
}

// ResolveStorePath 解析存储相对路径为绝对路径
// relPath: 相对于 {workDir} 的路径
func (s *Service) ResolveStorePath(relPath string) string {
	if strings.TrimSpace(relPath) == "" {
		return ""
	}
	workDir := s.getWorkDir()
	return filepath.Join(workDir, relPath)
}

// BuildVariantPath 从源 store 相对路径生成变体路径：保留目录与扩展名，文件名追加 suffix 后净化。
// 用于在已有 store 旁派生新 store 的相对路径（如合并产物在源视频轨旁加 _merged）。
// 纯路径变换，不落盘、不查库；空源路径返回空串。
func (s *Service) BuildVariantPath(sourceRelPath, suffix string) string {
	if strings.TrimSpace(sourceRelPath) == "" {
		return ""
	}
	ext := filepath.Ext(sourceRelPath)
	base := strings.TrimSuffix(filepath.Base(sourceRelPath), ext)
	// filepath.Join 在 Windows 产生反斜杠；store 路径以正斜杠入库（跨平台规范），统一转为正斜杠
	return filepath.ToSlash(filepath.Join(filepath.Dir(sourceRelPath), filename.SanitizeFileName(base+suffix)+ext))
}

// Name 引用方展示名（实现 backupGovernance.BackupReferencer：监视哨统计分组与备份管理面板用）
func (s *Service) Name() string {
	return "作品存储"
}

// ListReferencedBackupIDs 全量行内引用的备份清单行 ID（实现 backupGovernance.BackupReferencer）。
// 含已删行——软删行是合法引用者（回收站待复原），漏含即活备份被治理误判无主清删
func (s *Service) ListReferencedBackupIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ListReferencedBackupIds(ctx)
}

// ClearBackupRefsByBackupIDs 按引用目标清 backup_id（实现 backupGovernance.BackupReferencer：
// 治理方算出悬空 ID 后调用）。含已删行
func (s *Service) ClearBackupRefsByBackupIDs(ctx context.Context, ids []int64) error {
	return s.repo.ClearBackupRefsByBackupIds(ctx, ids)
}

// ClearIllegalAliveBackupRefs 清活行携带备份引用的非法态列（实现
// backupGovernance.IllegalBackupRefSanitizer）。构造上不可达：backup_id 与 deleted_at 单条
// UPDATE 同生共死、复原双列同清——检出即外部直改数据库痕迹，返回受影响行数
func (s *Service) ClearIllegalAliveBackupRefs(ctx context.Context) (int64, error) {
	return s.repo.ClearIllegalAliveBackupRefs(ctx)
}
