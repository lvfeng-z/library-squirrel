package reWorkTag

import (
	"context"
	"database/sql"
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

// fakeSiteTagReader 实现 SiteTagNamespaceReader，按 id 返回预设 siteTag（带 Namespace）。
type fakeSiteTagReader struct {
	byId map[int64]*domain.SiteTag
}

func (f fakeSiteTagReader) ListBySiteTagIds(_ context.Context, ids []int64) ([]*domain.SiteTag, error) {
	res := make([]*domain.SiteTag, 0, len(ids))
	for _, id := range ids {
		if st, ok := f.byId[id]; ok {
			res = append(res, st)
		}
	}
	return res, nil
}

func newSiteTag(id int64, ns string) *domain.SiteTag {
	st := domain.NewSiteTag()
	st.SetID(id)
	if ns != "" {
		st.Namespace = sql.NullString{String: ns, Valid: true}
	}
	return st
}

// TestLinkBatchToWork_LocalNamespaces local 关联用前端传的 namespaces（越界守卫 + 空→NULL）。
func TestLinkBatchToWork_LocalNamespaces(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.LOCAL, []int64{10, 11}, []string{"character", ""}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.rels) != 2 {
		t.Fatalf("expected 2 rels, got %d", len(repo.rels))
	}
	if repo.tagType != constant.LOCAL {
		t.Errorf("local 分支 upsert 应按 (work_id, local_tag_id) 冲突，tagType 得到 %d", repo.tagType)
	}
	if !repo.rels[0].Namespace.Valid || repo.rels[0].Namespace.String != "character" {
		t.Errorf("rels[0] namespace 应为 character，得到 %+v", repo.rels[0].Namespace)
	}
	if repo.rels[1].Namespace.Valid {
		t.Errorf("rels[1] namespace 应为 NULL（空串），得到 %+v", repo.rels[1].Namespace)
	}
}

// TestLinkBatchToWork_LenMismatch namespaces 与 tagIds 长度不匹配须报错且不落盘。
func TestLinkBatchToWork_LenMismatch(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.LOCAL, []int64{10, 11}, []string{"character"}); err != ErrNamespaceCountMismatch {
		t.Fatalf("期望 ErrNamespaceCountMismatch，得到 %v", err)
	}
	if len(repo.rels) != 0 {
		t.Errorf("长度不匹配不应落盘，得到 %d rels", len(repo.rels))
	}
}

// TestLinkBatchToWork_SiteMirror site 关联忽略前端传值，由后端按 site_tag.namespace 镜像。
func TestLinkBatchToWork_SiteMirror(t *testing.T) {
	repo := &fakeRepo{}
	reader := fakeSiteTagReader{byId: map[int64]*domain.SiteTag{
		20: newSiteTag(20, "parody"),
		21: newSiteTag(21, ""), // site_tag 无 namespace → 关联 NULL
	}}
	svc := NewService(repo, reader)
	// site 分支前端不传 ns（nil），后端反查镜像
	if err := svc.LinkBatchToWork(context.Background(), 1, constant.SITE, []int64{20, 21}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.rels) != 2 {
		t.Fatalf("expected 2 rels, got %d", len(repo.rels))
	}
	if repo.tagType != constant.SITE {
		t.Errorf("site 分支 upsert 应按 (work_id, site_tag_id) 冲突，tagType 得到 %d", repo.tagType)
	}
	if !repo.rels[0].Namespace.Valid || repo.rels[0].Namespace.String != "parody" {
		t.Errorf("rels[0]（site_tag 20）应镜像 parody，得到 %+v", repo.rels[0].Namespace)
	}
	if repo.rels[1].Namespace.Valid {
		t.Errorf("rels[1]（site_tag 21 无 ns）应为 NULL，得到 %+v", repo.rels[1].Namespace)
	}
}
