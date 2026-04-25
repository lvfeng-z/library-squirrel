# LocalTag模块重构总结 - BaseRepository复用原则实施指南

## 概述

本次重构以localTag模块为例，完整实施了BaseRepository复用原则，统一了分页方法签名，并优化了查询参数传递方式。本文档总结了重构方法，供其他模块参考实施。

## 最新更新 (2026-04-25)

### 方法命名优化
移除了分页查询方法中的"ByDTO"后缀，统一了命名规范：

- `ListSelectItemsByDTO` → `ListSelectItems`
- `QueryWithBaseTagPageByDTO` → `QueryWithBaseTagPage`
- `QuerySelectItemPageByWorkIdByDTO` → `QuerySelectItemPageByWorkId`

### 架构统一
- **Repository层**：统一使用`Page`对象作为参数，避免分散的参数传递
- **Service层**：简化方法实现，直接调用Repository方法
- **Handler层**：更新方法调用，保持接口一致性

## 核心原则

### 1. 分层架构分页接口原则
- **Service层**：分页接口必须接受`model.Page[T, QueryDTO]`类型的入参
- **Repository层**：分页接口必须接受`database.PageOption`类型的参数，当`database.PageOption`不能支持需求时可以添加额外的参数
- **转换职责**：Service层负责将`model.Page`转换为`database.PageOption`（以及可能存在的额外参数）
- **禁止**：Repository层直接接受`model.Page`类型参数

### 2. BaseRepository复用原则
- **禁止**：手动实现分页逻辑（手动构建WHERE、ORDER BY、LIMIT、OFFSET）
- **必须**：所有分页方法复用`BaseRepository.Page(ctx, opt)`
- **例外**：仅在复杂自定义查询无法通过PageOption表达时才允许自定义实现

### 3. 统一分页方法签名
- **Repository层**：`ctx context.Context, opt *database.PageOption` 作为统一签名
- **Service层**：`page *model.Page[T, QueryDTO]` 作为入参
- **Handler层**：直接传递Page对象，无需额外转换

### 4. 查询参数整合
- 将业务查询参数放入`Page.Query`中，避免方法签名过于复杂
- 使用`query.QueryAttribute[T]`类型支持灵活的查询条件

### 5. 命名规范
- **移除"ByDTO"后缀**：分页方法不再使用"ByDTO"后缀，保持简洁命名
- **统一方法命名**：所有分页查询方法使用一致的命名模式

## 实施步骤

### 步骤1：扩展QueryDTO
在模块的`query.go`中为QueryDTO添加业务需要的查询字段：

```go
type LocalTagQueryDTO struct {
    ID             query.QueryAttribute[int64]  `json:"-" query:"id"`
    BaseLocalTagID query.QueryAttribute[int64]  `json:"baseLocalTagId" query:"base_local_tag_id"`
    LocalTagName   query.QueryAttribute[string] `json:"localTagName" query:"local_tag_name"`
    UpdateTime     query.QueryAttribute[int64]  `json:"updateTime" query:"update_time"`
    CreateTime     query.QueryAttribute[int64]  `json:"createTime" query:"create_time"`
    WorkId         query.QueryAttribute[int64]  `json:"workId" query:"work_id"`  // 新增业务查询字段
}
```

### 步骤2：更新Repository接口
在`service.go`中更新Repository接口定义，遵循分层架构分页接口原则：

```go
type Repository interface {
    // 基础分页方法 - 接受database.PageOption参数
    Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalTag, any], error)
    
    // 业务分页方法 - 接受database.PageOption参数，当需要额外参数时添加
    QuerySelectItemPage(ctx context.Context, opt *database.PageOption, secondaryLabel string) (*model.Page[dto.SelectItem, LocalTagQueryDTO], error)
    QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error)
    QuerySelectItemPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64) (*model.Page[dto.SelectItem, LocalTagQueryDTO], error)
    QueryWithBaseTagPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.LocalTagWithBaseTagDTO, LocalTagQueryDTO], error)
    
    // 其他方法...
}
```

### 步骤3：实现Repository方法
在`repository.go`中实现分页方法，复用BaseRepository.Page：

