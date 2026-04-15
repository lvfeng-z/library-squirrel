package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/localTag"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== LocalTag Wails Bindings ====================

// LocalTagSave 保存本地标签
func (app *App) LocalTagSave(tag *domain.LocalTag) (int64, error) {
	if err := app.LocalTagService.Save(context.Background(), tag); err != nil {
		return 0, err
	}
	return tag.ID, nil
}

// LocalTagDeleteById 删除本地标签
func (app *App) LocalTagDeleteById(id int64) error {
	return app.LocalTagService.Delete(context.Background(), id)
}

// LocalTagUpdateById 更新本地标签
func (app *App) LocalTagUpdateById(tag *domain.LocalTag) error {
	return app.LocalTagService.UpdateById(context.Background(), tag)
}

// LocalTagGetById 获取本地标签
func (app *App) LocalTagGetById(id int64) (*domain.LocalTag, error) {
	return app.LocalTagService.GetById(context.Background(), id)
}

// LocalTagQueryPage 分页查询本地标签
func (app *App) LocalTagQueryPage(query *localTag.LocalTagQueryDTO) (*model.ApiResponse[*model.Page[domain.LocalTag]], error) {
	page, err := app.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.LocalTag]](err.Error()), nil
	}
	return model.Success(page), nil
}

// LocalTagQueryDTOPage DTO分页查询本地标签
func (app *App) LocalTagQueryDTOPage(query *localTag.LocalTagQueryDTO) (*model.ApiResponse[*model.Page[domain.LocalTag]], error) {
	page, err := app.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
	if err != nil {
		return model.Error[*model.Page[domain.LocalTag]](err.Error()), nil
	}
	return model.Success(page), nil
}

// LocalTagGetTree 获取标签树形结构
func (app *App) LocalTagGetTree(rootId int64, depth int) (*model.ApiResponse[[]*domain.LocalTag], error) {
	tree, err := app.LocalTagService.GetTree(context.Background(), rootId, depth)
	if err != nil {
		return model.Error[[]*domain.LocalTag](err.Error()), nil
	}
	return model.Success(tree), nil
}

// LocalTagListSelectItems 查询选择项列表
func (app *App) LocalTagListSelectItems(query *localTag.LocalTagQueryDTO) (*model.ApiResponse[[]*domain.SelectItem], error) {
	items, err := app.LocalTagService.ListSelectItemsByDTO(context.Background(), *query)
	if err != nil {
		return model.Error[[]*domain.SelectItem](err.Error()), nil
	}
	return model.Success(items), nil
}

// LocalTagQuerySelectItemPage 分页查询选择项
func (app *App) LocalTagQuerySelectItemPage(query *localTag.LocalTagQueryDTO) (*model.ApiResponse[*model.Page[domain.SelectItem]], error) {
	page, err := app.LocalTagService.QuerySelectItemPageByDTO(context.Background(), 1, 10, *query, "")
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error()), nil
	}
	return model.Success(page), nil
}

// LocalTagListByWorkId 查询作品关联的标签
func (app *App) LocalTagListByWorkId(workId int64) (*model.ApiResponse[[]*domain.LocalTag], error) {
	tags, err := app.LocalTagService.ListByWorkId(context.Background(), workId)
	if err != nil {
		return model.Error[[]*domain.LocalTag](err.Error()), nil
	}
	return model.Success(tags), nil
}

// LocalTagQuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (app *App) LocalTagQuerySelectItemPageByWorkId(query *localTag.LocalTagQueryDTO, workId int64) (*model.ApiResponse[*model.Page[domain.SelectItem]], error) {
	page, err := app.LocalTagService.QuerySelectItemPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error()), nil
	}
	return model.Success(page), nil
}