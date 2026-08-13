package fsmonitor

import (
	"context"
	"testing"
)

// stubFingerprinter 测试用指纹器：返回固定摘要（processMove 不用指纹，仅供 Process 非 nil 守卫通过）。
type stubFingerprinter struct{ digest string }

func (s stubFingerprinter) Fingerprint(ctx context.Context, absPath string) (Fingerprint, error) {
	return Fingerprint{Digest: s.digest}, nil
}

// mockStoreReader 测试用 StoreReader：按路径表查记录。
type mockStoreReader struct {
	byPath map[string]StoreRecord
}

func (m mockStoreReader) GetByFilePathComplete(ctx context.Context, filePath string) (*StoreRecord, error) {
	if r, ok := m.byPath[filePath]; ok {
		rec := r
		return &rec, nil
	}
	return nil, nil
}
func (m mockStoreReader) GetByFingerprint(ctx context.Context, fingerprint string, excludePath string) (*StoreRecord, error) {
	for _, r := range m.byPath {
		if r.ContentFingerprint == fingerprint && r.FilePath != excludePath {
			rec := r
			return &rec, nil
		}
	}
	return nil, nil
}
func (m mockStoreReader) ListValidComplete(ctx context.Context) ([]StoreRecord, error) {
	return nil, nil
}

func newTestCorrelator(store map[string]StoreRecord) *Correlator {
	return NewCorrelator(stubFingerprinter{digest: "fp"}, mockStoreReader{byPath: store}, func() string { return "X:/wd" })
}

// TestProcessMove_TrackedFile 验证：DB 有旧路径记录时，USN 已配对的 ChangeMove 产 SemanticMove（不依赖指纹）。
func TestProcessMove_TrackedFile(t *testing.T) {
	store := map[string]StoreRecord{
		"store/resource/A/旧.jpg": {ID: 42, FilePath: "store/resource/A/旧.jpg", ContentFingerprint: "fp"},
	}
	c := newTestCorrelator(store)
	sc := c.Process(context.Background(), FileChange{
		Kind: ChangeMove, Path: "store/resource/A/旧.jpg", ToPath: "store/resource/B/新.jpg", DetectedAt: 100,
	})
	if sc == nil {
		t.Fatal("期望产 SemanticMove，got nil")
	}
	if sc.Kind != SemanticMove {
		t.Fatalf("Kind = %v want SemanticMove", sc.Kind)
	}
	if sc.FromPath != "store/resource/A/旧.jpg" || sc.ToPath != "store/resource/B/新.jpg" {
		t.Fatalf("路径错误: From=%q To=%q", sc.FromPath, sc.ToPath)
	}
	if sc.StoreID != 42 {
		t.Fatalf("StoreID = %d want 42", sc.StoreID)
	}
	if sc.DetectedAt != 100 {
		t.Fatalf("DetectedAt = %d want 100", sc.DetectedAt)
	}
}

// TestProcessMove_UntrackedFile 验证：DB 无旧路径记录（非本库文件 rename）→ 不报告（对账兜底）。
func TestProcessMove_UntrackedFile(t *testing.T) {
	c := newTestCorrelator(map[string]StoreRecord{})
	sc := c.Process(context.Background(), FileChange{
		Kind: ChangeMove, Path: "store/resource/外部/x.jpg", ToPath: "store/resource/外部/y.jpg",
	})
	if sc != nil {
		t.Fatalf("非本库文件 rename 应返回 nil，got %+v", sc)
	}
}

// TestProcessMove_Directory 验证：ChangeMove(IsDir) 返回 nil（目录改名走 ChangeCreate+processDirCreate，不经此分支）。
func TestProcessMove_Directory(t *testing.T) {
	// 即便目录路径碰巧在 DB（实际不会，目录无记录），IsDir 也应短路返回 nil
	store := map[string]StoreRecord{
		"store/resource/旧目录": {ID: 1, FilePath: "store/resource/旧目录"},
	}
	c := newTestCorrelator(store)
	sc := c.Process(context.Background(), FileChange{
		Kind: ChangeMove, IsDir: true, Path: "store/resource/旧目录", ToPath: "store/resource/新目录",
	})
	if sc != nil {
		t.Fatalf("目录 ChangeMove 应返回 nil（走 processDirCreate），got %+v", sc)
	}
}

// TestProcessCreate_Regression 回归：ChangeCreate 仍按指纹配对产 SemanticMove/Untracked。
func TestProcessCreate_Regression(t *testing.T) {
	store := map[string]StoreRecord{
		"store/resource/old.jpg": {ID: 7, FilePath: "store/resource/old.jpg", ContentFingerprint: "fp"},
	}
	// stubFingerprinter 返回 "fp"，命中 DB 同指纹旧记录 → Move
	c := NewCorrelator(stubFingerprinter{digest: "fp"}, mockStoreReader{byPath: store}, func() string { return "X:/wd" })
	sc := c.Process(context.Background(), FileChange{
		Kind: ChangeCreate, Path: "store/resource/new.jpg", DetectedAt: 5,
	})
	if sc == nil || sc.Kind != SemanticMove || sc.StoreID != 7 {
		t.Fatalf("Create 回归：期望 SemanticMove(storeID=7)，got %+v", sc)
	}
}

// TestProcessRemove_Regression 回归：ChangeRemove 查 DB 路径命中 → SemanticDelete。
func TestProcessRemove_Regression(t *testing.T) {
	store := map[string]StoreRecord{
		"store/resource/gone.jpg": {ID: 9, FilePath: "store/resource/gone.jpg"},
	}
	c := newTestCorrelator(store)
	sc := c.Process(context.Background(), FileChange{
		Kind: ChangeRemove, Path: "store/resource/gone.jpg", DetectedAt: 8,
	})
	if sc == nil || sc.Kind != SemanticDelete || sc.StoreID != 9 {
		t.Fatalf("Remove 回归：期望 SemanticDelete(storeID=9)，got %+v", sc)
	}
}

// TestProcess_NilDeps 验证：Fingerprinter 或 StoreReader 为 nil 时 Process 降级返回 nil。
func TestProcess_NilDeps(t *testing.T) {
	c := NewCorrelator(nil, nil, func() string { return "X:/wd" })
	sc := c.Process(context.Background(), FileChange{Kind: ChangeMove, Path: "a", ToPath: "b"})
	if sc != nil {
		t.Fatalf("能力 nil 时应降级返回 nil，got %+v", sc)
	}
}
