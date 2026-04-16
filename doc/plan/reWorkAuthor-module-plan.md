# reWorkAuthor 模块补充开发计划

## 需求概述

本项目从 `main-go` 迁移过来后，缺失 `reWorkAuthor` 模块。`re_work_author` 是作品与作者的关联表（`local_author` 和 `site_author` 的关联），但目前没有对应的业务模块。此外，本应由 `reWorkAuthor` 实现的 `ListReWorkAuthor` 功能被分散到了 `work` 模块和 `localAuthor` 模块中。

## 需求详情

### 功能列表

1. **创建 `reWorkAuthor` 模块**（四层结构：model dto、repository、service、handler）
2. **迁移 `work` 模块的 `ListReWorkAuthor`**（单个作品 → 作者关联查询）到新模块
3. **迁移 `localAuthor` 模块的 `ListReWorkAuthor`**（批量作品 → 作者关联查询）到新模块
4. **整合分散的作者关联查询功能**（包括 `RankedLocalAuthor`、`RankedSiteAuthor` 等）
5. **创建前端 API 包装器** `reWorkAuthor.ts`

### 用户场景

- 查询单个作品关联的所有作者信息（本地作者 + 站点作者）
- 批量查询多个作品关联的作者信息
- 作品详情页展示作者信息

### 数据模型

#### 已有模型（无需修改）
- `ReWorkAuthor` (`internal/model/re_work_author.go`) - 作品与作者关联表
- `LocalAuthor` (`internal/model/local_author.go`) - 本地作者表
- `RankedLocalAuthor` (`internal/model/local_author_dto.go`) - 带排名的本地作者DTO
- `RankedSiteAuthor` (`internal/model/site_author_dto.go`) - 带排名的站点作者DTO

#### 需要新增的DTO
- `ReWorkAuthorDTO` - 请求参数
- `ReWorkAuthorResultDTO` - 返回结果

### API 设计

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| POST | `/api/reWorkAuthor/listByWorkId` | 获取单个作品的作者关联信息 |
| POST | `/api/reWorkAuthor/listByWorkIds` | 批量获取多个作品的作者关联信息 |

### 边界条件

- `workId` 为空或 0 时返回错误
- 作品没有关联作者时返回空列表
- 批量查询时 `workIds` 为空返回空结果

## 技术方案

### 涉及模块

- `internal/reWorkAuthor/` - 新模块（四层结构）
- `internal/model/` - 已有模型，新增 DTO
- `internal/work/` - 移除 `ListReWorkAuthor`，改为调用 `reWorkAuthor` 模块
- `internal/localAuthor/` - 移除 `ListReWorkAuthor`，改为调用 `reWorkAuthor` 模块
- `frontend/src/apis/http/wrappers/` - 新增 `reWorkAuthor.ts`

### 技术要点

1. **参考已有模块模式**：
   - `reWorkTag` 模块（作品-标签关联）- 完整四层结构
   - `siteAuthor` 模块 - 站点作者模块

2. **核心Repository方法**：
   - `ListByWorkId(workId)` - 单个作品的作者关联
   - `ListByWorkIds(workIds)` - 批量作品的作者关联
   - `ListRankedLocalAuthorWithWorkIdByWorkIds(workIds)` - 批量本地作者关联

3. **Service层职责**：
   - 封装业务逻辑
   - 调用 Repository
   - 处理错误

4. **Handler层职责**：
   - 定义 DTO
   - 参数绑定和转换
   - HTTP 请求处理

## 开发步骤

### Phase 1: 创建 reWorkAuthor 模块基础结构

1. 创建 `internal/reWorkAuthor/` 目录
2. 创建 `internal/reWorkAuthor/repository.go`
   - 定义 `Repository` 接口
   - 实现基于 `BaseRepository` 的 `reWorkAuthorRepository`
   - 迁移 `localAuthor` 中的批量查询 SQL
3. 创建 `internal/reWorkAuthor/service.go`
   - 定义 `Service` 接口
   - 实现 `ListByWorkId` 和 `ListByWorkIds` 方法
4. 创建 `internal/reWorkAuthor/handler.go`
   - 定义 `ReWorkAuthorDTO` 和 `ReWorkAuthorResultDTO`
   - 实现 HTTP 处理方法

### Phase 2: 迁移分散的功能

1. 从 `work/service.go` 迁移 `ListReWorkAuthor` 到 `reWorkAuthor/service.go`
2. 从 `work/handler.go` 迁移 `ListReWorkAuthor` 到 `reWorkAuthor/handler.go`
3. 从 `localAuthor/service.go` 移除 `ListReWorkAuthor` 方法（已迁移）
4. 修改 `work/service.go`，组合使用 `reWorkAuthor` 服务

### Phase 3: 前端 API

1. 创建 `frontend/src/apis/http/wrappers/reWorkAuthor.ts`
2. 参考 `siteAuthor.ts` 的包装模式

### Phase 4: 验证和测试

1. 执行 `go build` 确保编译通过
2. 检查类型定义是否正确

## 验收标准

- [ ] `reWorkAuthor` 模块存在且结构完整（四层）
- [ ] `work` 模块的 `ListReWorkAuthor` 已迁移到 `reWorkAuthor` 模块
- [ ] `localAuthor` 模块不再持有 `ListReWorkAuthor` 相关方法
- [ ] 前端 `reWorkAuthor.ts` 包装器已创建
- [ ] `go build` 编译通过
- [ ] 原有功能不受影响（通过 grep 确认无遗留）