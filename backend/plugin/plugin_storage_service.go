package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

const (
	// cipherV2Prefix v2 密文格式前缀，与 secretkey 包的 v2 格式一致——跨包协议级
	// 耦合（密文格式变更须两处同步），迁移逐行以前缀判定形态、无需迁移标记
	cipherV2Prefix = "v2:"
	// undecryptableKeyPrefix 双钥均解密失败的存量密文行的隔离改名前缀：加前缀
	// 后 Encrypted 置 false（不再进读取/迁移扫描面），原密文串保留（不物理删除，
	// 杜绝迁移缺陷误删好数据的不可逆路径）
	undecryptableKeyPrefix = "__undecryptable__"
)

// StorageRepository 插件自存信息仓储接口（由 Service 定义，Repository 实现）
type StorageRepository interface {
	GetByKey(ctx context.Context, pluginID int64, key string) (*domain.PluginStorage, error)
	ListByPlugin(ctx context.Context, pluginID int64) ([]*domain.PluginStorage, error)
	// ListAllEncrypted 列出全部加密行（跨插件，Encrypted=true），供存量密文迁移扫描
	ListAllEncrypted(ctx context.Context) ([]*domain.PluginStorage, error)
	DeleteByKey(ctx context.Context, pluginID int64, key string) error
	Create(ctx context.Context, entity *domain.PluginStorage) error
	Updates(ctx context.Context, entity *domain.PluginStorage) error
	// UpdatesWithColumns 按列集更新（强制写零值列），隔离改名置 Encrypted=false 时用
	UpdatesWithColumns(ctx context.Context, entity *domain.PluginStorage, columns []string) error
}

// StorageCipher 存储值加解密能力（由 plugin 包定义，secretkey.Provider 结构性实现：
// 加密产 v2 密文，解密按前缀分流——v2 用当前钥、旧格式裸 base64 用 legacy 固定钥回退）
type StorageCipher interface {
	Encrypt(plainText string) (string, error)
	Decrypt(cipherText string) (string, error)
}

// PluginStorageService 插件自存信息服务
// 统一 KV 存储：明文项直接读写，加密项 Value 存密文（Encrypted=true），加解密经注入的
// StorageCipher 承担。每条值随写盖 schemaVersion（写入时的插件配置 schema 版本，由调用方传入）
type PluginStorageService struct {
	repo   StorageRepository
	cipher StorageCipher
}

// NewPluginStorageService 创建插件自存信息服务（cipher 为存储加解密提供方，通常为
// secretkey.NewProvider 构造的实例）
func NewPluginStorageService(repo StorageRepository, cipher StorageCipher) *PluginStorageService {
	return &PluginStorageService{repo: repo, cipher: cipher}
}

// schemaVersionOf 从 entity 取 schema 版本（NullInt64→int32，无效/未设置为 0）
func schemaVersionOf(e *domain.PluginStorage) int32 {
	if e.SchemaVersion.Valid {
		return int32(e.SchemaVersion.Int64)
	}
	return 0
}

// decryptValue 解密存储值（加密项经注入 cipher 解密，明文直返）
func (s *PluginStorageService) decryptValue(e *domain.PluginStorage) (string, error) {
	if e.Encrypted.Valid && e.Encrypted.Bool {
		return s.cipher.Decrypt(e.Value.String)
	}
	return e.Value.String, nil
}

// MigrateLegacyCiphertexts 启动期一次性迁移存量加密行（跨插件）：无 v2: 前缀的 legacy
// 密文（旧固定钥裸 base64）解密后以当前钥重写为 v2；已具 v2: 前缀的行跳过（幂等：逐行
// 前缀判定，无需迁移标记，二次执行零命中）；双钥均解密失败的行隔离改名——key 加
// __undecryptable__ 前缀、Encrypted 置 false、原密文串保留。明文行（Encrypted=false）不
// 在扫描面，迁移不触碰；加密空串语义照常保留（空明文重写为 v2 空密文）。
// 单行失败（重加密/落库失败、隔离改名撞唯一索引）不阻断——Warn 记录后跳过，行保持
// legacy 形态供下次启动重试；err 仅在扫描失败时非nil（启动期数据修复语义，整体 error
// 由调用方记日志、不阻断启动）
func (s *PluginStorageService) MigrateLegacyCiphertexts(ctx context.Context) (migrated, quarantined int, err error) {
	rows, err := s.repo.ListAllEncrypted(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("扫描存量加密行失败: %w", err)
	}
	for _, row := range rows {
		if strings.HasPrefix(row.Value.String, cipherV2Prefix) {
			continue
		}
		plain, derr := s.cipher.Decrypt(row.Value.String)
		if derr != nil {
			oldKey := row.Key
			row.Key = undecryptableKeyPrefix + row.Key
			row.Encrypted = sql.NullBool{Bool: false, Valid: true}
			if uerr := s.repo.UpdatesWithColumns(ctx, row, []string{"key", "encrypted"}); uerr != nil {
				logger.Log.Warnw("存量密文隔离改名失败（跳过，下次启动重试）",
					"pluginId", row.PluginID, "key", oldKey, "error", uerr)
				continue
			}
			quarantined++
			logger.Log.Warnw("存量密文双钥均不可解，已隔离改名（原密文保留）",
				"pluginId", row.PluginID, "key", oldKey)
			continue
		}
		rewritten, eerr := s.cipher.Encrypt(plain)
		if eerr != nil {
			logger.Log.Warnw("存量密文重加密失败（跳过，下次启动重试）",
				"pluginId", row.PluginID, "key", row.Key, "error", eerr)
			continue
		}
		row.Value = sql.NullString{String: rewritten, Valid: true}
		if uerr := s.repo.Updates(ctx, row); uerr != nil {
			logger.Log.Warnw("存量密文重写落库失败（跳过，下次启动重试）",
				"pluginId", row.PluginID, "key", row.Key, "error", uerr)
			continue
		}
		migrated++
	}
	return migrated, quarantined, nil
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
	val, err := s.decryptValue(entity)
	if err != nil {
		return nil, err
	}
	return &pluginsdkdto.StorageValue{Value: val, SchemaVersion: schemaVersionOf(entity)}, nil
}

// SetValue 写入明文自存信息；schemaVersion 为该值对应的配置 schema 版本（调用方传入，host 盖戳）
func (s *PluginStorageService) SetValue(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error {
	return s.saveEntry(ctx, pluginID, key, value, false, schemaVersion)
}

// SetValueEncrypted 写入加密自存信息（Value 经注入 cipher 加密后存密文）
func (s *PluginStorageService) SetValueEncrypted(ctx context.Context, pluginID int64, key, value string, schemaVersion int64) error {
	encrypted, err := s.cipher.Encrypt(value)
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
		val, err := s.decryptValue(e)
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
