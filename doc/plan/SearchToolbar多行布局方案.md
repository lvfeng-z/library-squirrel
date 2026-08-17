# SearchToolbar 多行布局方案

## 审查摘要

**关键声明（抽查项）**

- 声明1：SearchToolbar 布局写死单行——容器 `height: calc(32px)`（`frontend/src/components/common/SearchToolbar.vue:74`）、内容区 `search-toolbar-main` 32px 单行 flex 无 wrap 无 gap（`:86-90`）；按钮间距仅靠 Element Plus 相邻按钮默认 margin，输入框与其余元素零间距（拥挤感来源之一）。
- 声明2：「更多选项」折叠面板为**绝对定位覆盖**，不占布局流——`CollapsePanel.vue:137-141`（根 relative、面板体 `collapse-panel-main` absolute），故现组件 32px 固定高仍可展开面板；面板锚点随其在流中的位置（主行之后）下移，**与主区行数天然协同**（已核验）。
- 声明3：SearchToolbar 有三个直接消费者，契约一致（`#main`/`#dropdown` slot + `searchButtonDisabled` + `searchButtonClicked`，ExchangeBox 下区另用 `reverse`）——`SearchTable.vue:145-156`、`SlotSearchTable.vue:145-156`、`ExchangeBox.vue:252-262 与 370-381`。
- 声明4：三消费者容器以 32px 工具栏高度做硬补偿，工具栏变高即溢出——`SearchTable.vue:337/347`、`SlotSearchTable.vue:208/218`、`ExchangeBox.vue:479/483 与 564/568`。
- 声明5：SearchTable/SlotSearchTable 被 7 个视图直接使用（TaskManage、LocalAuthorManage、SiteTagManage、SiteManage、SiteAuthorManage、PluginManage、LocalTagManage），TaskList 经 SlotSearchTable 间接消费——**对外契约不变则页面零改动**。

**待决策（已全部裁决 2026-08-17，当日实施完成）**

- 决策1 → **原组件就地改造**（用户裁决，按推荐）：不新建组件，改动仅布局壳 3 个样式点 + 三消费者高度补偿 6 个样式点。
- 决策2 → **组件不管分行**（用户裁决）：不设分行 prop/slot，自动折行与显式分行（消费者用全宽元素）并存，接口零变化。

**自曝风险**

- 风险1：三消费者高度补偿改 flex 自适应是全局布局变更——单行页面按推导视觉不变（37px=32px+5px 间距），仍需 7 视图 + TaskList + ExchangeBox 实机回归；页面 slot 内若有依赖裁切的超宽内容，将从「溢出裁切」变「折行撑高」显形（属修复非破坏，需过目）。
- 风险2：无宽度约束的输入框（如 PluginManage 名称搜索）折行点不可控——页面需自行给输入框设 flex-basis/width；PluginManage 试点验证。
- 风险3：`reverse` 形态（ExchangeBox 下区）下面板锚定在主行**上方**（column-reverse 流序），多行化后面板与内容末行的相对位置变化需在 ExchangeBox 页面实测确认观感。

---

## 一、背景与边界定稿

PluginManage 工具栏 4 元素拥挤暴露 SearchToolbar 布局壳写死单行的问题（声明1）。用户裁决的能力边界（2026-08-17）：**只改「单行布局壳」这一项**——交互内核（内置搜索触发器、「更多选项」折叠面板、reverse、皮肤消费者自置、slot/事件契约）全部原样保留；新诉求集中在**搜索按钮与可变多行布局的协同**。

关键前提已核验（声明2）：折叠面板是绝对定位覆盖体，不占布局流——多行化不影响面板机制，协同问题只剩搜索按钮自身的定位规则。

## 二、设计

### 2.1 布局壳改动（SearchToolbar 仅 3 个样式点）

```css
.search-toolbar {
  /* height: calc(32px) → 高度随内容自适应 */
  height: auto;
  min-height: 32px;
}
.search-toolbar-main {
  display: flex;
  flex-wrap: wrap;        /* 新增：拥挤自然折行 */
  gap: 8px;               /* 新增：统一间距（替代 EP 按钮默认 margin 的偶然间距） */
  align-items: center;
  width: 100%;
  /* height: 32px 移除，行高由内容撑开 */
}
```

### 2.2 搜索按钮与多行的协同规则（2026-08-17 实机反馈后修订为两列模型）

初版「搜索按钮参与折行流（落于内容流末行右缘）」在实机暴露缺陷：显式分行的自定义内容会把按钮挤到独占一行、且与内容之间留大片横向空白。修订为**左右两列**：

