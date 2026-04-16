package work

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// WorkQueryDTO 作品查询条件
type WorkQueryDTO struct {
	// 精确查询
	ID            *int64  `json:"-"`             // 作品ID（程序设置，不从JSON解析）
	SiteID        *int64  `json:"siteId"`        // 站点ID
	SiteWorkID    *string `json:"siteWorkId"`    // 站点作品ID
	SiteAuthorID  *string `json:"siteAuthorId"`  // 站点作者ID
	LocalAuthorID *int64  `json:"localAuthorId"` // 本地作者ID
	NickName      *string `json:"nickName"`      // 昵称（精确匹配）
	// 模糊查询
	SiteWorkNameLike *string `json:"siteWorkNameLike"` // 站点作品名称（模糊匹配）
	SiteWorkDescLike *string `json:"siteWorkDescLike"` // 站点作品描述（模糊匹配）
	NickNameLike     *string `json:"nickNameLike"`     // 昵称（模糊匹配）
	// 排序字段：create_time, update_time, site_upload_time, site_update_time, last_view, site_work_name
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *WorkQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time":      "create_time",
			"update_time":      "update_time",
			"site_upload_time": "site_upload_time",
			"site_update_time": "site_update_time",
			"last_view":        "last_view",
			"site_work_name":   "site_work_name",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// ========== 外部模块接口定义（由 work 模块定义自己需要的接口）==========

// LocalTagReader 本地标签读取接口
type LocalTagReader interface {
	// ListByWorkId 查询作品关联的本地标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error)
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalTag, error)
}

// LocalAuthorReader 本地作者读取接口
type LocalAuthorReader interface {
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error)
}

// SiteTagReader 站点标签读取接口
type SiteTagReader interface {
	// ListByWorkId 查询作品关联的站点标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.SiteTag, error)
}

// SiteAuthorReader 站点作者读取接口
type SiteAuthorReader interface {
	// ListByWorkId 查询作品关联的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error)
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.SiteAuthor, error)
}

// SiteReader 站点读取接口
type SiteReader interface {
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Site, error)
}

// ResourceReader 资源读取接口
type ResourceReader interface {
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error)
}

// ReWorkTagDeleter 作品-标签关联删除接口
type ReWorkTagDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// ReWorkWorkSetDeleter 作品-作品集关联删除接口
type ReWorkWorkSetDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// ResourceDeleter 资源删除接口
type ResourceDeleter interface {
	// DeleteByWorkId 根据作品ID删除所有资源
	DeleteByWorkId(ctx context.Context, workId int64) error
}

// Repository 作品仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, work *domain.Work) error
	// Update 更新
	Update(ctx context.Context, work *domain.Work) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Work, error)
	// List 查询列表
	List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.Work, error)
	// Count 统计数量
	Count(ctx context.Context, conditions []clause.Expression) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.Work], error)
	// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
	GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error)
	// ListByIds 根据ID列表批量查询
	ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error)
	// UpdateLastViewBatch 批量更新最后查看时间
	UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error
}

// Service 作品服务
type Service struct {
	repo Repository

	// 外部模块依赖（通过构造函数注入）
	localTagReader       LocalTagReader
	localAuthorReader    LocalAuthorReader
	siteTagReader        SiteTagReader
	siteAuthorReader     SiteAuthorReader
	siteReader           SiteReader
	resourceReader       ResourceReader
	reWorkTagDeleter     ReWorkTagDeleter
	reWorkWorkSetDeleter ReWorkWorkSetDeleter
	resourceDeleter      ResourceDeleter
}

// NewService 创建作品服务
func NewService(
	repo Repository,
	localTagReader LocalTagReader,
	localAuthorReader LocalAuthorReader,
	siteTagReader SiteTagReader,
	siteAuthorReader SiteAuthorReader,
	siteReader SiteReader,
	resourceReader ResourceReader,
	reWorkTagDeleter ReWorkTagDeleter,
	reWorkWorkSetDeleter ReWorkWorkSetDeleter,
	resourceDeleter ResourceDeleter,
) *Service {
	return &Service{
		repo:                 repo,
		localTagReader:       localTagReader,
		localAuthorReader:    localAuthorReader,
		siteTagReader:        siteTagReader,
		siteAuthorReader:     siteAuthorReader,
		siteReader:           siteReader,
		resourceReader:       resourceReader,
		reWorkTagDeleter:     reWorkTagDeleter,
		reWorkWorkSetDeleter: reWorkWorkSetDeleter,
		resourceDeleter:      resourceDeleter,
	}
}

// Save 保存作品
func (s *Service) Save(ctx context.Context, work *domain.Work) error {
	return s.repo.Save(ctx, work)
}

// UpdateById 更新作品
func (s *Service) UpdateById(ctx context.Context, work *domain.Work) error {
	if work.ID == 0 {
		return ErrWorkIdRequired
	}
	return s.repo.Update(ctx, work)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.Work, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, where clause.Expression, order clause.Expression, limit, offset int) ([]*domain.Work, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.List(ctx, conditions, order, limit, offset)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error) {
	return s.repo.ListByIds(ctx, ids)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, where clause.Expression) (int64, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Count(ctx, conditions)
}

