# 插件 slotType 重构方案

> 状态：待确认后执行
> 日期：2026-06-25
> 涉及：主程序（后端 + 前端）、SDK、pixiv/local、文档

## 一、背景与目标

当前 slotType 体系存在问题：
- `embed` 的 `topbar/toolbar/statusbar` position 与整个 `panel` slotType **完全无效**（渲染器组件孤立，无挂载点，MainLayout 无对应 DOM 容器）。
- `embed(dialog)` 实质是弹窗，与 embed（嵌入区域）语义混杂。
- 缺少「替换已有视图」的能力（插件只能新增 view，不能覆盖主程序页面）。

**目标**：重构为清晰、全部生效的 slotType 体系，新增 view 替换能力，让 embed 成为「主程序可控的具名插槽位」。

## 二、最终 slotType 体系

| slotType | 作用 | 与现状关系 |
|---|---|---|
| `view` | 新增页面（新路由） | 保留 |
| `replaceView` | **替换主程序已有页面**（覆盖路由 component） | **新增** |
| `menu` | 菜单项 | 保留 |
| `siteBrowserList` | 入口卡片 | 保留 |
| `dialog` | 弹窗（模态层） | **从 embed(dialog) 独立** |
| `embed` | 插入主程序**具名插槽位**（position=插槽位标识） | **重新定义** |
| ~~`panel`~~ | — | **删除**（无效） |

embed 的 `topbar/toolbar/statusbar` position 删除（原无效），position 语义重定义。

## 三、详细设计

### 3.1 replaceView（新增：替换已有页面）

**声明**（plugin.json）：
```jsonc
{
  "slotType": "replaceView",
  "id": "my-work-manage",
  "content": {
    "target": "work-manage",        // 主程序路由 name（覆盖目标）
    "contentType": "precompiled",
    "source": {"js": "views/work.js", "css": "views/work.css"}
  }
}
```

**机制**：
- 加载时，前端 `registerReplaceViewSlot`：用 `router.addRoute({ name: target, component: PluginComponent })` 覆盖同名路由（Vue Router 4 同 name 覆盖 component）。
- **记录原 component**：覆盖前保存 `originalComponents[target] = router.resolve(target).matched[].components.default`，供卸载恢复。
- **受 Vue 完全控制**：插件组件即路由组件，享有路由参数/守卫/生命周期/`<router-view>` 渲染。
- **多插件覆盖同一 target**：不自动仲裁，由用户在设置页启用/禁用插件的 replaceView 声明（后加载的覆盖先生效；用户控制启用顺序）。
- **卸载恢复**：插件卸载时 `router.addRoute({ name: target, component: originalComponents[target] })` 恢复主程序原组件。

**主程序路由清单**（让插件知道可覆盖哪些）：
- 集中定义路由 name 常量：`frontend/src/router/names.ts`（如 `ROUTE_WORK_MANAGE = 'work-manage'`），主程序路由注册引用该常量。
- 文档（plugin-dev-guide.md）声明可覆盖的路由 name 清单。
- 插件按 name 声明 target。

### 3.2 dialog（独立：弹窗层）

**声明**：
```jsonc
{
  "slotType": "dialog",
  "id": "my-dialog",
  "content": {
    "contentType": "precompiled",
    "source": {"js": "views/dialog.js", "css": "views/dialog.css"},
    "props": {...}                   // 传递给弹窗组件的属性
  }
}
```

**机制**：
- 前端 `SlotRegistryStore` 新增 `dialogSlots`（取代读 `embedSlotsByPosition('dialog')`）。
- `DialogSlotRenderer`（已挂载于 MainLayout:64）改为读 `store.dialogSlots`。
- dialog slot 不再有 position（弹窗统一在模态层）。

### 3.3 embed（重新定义：主程序具名插槽位）

**核心转变**：embed 的 `position` 从「固定枚举(topbar/toolbar/...)」转为「**主程序定义的具名插槽位标识**」。主程序在任意位置（某 view 的某区域、或非 view 的布局区域）暴露插槽位，插件声明 embed slot 插入。

**声明**（plugin.json）：
```jsonc
{
  "slotType": "embed",
  "id": "my-work-toolbar",
  "content": {
    "position": "work.toolbar",      // 主程序定义的插槽位标识（非固定枚举）
    "contentType": "precompiled",
    "source": {"js": "views/btn.js", "css": "views/btn.css"},
    "order": 1                        // 同一插槽位内多插件的排序
  }
}
```

**主程序暴露插槽位**（在 Vue 模板中）：
```vue
<!-- 主程序的 WorkManage.vue，在工具栏区域暴露插槽位 -->
<EmbedSlotRenderer position="work.toolbar">
  <!-- 默认内容（无插件插入时显示，可选） -->
  <el-button>默认按钮</el-button>
</EmbedSlotRenderer>
```

