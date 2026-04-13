package main

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Search Wails Bindings ====================

// SearchQuerySearchConditionPage 查询搜索条件分页
func (app *App) SearchQuerySearchConditionPage(keyword string, types []domain.SearchType) (*model.Page[domain.SelectItem], error) {
	return app.SearchService.QuerySearchConditionPage(context.Background(), 1, 10, &domain.SearchConditionQuery{Keyword: keyword, Types: types})
}

// SearchQueryWorkPage 查询作品分页
func (app *App) SearchQueryWorkPage(conditions []*domain.SearchCondition) (*model.Page[domain.WorkFullDTO], error) {
	return app.SearchService.QueryWorkPage(context.Background(), 1, 10, conditions)
}

// SearchQueryWorkSetPage 查询作品集分页
func (app *App) SearchQueryWorkSetPage(keyword string, siteId int64) (*model.Page[domain.SelectItem], error) {
	return app.SearchService.QueryWorkSetPage(context.Background(), 1, 10, keyword, siteId)
}
