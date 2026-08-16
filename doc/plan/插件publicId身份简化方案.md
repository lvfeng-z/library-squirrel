# 插件 publicId 身份简化方案

## 审查摘要

**关键声明（抽查项）**

- 声明1：publicId 现构造为 `author + "/" + id`（`backend/base/model/dto/plugin_types.go:59`），后端唯一调用方是同文件 `:80` 的 `ToPluginInstallDTO`（grep `GetPublicID\(\)` 全后端仅此两处）。
- 声明2：四个插件仓库 plugin.json 的 id 均含 UUID 后缀（各仓库 `plugin.json:2`）：`com.lvfeng.pixivSuite_001594e7-…`、`com.lvfeng.localImport_7a3b2c1d-…`、`com.lvfeng.bilibiliSuite_e068dd2e-…`、`com.lvfeng.test-plugin_00000000-…`。
- 声明3：InstallBundled 以 `GetByPublicId` 判已装（`backend/plugin/service.go:364`）、buildId 判据决定升级（`:384`）；升级重装复用原记录 ID 与 CreateTime（`service.go:462-464`）——迁移后记录 ID 不变即保住 plugin_storage。
- 声明4：plugin_storage 按 `(plugin_id, key)` 唯一索引（`backend/base/model/entity/plugin_storage.go:13-14`）；存量：dev 库 pixiv 2 条 + bilibili 1 条，生产库 0 条（复现：`SELECT plugin_id, COUNT(*) FROM plugin_storage GROUP BY plugin_id`）。
- 声明5：task 表按 publicId 引用插件（`backend/base/model/entity/task.go:21` `plugin_public_id` 列）；dev 库存量任务 43/61/152 条（pixiv/localImport/bilibili），生产库仅 localImport；任务执行按 publicId 解析执行器（`backend/taskManager/manager.go:1191-1195`）——该列需随迁移同步改写。
- 声明6：磁盘安装路径为 `filepath.Join(PublicID, Version)`（`service.go:421-422`），dev/prod 实际目录均为 `plugin/package/lvfeng/{id含UUID}/1.0.0/` 三层（`find plugin/package -maxdepth 3`）。
- 声明7：静态资源 URL 四段解析 `SplitN 4` 且前两段拼回 publicId（`backend/plugin/extension/static_resource_service.go:66-76`），URL 构建于 `app.go:648/667`——构建与解析在同一进程内，改段数无需兼容旧 URL。
- 声明8：启动时序 NewApp（AutoMigrate，`app.go:179`）→ InstallBundledPlugins（`main.go:144`）→ LoadPlugins（`main.go:153`）；`migration/migrate.go:33-54` 已有数据迁移先例（re_work_author 去重）。
- 声明9：activatePlugin 全程用 DB 记录的 publicId（`app.go:386-501`），manifest 的 `GetPublicID()` 不参与激活；扩展注册中心 key 为纯拼接无 split（`backend/plugin/extension/task_handler_registry.go:31-33`）——publicId 值变格式不破坏运行时。
- 声明10：SDK 侧 `GetPublicID`（`library-squirrel-sdk/dto/plugin_types.go:53`）与 proto `Task.PluginPublicId` 均无插件消费（grep 四插件仓库 + SDK 零调用方）。
- 声明11：生产库无 `build_id` 列（查询报 `no such column`）——生产为 buildId 机制发布前版本，本次将跨 buildId + publicId 两个机制一次升级。
- 声明12：前端对 publicId 纯透传、无 `/` 切分等格式假设（grep `frontend/src` 全部使用点：actions 参数/对话框 props/CSS 标识拼接）；捆绑清单（`backend/config/default_config.yaml:35-41`）只含 pixiv/local/bilibili 三个 zip，test-plugin 不捆绑。

**待决策（需用户拍板）**

- 决策1（阻塞实施）→ **已裁决（2026-08-16）：硬拒绝**。旧格式 zip（id 含 UUID 后缀）在 `loadPluginPackage` 安装入口 fail-fast 拒绝，防混存窗口双身份。
- 决策2（不阻塞）：静态资源 URL 改为三段 `/plugin/{publicId}/{cacheKey}/{rel}`（推荐）vs 保留四段占位。
- 决策3（不阻塞）：author 保持必填、纯展示（推荐）vs 降为可选。
- 决策4（不阻塞）：旧磁盘目录清理——installCore 重装时顺带删旧 RootPath 目录及空父目录（推荐）vs 一次性全扫 vs 不处理。

