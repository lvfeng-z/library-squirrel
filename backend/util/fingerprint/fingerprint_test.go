package fingerprint

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestHeadComputerDigestFormat 验证 digest 格式为 "<size>:<hex>"（落库与匹配口径不可变）
func TestHeadComputerDigestFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	content := []byte("hello fingerprint")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	c := NewHeadComputer()
	fp, err := c.Fingerprint(context.Background(), path)
	if err != nil {
		t.Fatalf("Fingerprint 返回错误: %v", err)
	}
	if fp.Size != int64(len(content)) {
		t.Errorf("Size = %d, 期望 %d", fp.Size, len(content))
	}
	parts := strings.SplitN(fp.Digest, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("Digest %q 不符合 <size>:<hex> 格式", fp.Digest)
	}
	if want := strconv.Itoa(len(content)); parts[0] != want {
		t.Errorf("Digest size 段 = %q, 期望 %s", parts[0], want)
	}
}

// TestHeadComputerStableAndSensitive 验证稳定性（相同内容一致）与敏感性（头部字节变化则变）
func TestHeadComputerStableAndSensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	content := []byte("the quick brown fox") // 19 字节
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	c := NewHeadComputer()
	fp1, err := c.Fingerprint(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := c.Fingerprint(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if fp1.Digest != fp2.Digest {
		t.Errorf("相同内容指纹不稳定: %q vs %q", fp1.Digest, fp2.Digest)
	}
	// 改首字节 t→T（size 不变 19），头部变化须检出
	if err := os.WriteFile(path, []byte("The quick brown fox"), 0644); err != nil {
		t.Fatal(err)
	}
	fp3, err := c.Fingerprint(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if fp3.Digest == fp1.Digest {
		t.Errorf("头部字节变化后指纹未变: %q", fp3.Digest)
	}
}

// TestHeadComputerMissingFile 验证文件不存在时返回错误
func TestHeadComputerMissingFile(t *testing.T) {
	c := NewHeadComputer()
	_, err := c.Fingerprint(context.Background(), filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("期望返回错误，实际 nil")
	}
}
