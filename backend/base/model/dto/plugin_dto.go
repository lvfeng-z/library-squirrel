package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// PluginDTO 插件
type PluginDTO struct {
	ID                     int64   `json:"id"`
	PublicID               *string `json:"publicId"`
	Author                 *string `json:"author"`
	Name                   *string `json:"name"`
	Version                *string `json:"version"`
	Description            *string `json:"description"`
	Changelog              *string `json:"changelog"`
	EntryPath              *string `json:"entryPath"`
	RootPath               *string `json:"rootPath"`
	BackupID               *int64  `json:"backupId"`
	SortNum                *int64  `json:"sortNum"`
	Uninstalled            *bool   `json:"uninstalled"`
	ActivationType         *string `json:"activationType"`
	Source                 *string `json:"source"`                 // 来源枚举 bundled/local/url/marketplace
	SourceDetail           *string `json:"sourceDetail"`           // 来源详情（安装包路径/URL）
	UpgradeDeclinedBuildID *string `json:"upgradeDeclinedBuildId"` // 用户拒绝升级的目标 buildId（非空=已跳过该构建，管理页渲染「已跳过」并提供重新提示）
	Trusted                *bool   `json:"trusted"`                // 信任标记；false=未信任（未激活，需手动信任）
	Official               *bool   `json:"official"`               // 官方身份；true=内容摘要命中官方指纹名单，NULL/false=未证实（前端按 === true 消费，与 trusted 同风格）
	CreateTime             int64   `json:"createTime"`
	UpdateTime             int64   `json:"updateTime"`
}

// NewPluginDTO 从 entity.Plugin 创建 PluginDTO
func NewPluginDTO(plugin *entity2.Plugin) *PluginDTO {
	if plugin == nil {
		return nil
	}
	return &PluginDTO{
		ID:                     plugin.GetID(),
		PublicID:               util.NullStringToPointer(plugin.PublicID),
		Author:                 util.NullStringToPointer(plugin.Author),
		Name:                   util.NullStringToPointer(plugin.Name),
		Version:                util.NullStringToPointer(plugin.Version),
		Description:            util.NullStringToPointer(plugin.Description),
		Changelog:              util.NullStringToPointer(plugin.Changelog),
		EntryPath:              util.NullStringToPointer(plugin.EntryPath),
		RootPath:               util.NullStringToPointer(plugin.RootPath),
		BackupID:               util.NullInt64ToPointer(plugin.BackupID),
		SortNum:                util.NullInt64ToPointer(plugin.SortNum),
		Uninstalled:            util.NullBoolToPointer(plugin.Uninstalled),
		ActivationType:         util.NullStringToPointer(plugin.ActivationType),
		Source:                 util.NullStringToPointer(plugin.Source),
		SourceDetail:           util.NullStringToPointer(plugin.SourceDetail),
		UpgradeDeclinedBuildID: util.NullStringToPointer(plugin.UpgradeDeclinedBuildID),
		Trusted:                util.NullBoolToPointer(plugin.Trusted),
		Official:               util.NullBoolToPointer(plugin.Official),
		CreateTime:             plugin.GetCreateTime(),
		UpdateTime:             plugin.GetUpdateTime(),
	}
}

// ToPluginEntity 将 PluginDTO 转换为 Plugin 实体
func ToPluginEntity(dto *PluginDTO) *entity2.Plugin {
	if dto == nil {
		return nil
	}

	entity := entity2.NewPlugin()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.PublicID != nil {
		entity.PublicID.Valid = true
		entity.PublicID.String = *dto.PublicID
	} else {
		entity.PublicID.Valid = false
	}

	if dto.Author != nil {
		entity.Author.Valid = true
		entity.Author.String = *dto.Author
	} else {
		entity.Author.Valid = false
	}

	if dto.Name != nil {
		entity.Name.Valid = true
		entity.Name.String = *dto.Name
	} else {
		entity.Name.Valid = false
	}

	if dto.Version != nil {
		entity.Version.Valid = true
		entity.Version.String = *dto.Version
	} else {
		entity.Version.Valid = false
	}

	if dto.Description != nil {
		entity.Description.Valid = true
		entity.Description.String = *dto.Description
	} else {
		entity.Description.Valid = false
	}

	if dto.Changelog != nil {
		entity.Changelog.Valid = true
		entity.Changelog.String = *dto.Changelog
	} else {
		entity.Changelog.Valid = false
	}

	if dto.EntryPath != nil {
		entity.EntryPath.Valid = true
		entity.EntryPath.String = *dto.EntryPath
	} else {
		entity.EntryPath.Valid = false
	}

	if dto.RootPath != nil {
		entity.RootPath.Valid = true
		entity.RootPath.String = *dto.RootPath
	} else {
		entity.RootPath.Valid = false
	}

	if dto.BackupID != nil {
		entity.BackupID.Valid = true
		entity.BackupID.Int64 = *dto.BackupID
	} else {
		entity.BackupID.Valid = false
	}

	if dto.SortNum != nil {
		entity.SortNum.Valid = true
		entity.SortNum.Int64 = *dto.SortNum
	} else {
		entity.SortNum.Valid = false
	}

	if dto.Uninstalled != nil {
		entity.Uninstalled.Valid = true
		entity.Uninstalled.Bool = *dto.Uninstalled
	} else {
		entity.Uninstalled.Valid = false
	}

	if dto.ActivationType != nil {
		entity.ActivationType.Valid = true
		entity.ActivationType.String = *dto.ActivationType
	} else {
		entity.ActivationType.Valid = false
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

// PendingUpgradeDTO 插件更新待办项（检查更新流 IPC DTO）。启动期检测产出、前端拉取消费：
// available 为可答复项（红点计数、管理页升级/跳过按钮），forced/error 为只读告知项
type PendingUpgradeDTO struct {
	PublicID         string `json:"publicId"`         // 插件身份键；error 项包解析失败无身份，为空串
	PluginName       string `json:"pluginName"`       // 展示名；error 项为包文件名
	InstalledVersion string `json:"installedVersion"` // 已装版本；error 项为空
	TargetVersion    string `json:"targetVersion"`    // 捆绑包版本
	TargetBuildID    string `json:"targetBuildId"`    // 捆绑包构建身份标识；未打标包为空（不进检查更新流）
	Direction        string `json:"direction"`        // up/down/none（version 语义排序，仅展示用）
	Kind             string `json:"kind"`             // available（可升级）/forced（已因契约不兼容强制升级）/error（捆绑包安装失败）
	Source           string `json:"source"`           // 更新来源：bundled（当前唯一；网络源接入后扩展）
	Message          string `json:"message"`          // forced/error 类的说明文案（错误摘要）
}
