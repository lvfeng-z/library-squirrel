package main

import (
	"context"

	"github.com/library-squirrel/wails/internal/localAuthor"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ==================== LocalAuthor Wails Bindings ====================

// LocalAuthorSave 保存本地作者
func (app *App) LocalAuthorSave(author *domain.LocalAuthor) (int64, error) {
	if err := app.LocalAuthorService.Save(context.Background(), author); err != nil {
		return 0, err
	}
	return author.ID, nil
}

// LocalAuthorDeleteById 删除本地作者
func (app *App) LocalAuthorDeleteById(id int64) error {
	return app.LocalAuthorService.Delete(context.Background(), id)
}

// LocalAuthorUpdateById 更新本地作者
func (app *App) LocalAuthorUpdateById(author *domain.LocalAuthor) error {
	return app.LocalAuthorService.UpdateById(context.Background(), author)
}

// LocalAuthorGetById 获取本地作者
func (app *App) LocalAuthorGetById(id int64) (*domain.LocalAuthor, error) {
	return app.LocalAuthorService.GetById(context.Background(), id)
}

// LocalAuthorQueryPage 分页查询本地作者
func (app *App) LocalAuthorQueryPage(query *localAuthor.LocalAuthorQueryDTO) (*model.Page[domain.LocalAuthor], error) {
	return app.LocalAuthorService.PageByDTO(context.Background(), 1, 10, *query)
}

// LocalAuthorListSelectItems 查询选择项列表
func (app *App) LocalAuthorListSelectItems(query *localAuthor.LocalAuthorQueryDTO) ([]*domain.SelectItem, error) {
	return app.LocalAuthorService.ListSelectItemsByDTO(context.Background(), *query)
}

// LocalAuthorQuerySelectItemPage 分页查询选择项
func (app *App) LocalAuthorQuerySelectItemPage(query *localAuthor.LocalAuthorQueryDTO) (*model.Page[domain.SelectItem], error) {
	return app.LocalAuthorService.QuerySelectItemPageByDTO(context.Background(), 1, 10, *query)
}
