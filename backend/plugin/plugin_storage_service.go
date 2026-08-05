package plugin

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
// 统一 KV 存储：明文项直接读写，加密项 Value 存密文（Encrypted=true），加解密由 util/crypto 承担。
// 每条值随写盖 schemaVersion（写入时的插件配置 schema 版本，由调用方传入）
type PluginStorageService struct {
	repo StorageRepository
}

// NewPluginStorageService 创建插件自存信息服务
func NewPluginStorageService(repo StorageRepository) *PluginStorageService {
	return &PluginStorageService{repo: repo}
}

// schemaVersionOf 从 entity 取 schema 版本（NullInt64→int32，无效/未设置为 0）
func schemaVersionOf(e *domain.PluginStorage) int32 {
	if e.SchemaVersion.Valid {
		return int32(e.SchemaVersion.Int64)
	}
	return 0
}

// decryptValue 解密存储值（加密项 util.Decrypt，明文直返）
func decryptValue(e *domain.PluginStorage) (string, error) {
	if e.Encrypted.Valid && e.Encrypted.Bool {
		return util.Decrypt(e.Value.String)
	}
	return e.Value.String, nil
}

// GetValue 读取自存信息，加密项自动解密，返回带 schema 版本；key 不存在返回 nil
func (s *PluginStorageService) GetValue(ctx context.Context, pluginID int64, key string) (*pluginsdkdto.StorageValue, error) {
	entity, err := s.repo.GetByKey(ctx, pluginID, key)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	val, err := decryptValue(entity)
	if err != nil {
		return nil, err
	}
	return &pluginsdkdto.StorageValue{Value: val, SchemaVersion: schemaVersionOf(entity)}, nil
}

// SetValue 写入明文自存信息；schemaVersion 为该值对应的配置 schema 版本（调用方传入，host 盖戳）
func (s *PluginStorageService) SetValue(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error {
	return s.saveEntry(ctx, pluginID, key, value, false, schemaVersion)
}

// SetValueEncrypted 写入加密自存信息（Value 经加密后存密文）
func (s *PluginStorageService) SetValueEncrypted(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error {
	encrypted, err := util.Encrypt(value)
	if err != nil {
		return err
	}
	return s.saveEntry(ctx, pluginID, key, encrypted, true, schemaVersion)
}

// DeleteValue 删除自存信息
func (s *PluginStorageService) DeleteValue(ctx context.Context, pluginID int64, key string) error {
	return s.repo.DeleteByKey(ctx, pluginID, key)
}

// GetAllValues 读取插件全部自存信息，加密项自动解密，返回带 schema 版本
func (s *PluginStorageService) GetAllValues(ctx context.Context, pluginID int64) (map[string]*pluginsdkdto.StorageValue, error) {
	entities, err := s.repo.ListByPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*pluginsdkdto.StorageValue, len(entities))
	for _, e := range entities {
		val, err := decryptValue(e)
		if err != nil {
			return nil, err
		}
		result[e.Key] = &pluginsdkdto.StorageValue{Value: val, SchemaVersion: schemaVersionOf(e)}
	}
	return result, nil
}

// saveEntry 写入一条自存信息，存在则更新；schemaVersion 随值落盘（host 盖戳）
func (s *PluginStorageService) saveEntry(ctx context.Context, pluginID int64, key, value string, encrypted bool, schemaVersion int64) error {
	existing, err := s.repo.GetByKey(ctx, pluginID, key)
	if err != nil {
		return err
	}
	sv := sql.NullInt64{Int64: schemaVersion, Valid: true}
	if existing != nil {
		existing.Value = sql.NullString{String: value, Valid: true}
		existing.Encrypted = sql.NullBool{Bool: encrypted, Valid: true}
		existing.SchemaVersion = sv
		return s.repo.Updates(ctx, existing)
	}
	entity := domain.NewPluginStorage()
	entity.PluginID = pluginID
	entity.Key = key
	entity.Value = sql.NullString{String: value, Valid: true}
	entity.Encrypted = sql.NullBool{Bool: encrypted, Valid: true}
	entity.SchemaVersion = sv
	return s.repo.Create(ctx, entity)
}
