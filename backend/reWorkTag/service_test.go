package reWorkTag

import (
	"context"
	"testing"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// fakeRepo 记录 UpsertBatch 创建的关联与传入的 tagType（其余 Repository 方法用 nil 接口嵌入满足签名——LinkBatchToWork 仅触达 UpsertBatch）。
type fakeRepo struct {
	Repository
	rels    []*domain.ReWorkTag
	tagType int
}

func (f *fakeRepo) UpsertBatch(_ context.Context, rels []*domain.ReWorkTag, tagType int) error {
	f.rels = append(f.rels, rels...)
	f.tagType = tagType
	return nil
}

// directTransactor 直接执行 fn（fake 仓储链无 DB，事务语义不适用）
type directTransactor struct{}

func (directTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// noopInventory 清单登记空实现（fake 链不触 DB）
type noopInventory struct{}

func (noopInventory) EnsureUsedBatch(context.Context, []string, int64) error { return nil }

// TestLinkBatchToWork_LocalNamespaces local 关联用调用方传的 namespaces（越界守卫 + 空→空串无 ns），
// 值归一化写入（大写折叠小写）。
func TestLinkBatchToWork_LocalNamespaces(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, directTransactor{}, noopInventory{})
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.LOCAL, []int64{10, 11}, []string{"Character ", ""}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.rels) != 2 {
		t.Fatalf("expected 2 rels, got %d", len(repo.rels))
	}
	if repo.tagType != constant.LOCAL {
		t.Errorf("local 分支 upsert 应按 (work_id, local_tag_id, namespace) 冲突，tagType 得到 %d", repo.tagType)
	}
	if repo.rels[0].Namespace != "character" {
		t.Errorf("rels[0] namespace 应归一化为 character，得到 %q", repo.rels[0].Namespace)
	}
	if repo.rels[1].Namespace != "" {
		t.Errorf("rels[1] namespace 应为空串（无 ns），得到 %q", repo.rels[1].Namespace)
	}
}

// TestLinkBatchToWork_LenMismatch namespaces 与 tagIds 长度不匹配须报错且不落盘。
func TestLinkBatchToWork_LenMismatch(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, directTransactor{}, noopInventory{})
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.LOCAL, []int64{10, 11}, []string{"character"}); err != ErrNamespaceCountMismatch {
		t.Fatalf("期望 ErrNamespaceCountMismatch，得到 %v", err)
	}
	if len(repo.rels) != 0 {
		t.Errorf("长度不匹配不应落盘，得到 %d rels", len(repo.rels))
	}
}

// TestLinkBatchToWork_SiteNamespaces site 关联同样用调用方传的 namespaces——namespace 是
// 关联级开放维度，site 轨开放用户自设（不再镜像 site_tag 行）；空串=无 ns。
func TestLinkBatchToWork_SiteNamespaces(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, directTransactor{}, noopInventory{})
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.SITE, []int64{20, 21}, []string{"parody", ""}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.rels) != 2 {
		t.Fatalf("expected 2 rels, got %d", len(repo.rels))
	}
	if repo.tagType != constant.SITE {
		t.Errorf("site 分支 upsert 应按 (work_id, site_tag_id, namespace) 冲突，tagType 得到 %d", repo.tagType)
	}
	if repo.rels[0].Namespace != "parody" {
		t.Errorf("rels[0] namespace 应为用户自设 parody，得到 %q", repo.rels[0].Namespace)
	}
	if repo.rels[1].Namespace != "" {
		t.Errorf("rels[1] namespace 应为空串（无 ns），得到 %q", repo.rels[1].Namespace)
	}
}
