# 前端 DTO 与 Bindings DTO 差异问题（待处理）

## 概述

后端 bindings 生成的 DTO 结构与前端期望的业务 DTO 结构存在差异，需要单独分析和处理。

---

## 一、业务逻辑差异（暂不处理，需要分析业务需求）

### 1.1 baseTag 嵌套对象不存在

**文件**: `LocalTagManage.vue`

**现象**: 前端代码访问 `rowData.baseTag.id`、`rowData.baseTag.localTagName`

**bindings LocalTagDTO 结构**:
```typescript
export class LocalTagDTO {
    "id": number;
    "localTagName": string | null;
    "baseLocalTagId": number | null;  // 只有 ID，没有嵌套对象
}
```

**前端旧代码期望**:
```typescript
// 旧前端 DTO 有嵌套的 baseTag 对象
interface LocalTagDTO {
  baseTag: {
    id: number
    localTagName: string
  }
}
```

**影响**: `getCacheData` 和 `setCacheData` 函数无法正常工作

---

### 1.2 LocalTag 类引用不存在

**文件**: `LocalTagManage.vue`

**现象**: `Cannot find name 'LocalTag'`

**原因**: 前端定义了 `LocalTag` 类/接口用于树形结构，但 bindings 只生成 `LocalTagDTO` 和 `LocalTagResultDTO`

---

### 1.3 LocalTagDTO 缺少多个属性

**文件**: `LocalTagManage.vue`

**现象**:
```
Type 'LocalTagDTO' is missing the following properties from type 'LocalTagDTO': isLeaf, baseTag, lastUse, createTime, updateTime
```

---

### 1.4 LocalAuthor 缺少属性

**文件**: `LocalAuthorManage.vue`

**现象**:
```
Type 'LocalAuthorDTO' is missing the following properties from type 'LocalAuthor': lastUse, createTime, updateTime
```

---

### 1.5 workId / boundOnWorkId 不存在

**文件**: `WorkDialog.vue`

**现象**:
```
Property 'workId' does not exist on type 'SiteTagQueryDTO'
Property 'boundOnWorkId' does not exist on type 'SiteTagQueryDTO'
Property 'workId' does not exist on type 'LocalTagQueryDTO'
```

**bindings SiteTagQueryDTO**: 无 workId 相关字段
**前端期望**: 可能用于关联作品和标签的关系查询

---

## 二、可直接修复的问题（已在本次修复中处理）

### 2.1 QueryDTO 导入路径错误

| 文件 | 状态 |
|------|------|
| `TaskDialog.vue` - TaskQueryDTO | ✅ 已修复 |
| `WorkDialog.vue` - LocalTagQueryDTO, SiteTagQueryDTO | ✅ 已修复 |

---

### 2.2 sort 属性迁移到 orderBy（待修复）

**文件**: `LocalTagManage.vue`、`LocalAuthorManage.vue`、`SiteTagManage.vue`

**现象**: `Property 'sort' does not exist on type 'LocalTagQueryDTO'`

**原因**: bindings 中使用 `orderBy: QueryAttribute` 而非 `sort: number`

**修复方案**:
```typescript
// 旧代码
localTagSearchParams.value.sort = page

// 新代码（使用 bindings orderBy）
import { QueryAttribute } from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
localTagSearchParams.value.orderBy = new QueryAttribute({ value: page, operator: "eq" })
```

---

## 三、已完成修复清单

| 文件 | 修复内容 |
|------|----------|
| `SelectItem.ts` | 改为 re-export bindings SelectItem |
| `SegmentedTagItem.ts` | 修复 CSelectItem 引用问题 |
| `TaskDialog.vue` | 修复 TaskQueryDTO 导入路径 |
| `WorkDialog.vue` | 修复 LocalTagQueryDTO、SiteTagQueryDTO 导入路径 |

---

## 四、解决方案建议

### 方案 A: 前端适配（推荐）

前端代码适配 bindings DTO 结构：
- 使用 `baseLocalTagId: number` 替代 `baseTag: { id, localTagName }`
- 使用 `orderBy: QueryAttribute` 替代 `sort`
- 导入 bindings 中的 QueryDTO 类型

### 方案 B: 后端扩展

如果业务确实需要嵌套的 baseTag 对象，需要后端在 DTO 中添加此结构

---

## 优先级

- **高优先级**: 修复 `sort` -> `orderBy` 的迁移（影响查询功能）
- **中优先级**: 修复模块引用路径问题
- **低优先级**: baseTag 嵌套对象结构（需要分析是否确实需要）
