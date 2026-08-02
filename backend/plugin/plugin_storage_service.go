package plugin

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// StorageRepository 插件自存信息仓储接口（由 Service 定义，Repository 实现）
type StorageRepository interface {
	GetByKey(ctx context.Context, pluginID int64, key string) (*domain.PluginStorage, error)
	ListByPlugin(ctx context.Context, pluginID int64) ([]*domain.PluginStorage, error)
	DeleteByKey(ctx context.Context, pluginID int64, key string) error
	Create(ctx context.Context, entity *domain.PluginStorage) error
	Updates(ctx context.Context, entity *domain.PluginStorage) error
}

// PluginStorageService 插件自存信息服务
// 统一 KV 存储：明文项直接读写，加密项 Value 存密文（Encrypted=true），加解密由 util/crypto 承担
type PluginStorageService struct {
	repo StorageRepository
}

// NewPluginStorageService 创建插件自存信息服务
func NewPluginStorageService(repo StorageRepository) *PluginStorageService {
	return &PluginStorageService{repo: repo}
}

// GetValue 读取自存信息，加密项自动解密
func (s *PluginStorageService) GetValue(ctx context.Context, pluginID int64, key string) (string, error) {
	entity, err := s.repo.GetByKey(ctx, pluginID, key)
	if err != nil {
		return "", err
	}
	if entity == nil {
		return "", nil
	}
	if entity.Encrypted.Valid && entity.Encrypted.Bool {
		return util.Decrypt(entity.Value.String)
	}
	return entity.Value.String, nil
}

// SetValue 写入明文自存信息
func (s *PluginStorageService) SetValue(ctx context.Context, pluginID int64, key, value string) error {
	return s.saveEntry(ctx, pluginID, key, value, false)
}

// SetValueEncrypted 写入加密自存信息（Value 经加密后存密文）
func (s *PluginStorageService) SetValueEncrypted(ctx context.Context, pluginID int64, key, value string) error {
	encrypted, err := util.Encrypt(value)
	if err != nil {
		return err
	}
	return s.saveEntry(ctx, pluginID, key, encrypted, true)
}

// DeleteValue 删除自存信息
func (s *PluginStorageService) DeleteValue(ctx context.Context, pluginID int64, key string) error {
	return s.repo.DeleteByKey(ctx, pluginID, key)
}

// GetAllValues 读取插件全部自存信息，加密项自动解密
func (s *PluginStorageService) GetAllValues(ctx context.Context, pluginID int64) (map[string]string, error) {
	entities, err := s.repo.ListByPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(entities))
	for _, e := range entities {
		if e.Encrypted.Valid && e.Encrypted.Bool {
			decrypted, err := util.Decrypt(e.Value.String)
			if err != nil {
				return nil, err
			}
			result[e.Key] = decrypted
		} else {
			result[e.Key] = e.Value.String
		}
	}
	return result, nil
}

// saveEntry 写入一条自存信息，存在则更新
func (s *PluginStorageService) saveEntry(ctx context.Context, pluginID int64, key, value string, encrypted bool) error {
	existing, err := s.repo.GetByKey(ctx, pluginID, key)
	if err != nil {
		return err
	}
	if existing != nil {
		existing.Value = sql.NullString{String: value, Valid: true}
		existing.Encrypted = sql.NullBool{Bool: encrypted, Valid: true}
		return s.repo.Updates(ctx, existing)
	}
	entity := domain.NewPluginStorage()
	entity.PluginID = pluginID
	entity.Key = key
	entity.Value = sql.NullString{String: value, Valid: true}
	entity.Encrypted = sql.NullBool{Bool: encrypted, Valid: true}
	return s.repo.Create(ctx, entity)
}
