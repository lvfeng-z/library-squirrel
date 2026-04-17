package secureStorage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/library-squirrel/wails/internal/database"
	"golang.org/x/crypto/nacl/secretbox"

	domain "github.com/library-squirrel/wails/internal/model"
)

// 错误定义
var (
	ErrNotAvailable  = errors.New("secure storage not available")
	ErrEncryptFailed = errors.New("encrypt failed")
	ErrDecryptFailed = errors.New("decrypt failed")
	ErrInvalidKey    = errors.New("invalid storage key")
	ErrKeyNotFound   = errors.New("key not found")
)

// Service 安全存储服务
type Service struct {
	repo Repository
}

// NewService 创建安全存储服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Repository 仓储接口
type RepositoryInterface interface {
	Save(ctx context.Context, entity *domain.SecureStorage) error
	Update(ctx context.Context, entity *domain.SecureStorage) error
	Delete(ctx context.Context, id int64) error
	DeleteByKey(ctx context.Context, storageKey string) (int64, error)
	GetById(ctx context.Context, id int64) (*domain.SecureStorage, error)
	GetByKey(ctx context.Context, storageKey string) (*domain.SecureStorage, error)
}

// Set 存储加密值
func (s *Service) Set(ctx context.Context, storageKey string, plainValue string, description string) (int64, error) {
	if storageKey == "" {
		return 0, ErrInvalidKey
	}

	encryptedValue, err := encrypt(plainValue)
	if err != nil {
		return 0, err
	}

	// 检查是否已存在
	existing, err := s.repo.GetByKey(ctx, storageKey)
	if err != nil {
		return 0, err
	}

	now := time.Now().UnixMilli()
	if existing != nil {
		// 更新
		existing.EncryptedValue = sql.NullString{String: encryptedValue, Valid: true}
		existing.Description = sql.NullString{String: description, Valid: true}
		existing.UpdateTime = now
		if err := s.repo.Update(ctx, existing); err != nil {
			return 0, err
		}
		return existing.ID, nil
	} else {
		// 新增
		entity := &domain.SecureStorage{
			StorageKey:     sql.NullString{String: storageKey, Valid: true},
			EncryptedValue: sql.NullString{String: encryptedValue, Valid: true},
			Description:    sql.NullString{String: description, Valid: true},
			CreateTime:     now,
			UpdateTime:     now,
		}
		if err := s.repo.Save(ctx, entity); err != nil {
			return 0, err
		}
		return entity.ID, nil
	}
}

// GetValue 获取解密后的值
func (s *Service) GetValue(ctx context.Context, storageKey string) (string, error) {
	if storageKey == "" {
		return "", ErrInvalidKey
	}

	entity, err := s.repo.GetByKey(ctx, storageKey)
	if err != nil {
		return "", err
	}
	if entity == nil || !entity.EncryptedValue.Valid || entity.EncryptedValue.String == "" {
		return "", nil
	}

	return decrypt(entity.EncryptedValue.String)
}

// Remove 删除存储键
func (s *Service) Remove(ctx context.Context, storageKey string) (int64, error) {
	if storageKey == "" {
		return 0, ErrInvalidKey
	}

	return s.repo.DeleteByKey(ctx, storageKey)
}

// HasKey 检查存储键是否存在
func (s *Service) HasKey(ctx context.Context, storageKey string) (bool, error) {
	if storageKey == "" {
		return false, ErrInvalidKey
	}

	entity, err := s.repo.GetByKey(ctx, storageKey)
	if err != nil {
		return false, err
	}
	return entity != nil, nil
}

// ListKeys 获取所有存储键
func (s *Service) ListKeys(ctx context.Context) ([]string, error) {
	results, err := s.repo.List(ctx, &database.QueryOption{})
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(results))
	for _, item := range results {
		if item.StorageKey.Valid && item.StorageKey.String != "" {
			keys = append(keys, item.StorageKey.String)
		}
	}
	return keys, nil
}

// StoreAndGetKey 存储加密值并返回存储键
func (s *Service) StoreAndGetKey(ctx context.Context, plainValue string, description string) (string, error) {
	// 生成唯一存储键
	storageKey := uuid.New().String()

	encryptedValue, err := encrypt(plainValue)
	if err != nil {
		return "", err
	}

	now := time.Now().UnixMilli()
	entity := &domain.SecureStorage{
		StorageKey:     sql.NullString{String: storageKey, Valid: true},
		EncryptedValue: sql.NullString{String: encryptedValue, Valid: true},
		Description:    sql.NullString{String: description, Valid: true},
		CreateTime:     now,
		UpdateTime:     now,
	}

	if err := s.repo.Save(ctx, entity); err != nil {
		return "", err
	}

	return storageKey, nil
}

// GetValueByKey 根据存储键获取解密后的值
func (s *Service) GetValueByKey(ctx context.Context, storageKey string) (string, error) {
	return s.GetValue(ctx, storageKey)
}

// ========== 加密解密实现 ==========

// 32字节密钥，用于 NaCl secretbox
// 注意：在生产环境中，应该从安全的密钥存储中获取
var encryptionKey [32]byte

func init() {
	// 使用一个固定密钥（实际应用中应从安全存储获取）
	// 这是为了与 Electron safeStorage 的行为类似，后者使用系统级密钥
	copy(encryptionKey[:], []byte("github.com/library-squirrel/wails-secure-key-32byte!"))
}

// encrypt 加密字符串
func encrypt(plainText string) (string, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ErrEncryptFailed
	}

	encrypted := secretbox.Seal(nil, []byte(plainText), &nonce, &encryptionKey)

	// 组合 nonce + encrypted 并进行 base64 编码
	result := make([]byte, 24+len(encrypted))
	copy(result[:24], nonce[:])
	copy(result[24:], encrypted)

	return base64.StdEncoding.EncodeToString(result), nil
}

// decrypt 解密字符串
func decrypt(encryptedBase64 string) (string, error) {
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", ErrDecryptFailed
	}

	if len(encryptedBytes) < 24 {
		return "", ErrDecryptFailed
	}

	var nonce [24]byte
	copy(nonce[:], encryptedBytes[:24])
	encrypted := encryptedBytes[24:]

	decrypted, ok := secretbox.Open(nil, encrypted, &nonce, &encryptionKey)
	if !ok {
		return "", ErrDecryptFailed
	}

	return string(decrypted), nil
}
