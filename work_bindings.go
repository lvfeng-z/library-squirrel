package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/work"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== Work Wails Bindings ====================

// WorkGetFullWorkInfoById 获取作品完整信息
func (app *App) WorkGetFullWorkInfoById(id int64) (*model.ApiResponse[*domain.WorkFullDTO], error) {
	info, err := app.WorkService.GetFullWorkInfoById(context.Background(), id)
	if err != nil {
		return model.Error[*domain.WorkFullDTO](err.Error()), nil
	}
	return model.Success(info), nil
}

// WorkQueryPage 分页查询作品
func (app *App) WorkQueryPage(query *work.WorkQueryDTO) (*model.ApiResponse[*model.Page[domain.Work]], error) {
	page, err := app.WorkService.Page(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.Work]](err.Error()), nil
	}
	return model.Success(page), nil
}

// WorkDeleteWorkAndSurroundingData 删除作品及其周围数据
func (app *App) WorkDeleteWorkAndSurroundingData(id int64) error {
	return app.WorkService.DeleteWorkAndSurroundingData(context.Background(), id)
}

// WorkGetById 获取作品
func (app *App) WorkGetById(id int64) (*model.ApiResponse[*domain.Work], error) {
	work, err := app.WorkService.GetById(context.Background(), id)
	if err != nil {
		return model.Error[*domain.Work](err.Error()), nil
	}
	return model.Success(work), nil
}

// WorkGetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (app *App) WorkGetBySiteAndSiteWorkID(siteId int64, siteWorkId string) (*model.ApiResponse[*domain.Work], error) {
	work, err := app.WorkService.GetBySiteAndSiteWorkID(context.Background(), siteId, siteWorkId)
	if err != nil {
		return model.Error[*domain.Work](err.Error()), nil
	}
	return model.Success(work), nil
}

// WorkListByIds 根据ID列表批量查询
func (app *App) WorkListByIds(ids []int64) (*model.ApiResponse[[]*domain.Work], error) {
	works, err := app.WorkService.ListByIds(context.Background(), ids)
	if err != nil {
		return model.Error[[]*domain.Work](err.Error()), nil
	}
	return model.Success(works), nil
}
