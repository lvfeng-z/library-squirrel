package secretkey

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/library-squirrel/backend/base/logger"
)

// TestMain 测试进程无 logger.Init，置 nop 防日志调用 panic
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// withHooks 注入装载/持久化桩并登记还原（钩子为包级 var，测试间须隔离）；
// 未提供的桩被触发时判测试失败
func withHooks(t *testing.T, load func(string) ([keySize]byte, error), save func(string, [keySize]byte) error) {
	t.Helper()
	if load == nil {
		load = func(string) ([keySize]byte, error) {
			t.Error("不应触发密钥装载")
			return [keySize]byte{}, errors.New("不应装载")
		}
	}
	if save == nil {
		save = func(string, [keySize]byte) error {
			t.Error("不应触发密钥落盘")
			return nil
		}
	}
	origLoad, origSave := loadKey, saveKey
	loadKey, saveKey = load, save
	t.Cleanup(func() { loadKey, saveKey = origLoad, origSave })
}

// withWarnLog 以 Warn 级 observer 替换全局日志，返回采集器
func withWarnLog(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	old := logger.Log
	core, logs := observer.New(zapcore.WarnLevel)
	logger.Log = zap.New(core).Sugar()
	t.Cleanup(func() { logger.Log = old })
	return logs
}

// fixedKey 构造内容确定的 32 字节测试钥
func fixedKey(seed byte) [keySize]byte {
	var key [keySize]byte
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

// sealWithKey 以指定钥构造裸 base64(nonce + secretbox 密文)——旧格式密文样本
func sealWithKey(t *testing.T, plain string, key *[keySize]byte) string {
	t.Helper()
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatalf("读取随机源失败: %v", err)
	}
	sealed := secretbox.Seal(nil, []byte(plain), &nonce, key)
	combined := make([]byte, 0, nonceSize+len(sealed))
	combined = append(combined, nonce[:]...)
	combined = append(combined, sealed...)
	return base64.StdEncoding.EncodeToString(combined)
}

