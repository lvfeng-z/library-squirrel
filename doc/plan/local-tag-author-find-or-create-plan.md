# 重构计划：本地标签/本地作者 Find-or-Create

## 目标

将任务模块对插件返回的本地标签和本地作者的处理逻辑，从"仅支持已有记录绑定（validateById）"改为"按名称查找或创建（find-or-create by name）"，使插件无需预先知道内部 DB ID 即可关联本地标签/作者。

## 核心原则

- **只增不改**：已存在的本地标签/作者不做任何修改，仅使用其 DB ID
- **最小创建**：新建实体仅填充 `name`，其余字段留空，交给用户后续编辑
- **批量优先**：遵循 `ELIMINATE_N_PLUS_1_QUERY` 规则，批量查询避免循环单查
- **接口隔离**：遵循 `SERVICE_DEPENDENCY_VIA_INTERFACE` 规则，work 模块定义自己需要的接口
- **不修改 DTO/Proto/gRPC 层**：复用现有 `LocalTagDTO` / `LocalAuthorDTO`，通过 `ID == 0` 区分模式

## 双模式语义

复用现有 `LocalTagDTO` 和 `LocalAuthorDTO`，不新增 DTO 类型：

| 条件 | 行为 |
|------|------|
| `ID > 0` | 校验存在性（现有行为），ID 不存在则报错 |
| `ID == 0` 且 `AuthorName`/`LocalTagName` 不为空 | 按名称 find-or-create，返回 DB ID |
| `ID == 0` 且名称也为空 | 报错：参数无效 |

## 变更范围

### 1. localTag 模块：新增 GetByNames 批量方法

**文件**: `backend/localTag/repository.go`

在 Repository 接口和实现中新增批量按名称查询方法：

```go
// GetByNames 根据名称列表批量查询本地标签
func (r *LocalTagRepository) GetByNames(ctx context.Context, names []string) ([]*entity.LocalTag, error) {
    var tags []*entity.LocalTag
    err := r.GORM().WithContext(ctx).
        Where(clause.IN{Column: "local_tag_name", Values: util.UniqueString(names)}).
        Find(&tags).Error
    return tags, err
}
```

**文件**: `backend/localTag/service.go`

新增对应方法，并在 Repository 接口中声明：

```go
// GetByNames 根据名称列表批量查询本地标签
func (s *Service) GetByNames(ctx context.Context, names []string) ([]*entity.LocalTag, error) {
    return s.repo.GetByNames(ctx, names)
}
```

> **参照**：`localAuthor/service.go` 已有 `GetByNames` 方法，直接复用其模式。

### 2. work/service.go 接口扩展

**文件**: `backend/work/service.go`

新增两个写入接口，用于 find-or-create：

```go
// LocalTagFindOrCreator 本地标签查找或创建接口
type LocalTagFindOrCreator interface {
    // GetByNames 根据名称列表批量查询
    GetByNames(ctx context.Context, names []string) ([]*entity2.LocalTag, error)
    // Save 创建本地标签
    Save(ctx context.Context, tag *entity2.LocalTag) error
}

// LocalAuthorFindOrCreator 本地作者查找或创建接口
type LocalAuthorFindOrCreator interface {
    // GetByNames 根据名称列表批量查询
    GetByNames(ctx context.Context, names []string) ([]*entity2.LocalAuthor, error)
    // Save 创建本地作者
    Save(ctx context.Context, author *entity2.LocalAuthor) error
}
```

在 `Service` 结构体中新增字段：

```go
type Service struct {
    // ... 现有字段 ...
    localTagFindOrCreator    LocalTagFindOrCreator
    localAuthorFindOrCreator LocalAuthorFindOrCreator
}
```

更新 `NewService` 构造函数，增加这两个参数。

> **可清理**：`LocalTagBatchReader` 和 `LocalAuthorBatchReader` 接口中的 `ListByIds` 方法在 `SaveWorkInfo` 流程中不再使用（validate 方法将被替换）。如果其他方法（如 `GetFullWorkInfoByIds`）仍使用它们，则保留。否则可考虑移除。

### 3. work/service.go 核心逻辑替换

**文件**: `backend/work/service.go`

#### 3.1 删除旧方法

删除以下方法：
- `validateLocalAuthorIds()`
- `validateLocalTagIds()`
- `enrichLocalAuthorDTOs()`
- `enrichLocalTagDTOs()`

#### 3.2 新增 resolveLocalTags

