# Phase 3 复杂查询模块开发计划

## 需求概述

实现三个复杂查询模块的接口：plugin（插件管理）、siteAuthor（站点作者管理）、siteTag（站点标签管理）。这些模块涉及批量操作、分页查询、关联关系处理等复杂业务逻辑。

## 需求详情

### 功能列表

#### plugin 模块 (3个接口)
1. `pluginSave` - 保存插件配置
2. `pluginUpdate` - 更新插件信息
3. `pluginReinstallFromPath` - 从指定路径重新安装插件

#### siteAuthor 模块 (7个接口)
1. `siteAuthorSaveBatch` - 批量保存站点作者
2. `siteAuthorQueryBoundOrUnboundInLocalAuthorPage` - 分页查询已绑定/未绑定到本地作者的站点作者
3. `siteAuthorQueryLocalRelateDTOPage` - 分页查询本地关联DTO
4. `siteAuthorListBySiteAuthorIds` - 根据站点作者ID列表获取作者信息
5. `siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds` - 根据作品ID列表获取带排名的站点作者
6. `siteAuthorUpdateBindLocalAuthor` - 更新站点作者与本地作者的绑定关系
7. `siteAuthorCreateAndBindSameNameLocalAuthor` - 创建同名本地作者并绑定

#### siteTag 模块 (7个接口)
1. `siteTagSaveBatch` - 批量保存站点标签
2. `siteTagQueryBoundOrUnboundToLocalTagPage` - 分页查询已绑定/未绑定到本地标签的站点标签
3. `siteTagQueryPageByWorkId` - 根据作品ID分页查询站点标签
4. `siteTagQueryLocalRelateDTOPage` - 分页查询本地关联DTO
5. `siteTagListBySiteTagIds` - 根据站点标签ID列表获取标签信息
6. `siteTagUpdateBindLocalTag` - 更新站点标签与本地标签的绑定关系
7. `siteTagCreateAndBindSameNameLocalTag` - 创建同名本地标签并绑定

### 用户场景

- **插件管理**：管理员可以安装、更新、重新安装插件，支持插件的热更新和配置管理
- **作者关联管理**：处理站点作者与本地作者的关联关系，支持批量导入、分页查询、自动绑定
- **标签关联管理**：处理站点标签与本地标签的关联关系，支持批量导入、分页查询、自动绑定

### 数据模型

#### 新增实体
- `Plugin` - 插件配置实体
- `SiteAuthor` - 站点作者实体
- `SiteTag` - 站点标签实体
- `LocalAuthor` - 本地作者实体（已有）
- `LocalTag` - 本地标签实体（已有）

#### 修改实体
- `Work` - 作品实体（需要添加作者关联）
- `Work` - 作品实体（需要添加标签关联）

### API 设计

| 模块 | 方法 | 路径 | 说明 |
|------|------|------|------|
| plugin | POST | /api/plugin/save | 保存插件 |
| plugin | PUT | /api/plugin/update | 更新插件 |
| plugin | POST | /api/plugin/reinstall | 从路径重新安装插件 |
| siteAuthor | POST | /api/siteAuthor/saveBatch | 批量保存站点作者 |
| siteAuthor | GET | /api/siteAuthor/query | 分页查询站点作者 |
| siteAuthor | GET | /api/siteAuthor/list | 根据ID列表获取作者 |
| siteAuthor | PUT | /api/siteAuthor/bind | 更新绑定关系 |
| siteAuthor | POST | /api/siteAuthor/createAndBind | 创建并绑定 |
| siteTag | POST | /api/siteTag/saveBatch | 批量保存站点标签 |
| siteTag | GET | /api/siteTag/query | 分页查询站点标签 |
| siteTag | GET | /api/siteTag/list | 根据ID列表获取标签 |
| siteTag | PUT | /api/siteTag/bind | 更新绑定关系 |
| siteTag | POST | /api/siteTag/createAndBind | 创建并绑定 |

### 边界条件

- 批量操作：处理大量数据时的性能优化和事务管理
- 关联关系：确保绑定关系的唯一性和一致性
- 分页查询：支持大数据量的分页加载
- 异常处理：处理重复数据、数据冲突等异常情况
- 并发控制：确保多用户操作时的数据一致性

## 技术方案

### 涉及模块

- `internal/plugin/` - 插件管理服务
- `internal/siteAuthor/` - 站点作者管理服务
- `internal/siteTag/` - 站点标签管理服务
- `internal/author/` - 本地作者服务（已有）
- `internal/tag/` - 本地标签服务（已有）
- `internal/work/` - 作品服务（需要扩展）
- `frontend/src/apis/http/wrappers/` - 前端接口包装器
- `frontend/bindings/` - Wails bindings

### 技术要点

1. **批量操作处理**：使用事务确保批量操作的原子性
2. **分页查询优化**：实现高效的分页查询，避免全表扫描
3. **关联关系管理**：建立清晰的关联关系模型，支持双向查询
4. **自动绑定逻辑**：实现智能的自动绑定机制，处理同名实体
5. **性能优化**：针对大数据量场景进行查询优化
6. **错误处理**：完善的异常处理和错误反馈机制

## 开发步骤

### Phase 1: 基础设施准备

1. 创建 plugin、siteAuthor、siteTag 三个模块的基础结构
2. 定义相关的 DTO 和实体类
3. 实现基础的 CRUD 操作
4. 配置数据库表结构

### Phase 2: 核心功能实现

1. 实现 plugin 模块的 3 个接口
2. 实现 siteAuthor 模块的 7 个接口
3. 实现 siteTag 模块的 7 个接口
4. 扩展 work 服务以支持作者和标签关联

### Phase 3: 前端集成

1. 更新前端 wrapper 文件，添加新的接口调用
2. 重新生成 Wails bindings
3. 实现前端界面组件（如管理页面）

### Phase 4: 测试与优化

1. 编写单元测试和集成测试
2. 性能测试和优化
3. 修复发现的 bug

## 验收标准

1. 所有接口能够正常调用并返回正确结果
2. 批量操作能够正确处理大数据量
3. 分页查询性能满足要求
4. 关联关系管理逻辑正确
5. 自动绑定功能正常工作
6. 前端能够正确调用后端接口
7. 所有类型检查和构建测试通过

## 预计工作量

- plugin 模块：2-3 天
- siteAuthor 模块：4-5 天
- siteTag 模块：4-5 天
- 前端集成：2-3 天
- 测试优化：2-3 天

总计：12-16 个工作日

## 风险评估

1. **性能风险**：分页查询和批量操作可能存在性能问题
2. **数据一致性风险**：关联关系管理可能存在数据不一致
3. **复杂度风险**：自动绑定逻辑较为复杂，需要仔细设计
4. **依赖风险**：需要依赖现有的作者和标签模块

## 应对措施

1. 进行充分的性能测试和优化
2. 实现完善的事务管理和错误处理
3. 详细设计自动绑定逻辑，进行充分测试
4. 确保与现有模块的兼容性