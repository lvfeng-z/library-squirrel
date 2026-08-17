# 消息系统接入指南（通知中心 + 消息提醒系统）

面向后续要向前端用户展示消息的生产者：未来的后台操作（合并、翻译、AI 自动加标签、查重等）与任何消费后端事件的前端模块。两个系统于 2026-08 建成，设计依据见 `doc/plan/通知中心通用化方案.md` 与 `doc/plan/通用消息提醒系统设计方案.md`。

## 一、四轨机制地图：先选对通道

| 你的需求 | 用哪个 | 形态 |
|---|---|---|
| 用户刚做的操作成败（保存/删除/加载失败） | ElMessage | 顶部居中，瞬时消失 |
| 后台任务的持久追踪（进度/状态/回看/跳转） | **通知中心** | 右侧边栏消息列表，终态保留 |
| 后台事件结局的醒目一过性提醒 | **消息提醒系统 `announce`** | 右上角堆叠卡片，自动关闭 |
| 异步事件需用户决策/动作 | 专用对话框（参考 `ChangeConfirmDialog`） | 模态 |

典型组合（任务型生产者）：通知中心全程追踪 + `announce` 终态提醒，**显式两调**，两系统互不代管（无联动 API）。

判定要点：
- 同步操作反馈绝不走 `announce`（那是 ElMessage 的职责）。
- 异步结局绝不走 ElMessage——用户可能停留在别的页面会错过，且无批量聚合。
- 要留档（事后回看）就必须自己调通知中心，`announce` 不会代写。

另有无文本的**常驻入口提示**通道：菜单红点（`useMenuBadgeStore`，`frontend/src/store/UseMenuBadgeStore.ts`）——按菜单项 slotId 写入计数即在侧边菜单对应按钮显示角标（0 隐藏），适用于「有 N 件事等在某页面处理」的待办类提醒（先例：插件检查更新红点）。它与四轨正交：红点管常驻可发现性，事件结局仍按上表选轨。

## 二、通知中心（持久追踪）

store：`useNotificationStore`（`frontend/src/store/UseNotificationStore.ts`）；UI：`NotificationList.vue`（MainLayout 挂载，右侧边栏）。

```ts
add(item: NewNotificationItem): string                      // 建通知，返回 id
update(id: string, partial: Partial<NewNotificationItem>): void  // 原地合并（顶层整体替换）
remove(id: string): void                                     // 撤销追踪时用；不是终态语义
get(id) / getRange(...) / count（总数） / activeCount（仅活跃）
```

字段（`frontend/src/model/util/NotificationItem.ts`）：

| 字段 | 说明 |
|---|---|
| `level` | `'info'\|'success'\|'warning'\|'error'`，严重度着色；**不是生命周期**，终态用 `terminal` |
| `title` | 标题 |
| `category?` | 业务分类（`'task'`/`'merge'`…），供未来过滤 |
| `statusText?` | 状态描述（"下载中"/"完成"/"失败"） |
| `progress?` | `{current, total, percent?}` 动态进度，面板渲染进度条 |
| `exception?` | 失败时的错误描述 |
| `route?` | `{name, params?, query?}` 点击跳转（vue-router 路由 name，如 `'taskManage'`） |
| `render?` | 兜底自绘（返回 VNode），存在则覆盖默认渲染 |
| `terminal?` | 终态标记；置 true 后须放弃 id 引用（见下） |

### 标准生命周期骨架

```ts
const notificationStore = useNotificationStore()
// 开始
const notificationId = notificationStore.add({
  level: 'info',
  category: 'myop',
  title: `操作【${name}】`,
  statusText: '进行中',
  progress: { current: 0, total: 100 },
  route: { name: 'targetPage' }
})
// 进度事件到达
notificationStore.update(notificationId, { progress: { current: finished, total } })
// 终态到达
notificationStore.update(notificationId, { terminal: true, level: 'success', statusText: '完成' })
useReminderStore().announce({ level: 'success', title: '操作完成', message: `操作【${name}】完成`, category: 'myop' })
myState.notificationId = undefined // 终态脱离：放弃引用
```

### 终态脱离不变量（最重要的纪律）

