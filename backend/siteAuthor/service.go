package siteAuthor

import (
	"context"
	"database/sql"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 站点作者仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, author *entity.SiteAuthor) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, authors []*entity.SiteAuthor) error
	// Updates 更新
	Updates(ctx context.Context, author *entity.SiteAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.SiteAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.SiteAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.SiteAuthor], error)
	// ListByWorkId 查询作品的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error)
	// ListBySiteAuthorIds 根据站点作者ID列表查询
	ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity.SiteAuthor, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error)
	// UpdateBindLocalAuthor 绑定本地作者
	UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (int64, error)
	// UpdateLastUseByIds 批量更新最后使用时间
	UpdateLastUseByIds(ctx context.Context, ids []int64, lastUse int64) error
	// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
	GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error)
	// Upsert 原子插入或更新
	Upsert(ctx context.Context, author *entity.SiteAuthor) error
	// BatchUpsert 批量插入或更新
	BatchUpsert(ctx context.Context, authors []*entity.SiteAuthor) error
	// ListBySiteAndSiteAuthorIDs 根据站点ID和站点作者ID列表批量查询
	ListBySiteAndSiteAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity.SiteAuthor, error)
	// ListFetchTargetsByIds 批量反查信息拉取目标行（JOIN site 反查 site_key；authorInfo 拉取编排消费）
	ListFetchTargetsByIds(ctx context.Context, ids []int64) ([]*dto.SiteAuthorFetchTarget, error)
	// UpdateAvatarStoreId 更新头像引用列（可入拉取编排事务；NULL=清除引用）
	UpdateAvatarStoreId(ctx context.Context, siteAuthorId int64, storeId sql.NullInt64) error
}

// Transactor 数据库事务执行器（删除编排用）
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// ReWorkAuthorDeleter 作品-作者关联删除接口（reWorkAuthor 服务实现）
type ReWorkAuthorDeleter interface {
	// DeleteBySiteAuthorId 删除站点作者的全部作品关联
	DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error
}

// LocalAuthorOperator 本地作者接口
type LocalAuthorOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error)
	GetByName(ctx context.Context, name string) (*entity.LocalAuthor, error)
	GetByNames(ctx context.Context, names []string) ([]*entity.LocalAuthor, error)
	Save(ctx context.Context, author *entity.LocalAuthor) error
}

// SiteOperator 站点接口
type SiteOperator interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error)
}

// AvatarFileCleaner 作者头像清理接口（authorInfo 服务实现）：删除联动的头像行面进本模块
// 删除事务、文件面在事务提交后清理
type AvatarFileCleaner interface {
	// DeleteAvatarStoreRows 事务内物理删头像 persistent_store 行（dbFromCtx 入本删除事务；
	// 须在作者行删除之后调用——FK NO ACTION，引用未释放时删行被拒）。返回被删行的文件
	// 相对路径清单（供事务提交后删文件）
	DeleteAvatarStoreRows(ctx context.Context, storeIds []int64) ([]string, error)
	// RemoveAvatarFiles 事务提交后物理删头像文件（含 fsmonitor 操作抑制登记；尽力而为，
	// 失败记日志留对账裁决）
	RemoveAvatarFiles(relPaths []string)
}

// AvatarStoreReader 头像 persistent_store 行批量读取（persistentStore.Service 实现）：GetByIds
// 经 GORM 软删 scope 自动排除死行，落盘完成判定由 dto.AvatarFilePathByStoreID 统一承担
type AvatarStoreReader interface {
	// GetByIds 根据 ID 列表批量查询记录
	GetByIds(ctx context.Context, ids []int64) ([]*entity.PersistentStore, error)
}

// Service 站点作者服务
type Service struct {
	repo          Repository
	localAuthorOp LocalAuthorOperator
	siteOp        SiteOperator
	// 事务执行器（删除编排）
	transactor Transactor
	// 删除编排的关联清理提供方（窄接口注入）
	reWorkAuthorDeleter ReWorkAuthorDeleter
	// 删除编排的头像清理提供方（authorInfo 在本服务之后创建，经 setter 注入；nil=跳过头像清理）
	avatarFileCleaner AvatarFileCleaner
	// 头像 store 行读取（persistentStore 在本服务之后创建，经 setter 注入；nil=跳过头像 enrich）
	avatarStoreReader AvatarStoreReader
}

