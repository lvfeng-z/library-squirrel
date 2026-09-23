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

// SiteAuthorFetcher 站点作者信息拉取能力（plugin 模块实现，候选广播路由内嵌于实现侧）：
// 候选按站点收窄——已激活插件中声明了站点作者拉取能力包、且有条目把该站点键列入声明站点、
// 且有可用服务客户端的插件条目（候选粒度 = 插件 × 条目，候选集恒等于归属集，插件不必自判归属，
// 也不存在「调用后才知不归属」）。按候选序逐个调用（请求携带候选条目 id），命中一个即止；
// 候选为空（无可归属插件条目）时返回错误。onMeta 收到首块元数据（恒为首块且仅一块），返回是否
// 继续接收头像字节——false 时实现侧取消流并按成功收尾；onMeta/onData 返回错误即中止流并作为
// 整体失败上抛
type SiteAuthorFetcher interface {
	// FetchSiteAuthorInfo 拉取单个站点作者信息。显选两键（chosenPluginPublicId + chosenExtensionId）
	// 联合命中候选集时该候选置于候选序首位，两键均空则候选全按全键字典序（自动面与单候选场景即此态）
	FetchSiteAuthorInfo(ctx context.Context, siteKey, siteAuthorId, chosenPluginPublicId, chosenExtensionId string,
		onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
		onData func(data []byte) error) error
	// ListSiteAuthorFetchCandidates 指定站点键下的候选清单（条目级消费面：插件 × siteAuthorFetch
	// 条目对，ExtensionId 携带条目 id），按候选全键（插件公开 ID 与条目 id 的 NUL 拼接）字典序，
	// 首位即默认选中项——交互面在调用前据清单判候选冲突与校验显选键
	ListSiteAuthorFetchCandidates(ctx context.Context, siteKey string) ([]*dto.PluginCandidate, error)
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

// LocalAuthorStore 本地作者行存取窄接口（localAuthor 模块实现）
type LocalAuthorStore interface {
	// GetById 查本地作者行（头像导入/移除入口：存在性校验与当前引用读取）
	GetById(ctx context.Context, id int64) (*entity.LocalAuthor, error)
	// UpdateAvatarStoreId 更新头像引用列（dbFromCtx 模式，可入导入编排事务；NULL=清除引用）
	UpdateAvatarStoreId(ctx context.Context, localAuthorId int64, storeId sql.NullInt64) error
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
	// GetById 查活行 store 行（换头像删旧时读旧行文件路径）
	GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
	// ListByIdsIncludeDeleted 按 ID 集合查 store 行（含软删行；删除联动读被引用头像行的文件
	// 路径——被删作者的软删失效行同样须一并物理删清，不留无主死行）
	ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity.PersistentStore
	// DeleteUnscopedByIds 物理删 store 行（dbFromCtx 模式，可入换头像/删除联动事务；不动文件）
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

// DisambiguationMemory 拉取候选冲突显选的粘性记忆窄接口（stickymemory.Service 实现）：
// 手动拉取面同站点多候选冲突时取回用户上次的显选（命中即直接路由不再询问），显选方随
// 「记住此选择」勾选且拉取成功后记入。记忆是消歧优化而非正确性依赖——读取未命中（含
// 读取失败）即回落冲突询问，写入失败不影响拉取结果。读写均只在手动交互路径发生：
// 自动触发面不做冲突检测，既不查询也不写入
type DisambiguationMemory interface {
	// Remember 记住一次显选（upsert 幂等）：domain 为记忆域常量，contextKey 为站点域与
	// 候选组合的编码串，value 为显选候选全键
	Remember(ctx context.Context, domain, contextKey, value string) error
	// Recall 按域与上下文键取回显选值，命中返回 (显选候选全键, true)
	Recall(ctx context.Context, domain, contextKey string) (string, bool)
}
