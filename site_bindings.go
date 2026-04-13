package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/site"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Site Wails Bindings ====================

// SiteSave 保存站点
func (app *App) SiteSave(site *domain.Site) (int64, error) {
	if err := app.SiteService.Save(context.Background(), site); err != nil {
		return 0, err
	}
	return site.GetID(), nil
}

// SiteDeleteById 删除站点
func (app *App) SiteDeleteById(id int64) error {
	return app.SiteService.Delete(context.Background(), id)
}

// SiteUpdateById 更新站点
func (app *App) SiteUpdateById(site *domain.Site) error {
	return app.SiteService.UpdateById(context.Background(), site)
}

// SiteGetById 获取站点
func (app *App) SiteGetById(id int64) (*domain.Site, error) {
	return app.SiteService.GetById(context.Background(), id)
}

// SiteQueryPage 分页查询站点
func (app *App) SiteQueryPage(query *site.SiteQueryDTO) (*model.Page[domain.Site], error) {
	return app.SiteService.Page(context.Background(), 1, 10, *query)
}

// SiteQuerySelectItemPage 分页查询选择项
func (app *App) SiteQuerySelectItemPage(query *site.SiteQueryDTO) (*model.Page[domain.SelectItem], error) {
	return app.SiteService.QuerySelectItemPage(context.Background(), 1, 10, *query)
}
