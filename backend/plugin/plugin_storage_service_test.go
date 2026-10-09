package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/secretkey"
	"gorm.io/gorm"
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

// ListAllEncrypted 内存桩：按 Encrypted=true 过滤（跨插件）
func (m *mockStorageRepo) ListAllEncrypted(ctx context.Context) ([]*domain.PluginStorage, error) {
	var list []*domain.PluginStorage
	for _, e := range m.data {
		if e.Encrypted.Valid && e.Encrypted.Bool {
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

// UpdatesWithColumns 内存桩简化为整体替换（不模拟列集语义；隔离改名场景走真库迁移测试）
func (m *mockStorageRepo) UpdatesWithColumns(ctx context.Context, entity *domain.PluginStorage, columns []string) error {
	m.data[mockStorageKey(entity.PluginID, entity.Key)] = entity
	return nil
}

// newTestStorageCipher 测试加密提供方：secretkey.NewProvider 指向 t.TempDir() 密钥
// 文件（首用生成新钥落盘），与生产装配同构（v2 前缀分流 + legacy 回退解密真实覆盖）
func newTestStorageCipher(t *testing.T) *secretkey.Provider {
	t.Helper()
	p, err := secretkey.NewProvider(filepath.Join(t.TempDir(), "secret.key"))
	if err != nil {
		t.Fatalf("构造测试加密提供方失败: %v", err)
	}
	return p
}

func TestStoragePlainSetGet(t *testing.T) {
	svc := NewPluginStorageService(newMockStorageRepo(), newTestStorageCipher(t))
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
	svc := NewPluginStorageService(repo, newTestStorageCipher(t))
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
	if !strings.HasPrefix(stored.Value.String, cipherV2Prefix) {
		t.Errorf("新写入加密项应为 v2 密文, 实得 %q", stored.Value.String)
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
	svc := NewPluginStorageService(newMockStorageRepo(), newTestStorageCipher(t))
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
	svc := NewPluginStorageService(repo, newTestStorageCipher(t))
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
	svc := NewPluginStorageService(newMockStorageRepo(), newTestStorageCipher(t))
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

// legacy 密文样本（旧固定钥加密的裸 base64，无 v2: 前缀）——由阶段1核验的历史固定
// 钥一次性生成后嵌入，明文分别为 "legacy-secret-token" 与 ""（加密空串）
const (
	legacyCipherToken = "U7mK51CVRA/6oqMU9wFPkNb7fowbaFUXJiTHWqePpv71w6/Mv0RGKjnNpbdO9mkZF5GTbqfpKxptZWw="
	legacyCipherEmpty = "Ka+x/YdjR8i7/B2So5RqUSqQps9kbOtXlEN5kHijR5qqxkJyZ2HSjQ=="
)

// seedStorageRow 真库种子一行插件自存信息（经工厂构造，与生产写入形态一致）
func seedStorageRow(t *testing.T, db *gorm.DB, pluginID int64, key, value string, encrypted bool) {
	t.Helper()
	row := domain.NewPluginStorage()
	row.PluginID = pluginID
	row.Key = key
	row.Value = sql.NullString{String: value, Valid: true}
	row.Encrypted = sql.NullBool{Bool: encrypted, Valid: true}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("种子行 %s 落库失败: %v", key, err)
	}
}

// fetchStorageRow 真库读回一行（缺失即测试失败）
func fetchStorageRow(t *testing.T, db *gorm.DB, pluginID int64, key string) *domain.PluginStorage {
	t.Helper()
	var row domain.PluginStorage
	if err := db.Where("plugin_id = ? AND key = ?", pluginID, key).First(&row).Error; err != nil {
		t.Fatalf("读回行 %d/%s 失败: %v", pluginID, key, err)
	}
	return &row
}

// TestMigrateLegacyCiphertextsOnRealDB 真库迁移端到端：OpenTestDB 种子四类行
// （legacy 密文 / v2 密文 / 双钥不可解垃圾 / 明文）→ 跑迁移 → 断言 legacy 行重写为
// v2 且可解（含加密空串）、v2 行原样跳过、双败行隔离改名（key 加前缀 + Encrypted=false
// + 原密文保留）、明文行不触碰；二次执行零命中（幂等，行值不变）
func TestMigrateLegacyCiphertextsOnRealDB(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	cipher := newTestStorageCipher(t)
	svc := NewPluginStorageService(NewStorageRepository(db), cipher)
	ctx := context.Background()

	// 外键强制（OpenTestDB 与生产库同构）：plugin_storage.plugin_id 引用 plugin 表，
	// 种子前先建两个父行，后续种子挂其 ID
	pluginIDs := make([]int64, 2)
	for i, publicID := range []string{"com.storage.migrate.a", "com.storage.migrate.b"} {
		p := domain.NewPlugin()
		p.PublicID = sql.NullString{String: publicID, Valid: true}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("种子插件父行 %s 落库失败: %v", publicID, err)
		}
		pluginIDs[i] = p.GetID()
	}

	// v2 行种子值先固定（断言迁移不触碰）
	v2Cipher, err := cipher.Encrypt("already-v2-value")
	if err != nil {
		t.Fatalf("构造 v2 种子密文失败: %v", err)
	}
	seedStorageRow(t, db, pluginIDs[0], "legacyToken", legacyCipherToken, true)
	seedStorageRow(t, db, pluginIDs[0], "legacyEmpty", legacyCipherEmpty, true)
	seedStorageRow(t, db, pluginIDs[1], "freshV2", v2Cipher, true)
	seedStorageRow(t, db, pluginIDs[1], "brokenCipher", "!!!双钥均不可解!!!", true)
	seedStorageRow(t, db, pluginIDs[0], "plainRow", "明文不动", false)

	migrated, quarantined, err := svc.MigrateLegacyCiphertexts(ctx)
	if err != nil {
		t.Fatalf("迁移执行失败: %v", err)
	}
	if migrated != 2 || quarantined != 1 {
		t.Fatalf("首次迁移计数 = (%d migrated, %d quarantined), want (2, 1)", migrated, quarantined)
	}

	// legacy 行：重写为 v2 且可解回原明文
	tokenRow := fetchStorageRow(t, db, pluginIDs[0], "legacyToken")
	if !strings.HasPrefix(tokenRow.Value.String, cipherV2Prefix) {
		t.Errorf("legacyToken 迁移后应为 v2 密文, 实得 %q", tokenRow.Value.String)
	}
	if plain, derr := cipher.Decrypt(tokenRow.Value.String); derr != nil || plain != "legacy-secret-token" {
		t.Errorf("legacyToken 迁移后解密 = %q, err %v, want %q", plain, derr, "legacy-secret-token")
	}
	// 加密空串行：语义照常保留（v2 空密文解回空串）
	emptyRow := fetchStorageRow(t, db, pluginIDs[0], "legacyEmpty")
	if !strings.HasPrefix(emptyRow.Value.String, cipherV2Prefix) {
		t.Errorf("legacyEmpty 迁移后应为 v2 密文, 实得 %q", emptyRow.Value.String)
	}
	if plain, derr := cipher.Decrypt(emptyRow.Value.String); derr != nil || plain != "" {
		t.Errorf("legacyEmpty 迁移后解密 = %q, err %v, want 空串", plain, derr)
	}
	// v2 行：原值原样（逐字节不变）
	freshRow := fetchStorageRow(t, db, pluginIDs[1], "freshV2")
	if freshRow.Value.String != v2Cipher {
		t.Errorf("freshV2 迁移不应触碰, 实得 %q, want %q", freshRow.Value.String, v2Cipher)
	}
	// 双败行：隔离改名 + Encrypted=false + 原密文保留
	brokenRow := fetchStorageRow(t, db, pluginIDs[1], undecryptableKeyPrefix+"brokenCipher")
	if !brokenRow.Encrypted.Valid || brokenRow.Encrypted.Bool {
		t.Errorf("隔离行 Encrypted 应为 false, 实得 %+v", brokenRow.Encrypted)
	}
	if brokenRow.Value.String != "!!!双钥均不可解!!!" {
		t.Errorf("隔离行应保留原密文串, 实得 %q", brokenRow.Value.String)
	}
	// 明文行：不触碰（key/value/encrypted 全不动）
	plainRow := fetchStorageRow(t, db, pluginIDs[0], "plainRow")
	if plainRow.Value.String != "明文不动" || plainRow.Encrypted.Bool || !plainRow.Encrypted.Valid {
		t.Errorf("明文行迁移不应触碰, 实得 value=%q encrypted=%+v", plainRow.Value.String, plainRow.Encrypted)
	}

	// 幂等：二次执行零命中（隔离行已出扫描面，v2/明文行不命中），全部行值不变
	migrated2, quarantined2, err := svc.MigrateLegacyCiphertexts(ctx)
	if err != nil {
		t.Fatalf("二次迁移执行失败: %v", err)
	}
	if migrated2 != 0 || quarantined2 != 0 {
		t.Fatalf("二次迁移应零命中, 实得 (%d migrated, %d quarantined)", migrated2, quarantined2)
	}
	if again := fetchStorageRow(t, db, pluginIDs[0], "legacyToken"); again.Value.String != tokenRow.Value.String {
		t.Errorf("二次迁移后 legacyToken 值被改动: %q → %q", tokenRow.Value.String, again.Value.String)
	}
}
