# 状态色调色小抄

> 状态语义 tone 色板的调色参考。体系规则见 `.claude/rules/frontend.md` 的 STATUS_TOKEN_USAGE，实现见 `frontend/src/styles/theme/tokens.css`，可视化校准见「状态色板」测试页（路由 `#/statusPalette`）。

## 体系一句话

状态色用**语义 tone 色板**，状态槽位与主题主色 EP 组件色族（`--app-color-*`）解耦（两条独立轨道）：`:root` 给出 default 主题默认色（text 沿用 Element Plus 经典色），forest/ocean/sakura 在各自 `theme-*.css` 独立覆盖这些 tone 的值——**状态色随主题变化**，主题填色时自行保证槽位色相分散避免撞色。bg/border 由 text 与白色 `color-mix` 派生。

```css
--app-status-{tone}-text:   <hex>;                                  /* 实色文字 */
--app-status-{tone}-bg:     color-mix(in srgb, <hex> 18%, white);   /* 浅底（text 与白混合 18%）*/
--app-status-{tone}-border: color-mix(in srgb, <hex> 26%, white);   /* 边框（混合 26%）*/
```

## 8 个 tone 与 default 主题默认色

> 下列为 default 主题（`tokens.css` :root）的 text 基色；forest/ocean/sakura 在各自 `theme-*.css` 覆盖这些值（逐主题调色留后）。

| tone | text hex（default） | 色 | 服务状态 |
|---|---|---|---|
| `active` | `#f97316` | 橙 | 进行中（task-processing） |
| `done` | `#67c23a` | 绿 | 完成（task-completed、toggle-enabled、resource-downloaded） |
| `fail` | `#f56c6c` | 红 | 失败（task-failed、resource-damaged）+ 破坏性按钮（el-button `tone-fail`） |
| `warn` | `#e6a23c` | 橙黄 | 警示/过渡（pausing/stopping/partly-finished/waiting-input、resource-missing） |
| `pending` | `#409eff` | 蓝 | 待激活（task-created） |
| `idle` | `#909399` | 灰 | 空闲（task-waiting/paused、toggle-disabled） |
| `source-local` | `var(--app-color-primary)` | 跟随主色 | 本地来源（default 下=#409eff 蓝，不在此调色） |
| `source-site` | `#8b5cf6` | 紫 | 站点来源（固定紫，不随主题主色变） |

## 调色操作（热重载即时生效）

色板分两层：`tokens.css` 的 `:root` 给 **default 主题**默认色，forest/ocean/sakura 在各自 `theme-*-light.css` 末尾「状态语义槽位」块独立覆盖（`source-local` 不覆盖，自动跟随主题主色）。

### 换某 tone 的颜色（某主题）
在目标文件里改 `--app-status-{tone}-text` 的 hex，**并同步把 bg/border 两行的 hex 改成同一个**（百分比不变）：

```css
/* 例：在 theme-forest-light.css 把 forest 的"完成"由绿改成金 #d4a017 */
--app-status-done-text: #d4a017;
--app-status-done-bg: color-mix(in srgb, #d4a017 18%, white);
--app-status-done-border: color-mix(in srgb, #d4a017 26%, white);
```
> 改 default 主题则改 `tokens.css` :root 对应三行。
> 注：`fail` tone 除驱动失败/损坏状态标签（StatusTag）外，还驱动破坏性操作按钮（`el-button type="danger"` + `tone-fail` class，见 `frontend/src/styles/tone-button.css`）——调 fail 色会同时影响二者。

### 调底色/边框浓淡（不改颜色）
改 color-mix 百分比：
- 底色太淡 → bg 百分比提高（`18%` → `22%`）
- 底色太浓、文字看不清 → 降低（`18%` → `14%`）
- 边框同理调 border 百分比（当前 `26%`）

### 全局统调浓淡
所有 tone 的 bg/border 百分比是统一模式（`18%, white)` / `26%, white)`），可用编辑器全局替换一次性改全部（注意 default 在 `tokens.css`、其它主题在各自 `theme-*-light.css`）。

## 校准流程

`task dev` → 打开「状态色板」测试页（侧边栏菜单，或路由 `#/statusPalette`）→ 一屏看全 8 tone + 17 状态别名在**当前主题**下的渲染；逐主题切换、核对 6 个通用槽位两两可辨且无与主色撞色。改对应主题文件（default 改 `tokens.css`、其余改 `theme-*-light.css`）保存即热重载，无需重新编译；满意后下次 `task build` 自动带上。

## 新增状态

- **复用现有 tone**：在 `tokens.css` 加 `--app-status-{类目}-{语义}-{bg/text/border}` 三行引用对应 tone，并在 `frontend/src/constants/StatusRegistry.ts` 登记 key。
- **需要新色相**：先在 `tokens.css` :root 加 tone 三分量（text hex + color-mix bg/border，作为 default 默认色），再让各 `theme-*-light.css` 跟随覆盖，最后加别名引用。
