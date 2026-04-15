# Handler 重构计划：基于 wails.Run + Bind 方案

## 概述

本计划旨在将现有架构重构为使用 Wails v3 的 `Bind[]` 参数模式，在每个业务模块创建 `handler.go` 文件作为接口暴露层，实现更清晰的关注点分离。

## 背景

当前架构存在以下问题：
1. `app.go` 包含所有服务实例，职责过重
2. 所有服务方法直接暴露给前端，缺乏统一的接口层
3. 前端 bindings 生成依赖于 `*_bindings.go` 文件

目标架构：
```
main.go
  └── NewApp() → 创建服务实例
  └── wailsApp.Run(Bind[...handlers])
                      │
          ┌───────────┴───────────┐
          │                       │
    ┌─────▼─────┐           ┌─────▼─────┐
    │LocalTag   │           │Work       │
    │Handler    │           │Handler    │
    └─────┬─────┘           └─────┬─────┘
          │                       │
    ┌─────▼─────┐           ┌─────▼─────┐
    │localTag.  │           │work.      │
    │Service    │           │Service    │
    └───────────┘           └───────────┘
```

## Handler 职责

Handler 层负责：
1. **接收前端请求参数**
2. **调用 Service 层执行业务逻辑**
3. **将结果包装为 `*model.ApiResponse[T]` 返回**
4. **处理错误转换**

Service 层保持不变，仅负责业务逻辑。

## 模块清单

| 模块 | Handler 文件 | 核心暴露方法 |
|------|-------------|-------------|
| localTag | `internal/localTag/handler.go` | Save, Delete, Update, GetById, QueryPage, GetTree, ListSelectItems |
| localAuthor | `internal/localAuthor/handler.go` | Save, Delete, Update, GetById, QueryPage, ListSelectItems |
| siteTag | `internal/siteTag/handler.go` | Save, Delete, Update, GetById, QueryPage, ListSelectItems |
| siteAuthor | `internal/siteAuthor/handler.go` | Save, Delete, Update, GetById, QueryPage, ListSelectItems |
| site | `internal/site/handler.go` | Save, Delete, Update, GetById, QueryPage, GetTree, ListByWorkId |
| resource | `internal/resource/handler.go` | Save, Delete, Update, GetById, QueryPage |
| reWorkTag | `internal/reWorkTag/handler.go` | Save, Delete, Update, GetById, QueryPage, ListSelectItems |
| work | `internal/work/handler.go` | Save, Delete, Update, GetById, QueryPage, ListByTagIds, Export, Import |
| workSet | `internal/workSet/handler.go` | Save, Delete, Update, GetById, QueryPage, GetTree |
| search | `internal/search/handler.go` | Query, QuerySimple, Suggest, GetStatistics |
| settings | `internal/settings/handler.go` | Get, Save |
| secureStorage | `internal/secureStorage/handler.go` | Get, Set, Delete, Keys |
| backup | `internal/backup/handler.go` | Create, Restore, List, Delete, Export, Import |
| appLauncher | `internal/appLauncher/handler.go` | Launch, GetPath |
| fileSysUtil | `internal/fileSysUtil/handler.go` | ReadTextFile, WriteTextFile, Exists, CreateDir, Remove, Copy |
| plugin | `internal/plugin/handler.go` | List, GetById, Install, Uninstall, Enable, Disable, GetConfig, SaveConfig |
| task | `internal/task/handler.go` | List, GetById, Cancel, GetProgress, ListByStatus |
| taskManager | `internal/taskManager/handler.go` | GetAllTasks, CancelTask, GetTaskProgress |
| slot | `internal/slot/handler.go` | GetAllSlots, SyncToRenderer |
| siteBrowser | `internal/siteBrowser/handler.go` | Register, Unregister, List |

## Handler 方法签名规范

```go
// 单个对象查询
func (h *LocalTagHandler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.LocalTag]

// 分页查询
func (h *LocalTagHandler) QueryPage(ctx context.Context, req *LocalTagQueryRequest) *model.ApiResponse[*model.Page[LocalTagVO]]

// 列表查询
func (h *LocalTagHandler) ListSelectItems(ctx context.Context, req *LocalTagQueryRequest) *model.ApiResponse[[]*domain.SelectItem]

// 增删改操作
func (h *LocalTagHandler) Save(ctx context.Context, tag *LocalTagDTO) *model.ApiResponse[int64]
func (h *LocalTagHandler) Delete(ctx context.Context, id int64) *model.ApiResponse[void]
```

## 关键设计决策

1. **Handler 不拥有数据库连接** - 仅持有 Service 实例
2. **每个 Handler 方法都返回 `*model.ApiResponse[T]`** - 统一前端调用格式
3. **DTO 转换在 Handler 层完成** - Service 层使用 domain 对象
4. **ctx context.Context 作为首个参数** - 符合 Go 规范
5. **ApiResponse 由后端生成** - 前端直接使用，无需再包装

## 实施步骤

### Phase 1: 创建参考实现 (localTag)
1. 创建 `internal/localTag/handler.go`
2. 定义 `LocalTagHandler` 结构体
3. 实现核心方法
4. 更新 `app.go` 创建 Handler 实例
5. 更新 `main.go` Bind 参数
6. 验证编译和 bindings 生成

### Phase 2: 批量创建 Handlers
按依赖顺序创建其余 19 个 Handler：
1. localAuthor
2. siteTag
3. siteAuthor
4. site
5. resource
6. reWorkTag
7. settings
8. secureStorage
9. backup
10. appLauncher
11. fileSysUtil
12. workSet
13. work
14. search
15. plugin
16. slot
17. siteBrowser
18. task
19. taskManager

### Phase 3: 清理和验证
1. 移除 `app.go` 中的服务实例（已转移给 Handlers）
2. 验证所有 Go 编译
3. 重新生成 Wails bindings
4. 验证前端构建

## 文件变更清单

### 新增文件 (20 个)
```
internal/localTag/handler.go
internal/localAuthor/handler.go
internal/siteTag/handler.go
internal/siteAuthor/handler.go
internal/site/handler.go
internal/resource/handler.go
internal/reWorkTag/handler.go
internal/work/handler.go
internal/workSet/handler.go
internal/search/handler.go
internal/settings/handler.go
internal/secureStorage/handler.go
internal/backup/handler.go
internal/appLauncher/handler.go
internal/fileSysUtil/handler.go
internal/plugin/handler.go
internal/task/handler.go
internal/taskManager/handler.go
internal/slot/handler.go
internal/siteBrowser/handler.go
```

### 修改文件
```
app.go       - 添加 Handler 创建逻辑，移除服务实例暴露
main.go      - 修改 Bind 参数使用 Handlers
```

## 验收标准

- [ ] 所有 20 个 handler.go 文件创建完成
- [ ] Go 编译通过 (`go build ./...`)
- [ ] Wails bindings 重新生成
- [ ] 前端构建通过 (`yarn build`)
- [ ] 服务可正常启动

## 时间估算

- Phase 1: 1-2 天（参考实现）
- Phase 2: 3-5 天（批量创建）
- Phase 3: 1-2 天（清理验证）

总计约 5-9 个工作日