**自曝风险**

- 风险1：回滚组合（旧主程序 + 新 zip，或回滚后再升级）会产生双记录/迁移冲突跳过——本方案不支持回滚到旧插件包组合，发布说明需注明（迁移冲突时跳过该记录并告警，不会崩溃）。
- 风险2：迁移派生规则假设旧 publicId 形如 `author/id[_uuid]` 且 author、id 均不含 `/`（旧版无校验）；异常数据靠「冲突跳过 + 日志」兜底，理论上存在漏网。
- 风险3：生产跨 buildId + publicId 两机制一次升级，依赖 InstallBundled 重装链路稳定（已有 `plugin_bundled_upgrade_test.go` 覆盖，但生产实机未验证过该组合）。
- 风险4：已卸载 local 插件（test-plugin）迁移后，其历史备份 zip 仍是旧 id，重装时在 `loadPluginPackage` 即被 UUID 残留校验拒绝（决策1）——需用新 zip 重装，可接受。
- 风险5：浏览器缓存的旧 `/plugin/…` immutable URL 在回滚窗口可能短暂命中旧资产——正常路径新配置全量换新 URL，无影响。
- 风险6：SDK `Task.PluginPublicId` 字段值格式随本变更变化（协议字段形状不变，仅值变）；当前无插件消费（声明10），未来插件若持久化该值需自行兼容。

## 一、背景与目标

设计方向已在《插件构建身份与升级判据机制》（决策6/声明8，2026-08-15 裁决）定案，本文不重新论证，仅实施：

- 身份键 = plugin.json `id`（纯反向域名，如 `com.lvfeng.pixivSuite`），publicId 即 id；
- author 降为纯展示属性，移出身份键；
- id 去除 UUID 后缀。

消除两个身份裂变源（author 改名裂变、UUID 再生成裂变）与三重唯一性冗余。

**命名策略**：`publicId` 名称全链保留（DB 列 `public_id`、DTO/IPC 字段、前端参数、函数签名 `GetByPublicId(publicId string)`），**只改值、不改名**——改名是独立的大扫除，不在本方案范围（避免前端/bindings 无谓 churn）。

## 二、现状触点地图（已核验）

| 触点 | 位置 | 现状 |
| --- | --- | --- |
| 身份构造 | `backend/base/model/dto/plugin_types.go:59-80` | `GetPublicID() = Author + "/" + ID`，仅 `ToPluginInstallDTO` 调用 |
| DB 身份列 | `backend/base/model/entity/plugin.go:12` | `public_id` uniqueIndex |
| 任务引用 | `backend/base/model/entity/task.go:21`、`backend/task/query.go:13` | `plugin_public_id` 列 + 查询过滤 |
| 自存数据 | `backend/base/model/entity/plugin_storage.go:13-14` | `(plugin_id, key)` 唯一——**记录 ID 换新即孤儿** |
| 安装判据 | `backend/plugin/service.go:364-392` | `GetByPublicId` 判已装；buildId 判升级 |
| 重装复用 | `backend/plugin/service.go:460-466` | 复用原记录 ID/CreateTime，`Save` 全字段覆盖 |
| 磁盘路径 | `backend/plugin/service.go:421-422` | `plugin/package/{publicId}/{version}/`，publicId 含 `/` 展开三层 |
| 静态 URL | `backend/plugin/extension/static_resource_service.go:60-76`、`app.go:648/667` | `/plugin/{author}/{id}/{cacheKey}/{rel}` 四段，前两段拼回 publicId |
| 激活流程 | `app.go:368-511` | 全程用 DB publicId；磁盘 manifest 直接 `json.Unmarshal`（不走 `loadPluginPackage` 校验） |
| 注册中心 | `backend/plugin/extension/task_handler_registry.go:31-33` 等 | key = `publicId + "/" + extensionId` 纯拼接，无 split |
| SDK | `library-squirrel-sdk/dto/plugin_types.go:49-69`、gen proto | `GetPublicID` 镜像副本；`ActivateRequest.PluginPublicId`/`Task.PluginPublicId` 运行时透传，无插件消费 |
| 插件仓库 | 四仓库 `plugin.json:2` | id 含 UUID 后缀；仓库代码不引用 publicId |
| 前端 | `frontend/src` 全部使用点 | 纯透传（actions/props/CSS 标识），无格式假设 |
| 存量环境 | dev + 生产（`D:\ProgramFile\Library Squirrel`）DB | 各 4 条旧格式记录；dev 有 plugin_storage 3 条、task 引用 256 条；生产 plugin_storage 0 条、task 引用仅 localImport；生产无 build_id 列（声明11） |
| 文档 | `doc/plugin-dev-guide.md:101/716/764`、`.claude/rules/plugin.md:160`、`backend/plugin/README.md` | `publicId = author/id` 表述需同步 |

