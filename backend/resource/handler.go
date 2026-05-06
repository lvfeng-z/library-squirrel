package resource

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// Handler 资源 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建资源 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存资源
func (h *Handler) Save(ctx context.Context, resource *ResourceDTO) *model.ApiResponse[int64] {
	domainResource := &domain.Resource{
		BaseEntity: &model.BaseEntity{},
		WorkID:     resource.WorkID,
	}
	if resource.FilePath != nil {
		domainResource.FilePath.Valid = true
		domainResource.FilePath.String = *resource.FilePath
	}
	if resource.FileName != nil {
		domainResource.FileName.Valid = true
		domainResource.FileName.String = *resource.FileName
	}

	if err := h.svc.Save(ctx, domainResource); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainResource.GetID())
}

// Delete 删除资源
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新资源
func (h *Handler) Update(ctx context.Context, resource *ResourceDTO) *model.ApiResponse[any] {
	domainResource := &domain.Resource{
		BaseEntity: &model.BaseEntity{},
	}
	domainResource.SetID(resource.ID)
	domainResource.WorkID = resource.WorkID
	if resource.FilePath != nil {
		domainResource.FilePath.Valid = true
		domainResource.FilePath.String = *resource.FilePath
	}
	if resource.FileName != nil {
		domainResource.FileName.Valid = true
		domainResource.FileName.String = *resource.FileName
	}

	if err := h.svc.Update(ctx, domainResource); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*ResourceResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*ResourceResultDTO](err)
	}
	return model.Success(ToResourceResultDTO(result))
}

// ListByWorkId 根据作品ID获取资源列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*ResourceResultDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.HandleError[[]*ResourceResultDTO](err)
	}
	// 转换为 ResultDTO
	resultDTOs := make([]*ResourceResultDTO, len(result))
	for i, resource := range result {
		resultDTOs[i] = ToResourceResultDTO(resource)
	}
	return model.Success(resultDTOs)
}

// DeleteByWorkId 根据作品ID删除资源
func (h *Handler) DeleteByWorkId(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteByWorkId(ctx, workId))
}

// ========== DTO 定义 ==========

// ResourceDTO 资源数据传输对象
type ResourceDTO struct {
	ID       int64   `json:"id"`
	WorkID   int64   `json:"workId"`
	FilePath *string `json:"filePath"`
	FileName *string `json:"fileName"`
}

// ResourceResultDTO 资源返回结果DTO（用于屏蔽sql.Null*类型）
type ResourceResultDTO struct {
	ID                int64   `json:"id"`
	WorkID            int64   `json:"workId"`
	TaskID            int64   `json:"taskId"`
	State             int     `json:"state"`
	FilePath          *string `json:"filePath"`
	FileName          *string `json:"fileName"`
	FilenameExtension *string `json:"filenameExtension"`
	SuggestName       *string `json:"suggestName"`
	ResourceSize      *int64  `json:"resourceSize"`
	Workdir           *string `json:"workdir"`
	ResourceComplete  int     `json:"resourceComplete"`
	CreateTime        int64   `json:"createTime"`
	UpdateTime        int64   `json:"updateTime"`
}

// ToResourceResultDTO 将 domain.Resource 转换为 ResourceResultDTO
func ToResourceResultDTO(resource *domain.Resource) *ResourceResultDTO {
	if resource == nil {
		return nil
	}
	return &ResourceResultDTO{
		ID:                resource.GetID(),
		WorkID:            resource.WorkID,
		TaskID:            resource.TaskID,
		State:             resource.State,
		FilePath:          nullStringToPointer(resource.FilePath),
		FileName:          nullStringToPointer(resource.FileName),
		FilenameExtension: nullStringToPointer(resource.FilenameExtension),
		SuggestName:       nullStringToPointer(resource.SuggestName),
		ResourceSize:      nullInt64ToPointer(resource.ResourceSize),
		Workdir:           nullStringToPointer(resource.Workdir),
		ResourceComplete:  resource.ResourceComplete,
		CreateTime:        resource.GetCreateTime(),
		UpdateTime:        resource.GetUpdateTime(),
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// nullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func nullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}
