package siteBrowser

import (
	"github.com/library-squirrel/wails/internal/extension"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Service 站点浏览器服务
type Service struct {
	registry *extension.SiteBrowserRegistry
}

// NewService 创建站点浏览器服务
func NewService(registry *extension.SiteBrowserRegistry) *Service {
	return &Service{
		registry: registry,
	}
}

// SiteBrowserDTO 站点浏览器DTO
type SiteBrowserDTO struct {
	ContributionID string `json:"contributionId"`
	PluginPublicID string `json:"pluginPublicId"`
	Name           string `json:"name"`
	PluginID       int64  `json:"pluginId"`
}

// PageResult 分页结果
type PageResult struct {
	Data       []*SiteBrowserDTO `json:"data"`
	PageNumber int               `json:"pageNumber"`
	PageSize   int               `json:"pageSize"`
	Total      int               `json:"total"`
}

// GetID 获取完整ID
func (d *SiteBrowserDTO) GetID() string {
	return d.PluginPublicID + "-" + d.ContributionID
}

// List 获取所有站点浏览器
func (s *Service) List() []*SiteBrowserDTO {
	extensions := s.registry.List()
	dtos := make([]*SiteBrowserDTO, 0, len(extensions))
	for _, ext := range extensions {
		dtos = append(dtos, toDTO(ext))
	}
	return dtos
}

// Page 分页查询
func (s *Service) Page(page, pageSize int) *PageResult {
	extensions := s.registry.List()
	total := len(extensions)

	// 计算分页范围
	start := (page - 1) * pageSize
	if start >= total {
		return &PageResult{
			Data:       []*SiteBrowserDTO{},
			PageNumber: page,
			PageSize:   pageSize,
			Total:      total,
		}
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	// 构建分页数据
	dtos := make([]*SiteBrowserDTO, 0, end-start)
	for i := start; i < end; i++ {
		dtos = append(dtos, toDTO(extensions[i]))
	}

	return &PageResult{
		Data:       dtos,
		PageNumber: page,
		PageSize:   pageSize,
		Total:      total,
	}
}

// GetByID 根据ID获取站点浏览器
func (s *Service) GetByID(pluginPublicId, contributionId string) (*SiteBrowserDTO, error) {
	ext, err := s.registry.Get(pluginPublicId, contributionId)
	if err != nil {
		return nil, err
	}
	return toDTO(ext), nil
}

// GetByPluginID 根据插件ID获取站点浏览器
func (s *Service) GetByPluginID(pluginId int64) []*SiteBrowserDTO {
	extensions := s.registry.List()
	dtos := make([]*SiteBrowserDTO, 0)
	for _, ext := range extensions {
		if ext.Metadata.PluginID == pluginId {
			dtos = append(dtos, toDTO(ext))
		}
	}
	return dtos
}

// Open 打开站点浏览器
func (s *Service) Open(pluginPublicId, contributionId string) error {
	ext, err := s.registry.Get(pluginPublicId, contributionId)
	if err != nil {
		return err
	}
	return ext.Instance.Open()
}

// toDTO 将扩展转换为DTO
func toDTO(ext *model.Extension[domain.SiteBrowser]) *SiteBrowserDTO {
	return &SiteBrowserDTO{
		ContributionID: ext.Metadata.ID,
		PluginPublicID: ext.Metadata.PluginPublicID,
		Name:           ext.Metadata.Name,
		PluginID:       ext.Metadata.PluginID,
	}
}