// Delete 删除作品
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// DeleteWorkAndSurroundingData 删除作品及其周围数据（级联删除）
func (s *Service) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
	// 1. 删除作品关联的标签
	if err := s.reWorkTagDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 2. 删除作品关联的作品集关系
	if err := s.reWorkWorkSetDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 3. 删除作品关联的资源
	if err := s.resourceDeleter.DeleteByWorkId(ctx, id); err != nil {
		return err
	}

	// 4. 删除作品本身
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
// Page 分页查询（基于 QueryDTO）
func (s *Service) Page(ctx context.Context, page, pageSize int, queryDTO WorkQueryDTO) (*model.Page[domain.Work], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.Page(ctx, page, pageSize, conditions, orderBy)
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (s *Service) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	return s.repo.GetBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
}

// GetFullWorkInfoById 获取作品完整信息
func (s *Service) GetFullWorkInfoById(ctx context.Context, id int64) (*domain.WorkFullDTO, error) {
	// 获取作品基本信息
	work, err := s.repo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}

	// 构建 WorkFullDTO
	fullDTO := domain.NewWorkFullDTO(work)

	// 获取本地作者信息
	if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
		localAuthor, err := s.localAuthorReader.GetById(ctx, work.LocalAuthorID.Int64)
		if err == nil && localAuthor != nil {
			authorName := ""
			if localAuthor.AuthorName.Valid {
				authorName = localAuthor.AuthorName.String
			}
			introduce := ""
			if localAuthor.Introduce.Valid {
				introduce = localAuthor.Introduce.String
			}
			lastUse := int64(0)
			if localAuthor.LastUse.Valid {
				lastUse = localAuthor.LastUse.Int64
			}
			fullDTO.LocalAuthor = &model.RankedLocalAuthor{
				ID:         localAuthor.ID,
				AuthorName: authorName,
				Introduce:  introduce,
				LastUse:    lastUse,
				CreateTime: localAuthor.CreateTime,
				UpdateTime: localAuthor.UpdateTime,
			}
		}
	}

	// 获取站点作者信息
	if work.SiteAuthorID.Valid && work.SiteAuthorID.String != "" {
		// 注意：SiteAuthorID 在表中是 string 类型，需要通过其他方式查询
		siteAuthors, err := s.siteAuthorReader.ListByWorkId(ctx, id)
		if err == nil && len(siteAuthors) > 0 {
			// 取第一个站点作者
			fullDTO.SiteAuthor = siteAuthors[0]
		}
	}

	// 获取站点信息
	if work.SiteID.Valid && work.SiteID.Int64 > 0 {
		site, err := s.siteReader.GetById(ctx, work.SiteID.Int64)
		if err == nil && site != nil {
			siteName := ""
			if site.SiteName.Valid {
				siteName = site.SiteName.String
			}
			fullDTO.Site = &domain.SelectItem{
				Value: site.ID,
				Label: siteName,
			}
		}
	}

	// 获取本地标签信息
	localTags, err := s.localTagReader.ListByWorkId(ctx, id)
	if err == nil && len(localTags) > 0 {
		fullDTO.LocalTags = make([]*domain.SelectItem, len(localTags))
		for i, tag := range localTags {
			tagName := ""
			if tag.LocalTagName.Valid {
				tagName = tag.LocalTagName.String
			}
			fullDTO.LocalTags[i] = &domain.SelectItem{
				Value: tag.ID,
				Label: tagName,
			}
		}
	}

	// 获取站点标签信息
	siteTags, err := s.siteTagReader.ListByWorkId(ctx, id)
	if err == nil && len(siteTags) > 0 {
		fullDTO.SiteTags = make([]*domain.SelectItem, len(siteTags))
		for i, tag := range siteTags {
			tagName := ""
			if tag.SiteTagName.Valid {
				tagName = tag.SiteTagName.String
			}
			fullDTO.SiteTags[i] = &domain.SelectItem{
				Value: tag.ID,
				Label: tagName,
			}
		}
	}

	// 获取资源信息
	resources, err := s.resourceReader.ListByWorkId(ctx, id)
	if err == nil && len(resources) > 0 {
		fullDTO.Resources = resources
	}

	return fullDTO, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的本地作者
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return []*model.RankedLocalAuthor{}, nil
	}
	// 获取作品列表
	works, err := s.repo.ListByIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 收集所有本地作者ID
	authorMap := make(map[int64]*model.RankedLocalAuthor)
	for _, work := range works {
		if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
			localAuthorId := work.LocalAuthorID.Int64
			if _, exists := authorMap[localAuthorId]; !exists {
				localAuthor, err := s.localAuthorReader.GetById(ctx, localAuthorId)
				if err == nil && localAuthor != nil {
					authorName := ""
					if localAuthor.AuthorName.Valid {
						authorName = localAuthor.AuthorName.String
					}
					introduce := ""
					if localAuthor.Introduce.Valid {
						introduce = localAuthor.Introduce.String
					}
					lastUse := int64(0)
					if localAuthor.LastUse.Valid {
						lastUse = localAuthor.LastUse.Int64
					}
					authorMap[localAuthorId] = &model.RankedLocalAuthor{
						ID:         localAuthor.ID,
						AuthorName: authorName,
						Introduce:  introduce,
						LastUse:    lastUse,
						CreateTime: localAuthor.CreateTime,
						UpdateTime: localAuthor.UpdateTime,
					}
				}
			}
		}
	}

	// 转换为列表
	result := make([]*model.RankedLocalAuthor, 0, len(authorMap))
	for _, author := range authorMap {
		result = append(result, author)
	}
	return result, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的站点作者
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthor, error) {
	if len(workIds) == 0 {
		return []*model.RankedSiteAuthor{}, nil
	}
	// 获取作品列表
	works, err := s.repo.ListByIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 收集所有站点作者ID
	authorMap := make(map[string]*model.RankedSiteAuthor)
	for _, work := range works {
		if work.SiteAuthorID.Valid && work.SiteAuthorID.String != "" {
			siteAuthorId := work.SiteAuthorID.String
			if _, exists := authorMap[siteAuthorId]; !exists {
				siteAuthor, err := s.siteAuthorReader.GetById(ctx, siteAuthorId)
				if err == nil && siteAuthor != nil {
					authorMap[siteAuthorId] = siteAuthor
				}
			}
		}
	}

	// 转换为列表
	result := make([]*model.RankedSiteAuthor, 0, len(authorMap))
	for _, author := range authorMap {
		result = append(result, author)
	}
	return result, nil
}

