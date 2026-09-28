# 状态色调色小抄

> 状态语义 tone 色板的调色参考。体系规则见私有环境仓 rules/frontend.md 的 STATUS_TOKEN_USAGE，实现见 `frontend/src/styles/theme/tokens.css`，可视化校准见「状态色板」测试页（路由 `#/statusPalette`）。

## 体系一句话

状态色用**语义 tone 色板**，状态槽位与主题主色 EP 组件色族（`--app-color-*`）解耦（两条独立轨道）：`tokens.css` `:root` 给出全部主题共用的统一基准（text 沿用 Element Plus 经典色），forest/ocean/sakura 不再独立覆盖、统一级联回该基准，仅两条例外覆盖（forest done、sakura fail）；`pending` 与 source 类目绑 `var(--app-color-primary)` 随各主题主色变化。bg/border 引用 `var(--app-status-{tone}-text)` 与白色 `color-mix` 派生。

```css
--app-status-{tone}-text:   <hex>;                                                         /* 实色文字（唯一手写处）*/
--app-status-{tone}-bg:     color-mix(in srgb, var(--app-status-{tone}-text) 18%, white);  /* 浅底（text 与白混合 18%）*/
--app-status-{tone}-border: color-mix(in srgb, var(--app-status-{tone}-text) 26%, white);  /* 边框（混合 26%）*/
```

> bg/border 引用 text 的 var 派生——**改 text 一行，bg/border 自动跟随**，无需三处同步 hex。

## 8 个 tone 与统一基准色

> 下列为 `tokens.css` :root 的统一基准 text 色（全部主题共用）；例外：pending 绑各主题主色、forest done / sakura fail 见下节。

| tone | text 值（统一基准） | 色 | 服务状态 |
|---|---|---|---|
| `active` | `#f97316` | 橙 | 进行中（task-processing） |
| `done` | `#67c23a` | 绿 | 完成（task-completed、toggle-enabled、resource-downloaded） |
| `fail` | `#f56c6c` | 红 | 失败（task-failed、resource-damaged）+ 破坏性按钮（el-button `tone-fail`） |
| `warn` | `#e6a23c` | 橙黄 | 警示/过渡（pausing/stopping/partly-finished/waiting-input、resource-missing） |
| `pending` | `var(--app-color-primary)` | 绑主题主色 | 待激活（task-created）；default 下=#409eff 蓝，随各主题主色（forest 翠绿/ocean 青/sakura 粉） |
| `idle` | `#909399` | 灰 | 空闲（task-waiting/paused、toggle-disabled） |
| `source-local` | `var(--app-color-primary)` | 跟随主色 | 本地来源（标准浓度 bg 18%/border 26%，不在此调色） |
| `source-site` | `var(--app-color-primary)` | 跟随主色（较浅） | 站点来源（与 local 同随主色，bg 10%/border 16% 较浅派生，靠深浅区分） |

## 调色操作（热重载即时生效）

基准集中在 `tokens.css` 的 `:root` 一处（全部主题共用）：改 text 一行，bg/border 经 var 派生自动跟随，三主题同步生效。主题文件不再整块覆盖 tone——forest/ocean/sakura 的 `theme-*-light.css` 末尾仅剩方针注释与个别例外覆盖：forest done=`#409eff`（pending 绑本主题主色翠绿 #008b45，与绿域 done 撞色，让位改 default 主色蓝）、sakura fail=`#fb3e3a`（鲜艳正红，与主色粉 #ff7ca5 拉开红粉界限）；`pending` 与 source 类目绑 `var(--app-color-primary)`，随各主题主色自动变化，无需在主题文件声明。

### 换某 tone 的颜色
只改 text 一行，bg/border 经 var 派生自动跟随（百分比不变，无需同步 hex）：

```css
/* 例：把"完成"由绿改成金 #d4a017——在 tokens.css 改这一行即可 */
--app-status-done-text: #d4a017;
```
> 个别主题确需不同色相时，在对应 `theme-*-light.css` 同样只加一行 text 覆盖声明（现有例外：forest done、sakura fail）。
> 注：`fail` tone 除驱动失败/损坏状态标签（StatusTag）外，还驱动破坏性操作按钮（`el-button type="danger"` + `tone-fail` class，见 `frontend/src/styles/tone-button.css`）——调 fail 色会同时影响二者。
> 另注：EP 危险色（`--app-color-danger*` 色族，经 ep-bridge 桥接驱动纯 EP 危险组件，如副视图关闭按钮）不在 fail tone 轨道内——改 EP 危险色在 `tokens.css` danger 族改一处即 default/forest/ocean 三主题生效；sakura 例外整族在 `theme-sakura-light.css` danger 族，与本主题 fail 覆盖同值（双轨合一），改任一侧需注意另一侧是否同步。

### 调底色/边框浓淡（不改颜色）
改 color-mix 百分比：
- 底色太淡 → bg 百分比提高（`18%` → `22%`）
- 底色太浓、文字看不清 → 降低（`18%` → `14%`）
- 边框同理调 border 百分比（当前 `26%`）

### 全局统调浓淡
所有 tone 的 bg/border 百分比是统一模式（`18%, white)` / `26%, white)`，例外 `source-site` 用较浅的 10%/16%），且全部集中在 `tokens.css` 一处，可用编辑器全局替换一次性改齐。

## 校准流程

`task dev` → 打开「状态色板」测试页（侧边栏菜单，或路由 `#/statusPalette`）→ 一屏看全 8 tone + 17 状态别名在**当前主题**下的渲染；逐主题切换、核对 6 个通用槽位两两可辨且无与主色撞色。改 `tokens.css`（或主题例外覆盖处）保存即热重载，无需重新编译；满意后下次 `task build` 自动带上。

## 新增状态

- **复用现有 tone**：在 `tokens.css` 加 `--app-status-{类目}-{语义}-{bg/text/border}` 三行引用对应 tone，并在 `frontend/src/constants/StatusRegistry.ts` 登记 key。
- **需要新色相**：在 `tokens.css` :root 加 tone（text hex 一行，bg/border 照 var 派生模式补两行），全部主题自动生效、无需各 `theme-*-light.css` 跟随；最后加别名引用。