## 三、方案设计

### 3.1 身份键与 manifest 校验

- `dto/plugin_types.go`：删除 `GetPublicID()`；`ToPluginInstallDTO` 中 `PublicID` 直接取 `p.ID`。`PluginInstallDTO.PublicID` 字段保留（IPC 契约不变）。
- `loadPluginPackage`（`service.go:306` 附近）新增 id 校验：
  - 格式：反向域名——至少两段 label（含一个 `.`），字符集 `[a-zA-Z0-9.-]`，不含 `/` 与空白；
  - 旧身份残留拒绝：id 匹配 `_[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$` 即返回明确错误（「插件 id 含旧身份 UUID 后缀，请使用新版插件包」）——按决策1，此校验同时封堵「新主程序 + 旧 zip 混存窗口」的双身份裂变路径。
- 校验只设在 zip 安装入口（`loadPluginPackage` 覆盖 InstallFromPath / InstallBundled / Reinstall 全部路径）；`activatePlugin` 读磁盘已装 plugin.json 不校验——已装旧插件照常激活（见 3.5 过渡矩阵）。

### 3.2 静态资源 URL（决策2 按推荐）

- URL 变为三段：`/plugin/{publicId}/{cacheKey}/{relativePath}`（publicId 不再含 `/`）。
- `ServeHTTP` 改 `SplitN(path, "/", 3)`：`parts[0]`=publicId、`parts[1]`=cacheKey、`parts[2]`=相对路径；`ResolveURL` 拼接逻辑不变。
- 无需兼容旧四段 URL：URL 构建与解析在同一进程（声明7），前端每次启动从 slot 配置拿新 URL，浏览器缓存的旧 URL 不再被请求。

### 3.3 DB 数据迁移（方案核心）

**位置与时序**：`migration/migrate.go` 的 `AutoMigrate` 末尾（模型迁移成功后）追加一次性数据迁移，整体包在 `db.Transaction` 内。它在 NewApp 内执行（`app.go:179`），先于 `InstallBundledPlugins`（`main.go:144`）与 `LoadPlugins`（`main.go:153`）——即**在捆绑安装判定之前**把旧记录改名为新 publicId，使 InstallBundled `GetByPublicId(新id)` 能命中存量记录、走升级重装、复用原记录 ID（声明3），plugin_storage（按记录 ID 关联，声明4）与 task 引用不孤儿。

**迁移规则**（幂等，触发条件 `public_id LIKE '%/%'`）：

1. 逐条取 plugin 表 `public_id` 含 `/` 的记录；
2. 派生新 publicId：取首个 `/` 之后的 id 段，剥离尾部 `_` + UUID 后缀（正则同 3.1）；无 UUID 后缀则仅去 author 前缀；
3. 派生结果与原值相同（异常数据）或目标 publicId 已被占用（回滚场景，风险1）→ 跳过该记录并 `Warnf` 日志；
4. 更新 `plugin.public_id`；
5. 同事务批量改写 `task.plugin_public_id`（`UPDATE task SET plugin_public_id = 新 WHERE plugin_public_id = 旧`，声明5）；
6. **不动 `root_path` / `entry_path`**：bundled 记录随后的 InstallBundled 重装会覆盖为新路径；local 记录继续指向旧磁盘目录，激活照常（激活读磁盘 manifest，不比对 DB publicId 与 manifest id，声明9）。

派生函数为纯函数，单测覆盖：标准 UUID 剥离、无 UUID 后缀仅去前缀、新格式（不含 `/`）不触发、空/异常输入跳过、目标冲突跳过、task 同步改写。

**为什么不做 publicId↔记录映射表**：新 id = 旧 id 去 UUID 后缀是确定性派生（四仓库 id 均满足），无需维护显式映射；生产/dev 存量已实证全部符合该形态（声明2/存量环境行）。