// NewService 创建站点作者服务。transactor 承载删除编排事务；reWorkAuthorDeleter 供
// Delete 清理被删站点作者挂载的作品-作者关联
func NewService(repo Repository, localAuthorQueryOp LocalAuthorOperator, siteOp SiteOperator, transactor Transactor, reWorkAuthorDeleter ReWorkAuthorDeleter) *Service {
	return &Service{
		repo:                repo,
		localAuthorOp:       localAuthorQueryOp,
		siteOp:              siteOp,
		transactor:          transactor,
		reWorkAuthorDeleter: reWorkAuthorDeleter,
	}
}

// Save 保存站点作者
func (s *Service) Save(ctx context.Context, author *entity.SiteAuthor) error {
	return s.repo.Create(ctx, author)
}

// SaveBatch 批量保存站点作者
func (s *Service) SaveBatch(ctx context.Context, authors []*entity.SiteAuthor) error {
	return s.repo.CreateBatch(ctx, authors)
}

// UpdateById 更新站点作者
func (s *Service) UpdateById(ctx context.Context, author *entity.SiteAuthor) error {
	if author.ID == 0 {
		return ErrAuthorIdRequired
	}
	return s.repo.Updates(ctx, author)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	return s.repo.UpdateLastUseByIds(ctx, ids, now)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity.SiteAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity.SiteAuthor, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除站点作者：同一事务内先删该作者挂载的全部作品-作者关联，再删作者行，最后物理删
// 头像 persistent_store 行（文件面在事务提交后清理）——re_work_author.site_author_id 与
// site_author.avatar_store_id 均有外键防线，先清子后删父为强制顺序；头像行删除排在作者行
// 之后（作者行即引用方，行未删即删 store 行会被外键拒绝）
func (s *Service) Delete(ctx context.Context, id int64) error {
	// 事务前读作者行头像引用（作者行删除后不可再读）
	var avatarStoreIds []int64
	if s.avatarFileCleaner != nil {
		author, err := s.repo.GetById(ctx, id)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			author = nil
		}
		if author != nil && author.AvatarStoreID.Valid {
			avatarStoreIds = append(avatarStoreIds, author.AvatarStoreID.Int64)
		}
	}
	var avatarRelPaths []string
	if err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.reWorkAuthorDeleter.DeleteBySiteAuthorId(txCtx, id); err != nil {
			return err
		}
		// 关联已清空，外键放行
		if err := s.repo.Delete(txCtx, id); err != nil {
			return err
		}
		if len(avatarStoreIds) == 0 {
			return nil
		}
		paths, err := s.avatarFileCleaner.DeleteAvatarStoreRows(txCtx, avatarStoreIds)
		if err != nil {
			return err
		}
		avatarRelPaths = paths
		return nil
	}); err != nil {
		return err
	}
	// 事务提交后清理头像文件（行面已消亡，文件面尽力而为）
	if len(avatarRelPaths) > 0 {
		s.avatarFileCleaner.RemoveAvatarFiles(avatarRelPaths)
	}
	return nil
}

// SetAvatarFileCleaner 注入头像清理提供方（authorInfo 服务在本服务之后创建，装配处接线）
func (s *Service) SetAvatarFileCleaner(cleaner AvatarFileCleaner) {
	s.avatarFileCleaner = cleaner
}

