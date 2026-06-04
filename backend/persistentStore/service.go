package persistentStore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/util"

	"go.uber.org/zap"
)

// Repository 文件持久存储仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存记录（Create 后通过指针回填 ID）
	Save(ctx context.Context, store *domain.PersistentStore) error
	// Update 更新记录
	Update(ctx context.Context, store *domain.PersistentStore) error
	// GetById 根据 ID 获取记录
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	// GetByFilePath 根据路径获取记录
	GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error)
	// Delete 删除记录
	Delete(ctx context.Context, id int64) error
	// ExistsByFilePath 检查文件路径是否已存在记录
	ExistsByFilePath(ctx context.Context, filePath string) bool
}

// Service 文件存取服务
type Service struct {
	repo Repository
}

// NewService 创建文件存取服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Store 存入文件
// relPath: 相对于 {workDir}/store/ 的路径（由调用方指定，必须匹配已注册子目录）
// fileName: 原始文件名
// reader: 文件内容
// 返回 persistent_store 记录 ID
func (s *Service) Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error) {
	// 1. 校验 relPath 是否匹配已注册子目录
	if err := validatePath(relPath); err != nil {
		return 0, err
	}

	workDir := util.RootPath()
	storeDir := filepath.Join(workDir, "store")
	absPath := filepath.Join(storeDir, relPath)

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

	written, err := io.Copy(file, reader)
	if err != nil {
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
		existing.FileSize.Valid = true
		existing.FileSize.Int64 = written
		if err := s.repo.Update(ctx, existing); err != nil {
			return 0, fmt.Errorf("更新记录失败: %w", err)
		}
		return existing.GetID(), nil
	}

	// 6. 创建记录
	store := domain.NewPersistentStore()
	store.FilePath.Valid = true
	store.FilePath.String = relPath
	store.FileName.Valid = true
	store.FileName.String = fileName
	store.FilenameExtension.Valid = true
	store.FilenameExtension.String = ext
	store.FileSize.Valid = true
	store.FileSize.Int64 = written

	if err := s.repo.Save(ctx, store); err != nil {
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

// GetByFilePath 根据路径获取记录
func (s *Service) GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error) {
	return s.repo.GetByFilePath(ctx, filePath)
}

// Delete 删除记录及对应文件（严格一致）
func (s *Service) Delete(ctx context.Context, id int64) error {
	// 1. 根据 ID 查询记录
	record, err := s.repo.GetById(ctx, id)
	if err != nil {
		return err
	}
	if record == nil {
		// 记录不存在，幂等返回
		return nil
	}

	// 2. 删除磁盘文件
	if record.FilePath.Valid {
		workDir := util.RootPath()
		absPath := filepath.Join(workDir, "store", record.FilePath.String)
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Log.Warn("删除文件失败（将仅删除记录）", zap.String("path", absPath), zap.Error(err))
		}
	}

	// 3. 删除数据库记录
	return s.repo.Delete(ctx, id)
}

// DeleteByFilePath 根据路径删除记录及文件
func (s *Service) DeleteByFilePath(ctx context.Context, filePath string) error {
	record, err := s.repo.GetByFilePath(ctx, filePath)
	if err != nil {
		return err
	}
	if record == nil {
		return nil
	}
	return s.Delete(ctx, record.GetID())
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
	workDir := util.RootPath()
	absPath := filepath.Join(workDir, "store", record.FilePath.String)
	_, err = os.Stat(absPath)
	return err == nil
}

// GetAbsPath 获取记录对应文件的绝对路径
func (s *Service) GetAbsPath(store *domain.PersistentStore) string {
	if store == nil || !store.FilePath.Valid {
		return ""
	}
	workDir := util.RootPath()
	return filepath.Join(workDir, "store", store.FilePath.String)
}

// ResolveStorePath 解析存储相对路径为绝对路径
// relPath: 相对于 {workDir}/store/ 的路径
func ResolveStorePath(relPath string) string {
	if strings.TrimSpace(relPath) == "" {
		return ""
	}
	workDir := util.RootPath()
	return filepath.Join(workDir, "store", relPath)
}
