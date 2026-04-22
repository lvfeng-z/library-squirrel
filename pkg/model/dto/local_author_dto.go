package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model/entity"
)

// LocalAuthorDTO 本地作者数据传输对象（无 sql.Null* 版本）
type LocalAuthorDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
	LastUse    *int64  `json:"lastUse"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// NewLocalAuthorDTO 从 entity.LocalAuthor 创建 LocalAuthorDTO
func NewLocalAuthorDTO(author *entity.LocalAuthor) *LocalAuthorDTO {
	if author == nil {
		return nil
	}
	return &LocalAuthorDTO{
		ID:         author.GetID(),
		AuthorName: util.NullStringToPointer(author.AuthorName),
		Introduce:  util.NullStringToPointer(author.Introduce),
		LastUse:    util.NullInt64ToPointer(author.LastUse),
		CreateTime: author.GetCreateTime(),
		UpdateTime: author.GetUpdateTime(),
	}
}