// SetAvatarStoreReader 注入头像 store 行读取（persistentStore 服务在本服务之后创建，装配处接线）
func (s *Service) SetAvatarStoreReader(reader AvatarStoreReader) {
	s.avatarStoreReader = reader
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity.SiteAuthor], query SiteAuthorQueryDTO) (*model.Page[entity.SiteAuthor], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (s *Service) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO) (*model.Page[dto.SiteAuthorLocalRelateDTO], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})

	var boundOnLocalAuthorId *bool
	if query.BoundOnLocalAuthorId.Value != nil {
		boundOnLocalAuthorId = query.BoundOnLocalAuthorId.Value
	}
	var localAuthorId *int64
	if query.LocalAuthorID.Value != nil {
		localAuthorId = query.LocalAuthorID.Value
		// 避免ToPageOption生成LocalAuthorID的默认条件
		query.LocalAuthorID.Value = nil
	}

	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}

	// 根据 boundOnLocalAuthorId 添加 localAuthorId 的过滤条件
	if localAuthorId != nil {
		if boundOnLocalAuthorId != nil && *boundOnLocalAuthorId {
			// 绑定到指定本地作者
			opt.Conditions = append(opt.Conditions, clause.Eq{Column: "local_author_id", Value: *localAuthorId})
		} else if boundOnLocalAuthorId != nil && !*boundOnLocalAuthorId {
			// 未绑定到指定本地作者（包括绑定到其他本地作者或从未绑定过本地作者的）
			opt.Conditions = append(opt.Conditions, clause.Expr{SQL: "(local_author_id != ? OR local_author_id IS NULL)", Vars: []any{*localAuthorId}})
		}
	}

	rawPage, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 填充关联数据（含 HasSameNameLocalAuthor）
	return s.enrichLocalRelateDTO(ctx, rawPage)
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO) (*model.Page[dto.SiteAuthorLocalRelateDTO], error) {
	conv := querypkg.NewConverter(entity.SiteAuthor{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}

	rawPage, err := s.repo.Page(ctx, opt)
	if err != nil {
		return nil, err
	}

	return s.enrichLocalRelateDTO(ctx, rawPage)
}

// enrichLocalRelateDTO 批量填充站点作者关联DTO的关联数据
func (s *Service) enrichLocalRelateDTO(ctx context.Context, rawPage *model.Page[entity.SiteAuthor]) (*model.Page[dto.SiteAuthorLocalRelateDTO], error) {
	siteAuthors := rawPage.Data
	if len(siteAuthors) == 0 {
		return model.NewPage[dto.SiteAuthorLocalRelateDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}

	// 收集关联 ID 和作者名称
	localAuthorIds := make([]int64, 0)
	siteIds := make([]int64, 0)
	authorNames := make([]string, 0)
	for _, author := range siteAuthors {
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			localAuthorIds = append(localAuthorIds, author.LocalAuthorID.Int64)
		}
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			siteIds = append(siteIds, author.SiteID.Int64)
		}
		if author.AuthorName.Valid && author.AuthorName.String != "" {
			authorNames = append(authorNames, author.AuthorName.String)
		}
	}

	// 批量查询 LocalAuthor by IDs
	localAuthorMap := make(map[int64]*sdkdto.LocalAuthorDTO)
	if len(localAuthorIds) > 0 {
		localAuthors, err := s.localAuthorOp.ListByIds(ctx, localAuthorIds)
		if err != nil {
			return nil, err
		}
		for _, lt := range localAuthors {
			localAuthorMap[lt.ID] = &sdkdto.LocalAuthorDTO{
				Id:         lt.GetID(),
				AuthorName: util.NullStringToPointer(lt.AuthorName),
				Introduce:  util.NullStringToPointer(lt.Introduce),
				CreateTime: lt.GetCreateTime(),
				UpdateTime: lt.GetUpdateTime(),
			}
		}
	}

	// 批量查询 Site
	siteMap := make(map[int64]*sdkdto.SiteDTO)
	if len(siteIds) > 0 {
		sites, err := s.siteOp.ListByIds(ctx, util.UniqueInt64(siteIds))
		if err != nil {
			return nil, err
		}
		for _, st := range sites {
			siteMap[st.ID] = dto.NewSiteDTO(st)
		}
	}

	// 批量检查 HasSameNameLocalAuthor
	sameNameMap := make(map[string]bool)
	if len(authorNames) > 0 {
		uniqueNames := util.UniqueString(authorNames)
		localAuthors, err := s.localAuthorOp.GetByNames(ctx, uniqueNames)
		if err != nil {
			return nil, err
		}
		for _, la := range localAuthors {
			if la.AuthorName.Valid {
				sameNameMap[la.AuthorName.String] = true
			}
		}
	}

	// 组装结果
	results := make([]*dto.SiteAuthorLocalRelateDTO, 0, len(siteAuthors))
	avatarPaths := make(map[int64]*string)
	if s.avatarStoreReader != nil {
		storeIds := make([]int64, 0, len(siteAuthors))
		for _, author := range siteAuthors {
			if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
				storeIds = append(storeIds, author.AvatarStoreID.Int64)
			}
		}
		stores, err := s.avatarStoreReader.GetByIds(ctx, util.UniqueInt64(storeIds))
		if err != nil {
			return nil, err
		}
		avatarPaths = dto.AvatarFilePathByStoreID(stores)
	}
	for _, author := range siteAuthors {
		relateDTO := dto.NewSiteAuthorLocalRelateDTO(author)
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			relateDTO.LocalAuthor = localAuthorMap[author.LocalAuthorID.Int64]
		}
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			relateDTO.Site = siteMap[author.SiteID.Int64]
		}
		if author.AuthorName.Valid {
			relateDTO.HasSameNameLocalAuthor = sameNameMap[author.AuthorName.String]
		}
		if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
			relateDTO.AvatarFilePath = avatarPaths[author.AvatarStoreID.Int64]
		}
		results = append(results, relateDTO)
	}

	return model.NewPage[dto.SiteAuthorLocalRelateDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// AvatarFilePathsByAuthorIds 批量解析站点作者头像展示路径（作者 DB id → workDir 相对路径；
// 无可展示头像的 id 不出现在返回 map，展示侧占位图兜底）。供本模块展示链组装与 reWorkAuthor
// 的 Ranked* 产出后置 enrich 共用
func (s *Service) AvatarFilePathsByAuthorIds(ctx context.Context, authorIds []int64) (map[int64]*string, error) {
	authorIds = util.UniqueInt64(authorIds)
	if len(authorIds) == 0 || s.avatarStoreReader == nil {
		return nil, nil
	}
	authors, err := s.repo.ListBySiteAuthorIds(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	storeIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
			storeIds = append(storeIds, author.AvatarStoreID.Int64)
		}
	}
	if len(storeIds) == 0 {
		return nil, nil
	}
	stores, err := s.avatarStoreReader.GetByIds(ctx, storeIds)
	if err != nil {
		return nil, err
	}
	avatarPaths := dto.AvatarFilePathByStoreID(stores)
	result := make(map[int64]*string, len(authors))
	for _, author := range authors {
		if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
			if path, ok := avatarPaths[author.AvatarStoreID.Int64]; ok {
				result[author.ID] = path
			}
		}
	}
	return result, nil
}

