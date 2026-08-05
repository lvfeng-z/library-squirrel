package dto

import "github.com/lvfeng-z/library-squirrel-sdk/dto/render"

// ToRenderContext 将主程序展示 WorkFullDTO 投射为插件资源渲染契约 render.Context。
// 投射单向：主程序展示 DTO 此后独立演进，不传导至 render 包（契约断链，见 render 包文档）。
// A 类子类型（Work/Site/LocalTag/LocalAuthor）为 proto 生成别名，直接复用；B 类子类型逐字段投射。
func ToRenderContext(work *WorkFullDTO) *render.Context {
	if work == nil {
		return nil
	}
	return &render.Context{
		Work:         work.Work,
		LocalAuthors: toRenderLocalAuthors(work.LocalAuthors),
		SiteAuthors:  toRenderSiteAuthors(work.SiteAuthors),
		Site:         work.Site,
		LocalTags:    work.LocalTags,
		SiteTags:     toRenderSiteTags(work.SiteTags),
		Resource:     toRenderResource(work.Resource),
	}
}

func toRenderLocalAuthors(authors []*RankedLocalAuthor) []*render.RankedLocalAuthor {
	if len(authors) == 0 {
		return nil
	}
	result := make([]*render.RankedLocalAuthor, len(authors))
	for i, a := range authors {
		if a == nil {
			continue
		}
		result[i] = &render.RankedLocalAuthor{
			Author:    a.Author,
			RoleName:  a.RoleName,
			SortOrder: a.SortOrder,
		}
	}
	return result
}

func toRenderSiteAuthors(authors []*RankedSiteAuthor) []*render.RankedSiteAuthor {
	if len(authors) == 0 {
		return nil
	}
	result := make([]*render.RankedSiteAuthor, len(authors))
	for i, a := range authors {
		if a == nil {
			continue
		}
		result[i] = &render.RankedSiteAuthor{
			Author:    toRenderSiteAuthorDTO(a.Author),
			RoleName:  a.RoleName,
			SortOrder: a.SortOrder,
		}
	}
	return result
}

func toRenderSiteAuthorDTO(a SiteAuthorDTO) render.SiteAuthorDTO {
	return render.SiteAuthorDTO{
		ID:                   a.ID,
		SiteID:               a.SiteID,
		SiteAuthorID:         a.SiteAuthorID,
		AuthorName:           a.AuthorName,
		FixedAuthorName:      a.FixedAuthorName,
		SiteAuthorNameBefore: a.SiteAuthorNameBefore,
		Introduce:            a.Introduce,
		Homepage:             a.Homepage,
		LocalAuthorID:        a.LocalAuthorID,
		LastUse:              a.LastUse,
		CreateTime:           a.CreateTime,
		UpdateTime:           a.UpdateTime,
	}
}

func toRenderSiteTags(tags []*SiteTagFullDTO) []*render.SiteTagFullDTO {
	if len(tags) == 0 {
		return nil
	}
	result := make([]*render.SiteTagFullDTO, len(tags))
	for i, t := range tags {
		if t == nil {
			continue
		}
		result[i] = &render.SiteTagFullDTO{
			SiteTag:  toRenderSiteTagDTO(t.SiteTag),
			LocalTag: t.LocalTag,
			Site:     t.Site,
		}
	}
	return result
}

func toRenderSiteTagDTO(t *SiteTagDTO) *render.SiteTagDTO {
	if t == nil {
		return nil
	}
	return &render.SiteTagDTO{
		ID:            t.ID,
		SiteID:        t.SiteID,
		SiteTagID:     t.SiteTagID,
		SiteTagName:   t.SiteTagName,
		BaseSiteTagID: t.BaseSiteTagID,
		Description:   t.Description,
		LocalTagID:    t.LocalTagID,
		LastUse:       t.LastUse,
		CreateTime:    t.CreateTime,
		UpdateTime:    t.UpdateTime,
	}
}

func toRenderResource(r *ResourceFullDTO) *render.ResourceFullDTO {
	if r == nil {
		return nil
	}
	return &render.ResourceFullDTO{
		ID:               r.ID,
		WorkID:           r.WorkID,
		TaskID:           r.TaskID,
		Enabled:          r.Enabled,
		SuggestName:      r.SuggestName,
		ResourceType:     r.ResourceType,
		ResourceComplete: r.ResourceComplete,
		Stores:           toRenderStores(r.Stores),
		WorkStore:        toRenderPersistentStore(r.WorkStore),
		ThumbnailStore:   toRenderPersistentStore(r.ThumbnailStore),
		CreateTime:       r.CreateTime,
		UpdateTime:       r.UpdateTime,
	}
}

func toRenderStores(stores []ResourceStoreDTO) []render.ResourceStoreDTO {
	if len(stores) == 0 {
		return nil
	}
	result := make([]render.ResourceStoreDTO, len(stores))
	for i, s := range stores {
		result[i] = render.ResourceStoreDTO{
			StoreType:  s.StoreType,
			Generation: s.Generation,
			Store:      toRenderPersistentStore(s.Store),
		}
	}
	return result
}

func toRenderPersistentStore(s *PersistentStoreDTO) *render.PersistentStoreDTO {
	if s == nil {
		return nil
	}
	return &render.PersistentStoreDTO{
		ID:                s.ID,
		FilePath:          s.FilePath,
		FileName:          s.FileName,
		FilenameExtension: s.FilenameExtension,
		Status:            s.Status,
		Width:             s.Width,
		Height:            s.Height,
		CreateTime:        s.CreateTime,
		UpdateTime:        s.UpdateTime,
	}
}
