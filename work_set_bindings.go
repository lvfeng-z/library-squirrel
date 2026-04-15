package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/workSet"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== WorkSet Wails Bindings ====================

// WorkSetSave 保存作品集
func (app *App) WorkSetSave(workSet *domain.WorkSet) error {
	return app.WorkSetService.Save(context.Background(), workSet)
}

// WorkSetUpdate 更新作品集
func (app *App) WorkSetUpdate(workSet *domain.WorkSet) error {
	return app.WorkSetService.Update(context.Background(), workSet)
}

// WorkSetDelete 删除作品集
func (app *App) WorkSetDelete(id int64) error {
	return app.WorkSetService.Delete(context.Background(), id)
}

// WorkSetGetById 获取作品集
func (app *App) WorkSetGetById(id int64) (*model.ApiResponse[*domain.WorkSet], error) {
	workSet, err := app.WorkSetService.GetById(context.Background(), id)
	if err != nil {
		return model.Error[*domain.WorkSet](err.Error()), nil
	}
	return model.Success(workSet), nil
}

// WorkSetQueryPage 分页查询作品集
func (app *App) WorkSetQueryPage(query *workSet.WorkSetQueryDTO) (*model.ApiResponse[*model.Page[domain.WorkSet]], error) {
	page, err := app.WorkSetService.PageByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.WorkSet]](err.Error()), nil
	}
	return model.Success(page), nil
}

// WorkSetQueryPageWithCover 带封面的作品集分页查询
func (app *App) WorkSetQueryPageWithCover(query *workSet.WorkSetQueryDTO) (*model.ApiResponse[*model.Page[workSet.WorkSetWithCoverDTO]], error) {
	page, err := app.WorkSetService.QueryPageWithCoverByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[workSet.WorkSetWithCoverDTO]](err.Error()), nil
	}
	return model.Success(page), nil
}

// WorkSetGetWorks 获取作品集关联的作品列表
func (app *App) WorkSetGetWorks(id int64) (*model.ApiResponse[[]*domain.Work], error) {
	works, err := app.WorkSetService.GetWorksByWorkSetId(context.Background(), id)
	if err != nil {
		return model.Error[[]*domain.Work](err.Error()), nil
	}
	return model.Success(works), nil
}

// WorkSetLinkBatch 批量链接作品到作品集
func (app *App) WorkSetLinkBatch(workSetId int64, workIds []int64) error {
	return app.WorkSetService.LinkBatchToWorkSet(context.Background(), workSetId, workIds)
}

// WorkSetRemoveBatch 批量从作品集移除作品
func (app *App) WorkSetRemoveBatch(workSetId int64, workIds []int64) error {
	return app.WorkSetService.RemoveBatchFromWorkSet(context.Background(), workSetId, workIds)
}

// WorkSetSetCover 设置作品集的封面作品
func (app *App) WorkSetSetCover(workSetId int64, workId int64) error {
	return app.WorkSetService.SetCoverWork(context.Background(), workSetId, workId)
}

// WorkSetUnsetCover 取消封面设置
func (app *App) WorkSetUnsetCover(workSetId int64, workId int64) error {
	return app.WorkSetService.UnsetCover(context.Background(), workSetId, workId)
}

// WorkSetGetCoverWorkId 获取封面作品ID
func (app *App) WorkSetGetCoverWorkId(workSetId int64) (int64, error) {
	return app.WorkSetService.GetCoverWorkId(context.Background(), workSetId)
}

// WorkSetListWorkSetWithWorkByIds 根据作品集ID列表获取作品集及其作品信息
func (app *App) WorkSetListWorkSetWithWorkByIds(workSetIds []int64) (*model.ApiResponse[[]*workSet.WorkSetWithWorksDTO], error) {
	workSets, err := app.WorkSetService.ListWorkSetWithWorkByIds(context.Background(), workSetIds)
	if err != nil {
		return model.Error[[]*workSet.WorkSetWithWorksDTO](err.Error()), nil
	}
	return model.Success(workSets), nil
}
