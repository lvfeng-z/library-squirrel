package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewLocalAuthorDTO 从 entity.LocalAuthor 创建 LocalAuthorDTO
func NewLocalAuthorDTO(author *entity.LocalAuthor) *sdkdto.LocalAuthorDTO {
	if author == nil {
		return nil
	}
	return &sdkdto.LocalAuthorDTO{
		Id:         author.GetID(),
		AuthorName: util.NullStringToPointer(author.AuthorName),
		Introduce:  util.NullStringToPointer(author.Introduce),
		LastUse:    util.NullInt64ToPointer(author.LastUse),
		CreateTime: author.GetCreateTime(),
		UpdateTime: author.GetUpdateTime(),
	}
}

// ToLocalAuthorEntity 将 LocalAuthorDTO 转换为 LocalAuthor 实体
func ToLocalAuthorEntity(dto *sdkdto.LocalAuthorDTO) *entity.LocalAuthor {
	if dto == nil {
		return nil
	}

	entity := entity.NewLocalAuthor()

	// 设置基础字段
	if dto.Id != 0 {
		entity.SetID(dto.Id)
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

// LocalAuthorFullDTO 本地作者宿主侧展示 DTO（SDK LocalAuthorDTO 作命名字段 + 头像展示路径，
// 管理页列表与详情数据源；与站点侧 SiteAuthorLocalRelateDTO 对称的包装形态——SDK 契约类型
// 不可加字段，宿主展示字段在包装层顶置）
type LocalAuthorFullDTO struct {
	Author sdkdto.LocalAuthorDTO `json:"author"`
	// AvatarFilePath 头像文件 workDir 相对路径（正斜杠；前端经 buildStoreUrl 转 /store/ URL）。
	// 仅头像引用指向活行且落盘完成时非空，其余（无头像/软删失效/未完成）为 nil——展示侧占位图兜底
	AvatarFilePath *string `json:"avatarFilePath,omitempty"`
}

// NewLocalAuthorFullDTO 从 entity.LocalAuthor 创建本地作者宿主侧展示 DTO（头像路径由组装处后置 enrich）
func NewLocalAuthorFullDTO(author *entity.LocalAuthor) *LocalAuthorFullDTO {
	if author == nil {
		return nil
	}
	authorDTO := NewLocalAuthorDTO(author)
	return &LocalAuthorFullDTO{
		Author: *authorDTO,
	}
}

// RankedLocalAuthor 带排序的本地作者
type RankedLocalAuthor struct {
	Author    sdkdto.LocalAuthorDTO `json:"author"`
	RoleName  string                `json:"roleName"`
	SortOrder int                   `json:"sortOrder"`
	// AvatarFilePath 头像文件 workDir 相对路径（正斜杠；前端经 buildStoreUrl 转 /store/ URL）。
	// 仅头像引用指向活行且落盘完成时非空，其余（无头像/软删失效/未完成）为 nil——展示侧占位图兜底
	AvatarFilePath *string `json:"avatarFilePath,omitempty"`
}

// RankedLocalAuthorWithWorkId 带作品ID的本地作者
type RankedLocalAuthorWithWorkId struct {
	Author    sdkdto.LocalAuthorDTO `json:"author"`
	RoleName  string                `json:"roleName"`
	SortOrder int                   `json:"sortOrder"`
	WorkId    int64                 `json:"workId"`
	// AvatarFilePath 头像文件 workDir 相对路径（语义同 RankedLocalAuthor.AvatarFilePath）
	AvatarFilePath *string `json:"avatarFilePath,omitempty"`
}