```go
// resolveLocalTags 处理本地标签，返回 DB ID 列表
// ID > 0 时校验存在性；ID == 0 时按名称 find-or-create
func (s *Service) resolveLocalTags(ctx context.Context, dtos []*sdkdto.LocalTagDTO) ([]int64, error) {
    if len(dtos) == 0 {
        return nil, nil
    }

    // 按 ID 模式和名称模式分组
    var idModeIds []int64
    var nameModeDtos []*sdkdto.LocalTagDTO
    for _, d := range dtos {
        if d.ID > 0 {
            idModeIds = append(idModeIds, d.ID)
        } else {
            nameModeDtos = append(nameModeDtos, d)
        }
    }

    // ID 模式：校验存在性（复用现有逻辑）
    if len(idModeIds) > 0 {
        results, err := s.localTagBatchReader.ListByIds(ctx, idModeIds)
        if err != nil {
            return nil, fmt.Errorf("查询本地标签失败: %w", err)
        }
        if len(results) != len(idModeIds) {
            found := make(map[int64]struct{}, len(results))
            for _, r := range results {
                found[r.ID] = struct{}{}
            }
            for _, id := range idModeIds {
                if _, ok := found[id]; !ok {
                    return nil, fmt.Errorf("本地标签不存在: ID=%d", id)
                }
            }
        }
    }

    // 名称模式：收集去重名称 → 批量查询 → 确定需要创建的 → 逐个创建
    var nameModeIds []int64
    if len(nameModeDtos) > 0 {
        names := make([]string, 0, len(nameModeDtos))
        nameSet := make(map[string]struct{}, len(nameModeDtos))
        for _, d := range nameModeDtos {
            name := ""
            if d.LocalTagName != nil {
                name = *d.LocalTagName
            }
            if name == "" {
                return nil, fmt.Errorf("本地标签 ID 为 0 时 LocalTagName 不能为空")
            }
            if _, ok := nameSet[name]; !ok {
                names = append(names, name)
                nameSet[name] = struct{}{}
            }
        }

        // 批量查询已有标签
        existing, err := s.localTagFindOrCreator.GetByNames(ctx, names)
        if err != nil {
            return nil, fmt.Errorf("查询本地标签失败: %w", err)
        }
        existingMap := make(map[string]int64, len(existing))
        for _, t := range existing {
            if t.LocalTagName.Valid {
                existingMap[t.LocalTagName.String] = t.ID
            }
        }

        // 创建不存在的标签
        now := util.GetCurrentTimestamp()
        for _, name := range names {
            if _, ok := existingMap[name]; ok {
                continue
            }
            tag := entity2.NewLocalTag()
            tag.LocalTagName = sql.NullString{String: name, Valid: true}
            tag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
            tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
            if err := s.localTagFindOrCreator.Save(ctx, tag); err != nil {
                return nil, fmt.Errorf("创建本地标签失败: %w", err)
            }
            existingMap[name] = tag.ID
        }

        // 按 DTO 顺序收集 ID（保留重复名称）
        nameModeIds = make([]int64, 0, len(nameModeDtos))
        for _, d := range nameModeDtos {
            if id, ok := existingMap[*d.LocalTagName]; ok {
                nameModeIds = append(nameModeIds, id)
            }
        }
    }

    // 合并结果
    ids := make([]int64, 0, len(dtos))
    idIdx := 0
    nameIdx := 0
    for _, d := range dtos {
        if d.ID > 0 {
            ids = append(ids, idModeIds[idIdx])
            idIdx++
        } else {
            ids = append(ids, nameModeIds[nameIdx])
            nameIdx++
        }
    }
    return ids, nil
}
```

#### 3.3 新增 resolveLocalAuthors

逻辑与 `resolveLocalTags` 对称，额外处理 `Introduce` 字段。结构相同：
- `ID > 0` → 校验存在性
- `ID == 0` 且 `AuthorName` 不为空 → 按 `AuthorName` 批量查询 → 不存在的创建（填充 `AuthorName` + `Introduce`）

> 这里不展开完整代码，与 resolveLocalTags 结构一致，区别仅在于字段名和实体类型。

#### 3.4 更新 SaveWorkInfo 主流程

将 Phase 1 中的调用从：

```go
localAuthorDBIds, err := s.validateLocalAuthorIds(ctx, workResp.LocalAuthors)
localTagDBIds, err := s.validateLocalTagIds(ctx, workResp.LocalTags)
```

改为：

```go
localAuthorDBIds, err := s.resolveLocalAuthors(ctx, workResp.LocalAuthors)
localTagDBIds, err := s.resolveLocalTags(ctx, workResp.LocalTags)
```

其余流程（Phase 2 回查、Phase 3 关联重建）不变，因为最终都是使用 DB ID 列表。

### 4. app.go 依赖注入调整

**文件**: `app.go` — `initAdvancedServices()` 函数

在 `work.NewService()` 调用中增加 `localTagFindOrCreator` 和 `localAuthorFindOrCreator` 参数，注入 `app.LocalTagService` 和 `app.LocalAuthorService`（它们已实现新接口的方法）。

## 变更文件清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `backend/localTag/repository.go` | 修改 | 新增 `GetByNames` 方法 |
| `backend/localTag/service.go` | 修改 | 新增 `GetByNames` 方法，更新 Repository 接口 |
| `backend/work/service.go` | 修改 | 新增接口、替换 validate → resolve、删除旧方法和 enrich 函数 |
| `app.go` | 修改 | `work.NewService()` 新增两个依赖参数 |

## 不变的部分

- SDK DTO（`LocalTagDTO` / `LocalAuthorDTO`）— 不变，通过 `ID == 0` 区分模式
- gRPC Proto — 不变
- gRPC 代理层转换逻辑 — 不变
- `buildLocalAuthorLinks()` / `buildLocalTagLinks()` — 不变，仍接收 DB ID 列表
- Phase 2 回查（站点标签/作者/作品集）— 不变
- Phase 3 关联重建逻辑 — 不变
- `ReWorkTag` / `ReWorkAuthor` 关联表 — 不变
- 前端非任务场景的本地标签/作者 CRUD — 不变

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 本地标签名称重复（不同父节点下同名） | 中 | `GetByNames` 返回所有匹配，按名称匹配到第一个；创建时若已存在则跳过。与现有 `GetByName` 行为一致 |
| 批量创建中的并发冲突 | 低 | 同一任务串行执行；不同任务创建同名标签时 `local_tag_name` 无唯一约束，可能创建重复。可在后续迭代中添加唯一约束 |
| ID 模式向后兼容 | 无 | `ID > 0` 时行为与现有完全一致，已有插件无需改动 |

## 执行顺序

1. localTag 模块新增 `GetByNames`
2. work/service.go 接口扩展 + 逻辑替换
3. app.go 依赖注入
4. 编译验证
