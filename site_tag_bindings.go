package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/siteTag"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== SiteTag Wails Bindings ====================

// SiteTagSave 保存站点标签
func (app *App) SiteTagSave(tag *domain.SiteTag) error {
	return app.SiteTagService.Save(context.Background(), tag)
}

// SiteTagSaveBatch 批量保存站点标签
func (app *App) SiteTagSaveBatch(tags []*domain.SiteTag) error {
	return app.SiteTagService.SaveBatch(context.Background(), tags)
}

// SiteTagDeleteById 删除站点标签
func (app *App) SiteTagDeleteById(id int64) error {
	return app.SiteTagService.Delete(context.Background(), id)
}

// SiteTagUpdateById 更新站点标签
func (app *App) SiteTagUpdateById(tag *domain.SiteTag) error {
	return app.SiteTagService.UpdateById(context.Background(), tag)
}

// SiteTagGetById 获取站点标签
func (app *App) SiteTagGetById(id int64) (*model.ApiResponse[*domain.SiteTag], error) {
	tag, err := app.SiteTagService.GetById(context.Background(), id)
	if err != nil {
		return model.Error[*domain.SiteTag](err.Error()), nil
	}
	return model.Success(tag), nil
}

// SiteTagQueryPage 分页查询站点标签
func (app *App) SiteTagQueryPage(query *siteTag.SiteTagQueryDTO) (*model.ApiResponse[*model.Page[domain.SiteTag]], error) {
	page, err := app.SiteTagService.PageByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.SiteTag]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SiteTagQueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (app *App) SiteTagQueryBoundOrUnboundToLocalTagPage(query *siteTag.SiteTagQueryDTO) (*model.ApiResponse[*model.Page[domain.SiteTagFullDTO]], error) {
	page, err := app.SiteTagService.QueryBoundOrUnboundToLocalTagPage(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.SiteTagFullDTO]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SiteTagQueryPageByWorkId 根据作品ID分页查询站点标签
func (app *App) SiteTagQueryPageByWorkId(query *siteTag.SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.ApiResponse[*model.Page[domain.SiteTagFullDTO]], error) {
	page, err := app.SiteTagService.QueryPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId, boundOnWorkId)
	if err != nil {
		return model.Error[*model.Page[domain.SiteTagFullDTO]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SiteTagQueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (app *App) SiteTagQueryLocalRelateDTOPage(query *siteTag.SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.ApiResponse[*model.Page[domain.SiteTagLocalRelateDTO]], error) {
	page, err := app.SiteTagService.QueryLocalRelateDTOPageByDTO(context.Background(), 1, 10, *query, workId, boundOnWorkId)
	if err != nil {
		return model.Error[*model.Page[domain.SiteTagLocalRelateDTO]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SiteTagQuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (app *App) SiteTagQuerySelectItemPageByWorkId(query *siteTag.SiteTagQueryDTO, workId int64) (*model.ApiResponse[*model.Page[domain.SelectItem]], error) {
	page, err := app.SiteTagService.QuerySelectItemPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SiteTagListByWorkId 查询作品的站点标签
func (app *App) SiteTagListByWorkId(workId int64) (*model.ApiResponse[[]*domain.SiteTag], error) {
	tags, err := app.SiteTagService.ListByWorkId(context.Background(), workId)
	if err != nil {
		return model.Error[[]*domain.SiteTag](err.Error()), nil
	}
	return model.Success(tags), nil
}

// SiteTagListBySiteTagIds 根据站点标签ID列表查询
func (app *App) SiteTagListBySiteTagIds(siteTagIds []int64) (*model.ApiResponse[[]*domain.SiteTag], error) {
	tags, err := app.SiteTagService.ListBySiteTagIds(context.Background(), siteTagIds)
	if err != nil {
		return model.Error[[]*domain.SiteTag](err.Error()), nil
	}
	return model.Success(tags), nil
}

// SiteTagUpdateBindLocalTag 绑定或解除本地标签绑定
func (app *App) SiteTagUpdateBindLocalTag(localTagId int64, siteTagIds []int64) (bool, error) {
	return app.SiteTagService.UpdateBindLocalTag(context.Background(), localTagId, siteTagIds)
}

// SiteTagCreateAndBindSameNameLocalTag 创建并绑定同名本地标签
func (app *App) SiteTagCreateAndBindSameNameLocalTag(tag *domain.SiteTag) (*model.ApiResponse[*domain.LocalTag], error) {
	localTag, err := app.SiteTagService.CreateAndBindSameNameLocalTag(context.Background(), tag)
	if err != nil {
		return model.Error[*domain.LocalTag](err.Error()), nil
	}
	return model.Success(localTag), nil
}
