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

// ToLocalAuthorEntity 将 LocalAuthorDTO 转换为 LocalAuthor 实体
func ToLocalAuthorEntity(dto *LocalAuthorDTO) *entity.LocalAuthor {
	if dto == nil {
		return nil
	}

	entity := entity.NewLocalAuthor()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.AuthorName != nil {
		entity.AuthorName.Valid = true
		entity.AuthorName.String = *dto.AuthorName
	} else {
		entity.AuthorName.Valid = false
	}

	if dto.Introduce != nil {
		entity.Introduce.Valid = true
		entity.Introduce.String = *dto.Introduce
	} else {
		entity.Introduce.Valid = false
	}

	if dto.LastUse != nil {
		entity.LastUse.Valid = true
		entity.LastUse.Int64 = *dto.LastUse
	} else {
		entity.LastUse.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		entity.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		entity.SetUpdateTime(dto.UpdateTime)
	}

	return entity
}

// RankedLocalAuthor 带排名的本地作者
type RankedLocalAuthor struct {
	ID         int64  `json:"id"`
	AuthorName string `json:"authorName"`
	Introduce  string `json:"introduce"`
	LastUse    int64  `json:"lastUse"`
	CreateTime int64  `json:"createTime"`
	UpdateTime int64  `json:"updateTime"`
	AuthorRank int    `json:"authorRank"`
}

// RankedLocalAuthorWithWorkId 带作品ID的本地作者
type RankedLocalAuthorWithWorkId struct {
	RankedLocalAuthor
	WorkId int64 `json:"workId"`
}
