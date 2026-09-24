# participation — 插件条目参与度真相层

## 是什么

插件在各派生面（作品拉取/站点作者拉取/站点浏览器/自定义资源类型/前端扩展）上**声明条目 × 参与度**的运行期唯一真相源，并承担 resolver 求值编排。设计出自 `library-squirrel-docs/plan/插件设置驱动的派生面热生效方案.md`（4.4-4.5 节）。

核心模型：**per 插件会话 = 清单声明集 ⊕ resolver 覆盖表**。

- **声明集**（账）：激活相位从清单（manifest）派生的条目键全集（point + id）。数据源是清单而非 loader 进程表——纯 UI 插件（无 entryFile、无进程）同经此路径。
- **覆盖表**（态）：resolver 脚本输出的『当前完整意愿』快照，**整体替换**语义（未列出的条目 = 基线参与）；只能作用于声明集内条目，声明集外条目单条拒收。
- **零持久化**：覆盖表随激活末尾由持久 KV 重算重建，停用即清。

## 提供什么

| 面 | API | 说明 |
| --- | --- | --- |
| 求值触发 | `TriggerEvaluation(publicId)`（manager.go） | 设置落库后异步触发；同插件串行、在途合并（只跑最新输入） |
| 生命周期 | `StartSession` / `StopSession`（manager.go） | 激活相位末尾登记会话并**同步**首评；停用清空（幂等） |
| 查询 | `EntryActive` / `Entries` / `StatusOf`（manager.go） | 声明集 ⊕ 覆盖表的有效参与态；Status 为降级态数据面（失败分类 + 时间） |
| 订阅 | `SubscribeChanges(handler)`（manager.go） | 条目级变更通知（携带 point/id 与变更后方向），供下游派生面联动 |
| 安装闸门 | `ValidateResolverForInstall`（declarations.go） | 清单校验簇同层：脚本在场/体积/语法/默认值 dry-run 过 shape 校验 |

## 依赖谁

- `plugin/settingresolver`：脚本求值运行器（受限运行时、超时中断、输出 shape 校验、失败分类）。
- `PluginStorageService`（结构性接口 `SettingsReader`）：插件全量设置读取（加密项解密随读取完成，app.go 装配注入）。
- 清单/实体 DTO（`base/model/dto`、`base/model/entity`）。

## 关键设计

- **三触发点**：①激活相位末尾（lifecycle 参与者的末位注册，app.go 装配——基线注册完毕且晚于进程 Activate RPC 的 KV 自迁移）；②SaveSetting 与 ③ResetSetting 落库后（`plugin_setting_service.go` 两处分别挂接，异步排队）。
- **串行化**（乱序窗口杜绝）：会话内单在途求值 + 合并标记；每轮求值开始时现读 KV，在途完成后跑的总是最新输入。
- **失败语义**：求值失败保留上一次覆盖表（首次失败 = 无覆盖 = 全基线），失败分类与时间入 Status 供管理页降级标注。
- **现势检查**：求值在锁外进行，结果应用时校验会话仍为管理器登记的当前会话——停用/重激活后在途结果整体丢弃，不复活已清空的表。
- app.go 中的 `participationParticipant` 适配本层为 `plugin.LifecycleParticipant`（本包不 import plugin 包，避免环）。
