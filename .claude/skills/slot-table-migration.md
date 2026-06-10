# SearchTable → SlotSearchTable 迁移技能

## 适用场景

将使用 `SearchTable`（基于 `thead` 属性）的页面迁移到 `SlotSearchTable`（基于插槽）。典型触发词：
- "迁移到 SlotSearchTable"
- "改用插槽表格"
- "这个页面用 SlotSearchTable 重写"

需要迁移的典型信号：
- 列值需要从 Pinia store 或其他 Vue 响应式数据源获取
- 需要自定义单元格渲染逻辑（模板语法比 render 函数更直观）
- 列的渲染逻辑复杂，`Thead` 配置难以表达

## 组件对照

### 架构差异

```
SearchTable  → DataTable  — 通过 thead: Thead<Data>[] 属性声明列，内部统一渲染
SlotSearchTable → SlotDataTable — 通过默认插槽由使用者自行定义 el-table-column
```

### 功能对照

| 功能 | SearchTable | SlotSearchTable |
|------|-------------|-----------------|
| 列定义 | `:thead="theadArray"` | 默认插槽中写 `<el-table-column>` |
| 单元格渲染 | 由 `PopperInput`/`CommonInput` 根据 `Thead.type` 自动处理 | 使用者在插槽模板中自由定义 |
| 操作列 | `:operation-button="ops"` + `@row-button-clicked` | 使用者在插槽中自行添加 `el-table-column` |
| 自定义操作列 | `:custom-operation-button="true"` + `#customOperations` 插槽 | 直接在默认插槽中添加 |
| 选择列 | 自动处理（支持） | 自动处理（支持） |
| 分页/搜索/排序 | 自动处理（支持） | 自动处理（支持） |
| 树形数据 | 自动处理（支持） | 自动处理（支持） |
| 数据更新刷新 | `refreshData()` | `refreshData()` |
| 编辑行追踪 | 自动（`changedRows` v-model） | 调用者自行管理（直接修改行对象） |
| 远程选择列数据 | `Thead.remote`/`remotePageMethod` | 使用者自行管理 |
| 内联编辑 | `PopperInput`/`CommonInput` 自动 | 使用者自行实现 |

### Props 对照

| Prop | SearchTable | SlotSearchTable |
|------|-------------|-----------------|
| `selectable` | ✅ | ✅ |
| `multiSelect` | ✅ | ✅ |
| `clickRowSelect` | ✅ | ✅ |
| `dataKey` | ✅ | ✅ |
| `rowClassName` | ✅ | ✅ |
| `thead` | ✅ | ❌ 已移除 |
| `operationButton` | ✅ | ❌ 已移除 |
| `operationWidth` | ✅ | ❌ 已移除 |
| `customOperationButton` | ✅ | ❌ 已移除 |
| `treeData` | ✅ | ✅ |
| `treeLazy` | ✅ | ✅ |
| `treeLoad` | ✅ | ✅ |
| `border` | ✅ | ✅ |
| `stripe` | ✅ | ✅ |
| `search` | ✅ | ✅ |
| `updateLoad` | ✅ | ✅ |
| `updateProperties` | ✅ | ✅ |
| `createButton` | ✅ | ✅ |
| `pageSizes` | ✅ | ✅ |
| `searchButtonDisabled` | ✅ | ✅ |

### Events 对照

| Event | SearchTable | SlotSearchTable |
|-------|-------------|-----------------|
| `rowButtonClicked` | ✅ | ❌ 已移除（操作列由插槽实现） |
| `selectionChange` | ✅ | ✅ |
| `sortChange` | ✅ | ✅ |
| `scroll` | ✅ | ✅ |
| `createButtonClicked` | ✅ | ✅ |

### Models 对照

| Model | SearchTable | SlotSearchTable    |
|-------|-------------|--------------------|
| `data` | ✅ | ✅                  |
| `page` | ✅ | ✅                  |
| `toolbarParams` | ✅ | ❌ 已移除（调用者自行管理查询参数） |
| `changedRows` | ✅ | ❌ 已移除（调用者自行管理编辑状态） |
| `sort` | ✅ | ✅                  |

## 迁移步骤

### 1. 组件替换

```vue
<!-- 迁移前 -->
<search-table :thead="thead" :operation-button="ops" ... >
  <template #customOperations="{ row }">...</template>
</search-table>

<!-- 迁移后 -->
<slot-search-table ref="tableRef" ... >
  <!-- 列定义 -->
  <el-table-column ... />
  <!-- 操作列 -->
  <el-table-column fixed="right" label="操作" width="120">
    <template #default="{ row }">...</template>
  </el-table-column>
</slot-search-table>
```

导入路径变更：
```ts
// 迁移前
import SearchTable from '@renderer/components/common/SearchTable.vue'

// 迁移后
import SlotSearchTable from '@renderer/components/common/SlotSearchTable.vue'
```