置 `terminal: true` 后**必须放弃持有的 id 引用**（置 undefined / 从跟踪表移除）。终态条目靠"无人引用"存活：任何全量重建/清理逻辑（如快照 `loadSnapshot`）只 remove 仍被引用的通知；终态后仍持引用并在清理路径 remove，会把用户的回看条目删掉。参考实现：`UseTaskStore` 的 `taskStoreObj.notificationId` 在终态 update 后立即置 undefined。

### 行为边界

- 会话级、不持久化：刷新即清空；无自动淘汰——低价值高频事件不要每条都 `add`。
- `update` 顶层整体替换：`update(id, { progress: { current: 5 } })` 会丢 `total`，progress 必须传完整对象。
- 角标 = 仅活跃（非终态）条目数；列表最新在前（分页第 1 页为最新条目），终态条目变暗。
- `route` 只能跳路由页；对话框类入口（如作品详情）无法精确落点。

## 三、消息提醒系统（瞬时醒目）

store：`useReminderStore`（`frontend/src/store/UseReminderStore.ts`）；UI：`ReminderStack.vue`（MainLayout 挂载，右上角）。

```ts
announce(input: ReminderInput): void
// { level, title, message, category?, duration? }
```

聚合语义（调用前必读）：

- 300ms 聚合窗内按（**category + level + title**）合并为一张卡片（一次快照触发的多条 announce 在同一同步循环入队，短窗即可完整聚合同批条目）。
- 组内 1 条 → 原样展示；N 条 → 计数 + 前 3 条 + "等 N 条"。
- 同屏最多 3 张卡片，超出排队（尾部显示排队数，随关闭补位）。
- **title 即分组头**：希望批量合并就给相同 title（如「任务完成」「合并完成」）；每条给不同 title 则聚合失效、弹一堆。
- `duration` 缺省按 level：普通 4.5s，error 8s（更长供阅读）。
- 卡片可手动关闭（标题行 × 按钮），关闭后排队卡片自动补位。

使用时机：异步后台事件的**结局**（完成/失败/部分完成），此时用户可能停留在任意页面。同步操作反馈禁用此通道（用 ElMessage）。

## 四、数据源是后端事件时的接入骨架

消息源自后端（Wails Events）时，在 `MainIpcListener.ts` 注册监听并转译为上述两系统调用。两种形态：

- **增量事件**（后端推状态变化）→ 直接映射 `add`/`update`/`announce`。
- **全量快照**（后端防抖推完整状态）→ 消费端必须做**迁移检测**：对比替换前状态（prev），判据是「prev 是否已终态」——prev 非终态（活跃/待确认/暂停等）→终态即转终态 + `announce` + 脱离；prev 已终态则跳过（防重复）；prev 缺席按通道区分：快照的移除缓冲（removedItems）仅含本会话新近终止任务，缺席即瞬时任务（全生命周期落在后端防抖窗内），**要提醒**；live 列表可能携带从 DB 装载的历史终态条目（用户 Start 连历史终态子任务一起装载），**不提醒**。参考实现：`UseTaskStore.loadSnapshot`。

教训：任务模块曾把终态逻辑只挂在增量事件上，而后端默认走快照通道（增量通道从不发射），终态提醒静默失效。接入前先确认后端实际发射的通道形态。

## 五、禁忌速查

| 禁忌 | 后果 | 正确做法 |
|---|---|---|
| 异步结局用 ElMessage | 用户在别的页面错过；无聚合 | `announce` |
| 终态后仍持 id 且清理时 remove | 回看条目被误删 | 终态脱离（置 undefined） |
| `announce` 每条不同 title | 聚合失效弹一堆 | 想合并用相同 title |
| `update` progress 只传 current | total 被丢、进度条错乱 | progress 传完整对象 |
| 把 `remove` 当终态用 | 无回看、无提醒 | `update(terminal)` + `announce` |
| level 当生命周期用 | 角标/终态语义混乱 | level 表严重度，terminal 表终态 |

## 六、已知边界与延后项

- 提醒卡片无点击跳转（详情走通知中心的 `route`）。
- 卡片无 hover 暂停自动关闭（增强项，可后补）。
- 桌面级系统通知（声音/托盘）未做：未来挂在 `announce` 内部实现，生产者 API 不变。
- 两系统均为纯前端 store，后端无法直接推送：须经前端 Events 监听转译。
- 预计接入方：合并（longops 阶段2/3 完成后经任务模块间接接入）、翻译/AI 自动加标签/查重（发布后）。
