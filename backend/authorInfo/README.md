# authorInfo 模块说明

## 一句话职责

作者个人信息（当前为头像，含站点侧元数据）的编排宿主：以发起方身份串联插件拉取能力、
site_author/local_author 元数据与引用列写入、persistentStore 入库事务，提供 site 侧拉取入口
（手动 + 作品入库后自动）、local 侧头像手动导入/移除入口，以及 siteAuthor/localAuthor 删除
联动的头像行与文件清理。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `FetchSiteAuthorInfo(ctx, siteAuthorId, chosenPlugins)` | 手动拉取单个站点作者信息（行操作）。`chosenPlugins` 为交互面显选清单（站点键 → 插件，空=未显选）：该站点候选 ≥2 且未显选时返回冲突载荷 `SiteAuthorFetchConflict{Conflict, SiteKey, Candidates}`（未调用任何插件），前端选择后带键重发；失败上抛（`handler.go:38-48`） |
| `FetchSiteAuthorsInfo(ctx, siteAuthorIds, chosenPlugins)` | 手动批量拉取（逐作者串行，返回逐条成功/失败清单 `[]*SiteAuthorFetchItemResult`）；候选冲突**按站点分组整批前置问一次**（同站点只问一次，未调用任何插件），否则逐条结果与入参顺序一致（`handler.go:50-62`） |
| `SetLocalAuthorAvatar(ctx, localAuthorId, sourceAbsPath)` | 为本地作者导入头像（源=前端文件对话框选取的任意盘绝对路径；换头像先删旧） |
| `RemoveLocalAuthorAvatar(ctx, localAuthorId)` | 移除本地作者头像（显式破坏操作，前端二次确认；无头像幂等成功） |

另被 work 消费：实现 work 的 `SiteAuthorRefreshScheduler` 窄接口（`OnSiteAuthorsUpserted(siteAuthorIds)`，
入库链 upsert 站点作者后调用，实现侧非阻塞）。

另被 siteAuthor / localAuthor 消费：实现二者的 `AvatarFileCleaner` 窄接口（删除联动的头像
清理，见「关键设计」）。

## 核心概念

- **单作者拉取序列（site 侧）**：能力广播定位插件条目（`SiteAuthorFetcher`，plugin 模块实现）→ RPC
  流式首块 meta 回写 site_author（upsert 白名单列，与任务链重拉同一套覆盖语义）→ 判定需落字节
  （站点声明头像且（来源 URL 变化或行无 avatar_store_id））→ 换头像先删旧 → 字节写暂存（10MB
  上限）→ persistentStore 四调用入库（撤回处置恒丢弃）→ 业务事务内建 store 行 + 同事务写
  `site_author.avatar_store_id` → 作用域回收。
- **能力广播路由**：候选=声明 `extensions.siteAuthorFetch` 条目、且该条目 `sites` 含本次请求站点键、插件有可用服务客户端的 (插件, 条目) 对（条目级消费面，候选粒度=条目）。候选在发现侧即按站点归属收窄，故**候选集恒等于归属集**，插件不自判归属、「调用后才知不归属」态不复存在。经 `backend/route` 基座按候选全键（插件公开 ID 与条目 id 的 NUL 拼接）字典序逐个调用（拉取请求携带候选条目 id，插件侧按条目分派），命中一个即止、任一候选失败即终止并点名候选（单极，无不适配顺延）；零候选单态收口报「无插件覆盖该站点」（实现在 plugin/extension 的 `site_author_fetcher.go`）。
- **候选冲突前置检测**：两个手动拉取入口在任何插件调用之前先经 `resolveFetchSelection` 做冲突检测与显选解析——候选按站点收窄，故须**先解析目标行拿站点键**再做检测；按本次触发的站点键逐站枚举候选，未显选且该站点候选 ≥2 即记一组冲突（同站点只记一组，返回冲突载荷且未调用任何插件），显选键（插件公开 ID + 条目 id 两键）解析为该站点候选集内的具体条目——条目 id 缺省时该插件在候选集内须恰有一个条目（单实例插件显选补全为该条目），未命中或无法定位报 `ErrChosenPluginInvalid`。批量拉取**按站点分组整批问一次**（非逐作者问）。候选清单经 `SiteAuthorFetcher.ListSiteAuthorFetchCandidates` 枚举面取得（按候选全键字典序，首位即默认选中项；展示名=「插件名 · 条目名」）。
- **两触发面共用在途去重**：mutex + map 的作者 DB ID 集合；在途时再次手动拒绝（409）、批量记
  跳过、自动触发静默跳过。开关 `authorSettings.autoFetchInfo`（默认开）只控制自动触发面。
