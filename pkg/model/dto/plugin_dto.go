package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// PluginDTO 插件数据传输对象（无 sql.Null* 版本）
type PluginDTO struct {
	ID             int64   `json:"id"`
	PublicID       *string `json:"publicId"`
	Author         *string `json:"author"`
	Name           *string `json:"name"`
	Version        *string `json:"version"`
	EntryPath      *string `json:"entryPath"`
	RootPath       *string `json:"rootPath"`
	BackupID       *int64  `json:"backupId"`
	SortNum        *int64  `json:"sortNum"`
	PluginData     *string `json:"pluginData"`
	Uninstalled    *int64  `json:"uninstalled"`
	ActivationType *string `json:"activationType"`
	CreateTime     int64   `json:"createTime"`
	UpdateTime     int64   `json:"updateTime"`
}

// NewPluginDTO 从 entity.Plugin 创建 PluginDTO
func NewPluginDTO(plugin *entity2.Plugin) *PluginDTO {
	if plugin == nil {
		return nil
	}
	return &PluginDTO{
		ID:             plugin.GetID(),
		PublicID:       util.NullStringToPointer(plugin.PublicID),
		Author:         util.NullStringToPointer(plugin.Author),
		Name:           util.NullStringToPointer(plugin.Name),
		Version:        util.NullStringToPointer(plugin.Version),
		EntryPath:      util.NullStringToPointer(plugin.EntryPath),
		RootPath:       util.NullStringToPointer(plugin.RootPath),
		BackupID:       util.NullInt64ToPointer(plugin.BackupID),
		SortNum:        util.NullInt64ToPointer(plugin.SortNum),
		PluginData:     util.NullStringToPointer(plugin.PluginData),
		Uninstalled:    util.NullInt64ToPointer(plugin.Uninstalled),
		ActivationType: util.NullStringToPointer(plugin.ActivationType),
		CreateTime:     plugin.GetCreateTime(),
		UpdateTime:     plugin.GetUpdateTime(),
	}
}
