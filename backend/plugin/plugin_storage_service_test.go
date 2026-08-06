package plugin

import (
	"context"
	"fmt"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// mockStorageRepo 内存模拟 StorageRepository
type mockStorageRepo struct {
	data      map[string]*domain.PluginStorage
	idCounter int64
}

func newMockStorageRepo() *mockStorageRepo {
	return &mockStorageRepo{data: make(map[string]*domain.PluginStorage)}
}

func mockStorageKey(pluginID int64, key string) string {
	return fmt.Sprintf("%d:%s", pluginID, key)
}

func (m *mockStorageRepo) GetByKey(ctx context.Context, pluginID int64, key string) (*domain.PluginStorage, error) {
	if e, ok := m.data[mockStorageKey(pluginID, key)]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, nil
}

func (m *mockStorageRepo) ListByPlugin(ctx context.Context, pluginID int64) ([]*domain.PluginStorage, error) {
	var list []*domain.PluginStorage
	for _, e := range m.data {
		if e.PluginID == pluginID {
			cp := *e
			list = append(list, &cp)
		}
	}
	return list, nil
}

func (m *mockStorageRepo) DeleteByKey(ctx context.Context, pluginID int64, key string) error {
	delete(m.data, mockStorageKey(pluginID, key))
	return nil
}

func (m *mockStorageRepo) Create(ctx context.Context, entity *domain.PluginStorage) error {
	m.idCounter++
	entity.SetID(m.idCounter)
	m.data[mockStorageKey(entity.PluginID, entity.Key)] = entity
	return nil
}

func (m *mockStorageRepo) Updates(ctx context.Context, entity *domain.PluginStorage) error {
	m.data[mockStorageKey(entity.PluginID, entity.Key)] = entity
	return nil
}

func TestStoragePlainSetGet(t *testing.T) {
	svc := NewPluginStorageService(newMockStorageRepo())
	ctx := context.Background()
	if err := svc.SetValue(ctx, 1, "path", "/data", 1); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetValue(ctx, 1, "path")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Value != "/data" {
		t.Errorf("明文 GetValue = %+v, want Value %q", got, "/data")
	}
}

func TestStorageEncryptedSetGet(t *testing.T) {
	repo := newMockStorageRepo()
	svc := NewPluginStorageService(repo)
	ctx := context.Background()
	secret := "super-secret-token"
	if err := svc.SetValueEncrypted(ctx, 2, "accessToken", secret, 1); err != nil {
		t.Fatal(err)
	}
	// 底层存储的 Value 应为密文，且 Encrypted 标记为 true
	stored := repo.data[mockStorageKey(2, "accessToken")]
	if stored.Value.Valid && stored.Value.String == secret {
		t.Error("加密项底层 Value 不应是明文")
	}
	if !stored.Encrypted.Bool {
		t.Error("加密项 Encrypted 标记应为 true")
	}
	// GetValue 解密返回原文
	got, err := svc.GetValue(ctx, 2, "accessToken")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Value != secret {
		t.Errorf("加密项 GetValue = %+v, want Value %q", got, secret)
	}
}

func TestStorageGetAllMixed(t *testing.T) {
	svc := NewPluginStorageService(newMockStorageRepo())
	ctx := context.Background()
	svc.SetValue(ctx, 1, "plain", "hello", 1)
	svc.SetValueEncrypted(ctx, 1, "secret", "world", 1)
	all, err := svc.GetAllValues(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("GetAllValues 返回 %d 项, want 2", len(all))
	}
	if all["plain"] == nil || all["plain"].Value != "hello" {
		t.Errorf("plain = %+v, want Value hello", all["plain"])
	}
	if all["secret"] == nil || all["secret"].Value != "world" {
		t.Errorf("secret = %+v, want Value world", all["secret"])
	}
}

func TestStorageUpdateExisting(t *testing.T) {
	repo := newMockStorageRepo()
	svc := NewPluginStorageService(repo)
	ctx := context.Background()
	svc.SetValue(ctx, 1, "k", "v1", 1)
	svc.SetValue(ctx, 1, "k", "v2", 1)
	if len(repo.data) != 1 {
		t.Errorf("同 key 二次写入应更新而非新增, got %d 条", len(repo.data))
	}
	got, _ := svc.GetValue(ctx, 1, "k")
	if got == nil || got.Value != "v2" {
		t.Errorf("更新后 GetValue = %+v, want Value v2", got)
	}
}

func TestStorageDeleteAndMissing(t *testing.T) {
	svc := NewPluginStorageService(newMockStorageRepo())
	ctx := context.Background()
	// 不存在的 key 应返回 nil 无错误
	got, err := svc.GetValue(ctx, 1, "nope")
	if err != nil || got != nil {
		t.Errorf("不存在 key 应返回 nil 无错, got %+v err %v", got, err)
	}
	svc.SetValue(ctx, 1, "k", "v", 1)
	if err := svc.DeleteValue(ctx, 1, "k"); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetValue(ctx, 1, "k")
	if got != nil {
		t.Errorf("删除后应返回 nil, got %+v", got)
	}
}