**机制**：
- 主程序在任何想开放的位置放置 `<EmbedSlotRenderer position="xxx">默认内容</EmbedSlotRenderer>`。
- `EmbedSlotRenderer` 读 `store.embedSlotsByPosition(position)`：
  - 有插件 embed slot → 渲染插件组件（多个按 order 排列）。
  - 无插件 → 渲染默认内容（slot children，即主程序在标签内放的原内容）；无默认内容则空。
- 这让 **embed 完全受主程序控制**（主程序决定在哪暴露插槽、提供什么默认），且**生效**（不再孤立）。

**插槽位清单**（让插件知道有哪些位）：
- 文档声明主程序暴露的插槽位标识（如 `work.toolbar`、`global.statusbar`）。
- 渐进暴露：初期少量核心位，按需扩展。

**与 replaceView 的分工**：
- `replaceView`：覆盖**整个页面**（路由级，插件自由覆盖任意 view，主程序不预定义）。
- `embed`：插入**页面内的具名区域**（主程序主动暴露插槽位，插件插入；主程序可控、有默认内容）。

## 四、删除项

- `panel` slotType：整个删除（PanelSlotRenderer 孤立，无效）。
- `embed` 的 `topbar/toolbar/statusbar` position：删除（无效）；position 语义转为具名插槽位。
- 孤立组件 `PanelSlotRenderer.vue`、`ViewSlotRenderer.vue`：删除（ViewSlotRenderer 一直孤立，view 走 router；PanelSlotRenderer 删 panel 后无用）。

## 五、改动清单

### 后端（base/slot.go + dto/plugin_types.go）
- `SlotType` 枚举：移除 `SlotTypePanel`，新增 `SlotTypeReplaceView`、`SlotTypeDialog`；`SlotTypeEmbed` 保留（position 语义变）。
- `SlotDeclaration`/`EmbedSlotContent`：embed 的 position 注释更新（具名插槽位，非枚举）。
- 新增 `ReplaceViewSlotContent`（target 字段）。
- `DialogSlotContent`（从 embed dialog content 调整，去 position）。

### 前端
- `SlotRegistryStore.ts`：新增 `replaceViewSlots`、`dialogSlots` getter；`registerReplaceViewSlot`（路由覆盖 + 记录原 component）、`registerDialogSlot`、`unregisterReplaceViewSlot`（恢复原 component）。
- `useSlotSyncListener.ts`：按新 slotType 分发注册。
- 渲染器：`EmbedSlotRenderer.vue`（启用，主程序在插槽位挂载）、`DialogSlotRenderer.vue`（改读 dialogSlots）；删除 `PanelSlotRenderer.vue`、`ViewSlotRenderer.vue`。
- `MainLayout.vue`：dialog 渲染走新 dialogSlots。
- `router/names.ts`：新增路由 name 常量。
- `SlotConfigs.ts`：类型按新 slotType 调整。

### 主程序插槽位暴露（渐进）
- 在选定的 view/布局区域放置 `<EmbedSlotRenderer position="xxx">默认</EmbedSlotRenderer>`（初期可先暴露 1-2 个验证）。

### SDK / 插件
- pixiv/local 的 plugin.json slot 声明：若有 embed(dialog)，改为 `slotType: "dialog"`；若有 panel/embed(topbar等)，调整或移除。
- SDK `SlotType` 常量同步。

### 文档
- `plugin-dev-guide.md`、`.claude/rules/plugin.md`：新 slotType 体系、replaceView、dialog、embed 插槽位、路由 name 清单、插槽位清单。

## 六、实施顺序

1. 后端 slotType 枚举 + DTO 调整（删 panel、加 replaceView/dialog、embed position 语义）。
2. 前端 SlotRegistryStore + useSlotSyncListener（新 slotType 注册/覆盖/恢复逻辑）。
3. 渲染器（EmbedSlotRenderer 启用、DialogSlotRenderer 改读、删孤立组件）。
4. 路由 name 常量 + replaceView 覆盖/恢复。
5. pixiv/local plugin.json 迁移。
6. 文档同步。
7. 编译 + 验证。

## 七、验证

- `replaceView`：插件声明 target=work-manage → 访问作品管理页 → 显示插件组件；卸载插件 → 恢复原页面。
- `dialog`：插件声明 dialog slot → 主程序弹窗层显示。
- `embed`：主程序在某 view 放 `<EmbedSlotRenderer position="work.toolbar">默认</EmbedSlotRenderer>` → 插件声明 embed position=work.toolbar → 显示插件组件；无插件 → 显示默认。
- `panel`/旧 embed position：确认不再被识别（声明报错或忽略）。

## 八、风险与注意

- **replaceView 卸载恢复**：必须准确记录原 component（含 named views 的所有 components），否则恢复错乱。
- **embed 插槽位标识稳定性**：主程序改了插槽位 position 标识，插件 embed 声明失效（类似 replaceView 的路由 name）——需文档约定 + 版本管理。
- **多插件 replaceView 同 target**：用户控制启用（后启用覆盖），需设置页支持「禁用某插件的 replaceView」。
- **渐进暴露**：embed 插槽位初期少，插件能用的位有限；随主程序逐步暴露扩展。