- `search-toolbar-main` = 横向 flex 两列：左列 `search-toolbar-content`（包 `#main` slot，`flex:1`，内部 flex-wrap 自动折行/全宽元素显式分行）+ 右列搜索按钮（`flex-shrink:0`）
- 两列 `align-items: center`——**自定义部分任意行数时，搜索按钮相对其总高垂直居中、恒居右缘**，与自定义内容左右分布
- 单行场景与原版视觉一致；代价是按钮列在每一行都占右侧固定宽度（左列可用宽度略减）
- 「更多选项」面板（absolute 覆盖，声明2）：锚点随主区之后，多行时下移至工具栏下方展开，无需适配
- `reverse` 形态：面板与主区的流序翻转规则不变（风险3 需实测）
- **内嵌分隔线（inset divider，2026-08-17 二次调整）**：组件在内容列与搜索按钮列之间加竖向分隔线——1px 宽，`align-self: stretch` + 上下 margin 8px：高度**跟随内容区总高伸缩**（flex 拉伸项随行内最高子项），仅上下各缩进 8px 不与边缘相接（Material Design 称 inset divider，与贯通式 full-bleed divider 相对）；初版固定 16px 跟随搜索按钮，实机反馈后改为跟随内容区。
- **皮肤内边距（2026-08-17 实机反馈补丁）**：工具栏卡片化后内容贴边暴露皮肤缺内边距——`search-toolbar-main` 加 `padding: 4px 8px`（一处覆盖内容列、分隔线、搜索按钮三者的边缘留白）。归属裁决：**皮肤与边距同宿主**（组件内解决，SearchToolbar 层使三消费者一次收敛），消费者只管内容排布不加边距；单行工具栏总高随之 32→40px（7 存量页面统一微调，卡片化设计的连贯收尾）。
- **消费者侧视觉分组（组件不管）**：多行自定义区与搜索按钮的一体感由消费者自行处理。PluginManage 三次迭代后定稿（2026-08-17）：**容器去底色 + SearchTable 自身卡片面**——页面容器（`plugin-manage-container`）不再铺底色，一体感由 SearchTable 内部的工具栏面（`search-table-toolbar`，surface+上圆角）与数据/分页面连成的一张卡片承担，内嵌分隔线划界；包装线与统一底色两代消费者侧包装方案均退役为备选记录；条件显示的告知面板自带独立卡片面（surface+圆角）。

### 2.3 工具栏/数据栏分离视觉参数化（2026-08-17 三次调整）

SearchTable 新增五个皮肤 props（SFC CSS `v-bind` 注入，同 CollapsePanel 先例），支持工具栏块与数据栏块的分离视觉，默认值与历史视觉一致（7 视图零改动）：

| prop | 默认值 | 作用 |
| --- | --- | --- |
| `toolbarBackground` | `var(--app-bg-surface)` | 工具栏块底色 |
| `dataBackground` | `transparent` | 数据栏块底色（透明=表体与分页壳各自承担，现状） |
| `toolbarDataGap` | `5px` | 两栏间隔高度（数据栏 margin-top） |
| `toolbarRadius` | `var(--app-radius) var(--app-radius) 0 0` | 工具栏块圆角（默认上圆下直，历史视觉） |
| `dataRadius` | `0` | 数据栏块圆角 |

注（2026-08-17 实测修正）：初版 `dataBackground`/`dataRadius` 作用于容器但不生效——背景被表体（el-table EP 底色）与分页壳（surface 底）整面覆盖，圆角因 `border-radius` 不裁剪子元素被方角子底盖掉。修正为 **A+B1**：容器加 `overflow: hidden`（圆角裁剪子元素生效）；`dataBackground` 非 transparent 时挂 `search-table-data-custom-bg` 类，级联放行容器底色——el-table 底色经 CSS 变量级覆盖置透明（`--el-table-bg-color`/`-tr-bg-color`/`-header-bg-color`/`-expanded-cell-bg-color`，仅自定义时命中、默认 EP 原生观感零改动），分页壳与其内滚动条同步透明。深色底时表格文字对比度由传色方自负；斑马纹（stripe）行底未纳入透明化（当前无消费者用 stripe）。SlotSearchTable 的同构样式未纳入本次参数化（无消费者诉求，需要时照搬）。

### 2.4 页面侧分行手法（决策2）

- 自动折行：内容超宽自然换行，页面无需干预
- 显式分行：`#main` 内用全宽元素（`<div style="width: 100%">` 或 flex-basis:100%）在指定位置强制断行
- 组件不新增任何 prop/slot——接口零变化

## 三、实施清单

1. `SearchToolbar.vue`：2.1 的 3 个样式点（无 TS/模板改动）
2. 高度补偿 flex 化（声明4 的 6 个样式点）：
   - `SearchTable.vue:347` `calc(100% - 37px)` → `flex: 1; min-height: 0`；`:337` 32px 固定高移除
   - `SlotSearchTable.vue:218/:208` 同上
   - `ExchangeBox.vue:479/564` `calc(100% - 32px)` → flex 自适应；`:483/:568` 32px 固定高移除
3. PluginManage 试点：名称搜索输入框给宽度约束（flex-basis），验证折行观感
4. 回归：7 视图 + TaskList + ExchangeBox 页面工具栏区过一遍（单行页面应与现状一致）
5. `yarn build` + 文档同步（`.claude/rules/frontend.md` 无需改——组件目录未变，行为属内部布局）

## 四、非目标

- 不改 SearchToolbar 对外接口（slot 名/props/事件）
- 不做响应式断点收纳、不做皮肤内置、不新增 prop
- 不动 CollapsePanel 本体
- 不新建组件（决策1 通过就地改方案后）