// ListReWorkAuthor 获取作品关联的作者信息（包含本地作者和站点作者）
func (s *Service) ListReWorkAuthor(ctx context.Context, workId int64) (*WorkAuthorDTO, error) {
	work, err := s.repo.GetById(ctx, workId)
	if err != nil {
		return nil, err
	}

	dto := &WorkAuthorDTO{}

	// 获取本地作者信息
	if work.LocalAuthorID.Valid && work.LocalAuthorID.Int64 > 0 {
		localAuthor, err := s.localAuthorReader.GetById(ctx, work.LocalAuthorID.Int64)
		if err == nil && localAuthor != nil {
			authorName := ""
			if localAuthor.AuthorName.Valid {
				authorName = localAuthor.AuthorName.String
			}
			introduce := ""
			if localAuthor.Introduce.Valid {
				introduce = localAuthor.Introduce.String
			}
			lastUse := int64(0)
			if localAuthor.LastUse.Valid {
				lastUse = localAuthor.LastUse.Int64
			}
			dto.LocalAuthor = &model.RankedLocalAuthor{
				ID:         localAuthor.ID,
				AuthorName: authorName,
				Introduce:  introduce,
				LastUse:    lastUse,
				CreateTime: localAuthor.CreateTime,
				UpdateTime: localAuthor.UpdateTime,
			}
		}
	}

	// 获取站点作者信息
	if work.SiteAuthorID.Valid && work.SiteAuthorID.String != "" {
		siteAuthor, err := s.siteAuthorReader.GetById(ctx, work.SiteAuthorID.String)
		if err == nil && siteAuthor != nil {
			dto.SiteAuthor = siteAuthor
		}
	}

	return dto, nil
}

// UpdateLastUsed 批量更新作品最后使用时间
func (s *Service) UpdateLastUsed(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.repo.UpdateLastViewBatch(ctx, ids, model.GetCurrentTimestamp())
}

// WorkAuthorDTO 作品作者信息
type WorkAuthorDTO struct {
	LocalAuthor *model.RankedLocalAuthor `json:"localAuthor,omitempty"`
	SiteAuthor  *model.RankedSiteAuthor  `json:"siteAuthor,omitempty"`
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *WorkQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.SiteID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
	}
	if dto.SiteWorkID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_work_id", Value: *dto.SiteWorkID})
	}
	if dto.SiteAuthorID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_author_id", Value: *dto.SiteAuthorID})
	}
	if dto.LocalAuthorID != nil {
		conditions = append(conditions, clause.Eq{Column: "local_author_id", Value: *dto.LocalAuthorID})
	}
	if dto.NickName != nil {
		conditions = append(conditions, clause.Eq{Column: "nick_name", Value: *dto.NickName})
	}
	if dto.SiteWorkNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_work_name", Value: *dto.SiteWorkNameLike})
	}
	if dto.SiteWorkDescLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_work_description", Value: *dto.SiteWorkDescLike})
	}
	if dto.NickNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "nick_name", Value: *dto.NickNameLike})
	}

	return conditions
}

// combineConditions 将多个条件组合成单个表达式
func combineConditions(conditions []clause.Expression) clause.Expression {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := clause.AndConditions{}
	for _, cond := range conditions {
		result.Exprs = append(result.Exprs, cond)
	}
	return result
}

// ErrWorkIdRequired 错误定义
var (
	ErrWorkIdRequired = &BusinessError{Code: 400, Message: "更新作品失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