### 3.4 磁盘目录

- 新安装路径自然变为 `plugin/package/{publicId}/{version}/`（少一层 author，`service.go:421` 逻辑不变，值变即生效）。
- 旧目录清理（决策4 按推荐）：`installCore` 在重装（`reusePlugin != nil`）且旧 `RootPath` ≠ 新安装路径时，于**解压与落库成功之后**删除旧 RootPath 目录，随后对空父目录尝试 `os.Remove`（仅空目录会成功，无害）——顺带修复「版本升级后旧版本目录遗留」的既有问题；失败顺序保证了清理失败最多留垃圾、不会出现「目录已删而落库失败」的半损态。bundled 三插件将在首启升级重装时触发此清理。
- local 源未重装插件（如 test-plugin）的旧目录保留在用，其 author 层父目录在 bundled 清理后若空则被顺带移除，不空则留置（无害）。

### 3.5 过渡矩阵

| 组合 | 行为 |
| --- | --- |
| 新主程序 + 新 zip + 旧 DB（**主路径**，dev/prod 均此形态） | 迁移改名 → InstallBundled 命中存量记录 → buildId 不同触发升级重装（plugin.json 改动 → git describe 变）→ 复用 ID、落新目录、清旧目录 |
| 新主程序 + 旧 zip（未重建即运行） | `loadPluginPackage` 拒绝（决策1），日志报错、不安装——不产生双记录；已装插件仍从旧磁盘目录照常激活（激活不比对 id），仅无法升级/新装，zip 重建后自愈 |
| 新主程序 + 已迁移 DB + 未重装 local 插件 | DB 已是新 publicId，磁盘旧 manifest 照常激活（不比对）；其下次重装须用新 zip |
| 旧主程序 + 新 zip（用户回滚） | 旧主程序按 `author/id` 找不到记录 → 全新安装双记录、storage 孤儿——**不支持**（风险1），发布说明注明 |
| 全新环境 | 无迁移发生（无 `/` 格式记录），直接新格式 |

### 3.6 SDK 与插件仓库（同一波发布）

- SDK `dto/plugin_types.go`：同步删除 `GetPublicID`，`PublicID` 直接取 `ID`（无调用方，声明10）；gen proto 不动（`Task.PluginPublicId` 字段形状不变，值随主程序变）。
- 四仓库 `plugin.json` 的 id 去 UUID 后缀：`com.lvfeng.pixivSuite`、`com.lvfeng.localImport`、`com.lvfeng.bilibiliSuite`、`com.lvfeng.test-plugin`。author 字段保持不动（决策3）。
- `task build:plugins` 重产出 zip 到 `resources/bundled-plugins/`（该目录 zip 为 gitignore 的构建产物，随发布打包产出、不进版本库）。**主程序改版与 zip 重建必须同一波发布**（否则落入矩阵第二行被拒绝，属安全失败）。

## 四、实施清单（顺序）

1. 主程序身份键：`dto/plugin_types.go` 删 `GetPublicID`、`PublicID = p.ID`；`loadPluginPackage` 加 id 校验（决策1）。
2. 静态 URL 三段化：`static_resource_service.go` `ServeHTTP` 解析调整 + 注释更新。
3. `installCore` 旧 RootPath 目录清理 + 空父目录回收（决策4）。
4. `migration/migrate.go` 数据迁移（3.3）+ 迁移派生函数单测。
5. SDK `dto/plugin_types.go` 同步。
6. 四仓库 plugin.json 改 id（四仓库各一笔提交）。
7. `task build:plugins` 产出新 zip，更新 `resources/bundled-plugins/`。
8. 测试适配：`plugin_bundled_upgrade_test.go` 增「旧 publicId 迁移后 InstallBundled 复用记录 ID」用例；`task/service_create_test.go`、`taskManager/model_multi_stream_test.go` 字面量更新。
9. 文档同步：`doc/plugin-dev-guide.md`（101/716/764 行的 `author/id` 表述与安装路径）、`.claude/rules/plugin.md`（160 行 URL 形态确认）、`backend/plugin/README.md`。
10. bindings 无需再生成（IPC 形状零变化）；前端零代码改动。

## 五、验证计划

