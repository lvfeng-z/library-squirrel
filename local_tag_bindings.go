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
func (app *App) LocalTagQueryPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	return app.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
}

// LocalTagQueryDTOPage DTO分页查询本地标签
func (app *App) LocalTagQueryDTOPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.LocalTag], error) {
	return app.LocalTagService.PageByDTO(context.Background(), 1, 10, *query)
}

// LocalTagGetTree 获取标签树形结构
func (app *App) LocalTagGetTree(rootId int64, depth int) ([]*domain.LocalTag, error) {
	return app.LocalTagService.GetTree(context.Background(), rootId, depth)
}

// LocalTagListSelectItems 查询选择项列表
func (app *App) LocalTagListSelectItems(query *localTag.LocalTagQueryDTO) ([]*domain.SelectItem, error) {
	return app.LocalTagService.ListSelectItemsByDTO(context.Background(), *query)
}

// LocalTagQuerySelectItemPage 分页查询选择项
func (app *App) LocalTagQuerySelectItemPage(query *localTag.LocalTagQueryDTO) (*model.Page[domain.SelectItem], error) {
	return app.LocalTagService.QuerySelectItemPageByDTO(context.Background(), 1, 10, *query, "")
}

// LocalTagListByWorkId 查询作品关联的标签
func (app *App) LocalTagListByWorkId(workId int64) ([]*domain.LocalTag, error) {
	return app.LocalTagService.ListByWorkId(context.Background(), workId)
}

// LocalTagQuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (app *App) LocalTagQuerySelectItemPageByWorkId(query *localTag.LocalTagQueryDTO, workId int64) (*model.Page[domain.SelectItem], error) {
	return app.LocalTagService.QuerySelectItemPageByWorkIdByDTO(context.Background(), 1, 10, *query, workId)
}