```go
// Page方法 - 直接使用PageOption参数
func (r *LocalTagRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalTag, any], error) {
    return r.BaseRepository.Page(ctx, opt)
}

// 业务分页方法 - 直接使用PageOption参数
func (r *LocalTagRepository) QuerySelectItemPage(ctx context.Context, opt *database.PageOption, secondaryLabel string) (*model.Page[dto.SelectItem, LocalTagQueryDTO], error) {
    rawPage, err := r.BaseRepository.Page(ctx, opt)
    if err != nil {
        return nil, err
    }
    // ... 转换和返回逻辑
}
```

### 步骤4：更新Service方法
在`service.go`中更新Service方法，负责model.Page到database.PageOption的转换：

```go
// 基础分页服务方法 - 转换model.Page为database.PageOption
func (s *Service) Page(ctx context.Context, page *model.Page[domain.LocalTag, LocalTagQueryDTO]) (*model.Page[domain.LocalTag, any], error) {
    conv := query.NewConverter(domain.LocalTag{})
    opt, err := conv.ToPageOption(page.Query, page.PageNumber, page.PageSize, nil)
    if err != nil {
        return nil, err
    }
    return s.repo.Page(ctx, opt)
}

// 业务分页服务方法 - 转换并传递额外参数
func (s *Service) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem, LocalTagQueryDTO], secondaryLabel string) (*model.Page[dto.SelectItem, LocalTagQueryDTO], error) {
    conv := query.NewConverter(domain.LocalTag{})
    queryOpt, err := conv.ToQueryOption(page.Query, nil)
    if err != nil {
        return nil, err
    }
    // 构建PageOption
    opt := &database.PageOption{
        QueryOption: database.QueryOption{
            Conditions: []clause.Expression{where},
            OrderBy:    []clause.Expression{order},
        },
        Page:     page.PageNumber,
        PageSize: page.PageSize,
    }
    return s.repo.QuerySelectItemPage(ctx, opt, secondaryLabel)
}
```

### 步骤5：更新Handler方法
在`handler.go`中更新Handler方法：

```go
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalTagDTO, LocalTagQueryDTO]) *model.ApiResponse[*model.Page[dto.LocalTagDTO, LocalTagQueryDTO]] {
    if page == nil {
        page = &model.Page[dto.LocalTagDTO, LocalTagQueryDTO]{}
    }
    
    // 创建domain类型的Page对象
    domainPage := &model.Page[domain.LocalTag, LocalTagQueryDTO]{
        PageNumber: page.PageNumber,
        PageSize:   page.PageSize,
        Query:      page.Query,
    }
    
    result, err := h.svc.Page(ctx, domainPage)  // 移除ByDTO后缀
    if err != nil {
        return model.Error[*model.Page[dto.LocalTagDTO, LocalTagQueryDTO]](err.Error())
    }
    
    // 转换为DTO
    data := make([]*dto.LocalTagDTO, 0, len(result.Data))
    for _, tag := range result.Data {
        data = append(data, dto.NewLocalTagDTO(tag))
    }
    
    return model.Success(&model.Page[dto.LocalTagDTO, LocalTagQueryDTO]{
        PageNumber:   result.PageNumber,
        PageSize:     result.PageSize,
        PageCount:    result.PageCount,
        DataCount:    result.DataCount,
        CurrentCount: result.CurrentCount,
        Data:         data,
    })
}

// 选择项列表查询 - 更新方法调用
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalTagQueryDTO) *model.ApiResponse[[]*dto.SelectItem] {
    if queryDTO == nil {
        queryDTO = &LocalTagQueryDTO{}
    }
    result, err := h.svc.ListSelectItems(ctx, *queryDTO)  // 移除ByDTO后缀
    if err != nil {
        return model.Error[[]*dto.SelectItem](err.Error())
    }
    return model.Success(result)
}
```

## 关键技术点