- 单测：迁移派生规则全分支；installCore 旧目录清理；loadPluginPackage 对 UUID 后缀 id 的拒绝。
- dev 环境实机：迁移前记录 dev DB 四记录 ID 与 plugin_storage 条数 → 启动新版 → 断言记录 ID 不变、storage 条数不变、public_id 为新格式、task.plugin_public_id 已改写、旧磁盘目录已清、新目录落位、插件激活正常（pixiv 前端扩展可加载）。
- 生产环境同 checklist（storage 为 0，主要验证记录 ID 与任务引用）。

## 六、实施结果（2026-08-16，dev 环境已验证）

- 决策1 硬拒绝、决策2 三段 URL、决策3 author 保持必填、决策4 installCore 顺带清理——均按推荐落地。
- 单测：`backend/migration/migrate_test.go`（派生规则 8 分支）、`backend/base/model/dto/plugin_types_test.go`（id 校验/后缀剥离）、`backend/plugin/extension/static_resource_service_test.go`（三段 URL 命中/旧四段 404）全部通过；`backend/plugin`、`backend/task` 既有测试通过。
- dev 实机（启动编译产物一次走完全链）：4 条记录 publicId 全部迁移（含 uninstalled 的 test-plugin）且**记录 ID 1-4 不变**；plugin_storage 计数 pixiv=2/bilibili=1 保持；task 引用 43/61/152 条全部改写为新格式；bundled 三插件升级重装落新目录 `plugin/package/com.lvfeng.*/1.0.0/` 并激活成功（日志「正在激活插件: com.lvfeng.pixivSuite (root=…\com.lvfeng.pixivSuite\1.0.0)」）；旧 `plugin/package/lvfeng/` 树已清除。
- **风险2 实证踩中**：dev 库 buildId 已是 dirty 值（前晚 dirty 构建写入），新 zip 同 SHA 同 dirty → buildId 相等 → 首启判定「跳过升级」。生产不受影响（生产 build_id 为 NULL 必触发重装）。dev 处置：清空 bundled 记录 build_id 模拟生产态后重启，升级路径走通。后续插件仓库提交后重构建会得到 clean SHA buildId，自然走出 dirty 盲区。
- 既有问题（与本项目无关，未处理）：`backend/taskManager/model_multi_stream_test.go:263`（`SiteAuthorID` vs SDK 实名 `SiteAuthorId`）与 `backend/util/filename/template_test.go:135`（`SiteWorkID` vs `SiteWorkId`）字段名漂移导致两包测试构建失败。
- 遗留小项：installCore 清理旧目录遇文件锁（前次运行被强杀遗留的孤儿子进程锁 exe）时仅告警不重试，本次 dev 旧目录为手动清除；正常退出流程（优雅关停子进程）不触发此况。dev 库 test-plugin（uninstalled）记录的 root_path 指向已删目录，不影响激活（未卸载状态不激活），重装走新 zip 即落新目录。

## 七、后裁决：旧体系兼容整体移除（2026-08-16 用户拍板）

软件未正式发布、「生产环境」为开发者自用环境，无需兼顾旧版本数据。据此移除本方案 3.3 节的数据迁移及全部仅服务旧记录的兼容代码：

- `migration/migrate.go` 的 `migratePluginPublicId`/`deriveNewPublicId` 及调用、`migrate_test.go` 整文件；
- dto 的 `legacyUUIDSuffixRe`/`StripLegacyUUIDSuffix` 与 `ValidatePluginID` 的 UUID 后缀拒绝分支（反向域名字符集不含 `_`，旧式 id 天然被格式校验拒绝，语义不损）；
- `plugin.Service.BackfillLegacyPlugins`（source/trusted NULL 回填）及其在 `LoadPlugins` 的调用；
- 仅服务 NULL 旧行的兜底：`loadInstalledPlugins`/`activatePlugin` 的 trusted=NULL 放行、受限模式的 NULL 来源视作 bundled——统一收紧为「trusted/source 未设置按不利解释」。

**未迁移旧环境的处置**（如 D:\ProgramFile\Library Squirrel 直接升级新版）：旧格式记录不迁移、不激活旧兼容，InstallBundled 按 publicId 查无 → 全新安装新记录，旧记录并存显示为重复条目——在该环境插件管理页手工卸载旧条目即可（顺带删除旧磁盘目录）；其 plugin_storage 为空（2026-08-16 查证），无数据损失。
