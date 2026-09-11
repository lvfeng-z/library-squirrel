package resource

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// ==== 合并产物路径派生测试（与下载侧同口径：store/resource/{作品目录}/videoMain_000.{ext}）====

// mergeNamingResource 资源桩：资源 700 属作品 500
type mergeNamingResource struct{}

func (mergeNamingResource) GetById(ctx context.Context, id int64) (*domain.Resource, error) {
	res := domain.NewResource()
	res.ID = 700
	res.WorkID = 500
	return res, nil
}
func (mergeNamingResource) Updates(ctx context.Context, resource *domain.Resource) error { return nil }

// mergeNamingWork 作品桩：作品 500 的站点复合键可配（站点 11）
type mergeNamingWork struct {
	siteId     sql.NullInt64
	siteWorkId sql.NullString
}

func (w mergeNamingWork) GetById(ctx context.Context, id int64) (*domain.Work, error) {
	wk := domain.NewWork()
	wk.ID = 500
	wk.SiteID = w.siteId
	wk.SiteWorkID = w.siteWorkId
	return wk, nil
}

// mergeNamingSite 站点桩：站点 11 的 site_key 可配
type mergeNamingSite struct{ siteKey string }

func (s mergeNamingSite) GetById(ctx context.Context, id int64) (*domain.Site, error) {
	si := domain.NewSite()
	si.ID = 11
	si.SiteKey = s.siteKey
	return si, nil
}

func newMergeNamingService(work mergeNamingWork, site mergeNamingSite) *MergeService {
	return NewMergeService(nil, mergeNamingResource{}, work, site, nil, nil, nil, nil, nil, nil, nil)
}

// TestDeriveMergedPathsMatchesDownloadLayout 产物路径与文件名为派生形态（与下载侧
// store/resource/{siteKey}_{siteWorkId}/{role}_{seq}.{ext} 同口径）：合并产物是 videoMain
// 单实例派生 store，seq 恒 0；文件名同时作为 store 行 file_name
func TestDeriveMergedPathsMatchesDownloadLayout(t *testing.T) {
	s := newMergeNamingService(
		mergeNamingWork{
			siteId:     sql.NullInt64{Int64: 11, Valid: true},
			siteWorkId: sql.NullString{String: "BV1xx411c7mD_4538792", Valid: true},
		},
		mergeNamingSite{siteKey: "bilibili"},
	)
	relPath, fileName, err := s.deriveMergedPaths(t.Context(), 700, ".mp4")
	if err != nil {
		t.Fatalf("派生应成功，实际失败: %v", err)
	}
	if want := "store/resource/bilibili_BV1xx411c7mD_4538792/videoMain_000.mp4"; relPath != want {
		t.Errorf("产物路径 = %q, want %q", relPath, want)
	}
	if want := "videoMain_000.mp4"; fileName != want {
		t.Errorf("产物文件名 = %q, want %q", fileName, want)
	}
}

// TestDeriveMergedPathsRejectsMissingSiteKey 站点复合键缺失：写入路径严格识别显式报错，
// 不回落派生（siteWorkId 无效与 siteId 无效同族）
func TestDeriveMergedPathsRejectsMissingSiteKey(t *testing.T) {
	cases := []struct {
		name string
		work mergeNamingWork
	}{
		{"siteWorkId 无效", mergeNamingWork{siteId: sql.NullInt64{Int64: 11, Valid: true}}},
		{"siteId 无效", mergeNamingWork{siteWorkId: sql.NullString{String: "128937464", Valid: true}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := newMergeNamingService(tt.work, mergeNamingSite{siteKey: "pixiv"})
			if _, _, err := s.deriveMergedPaths(t.Context(), 700, ".mp4"); err == nil {
				t.Errorf("复合键缺失应报错，实际通过")
			}
		})
	}
}
