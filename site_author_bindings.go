package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/siteAuthor"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== SiteAuthor Wails Bindings ====================

// SiteAuthorSave 保存站点作者
func (app *App) SiteAuthorSave(author *domain.SiteAuthor) error {
	return app.SiteAuthorService.Save(context.Background(), author)
}

// SiteAuthorSaveBatch 批量保存站点作者
func (app *App) SiteAuthorSaveBatch(authors []*domain.SiteAuthor) error {
	return app.SiteAuthorService.SaveBatch(context.Background(), authors)
}

// SiteAuthorDeleteById 删除站点作者
func (app *App) SiteAuthorDeleteById(id int64) error {
	return app.SiteAuthorService.Delete(context.Background(), id)
}

// SiteAuthorUpdateById 更新站点作者
func (app *App) SiteAuthorUpdateById(author *domain.SiteAuthor) error {
	return app.SiteAuthorService.UpdateById(context.Background(), author)
}

// SiteAuthorGetById 获取站点作者
func (app *App) SiteAuthorGetById(id int64) (*domain.SiteAuthor, error) {
	return app.SiteAuthorService.GetById(context.Background(), id)
}

// SiteAuthorQueryPage 分页查询站点作者
func (app *App) SiteAuthorQueryPage(query *siteAuthor.SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthor], error) {
	return app.SiteAuthorService.PageByDTO(context.Background(), 1, 10, *query)
}

// SiteAuthorQueryBoundOrUnboundInLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (app *App) SiteAuthorQueryBoundOrUnboundInLocalAuthorPage(query *siteAuthor.SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthorFullDTO], error) {
	return app.SiteAuthorService.QueryBoundOrUnboundToLocalAuthorPageByDTO(context.Background(), 1, 10, *query)
}

// SiteAuthorQueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (app *App) SiteAuthorQueryLocalRelateDTOPage(query *siteAuthor.SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthorLocalRelateDTO], error) {
	return app.SiteAuthorService.QueryLocalRelateDTOPageByDTO(context.Background(), 1, 10, *query)
}

// SiteAuthorListByWorkId 查询作品的站点作者
func (app *App) SiteAuthorListByWorkId(workId int64) ([]*model.RankedSiteAuthor, error) {
	return app.SiteAuthorService.ListByWorkId(context.Background(), workId)
}

// SiteAuthorListBySiteAuthorIds 根据站点作者ID列表查询
func (app *App) SiteAuthorListBySiteAuthorIds(siteAuthorIds []int64) ([]*domain.SiteAuthor, error) {
	return app.SiteAuthorService.ListBySiteAuthorIds(context.Background(), siteAuthorIds)
}

// SiteAuthorListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (app *App) SiteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error) {
	return app.SiteAuthorService.ListRankedSiteAuthorWithWorkIdByWorkIds(context.Background(), workIds)
}

// SiteAuthorUpdateBindLocalAuthor 绑定或解除本地作者绑定
func (app *App) SiteAuthorUpdateBindLocalAuthor(localAuthorId int64, siteAuthorIds []int64) (bool, error) {
	return app.SiteAuthorService.UpdateBindLocalAuthor(context.Background(), localAuthorId, siteAuthorIds)
}

// SiteAuthorCreateAndBindSameNameLocalAuthor 创建并绑定同名的本地作者
func (app *App) SiteAuthorCreateAndBindSameNameLocalAuthor(author *domain.SiteAuthor) (bool, error) {
	return app.SiteAuthorService.CreateAndBindSameNameLocalAuthor(context.Background(), author)
}