### 2. thead 转换为 el-table-column

逐个 `Thead` 配置项转为 `<el-table-column>`：

| Thead 属性 | el-table-column 对应 |
|-----------|---------------------|
| `key` | `prop`（简单路径）；嵌套路径需 `#default` 插槽手动取值 |
| `title` | `label` |
| `width` | `width` |
| `minWidth` | `min-width` |
| `dataAlign` | `align` |
| `fixed` | `fixed` |
| `sortable` | `sortable` |
| `showOverflowTooltip` | `show-overflow-tooltip` |

#### 简单文本列

```ts
// thead 配置
{ key: 'siteName', title: '站点名称', type: 'text', width: 100 }
```

```vue
<!-- 插槽版 -->
<el-table-column prop="siteName" label="站点名称" width="100" />
```

#### 嵌套路径列（如 `siteTag.siteTagName`）

```ts
// thead 配置
{ key: 'siteTag.siteTagName', title: '标签名', type: 'text', width: 250 }
```

```vue
<!-- 插槽版 — el-table-column 的 prop 不支持嵌套路径，需用插槽 -->
<el-table-column label="标签名" width="250">
  <template #default="{ row }">
    {{ row.siteTag?.siteTagName ?? '-' }}
  </template>
</el-table-column>
```

#### 日期时间列

```ts
// thead 配置
{ key: 'updateTime', title: '更新时间', type: 'datetime', width: 200 }
```

```vue
<!-- 插槽版 — 需手动格式化 -->
<el-table-column label="更新时间" width="200">
  <template #default="{ row }">
    {{ formatDatetime(row.updateTime) }}
  </template>
</el-table-column>
```

#### 选择列（autoLoadSelect）

```ts
// thead 配置
{
  key: 'siteTag.localTagId', title: '本地标签', type: 'autoLoadSelect', width: 150,
  remote: true, remotePaging: true, remotePageMethod: localTagQuerySelectItemPageByName,
  getCacheData: (row) => row.siteTag.localTagSelectItem,
  setCacheData: (row, data) => { row.siteTag.localTagSelectItem = data }
}
```

```vue
<!-- 插槽版 — 使用者自行管理选择数据和缓存 -->
<el-table-column label="本地标签" width="150">
  <template #default="{ row }">
    <!-- 使用者自行实现选择组件和数据加载逻辑 -->
  </template>
</el-table-column>
```

#### 自定义渲染列

```ts
// thead 配置 — render 函数
{
  key: 'taskProgress.task.status', title: '状态', type: 'custom', width: 110,
  render: (data) => h(ElTag, { type: statusTypeMap[data] }, () => statusTextMap[data])
}
```

```vue
<!-- 插槽版 — 直接用模板，更直观 -->
<el-table-column label="状态" width="110">
  <template #default="{ row }">
    <el-tag :type="getStatusType(row.taskProgress.task.status)">
      {{ getStatusText(row.taskProgress.task.status) }}
    </el-tag>
  </template>
</el-table-column>
```

### 3. 操作列迁移

原 `operationButton` + `@row-button-clicked` 模式转为插槽中的 `el-table-column`：

```vue
<!-- 操作列 -->
<el-table-column fixed="right" label="操作" width="140" align="center">
  <template #header>
    <el-tag type="warning">操作</el-tag>
  </template>
  <template #default="{ row }">
    <el-button type="primary" @click="handleEdit(row)">编辑</el-button>
    <el-button type="danger" @click="handleDelete(row)">删除</el-button>
  </template>
</el-table-column>
```

### 4. changedRows 迁移

`SlotSearchTable` 不提供 `changedRows`，调用者直接修改行对象并自行管理编辑状态：

```vue
<script setup lang="ts">
const changedRows = ref<SiteDTO[]>([])

function handleRowChange(row: SiteDTO) {
  if (!changedRows.value.includes(row)) {
    changedRows.value.push(row)
  }
}
</script>

<slot-search-table ...>
  <el-table-column label="名称">
    <template #default="{ row }">
      <el-input v-model="row.name" @change="handleRowChange(row)" />
    </template>
  </el-table-column>
</slot-search-table>
```

### 5. 清理不再需要的代码

迁移完成后删除视图中的：
- `thead` 变量及其 `Thead` 对象构造代码
- `operationButton` 变量及其 `OperationItem` 构造代码
- `Thead`、`OperationItem`、`DataTableOperationResponse` 的 import
- 相关的 `getCacheData`/`setCacheData` 辅助函数

## 验证

1. `cd frontend && yarn build` 编译通过
2. 页面功能验证：
   - 列数据正确显示
   - 分页、搜索、排序正常
   - 选择功能正常（单选/多选）
   - 操作按钮功能正常
   - 编辑状态管理正常（如有）