// fillRankedAvatarFilePaths 批量填充带排序站点作者 DTO 的头像字段（后置 enrich）
func (s *Service) fillRankedAvatarFilePaths(ctx context.Context, authors []*dto.RankedSiteAuthor) error {
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.ID > 0 {
			authorIds = append(authorIds, author.Author.ID)
		}
	}
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.ID]
	}
	return nil
}

// ListByWorkId 查询作品的站点作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	results, err := s.repo.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if err := s.fillRankedAvatarFilePaths(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (s *Service) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity.SiteAuthor, error) {
	return s.repo.ListBySiteAuthorIds(ctx, siteAuthorIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
	results, err := s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	authorIds := make([]int64, 0, len(results))
	for _, author := range results {
		if author.Author.ID > 0 {
			authorIds = append(authorIds, author.Author.ID)
		}
	}
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	for _, author := range results {
		author.AvatarFilePath = avatarPaths[author.Author.ID]
	}
	return results, nil
}

// UpdateBindLocalAuthor 绑定或解除本地作者绑定
func (s *Service) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (bool, error) {
	if len(siteAuthorIds) == 0 {
		return true, nil
	}
	affected, err := s.repo.UpdateBindLocalAuthor(ctx, localAuthorId, siteAuthorIds)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// CreateSameNameLocalAuthor 创建或获取同名的本地作者
func (s *Service) CreateSameNameLocalAuthor(ctx context.Context, siteAuthor *entity.SiteAuthor) (int64, error) {
	if !siteAuthor.AuthorName.Valid {
		return 0, nil
	}
	// 查询是否已有同名作者（通过依赖注入的 LocalAuthorOperator）
	existing, err := s.localAuthorOp.GetByName(ctx, siteAuthor.AuthorName.String)
	if err == nil && existing != nil {
		return existing.ID, nil
	}

	// 新增同名作者（通过依赖注入的 LocalAuthorOperator）
	newLocalAuthor := entity.NewLocalAuthor()
	newLocalAuthor.AuthorName = siteAuthor.AuthorName
	newLocalAuthor.Introduce = siteAuthor.Introduce

	if err := s.localAuthorOp.Save(ctx, newLocalAuthor); err != nil {
		return 0, err
	}
	return newLocalAuthor.ID, nil
}

// CreateAndBindSameNameLocalAuthor 创建并绑定同名的本地作者
func (s *Service) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *entity.SiteAuthor) (bool, error) {
	if siteAuthor.ID == 0 {
		return false, pkgerr.NewBusinessError(400, "创建同名本地作者失败，作者ID不能为空")
	}
	if !siteAuthor.AuthorName.Valid || siteAuthor.AuthorName.String == "" {
		return false, pkgerr.NewBusinessError(400, "创建同名本地作者失败，作者名称不能为空")
	}

	localAuthorId, err := s.CreateSameNameLocalAuthor(ctx, siteAuthor)
	if err != nil {
		return false, err
	}

	return s.UpdateBindLocalAuthor(ctx, &localAuthorId, []int64{siteAuthor.ID})
}

// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
func (s *Service) GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error) {
	return s.repo.GetBySiteAndSiteAuthorID(ctx, siteId, siteAuthorId)
}

// SaveOrUpdateByCompositeKey 按 (siteId, siteAuthorId) 原子保存或更新站点作者，返回内部 DB ID
func (s *Service) SaveOrUpdateByCompositeKey(ctx context.Context, author *entity.SiteAuthor) (int64, error) {
	if err := s.repo.Upsert(ctx, author); err != nil {
		return 0, err
	}
	if author.ID > 0 {
		return author.ID, nil
	}
	// OnConflict 更新场景下 ID 可能未回填，查询获取
	existing, err := s.repo.GetBySiteAndSiteAuthorID(ctx, author.SiteID.Int64, author.SiteAuthorID.String)
	if err != nil {
		return 0, err
	}
	return existing.ID, nil
}

// BatchUpsert 批量插入或更新站点作者
func (s *Service) BatchUpsert(ctx context.Context, authors []*entity.SiteAuthor) error {
	return s.repo.BatchUpsert(ctx, authors)
}

// ListBySiteAndSiteAuthorIDs 根据站点ID和站点作者ID列表批量查询
func (s *Service) ListBySiteAndSiteAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity.SiteAuthor, error) {
	return s.repo.ListBySiteAndSiteAuthorIDs(ctx, siteId, siteAuthorIds)
}

// Upsert 原子插入或更新站点作者（插件权威域白名单列覆盖语义——拉取元数据回写经此复用，
// 与任务链重拉一套覆盖语义）
func (s *Service) Upsert(ctx context.Context, author *entity.SiteAuthor) error {
	return s.repo.Upsert(ctx, author)
}

// ListFetchTargetsByIds 批量反查站点作者信息拉取目标行（authorInfo 拉取编排消费）
func (s *Service) ListFetchTargetsByIds(ctx context.Context, ids []int64) ([]*dto.SiteAuthorFetchTarget, error) {
	return s.repo.ListFetchTargetsByIds(ctx, ids)
}

// UpdateAvatarStoreId 更新站点作者头像引用列（可入拉取编排事务；NULL=清除引用）
func (s *Service) UpdateAvatarStoreId(ctx context.Context, siteAuthorId int64, storeId sql.NullInt64) error {
	return s.repo.UpdateAvatarStoreId(ctx, siteAuthorId, storeId)
}

// 错误定义
var (
	ErrAuthorIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新站点作者失败，id不能为空"}
)
