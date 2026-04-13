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
func (app *App) SiteTagGetById(id int64) (*domain.SiteTag, error) {
	return app.SiteTagService.GetById(context.Background(), id)
}

// SiteTagQueryPage 分页查询站点标签
func (app *App) SiteTagQueryPage(query *siteTag.SiteTagQueryDTO) (*model.Page[domain.SiteTag], error) {
	return app.SiteTagService.PageByDTO(context.Background(), 1, 10, *query)
}

// SiteTagQueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (app *App) SiteTagQueryBoundOrUnboundToLocalTagPage(query *siteTag.SiteTagQueryDTO) (*model.Page[domain.SiteTagFullDTO], error) {
	return app.SiteTagService.QueryBoundOrUnboundToLocalTagPage(context.Background(), 1, 10, *query)
}

// SiteTagQueryPageByWorkId 根据作品ID分页查询站点标签
func (app *App) SiteTagQueryPageByWorkId(query *siteTag.SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO], error) {
	return app.SiteTagService.QueryPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId, boundOnWorkId)
}

// SiteTagQueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (app *App) SiteTagQueryLocalRelateDTOPage(query *siteTag.SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO], error) {
	return app.SiteTagService.QueryLocalRelateDTOPageByDTO(context.Background(), 1, 10, *query, workId, boundOnWorkId)
}

// SiteTagQuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (app *App) SiteTagQuerySelectItemPageByWorkId(query *siteTag.SiteTagQueryDTO, workId int64) (*model.Page[domain.SelectItem], error) {
	return app.SiteTagService.QuerySelectItemPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId)
}

// SiteTagListByWorkId 查询作品的站点标签
func (app *App) SiteTagListByWorkId(workId int64) ([]*domain.SiteTag, error) {
	return app.SiteTagService.ListByWorkId(context.Background(), workId)
}

// SiteTagListBySiteTagIds 根据站点标签ID列表查询
func (app *App) SiteTagListBySiteTagIds(siteTagIds []int64) ([]*domain.SiteTag, error) {
	return app.SiteTagService.ListBySiteTagIds(context.Background(), siteTagIds)
}

// SiteTagUpdateBindLocalTag 绑定或解除本地标签绑定
func (app *App) SiteTagUpdateBindLocalTag(localTagId int64, siteTagIds []int64) (bool, error) {
	return app.SiteTagService.UpdateBindLocalTag(context.Background(), localTagId, siteTagIds)
}

// SiteTagCreateAndBindSameNameLocalTag 创建并绑定同名本地标签
func (app *App) SiteTagCreateAndBindSameNameLocalTag(tag *domain.SiteTag) (*domain.LocalTag, error) {
	return app.SiteTagService.CreateAndBindSameNameLocalTag(context.Background(), tag)
}
