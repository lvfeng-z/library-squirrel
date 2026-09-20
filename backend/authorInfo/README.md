# authorInfo 模块说明

## 一句话职责

作者个人信息（当前为头像，含站点侧元数据）的拉取编排宿主：以发起方身份串联插件拉取能力、
site_author 元数据回写与 persistentStore 入库事务，提供手动拉取入口与作品入库后自动触发面。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `FetchSiteAuthorInfo(ctx, siteAuthorId)` | 手动拉取单个站点作者信息（行操作，失败上抛） |
| `FetchSiteAuthorsInfo(ctx, siteAuthorIds)` | 手动批量拉取（逐作者串行，返回逐条成功/失败清单 `[]*SiteAuthorFetchItemResult`） |

（local 侧导入入口 `SetLocalAuthorAvatar` / `RemoveLocalAuthorAvatar` 属删除联动+local 导入阶段。）

另被 work 消费：实现 work 的 `SiteAuthorRefreshScheduler` 窄接口（`OnSiteAuthorsUpserted(siteAuthorIds)`，
入库链 upsert 站点作者后调用，实现侧非阻塞）。

## 核心概念

- **单作者拉取序列**：能力广播定位插件（`SiteAuthorFetcher`，plugin 模块实现）→ RPC 流式首块
  meta 回写 site_author（upsert 白名单列，与任务链重拉同一套覆盖语义）→ 判定需落字节（站点
  声明头像且（来源 URL 变化或行无 avatar_store_id））→ 换头像先删旧 → 字节写暂存（10MB 上限）→
  persistentStore 四调用入库（撤回处置恒丢弃）→ 业务事务内建 store 行 + 同事务写
  `site_author.avatar_store_id` → 作用域回收。
- **能力广播路由**：按 siteKey 遍历声明 `siteAuthorFetch` 能力的已激活插件逐个调用，插件归属
  自判（未归属=PermissionDenied 静默跳过），命中一个即止；无归属收口报错。
- **两触发面共用在途去重**：mutex + map 的作者 DB ID 集合；在途时再次手动拒绝（409）、批量记
  跳过、自动触发静默跳过。开关 `authorSettings.autoFetchInfo`（默认开）只控制自动触发面。
- **失败语义**：meta 回写与资源落库各自独立成功（头像缺省是合法态）；流中断/入库失败不留
  半成品（无 store 行、无引用、暂存回收）；单次触发单次尝试不重试，失败 Warn 记日志。
- **换头像**：先删旧（事务内清引用列 + 删 store 行，事务外物理删文件含操作抑制）再走新序列，
  崩溃窗口收敛为「无头像」；ext 不同即路径不同，不做同路径覆写。

## 依赖关系

- 依赖：plugin/extension（`SiteAuthorFetcher` 能力桥，`SetSiteAuthorFetcher` 延迟注入）、
  siteAuthor（`SiteAuthorStore`：目标行 JOIN site 反查 / 元数据回写 upsert / 引用列更新）、
  persistentStore（`StoreIngestor` 四调用 + `AvatarStoreOps` 行查询与事务内物理删）、
  settings（`AuthorFetchSettings` 开关 + `WorkDirProvider`）、database 事务执行器、
  staging（`OwnerAuthorInfo` 作用域，键=作者行 DB id）。
- 被依赖：work（入库后自动触发面）、前端站点作者管理页（手动拉取入口）。

## 关键设计

- 本模块无自有仓储——作者行经 siteAuthor 窄接口、store 行经 persistentStore（ORCHESTRATION_BY_CALLER）。
- 头像落盘路径派生（`SiteAvatarRelPath` / `LocalAvatarRelPath`，paths.go）与 store 存储目录注册
  （`RegisterStoreDirs`，store_dir.go）以本包为单一源，装配期（app.go）先于 fsmonitor 调用。
- 引用列 `site_author.avatar_store_id` 仅拉取编排事务内写（不进 upsert 覆盖域，随行保留）。
