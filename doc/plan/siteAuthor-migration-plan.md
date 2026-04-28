# siteAuthor 模块迁移修复计划

## 背景

siteAuthor 模块在修复 localAuthor 时已被部分修改（依赖注入、nullable localAuthorId、N+1 消除于 QueryBoundOrUnboundToLocalAuthorPage），但模块自身的完整功能链尚未修复。

参考：`ai-assistant/module-migration-guide.md`

---

## 后端修复项

### B1. [高] QueryLocalRelateDTOPage N+1 查询消除

**位置**: `internal/siteAuthor/repository.go` QueryLocalRelateDTOPage 方法（每行 3 次额外查询）

**方案**:
- 删除 Repository 中的 `QueryLocalRelateDTOPage` 方法
- Service 层复用 `repo.Page()` 获取原始分页
- 新增 `enrichLocalRelateDTO()` 方法：收集 localAuthorIds + siteIds → 批量查询 → Map 组装
- `HasSameNameLocalAuthor` 检查改为：收集所有 authorName → 批量查 LocalAuthor 存在性 → Map
- 在 `LocalAuthorOperator` 接口新增 `GetByNames(ctx, []string) ([]*LocalAuthor, error)` 方法

### B2. [中] UpdateLastUse N+1 查询消除

**位置**: `internal/siteAuthor/service.go` UpdateLastUse 方法（循环逐条查询+更新）

**方案**:
- Repository 新增 `UpdateLastUseByIds(ctx, ids []int64, lastUse int64) error` 方法
- 使用单条 `UPDATE site_author SET last_use = ? WHERE id IN (?)` SQL

### B3. [中] Repository 跨域查询迁移到 Service

**位置**: `internal/siteAuthor/repository.go` GetLocalAuthorByName / SaveLocalAuthor

**方案**:
- 在 `LocalAuthorOperator` 接口新增 `GetByName(ctx, name string) (*LocalAuthor, error)` 和 `Save(ctx, *LocalAuthor) error`
- Service 的 `CreateSameNameLocalAuthor` 改为通过 `localAuthorOp` 调用
- Repository 删除 `GetLocalAuthorByName` 和 `SaveLocalAuthor`
- Repository 接口删除这两个方法

### B4. [低] 手动 IN 占位符改为 GORM 原生

**位置**: `internal/siteAuthor/repository.go` ListBySiteAuthorIds / ListRankedSiteAuthorWithWorkIdByWorkIds

**方案**: 使用 `db.Where("id IN ?", ids)` 替代手动拼接占位符

---

## 前端修复项

### F1. Page 类型统一

**位置**: `SiteAuthorManage.vue` + `SiteAuthorDialog.vue`

**方案**:
- 替换 `import Page from '@renderer/model/util/Page.ts'` → `import { Page } from "@bindings/.../pkg/model"`
- 使用 `newPage<T>()` / `copyPage<T>()` 工具函数

### F2. 统一查询参数，适配 QueryAttribute

**位置**: `SiteAuthorManage.vue`

**方案**:
- 删除独立的 `siteAuthorSearchParams`，统一使用 `siteAuthorQuery`
- 模板中所有输入绑定改为 `.value` 访问器
- 添加 `@clear` 事件重置为 null
- 查询函数中设置 `authorName.operator = Operator.OpLike`

### F3. 分页查询错误处理规范化

**位置**: `SiteAuthorManage.vue` siteAuthorQueryPageFn

**方案**:
- 返回类型改为 `Promise<Page<DTO>>`（不含 undefined）
- 失败时 throw Error 而非 return undefined
- 去除 `new Page(responsePage)` 包装，直接返回

### F4. Wrapper 层冗余类型清理

**位置**: `frontend/src/apis/http/wrappers/siteAuthor.ts`

**方案**:
- 删除 `SiteAuthorVO`、`PageResult`、`toSiteAuthorVO` 等 redundant 类型
- `siteAuthorUpdateById` 改为直接接受 `SiteAuthorDTO`
- `siteAuthorSave` 改为直接接受 `SiteAuthorDTO`
- `siteAuthorCreateAndBindSameNameLocalAuthor` 修正并简化

### F5. SiteAuthorDialog 适配

**位置**: `SiteAuthorDialog.vue`

**方案**:
- Page 类型替换
- Adapter 函数使用 `copyPage` / `newPage`
- 直接传递 DTO 而非手动映射字段

---

## 修改范围

| 文件 | 修改内容 |
|------|----------|
| `internal/siteAuthor/repository.go` | 删除 QueryLocalRelateDTOPage、GetLocalAuthorByName、SaveLocalAuthor；新增 UpdateLastUseByIds；优化 ListBySiteAuthorIds |
| `internal/siteAuthor/service.go` | 新增 enrichLocalRelateDTO；重写 QueryLocalRelateDTOPage；重写 UpdateLastUse；重写 CreateSameNameLocalAuthor；更新 Repository 接口 |
| `internal/siteAuthor/handler.go` | 使用 ToSiteAuthorEntity 简化转换 |
| `internal/localAuthor/service.go` | 新增 GetByName、GetByNames、Save 方法 |
| `frontend/src/views/SiteAuthorManage.vue` | Page 类型、QueryAttribute、错误处理、参数统一 |
| `frontend/src/views/SiteAuthorDialog.vue` | Page 类型、DTO 直接传递 |
| `frontend/src/apis/http/wrappers/siteAuthor.ts` | 冗余类型清理、签名简化 |

---

## 验证要点

1. `yarn typecheck` 通过
2. `yarn build` 通过
3. 站点作者分页查询 + 搜索正常
4. 新增/编辑/删除站点作者正常
5. 创建同名本地作者功能正常
6. 绑定/解绑本地作者正常
