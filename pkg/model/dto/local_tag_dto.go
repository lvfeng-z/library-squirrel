package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model/entity"
)

// LocalTagDTO 本地标签数据传输对象（无 sql.Null* 版本）
type LocalTagDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
	Description    *string `json:"description"`
	LastUse        *int64  `json:"lastUse"`
	CreateTime     int64   `json:"createTime"`
	UpdateTime     int64   `json:"updateTime"`
}

// NewLocalTagDTO 从 entity.LocalTag 创建 LocalTagDTO
func NewLocalTagDTO(tag *entity.LocalTag) *LocalTagDTO {
	if tag == nil {
		return nil
	}
	return &LocalTagDTO{
		ID:             tag.GetID(),
		LocalTagName:   util.NullStringToPointer(tag.LocalTagName),
		BaseLocalTagID: util.NullInt64ToPointer(tag.BaseLocalTagID),
		Description:    util.NullStringToPointer(tag.Description),
		LastUse:        util.NullInt64ToPointer(tag.LastUse),
		CreateTime:     tag.GetCreateTime(),
		UpdateTime:     tag.GetUpdateTime(),
	}
}

// LocalTagParamDTO 本地标签数据传输对象（增删改参数）
type LocalTagParamDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
}
