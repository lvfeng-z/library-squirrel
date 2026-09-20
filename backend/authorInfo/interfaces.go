package authorInfo

// 本文件集中登记 authorInfo 作为拉取编排发起方（ORCHESTRATION_BY_CALLER）依赖的提供方窄接口：
// 接口由本模块定义、提供方模块实现，app.go 装配接线。拉取编排自身不建仓储——作者行经
// siteAuthor、store 行经 persistentStore。

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/persistentStore"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// SiteAuthorFetcher 站点作者信息拉取能力（plugin 模块实现，能力广播路由内嵌于实现侧：
// 遍历声明 siteAuthorFetch 能力的已激活插件，插件按请求 siteKey 归属自判，未归属静默跳过、
// 命中一个即止；无归属插件时返回错误）。onMeta 收到首块元数据（恒为首块且仅一块），返回
// 是否继续接收头像字节——false 时实现侧取消流并按成功收尾；onMeta/onData 返回错误即中止流
// 并作为整体失败上抛
type SiteAuthorFetcher interface {
	FetchSiteAuthorInfo(ctx context.Context, siteKey, siteAuthorId string,
		onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
		onData func(data []byte) error) error
}

// SiteAuthorStore 站点作者行存取窄接口（siteAuthor 模块实现）
type SiteAuthorStore interface {
	// ListFetchTargetsByIds 批量反查拉取目标行（JOIN site 反查 site_key；行缺失不报错返回交集）
	ListFetchTargetsByIds(ctx context.Context, ids []int64) ([]*dto.SiteAuthorFetchTarget, error)
	// Upsert 回写拉回元数据（插件权威域白名单列覆盖，与任务链重拉同一套覆盖语义；
	// 引用列 avatar_store_id 不在覆盖域，按行保留）
	Upsert(ctx context.Context, author *entity.SiteAuthor) error
	// UpdateAvatarStoreId 更新头像引用列（dbFromCtx 模式，可入拉取编排事务；NULL=清除引用）
	UpdateAvatarStoreId(ctx context.Context, siteAuthorId int64, storeId sql.NullInt64) error
}

// StoreIngestor 提交点入库事务窄接口（persistentStore.Service 实现，四调用拆分见
// backend/persistentStore/README.md「入库事务机制」）
type StoreIngestor interface {
	PrepareIngest(ctx context.Context, items []persistentStore.IngestItem) ([]int64, error)
	PlaceIngest(ctx context.Context, intentIds []int64) error
	CommitIngest(ctx context.Context, intentId int64, relPath string, fileName string,
		expectedSha, actualSha sql.NullString) (int64, error)
	AbortIngest(ctx context.Context, intentIds []int64) error
}

// AvatarStoreOps 头像 persistent_store 行操作窄接口（persistentStore.Service 实现）
type AvatarStoreOps interface {
	// GetById 查 store 行（换头像删旧时读旧行文件路径）
	GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
	// DeleteUnscopedByIds 物理删 store 行（dbFromCtx 模式，可入换头像事务；不动文件）
	DeleteUnscopedByIds(ctx context.Context, ids []int64) error
}

// AuthorFetchSettings 作者信息拉取设置读取（settings.Service 实现）：开关只控制作品入库后
// 自动触发面，手动拉取不受其限制
type AuthorFetchSettings interface {
	AuthorAutoFetchInfoEnabled() bool
}

// WorkDirProvider 工作目录读取（settings.Service 实现）：空串=未配置，拉取入口守卫拦截
type WorkDirProvider interface {
	GetWorkDir() string
}

// Transactor 数据库事务执行器（拉取提交点与换头像删旧事务）
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