### 1. 分层架构分页接口实现
```go
// Repository层 - 接受database.PageOption
func (r *Repository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Entity, any], error) {
    return r.BaseRepository.Page(ctx, opt)
}

// Service层 - 接受model.Page，负责转换
func (s *Service) Page(ctx context.Context, page *model.Page[domain.Entity, QueryDTO]) (*model.Page[domain.Entity, any], error) {
    conv := query.NewConverter(domain.Entity{})
    opt, err := conv.ToPageOption(page.Query, page.PageNumber, page.PageSize, nil)
    if err != nil {
        return nil, err
    }
    return s.repo.Page(ctx, opt)
}

// Handler层 - 传递model.Page对象
domainPage := &model.Page[domain.Entity, QueryDTO]{
    PageNumber: page.PageNumber,
    PageSize:   page.PageSize,
    Query:      page.Query,
}
result, err := h.svc.Page(ctx, domainPage)
```

### 2. PageOption构建模式
```go
// 从QueryDTO构建PageOption
conv := query.NewConverter(domain.Entity{})
queryOpt, err := conv.ToQueryOption(page.Query, nil)
if err != nil {
    return nil, err
}

// 构建完整的PageOption
opt := &database.PageOption{
    QueryOption: database.QueryOption{
        Conditions: []clause.Expression{where},
        OrderBy:    []clause.Expression{order},
        Joins:      []clause.Expression{join}, // 如需要JOIN
    },
    Page:     page.PageNumber,
    PageSize: page.PageSize,
}
```

### 3. 方法命名规范化
- **移除ByDTO后缀**：`ListSelectItemsByDTO` → `ListSelectItems`
- **保持一致性**：所有分页方法使用统一的命名模式
- **简化调用**：方法调用更简洁直观

### 4. QueryAttribute参数提取
```go
if page.Query.WorkId.Value == nil {
    return nil, errors.New("workId is required")
}
workId := *page.Query.WorkId.Value
```

### 5. 类型转换保持兼容性
```go
return model.NewPage[NewType, QueryDTO](rawPage.Data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize)
```

## 验证方法

### 1. 编译检查
```bash
go build ./internal/localTag
```

### 2. 接口匹配检查
确保所有Repository实现都正确实现了Service定义的接口。

### 3. 功能测试
验证所有分页查询功能正常工作，包括：
- 基础分页
- 条件查询
- 排序
- JOIN查询
- 选择项查询

## 常见问题及解决方案

### 问题1：类型不匹配
**现象**：`cannot use page (variable of type *model.Page[dto.DTO, QueryDTO]) as *model.Page[entity.Entity, QueryDTO]`
**解决**：在Handler中创建正确的domain类型Page对象

### 问题2：指针解引用错误
**现象**：`workId (variable of type *int64) as int64`
**解决**：检查nil后解引用：`if page.Query.WorkId.Value == nil { return nil, errors.New("workId is required") }; workId := *page.Query.WorkId.Value`

### 问题3：重复代码
**现象**：Service中有重复的Service结构体定义
**解决**：删除重复的代码块

### 问题4：方法命名冲突
**现象**：移除ByDTO后缀后出现方法名冲突
**解决**：检查现有方法签名，确保新名称不与现有方法冲突

## 实施清单

- [x] 扩展QueryDTO，添加业务查询字段
- [x] 更新Repository接口定义，移除重复签名
- [x] 实现Repository分页方法，复用BaseRepository.Page
- [x] 更新Service方法，使用model.Page作为入参，移除ByDTO后缀
- [x] 更新Handler方法，确保类型匹配，更新方法调用
- [x] 编译检查无错误
- [x] 功能测试通过

## 总结

通过本次重构，localTag模块成功实施了分层架构分页接口原则和BaseRepository复用原则，显著减少了代码重复，提高了可维护性。核心改进包括：

- **分层架构清晰**：Service层统一接受`model.Page`入参，Repository层统一接受`database.PageOption`参数，职责分离明确
- **代码复用最大化**：所有分页查询都复用`BaseRepository.Page`，避免重复实现分页逻辑
- **接口一致性**：移除了过时的"ByDTO"后缀，统一了方法命名规范
- **类型安全**：通过泛型确保类型安全，同时保持向后兼容性

其他模块可以按照此指南进行类似重构，实现统一的查询接口和更好的代码复用。