- **失败语义**：meta 回写与资源落库各自独立成功（头像缺省是合法态）；流中断/入库失败不留
  半成品（无 store 行、无引用、暂存回收）；单次触发单次尝试不重试，失败 Warn 记日志。
- **换头像（site 侧）**：先删旧（事务内清引用列 + 删 store 行，事务外物理删文件含操作抑制）再走
  新序列，崩溃窗口收敛为「无头像」；ext 不同即路径不同，不做同路径覆写。
- **local 侧头像导入**：源文件校验图片格式白名单（jpg/jpeg/png/gif/webp/bmp）→ 拷入暂存作用域
  （源在任意盘跨卷不可 rename，恒 copy；尺寸上限与 site 侧一致）→ 旧头像按换头像形态先行删除
  （同扩展名时新旧路径相同，先删旧保证不覆写、不撞 file_path 活行唯一索引；拷贝成功后才删旧，
  源不可读/超限的失败不伤现有头像）→ 四调用入库 → 业务事务内建行 + 同事务写
  `local_author.avatar_store_id`。暂存作用域键 = `local-{作者行 DB id}`（与 site 侧裸数字键
  前缀消歧，防两表 id 跨表撞作用域目录）。

## 依赖关系

- 依赖：plugin/extension（`SiteAuthorFetcher` 能力桥，`SetSiteAuthorFetcher` 延迟注入；该窄接口含拉取与
  候选枚举两面 `FetchSiteAuthorInfo` / `ListSiteAuthorFetchCandidates`）、
  siteAuthor（`SiteAuthorStore`：目标行 JOIN site 反查 / 元数据回写 upsert / 引用列更新）、
  localAuthor（`LocalAuthorStore`：行查询 / 引用列更新）、persistentStore（`StoreIngestor` 四
  调用入库 + `AvatarStoreOps` 行查询与事务内物理删）、settings（`AuthorFetchSettings` 开关 +
  `WorkDirProvider`）、database 事务执行器、staging（`OwnerAuthorInfo` 作用域）、storeRegistry
  （文件删除的操作抑制登记）。
- 被依赖：work（入库后自动触发面）、siteAuthor / localAuthor（删除联动 `AvatarFileCleaner`）、
  前端站点作者管理页（手动拉取入口）与本地作者管理页（头像导入/移除入口，前端展示属后续阶段）。

## 关键设计

- 本模块无自有仓储——作者行经 siteAuthor/localAuthor 窄接口、store 行经 persistentStore
  （ORCHESTRATION_BY_CALLER）。
- 头像落盘路径派生（`SiteAvatarRelPath` / `LocalAvatarRelPath`，paths.go）与 store 存储目录注册
  （`RegisterStoreDirs`，store_dir.go）以本包为单一源，装配期（app.go）先于 fsmonitor 调用。
- 引用列 `site_author.avatar_store_id` / `local_author.avatar_store_id` 仅本模块编排事务内写
  （不进 upsert 覆盖域，随行保留）。
- **删除联动（`AvatarFileCleaner`，两阶段形态）**：siteAuthor/localAuthor 的 Delete 编排注入本
  服务——事务前调用方读作者行头像引用；删除事务内在**作者行删除之后**调 `DeleteAvatarStoreRows`
  物理删 store 行（FK NO ACTION 下引用未释放即删行被拒，先删父引用方为强制顺序；被删作者的
  软删失效行一并物理删清，不留无主死行）；事务提交后调 `RemoveAvatarFiles` 物理删文件（含操作
  抑制登记，尽力而为）。物理删不进备份（裁决：头像体积小、可重拉、无复原价值，无
  BackupReferencer 登记项）。
- 接口由调用方（siteAuthor/localAuthor）各自定义同形窄接口、本服务以同一方法集实现
  （SERVICE_DEPENDENCY_VIA_INTERFACE，Go 结构化匹配天然满足）。
