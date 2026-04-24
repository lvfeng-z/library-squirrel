# LocalTag模块重构总结 - BaseRepository复用原则实施指南

## 概述

本次重构以localTag模块为例，完整实施了BaseRepository复用原则，统一了分页方法签名，并优化了查询参数传递方式。本文档总结了重构方法，供其他模块参考实施。

## 核心原则

### 1. BaseRepository复用原则
- **禁止**：手动实现分页逻辑（手动构建WHERE、ORDER BY、LIMIT、OFFSET）
- **必须**：所有分页方法复用`BaseRepository.Page(ctx, opt)`
- **例外**：仅在复杂自定义查询无法通过PageOption表达时才允许自定义实现

### 2. 统一分页方法签名
- **Repository层**：`ctx context.Context, opt *PageOption` 作为基础签名
- **Service层**：`page *model.Page[T, QueryDTO]` 作为入参
- **额外参数**：在opt参数后追加，但优先考虑放入Page.Query中

### 3. 查询参数整合
- 将业务查询参数放入`Page.Query`中，避免方法签名过于复杂
- 使用`query.QueryAttribute[T]`类型支持灵活的查询条件

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
在`service.go`中更新Repository接口定义：

```go
type Repository interface {
    // 基础分页方法 - 复用BaseRepository.Page
    Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Entity, any], error)
    
    // 业务分页方法 - 使用PageOption签名
    QueryDTOPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Entity, QueryDTO], error)
    QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64) (*model.Page[domain.Entity, QueryDTO], error)
    
    // 其他方法...
}
```

### 步骤3：实现Repository方法
在`repository.go`中实现分页方法，通过设���`opt.Joins`、`opt.Conditions`等实现自定义查询：

```go
// 简单分页 - 直接复用
func (r *Repository) QueryDTOPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Entity, QueryDTO], error) {
    rawPage, err := r.Page(ctx, opt)
    if err != nil {
        return nil, err
    }
    return model.NewPage[domain.Entity, QueryDTO](rawPage.Data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// 带JOIN的分页 - 设置opt.Joins
func (r *Repository) QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64) (*model.Page[domain.Entity, QueryDTO], error) {
    // 添加JOIN
    opt.Joins = []clause.Expression{
        clause.Join{
            Type: clause.InnerJoin,
            Table: clause.Table{Name: "relation_table"},
            ON: clause.Where{Exprs: []clause.Expression{
                clause.Expr{SQL: "entity.id = relation_table.entity_id"}
            }}
        }
    }
    // 添加条件
    opt.Conditions = append(opt.Conditions, clause.Eq{Column: "relation_table.work_id", Value: workId})
    
    rawPage, err := r.Page(ctx, opt)
    if err != nil {
        return nil, err
    }
    return model.NewPage[domain.Entity, QueryDTO](rawPage.Data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
```

### 步骤4：更新Service方法
在`service.go`中更新Service方法，使用`model.Page`作为入参：

```go
// 基础分页服务方法
func (s *Service) PageByDTO(ctx context.Context, page *model.Page[domain.Entity, QueryDTO]) (*model.Page[domain.Entity, any], error) {
    conv := query.NewConverter(domain.Entity{})
    opt, err := conv.ToPageOption(page.Query, page.PageNumber, page.PageSize, nil)
    if err != nil {
        return nil, err
    }
    return s.repo.Page(ctx, opt)
}

// 业务分页服务方法 - 从Page.Query中提取参数
func (s *Service) QueryPageByWorkId(ctx context.Context, page *model.Page[domain.Entity, QueryDTO]) (*model.Page[domain.Entity, QueryDTO], error) {
    if page.Query.WorkId.Value == nil {
        return nil, errors.New("workId is required")
    }
    workId := *page.Query.WorkId.Value
    
    conv := query.NewConverter(domain.Entity{})
    queryOpt, err := conv.ToQueryOption(page.Query, nil)
    if err != nil {
        return nil, err
    }
    
    opt := &database.PageOption{
        QueryOption: database.QueryOption{
            Conditions: queryOpt.Conditions,
            OrderBy:    queryOpt.OrderBy,
        },
        Page:     page.PageNumber,
        PageSize: page.PageSize,
    }
    
    return s.repo.QueryPageByWorkId(ctx, opt, workId)
}
```

### 步骤5：更新Handler方法
在`handler.go`中更新Handler方法，确保类型匹配：

```go
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.DTO, QueryDTO]) *model.ApiResponse[*model.Page[dto.DTO, QueryDTO]] {
    if page == nil {
        page = &model.Page[dto.DTO, QueryDTO]{}
    }
    
    // 创建domain类型的Page对象
    domainPage := &model.Page[domain.Entity, QueryDTO]{
        PageNumber: page.PageNumber,
        PageSize:   page.PageSize,
        Query:      page.Query,
    }
    
    result, err := h.svc.PageByDTO(ctx, domainPage)
    if err != nil {
        return model.Error[*model.Page[dto.DTO, QueryDTO]](err.Error())
    }
    
    // 转换为DTO
    data := make([]*dto.DTO, 0, len(result.Data))
    for _, entity := range result.Data {
        data = append(data, dto.NewDTO(entity))
    }
    
    return model.Success(&model.Page[dto.DTO, QueryDTO]{
        PageNumber:   result.PageNumber,
        PageSize:     result.PageSize,
        PageCount:    result.PageCount,
        DataCount:    result.DataCount,
        CurrentCount: result.CurrentCount,
        Data:         data,
    })
}
```

## 关键技术点

### 1. PageOption.Joins的使用
```go
opt.Joins = []clause.Expression{
    clause.Join{
        Type: clause.InnerJoin,  // 或 clause.LeftJoin
        Table: clause.Table{Name: "table_name"},
        ON: clause.Where{Exprs: []clause.Expression{
            clause.Expr{SQL: "main_table.id = join_table.main_id"}
        }}
    }
}
```

### 2. 动态条件追加
```go
opt.Conditions = append(opt.Conditions, clause.Eq{Column: "column_name", Value: value})
```

### 3. QueryAttribute参数提取
```go
if page.Query.FieldName.Value == nil {
    return nil, errors.New("fieldName is required")
}
fieldValue := *page.Query.FieldName.Value
```

### 4. 类型转换保持兼容性
```go
return model.NewPage[NewType, QueryDTO](rawPage.Data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize)
```

## 验证方法

### 1. 编译检查
```bash
go build ./internal/module_name
```

### 2. 接口匹配检查
确保所有Repository实现都正确实现了Service定义的接口。

### 3. 功能测试
验证所有分页查询功能正常工作，包括：
- 基础分页
- 条件查询
- 排序
- JOIN查询

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

## 实施清单

- [ ] 扩展QueryDTO，添加业务查询字段
- [ ] 更新Repository接口定义
- [ ] 实现Repository分页方法，复用BaseRepository.Page
- [ ] 更新Service方法，使用model.Page作为入参
- [ ] 更新Handler方法，确保类型匹配
- [ ] 编译检查无错误
- [ ] 功能测试通过

## 总结

通过本次重构，localTag模块成功实施了BaseRepository复用原则，显著减少了代码重复，提高了可维护性。其他模块可以按照此指南进行类似重构，实现统一的查询接口和更好的代码复用。</content>
<parameter name="filePath">E:\code\lvfeng\library-squirrel\doc\localTag-refactor-summary.md