// TestEncryptV2PrefixAndDecryptRoundTrip 新钥加密产出 v2: 前缀密文且解密还原；
// 相同明文两次加密因随机 nonce 产出不同密文
func TestEncryptV2PrefixAndDecryptRoundTrip(t *testing.T) {
	key := fixedKey(1)
	withHooks(t, func(string) ([keySize]byte, error) { return key, nil }, nil)
	p, err := NewProvider("irrelevant.key")
	if err != nil {
		t.Fatalf("构造 Provider 失败: %v", err)
	}

	const plain = "机密明文 hello"
	cipher, err := p.Encrypt(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if !strings.HasPrefix(cipher, v2Prefix) {
		t.Fatalf("密文缺少 %q 前缀: %s", v2Prefix, cipher)
	}

	restored, err := p.Decrypt(cipher)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if restored != plain {
		t.Fatalf("解密还原不符: 期望 %q, 实得 %q", plain, restored)
	}

	again, err := p.Encrypt(plain)
	if err != nil {
		t.Fatalf("二次加密失败: %v", err)
	}
	if again == cipher {
		t.Fatalf("相同明文两次加密应产出不同密文（随机 nonce）")
	}
}

// TestDecryptLegacyBareCiphertext 旧格式密文（裸 base64、旧固定钥）可回退解密；
// 样本以历史固定钥串独立整串拷贝构造（截断为 32 字节），与 legacy 钥实现互为印证
func TestDecryptLegacyBareCiphertext(t *testing.T) {
	withHooks(t, func(string) ([keySize]byte, error) { return fixedKey(2), nil }, nil)
	p, err := NewProvider("irrelevant.key")
	if err != nil {
		t.Fatalf("构造 Provider 失败: %v", err)
	}

	var historical [keySize]byte
	copy(historical[:], []byte("github.com/library-squirrel/wails-secure-key-32byte!"))
	if historical != legacyKey() {
		t.Fatalf("legacy 钥与历史固定钥（整串拷贝截断）不一致")
	}

	const plain = "旧格式机密数据"
	legacyCipher := sealWithKey(t, plain, &historical)
	restored, err := p.Decrypt(legacyCipher)
	if err != nil {
		t.Fatalf("旧格式密文解密失败: %v", err)
	}
	if restored != plain {
		t.Fatalf("旧格式密文解密还原不符: 期望 %q, 实得 %q", plain, restored)
	}
}

// TestGenerateAndPersistWhenKeyFileMissing 密钥文件不存在时生成 32 字节随机钥
// 并落盘（桩捕获），Provider 持有的即落盘的那把钥
func TestGenerateAndPersistWhenKeyFileMissing(t *testing.T) {
	const keyPath = "generated.key"
	saveCalls := 0
	var savedPath string
	var savedKey [keySize]byte
	withHooks(t,
		func(string) ([keySize]byte, error) { return [keySize]byte{}, fs.ErrNotExist },
		func(path string, key [keySize]byte) error {
			saveCalls++
			savedPath, savedKey = path, key
			return nil
		})

	p, err := NewProvider(keyPath)
	if err != nil {
		t.Fatalf("构造 Provider 失败: %v", err)
	}
	if saveCalls != 1 {
		t.Fatalf("应恰好落盘一次新生成密钥, 实得 %d 次", saveCalls)
	}
	if savedPath != keyPath {
		t.Fatalf("落盘路径不符: 期望 %q, 实得 %q", keyPath, savedPath)
	}
	if savedKey == [keySize]byte{} {
		t.Fatalf("生成的密钥不应为全零")
	}

	const plain = "生成后的数据"
	cipher, err := p.Encrypt(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	restored, err := openSealed(strings.TrimPrefix(cipher, v2Prefix), &savedKey)
	if err != nil {
		t.Fatalf("以落盘密钥解密失败: %v", err)
	}
	if restored != plain {
		t.Fatalf("Provider 未使用落盘的密钥: 期望 %q, 实得 %q", plain, restored)
	}
}

// TestRegenerateWhenLoadKeyFails 密钥文件存在但装载失败（DPAPI 解封失败/文件
// 损坏类错误）时不阻断：记 Warn 日志、重生成新钥并落盘
func TestRegenerateWhenLoadKeyFails(t *testing.T) {
	logs := withWarnLog(t)
	loadErr := errors.New("DPAPI 解封失败: 模拟损坏")
	saveCalls := 0
	var savedKey [keySize]byte
	withHooks(t,
		func(string) ([keySize]byte, error) { return [keySize]byte{}, loadErr },
		func(path string, key [keySize]byte) error {
			saveCalls++
			savedKey = key
			return nil
		})

	// 目录不存在的路径：重生成分支的移除旧文件步骤对不存在的路径静默容忍
	p, err := NewProvider("no-such-dir-9f3a/corrupt.key")
	if err != nil {
		t.Fatalf("装载失败应重生成而非报错: %v", err)
	}
	if saveCalls != 1 {
		t.Fatalf("应重新生成并落盘一次新钥, 实得 %d 次", saveCalls)
	}
	if savedKey == [keySize]byte{} {
		t.Fatalf("重新生成的密钥不应为全零")
	}

	warned := false
	for _, entry := range logs.All() {
		if entry.Level == zapcore.WarnLevel && strings.Contains(entry.Message, "重新生成新钥") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("应记录含「重新生成新钥」的 Warn 日志, 实得 %v", logs.All())
	}

	const plain = "重生成的钥加密"
	cipher, err := p.Encrypt(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if restored, err := p.Decrypt(cipher); err != nil || restored != plain {
		t.Fatalf("重生成密钥加解密不可用: 还原 %q, 错误 %v", restored, err)
	}
}

// TestAdoptExistingKeyOnConcurrentCreate 双实例并发竞争落盘（saveKey 撞
// fs.ErrExist）时弃用本进程生成的钥、回读并采用既有文件中的钥
func TestAdoptExistingKeyOnConcurrentCreate(t *testing.T) {
	existing := fixedKey(7)
	loadCalls := 0
	withHooks(t,
		func(string) ([keySize]byte, error) {
			loadCalls++
			if loadCalls == 1 {
				return [keySize]byte{}, fs.ErrNotExist
			}
			return existing, nil
		},
		func(string, [keySize]byte) error { return fs.ErrExist })

	p, err := NewProvider("race.key")
	if err != nil {
		t.Fatalf("构造 Provider 失败: %v", err)
	}
	if loadCalls != 2 {
		t.Fatalf("落盘竞争后应回读既有密钥文件（共装载两次）, 实得 %d 次", loadCalls)
	}

	const plain = "竞争采用既有钥"
	cipher, err := p.Encrypt(plain)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	restored, err := openSealed(strings.TrimPrefix(cipher, v2Prefix), &existing)
	if err != nil {
		t.Fatalf("以既有密钥解密失败: %v", err)
	}
	if restored != plain {
		t.Fatalf("Provider 未采用既有密钥: 期望 %q, 实得 %q", plain, restored)
	}
}

// TestDecryptFailures 非法密文（非 base64、短于 nonce、他钥密文）统一返回
// ErrDecryptFailed
func TestDecryptFailures(t *testing.T) {
	withHooks(t, func(string) ([keySize]byte, error) { return fixedKey(3), nil }, nil)
	p, err := NewProvider("irrelevant.key")
	if err != nil {
		t.Fatalf("构造 Provider 失败: %v", err)
	}

	cases := []string{
		"!!!非法base64!!!",
		base64.StdEncoding.EncodeToString(make([]byte, 10)), // 短于 nonce
	}
	for _, c := range cases {
		if _, err := p.Decrypt(c); !errors.Is(err, ErrDecryptFailed) {
			t.Fatalf("非法密文 %q 应返回 ErrDecryptFailed, 实得 %v", c, err)
		}
	}

	otherKey := fixedKey(9)
	foreignV2 := v2Prefix + sealWithKey(t, "他钥数据", &otherKey)
	if _, err := p.Decrypt(foreignV2); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("他钥 v2 密文应返回 ErrDecryptFailed, 实得 %v", err)
	}
	foreignBare := sealWithKey(t, "他钥旧格式数据", &otherKey)
	if _, err := p.Decrypt(foreignBare); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("他钥裸 base64 密文应返回 ErrDecryptFailed, 实得 %v", err)
	}
}
