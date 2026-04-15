package main

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Search Wails Bindings ====================

// SearchQuerySearchConditionPage 查询搜索条件分页
func (app *App) SearchQuerySearchConditionPage(keyword string, types []domain.SearchType) (*model.ApiResponse[*model.Page[domain.SelectItem]], error) {
	page, err := app.SearchService.QuerySearchConditionPage(context.Background(), 1, 10, &domain.SearchConditionQuery{Keyword: keyword, Types: types})
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SearchQueryWorkPage 查询作品分页
func (app *App) SearchQueryWorkPage(conditions []*domain.SearchCondition) (*model.ApiResponse[*model.Page[domain.WorkFullDTO]], error) {
	page, err := app.SearchService.QueryWorkPage(context.Background(), 1, 10, conditions)
	if err != nil {
		return model.Error[*model.Page[domain.WorkFullDTO]](err.Error()), nil
	}
	return model.Success(page), nil
}

// SearchQueryWorkSetPage 查询作品集分页
func (app *App) SearchQueryWorkSetPage(keyword string, siteId int64) (*model.ApiResponse[*model.Page[domain.SelectItem]], error) {
	page, err := app.SearchService.QueryWorkSetPage(context.Background(), 1, 10, keyword, siteId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error()), nil
	}
	return model.Success(page), nil
}
