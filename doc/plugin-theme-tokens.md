# 插件主题令牌契约

> 主程序提供主题令牌（CSS 自定义属性 `--app-*`），插件遵循本契约使用这些令牌，即可让插件 UI 自动跟随用户选择的主题。本文件是插件开发者的参考契约。

## 背景机制

- 主程序通过 `<html data-theme="<id>">` 切换主题，主题令牌定义在全局 `:root` 与各主题 CSS（`frontend/src/styles/theme/`）中。
- 插件 CSS 被注入主程序全局 `<head>`（见 `useSlotSyncListener.ts`），因此插件样式与主程序**共享同一 CSS 变量作用域**。
- 插件**只要使用** `var(--app-*)` 令牌，主题切换时插件 UI 会**自动跟随**，无需 JS 通知。

## 可用令牌清单

> 完整定义见 `frontend/src/styles/theme/tokens.css`。下表为语义说明。

### 主色与状态色
| 令牌 | 语义 |
|---|---|
| `--app-color-primary` | 主色 |
| `--app-color-primary-light-1` ~ `--app-color-primary-light-9` | 主色浅色档（hover / 浅底） |
| `--app-color-primary-dark-2` | 主色深色档（active） |
| `--app-color-{success,warning,danger,info}` 及各自 `-light-N` / `-dark-2` | 状态色及派生档位 |

> 状态色（success/warning/danger/info）跨主题保持一致；仅主色 primary 随主题变化。

### 背景 / 文字 / 边框 / 填充
| 令牌 | 语义 |
|---|---|
| `--app-bg-page` | 页面主背景 |
| `--app-bg-surface` | 卡片 / 容器表面 |
| `--app-bg-surface-variant` | 次级表面（条纹 / 分区） |
| `--app-bg-sidebar` | 侧边栏背景 |
| `--app-text-primary` / `-regular` / `-secondary` / `-placeholder` | 文字层级 |
| `--app-border-color` 及 `-light` / `-lighter` / `-extra-light` / `-darker` | 边框 |
| `--app-fill-color` 及 `-light` / `-lighter` / `-dark` | 填充 |

### 分类标签
| 令牌 | 语义 |
|---|---|
| `--app-tag-{blue,green,red,purple,orange}-bg` | 标签背景（半透明，叠于 surface） |
| `--app-tag-{blue,green,red,purple,orange}-text` | 标签文字 |

### 圆角与阴影
| 令牌 | 值 |
|---|---|
| `--app-radius-sm` / `--app-radius` / `--app-radius-lg` | 4px / 6px / 10px |
| `--app-shadow-sm` / `--app-shadow` / `--app-shadow-card` | 小阴影 / 中阴影 / 卡片阴影 |

## 使用规范

- **必须**使用 `var(--app-*)` 令牌。
- **禁止**硬编码颜色值（如 `#409eff`、`rgb(...)`）。
- **禁止**直接使用 Element Plus 的 `var(--el-*)`（`--el-font-size-*` 等非颜色字号变量除外）。

示例（插件组件样式）：
```css
.my-plugin-card {
  background-color: var(--app-bg-surface);
  color: var(--app-text-primary);
  border: 1px solid var(--app-border-color);
  border-radius: var(--app-radius);
}
.my-plugin-primary-btn {
  background-color: var(--app-color-primary);
  color: var(--app-bg-surface);
}
```

## JS 侧读取当前主题

需要感知"当前主题 id"做逻辑判断（如根据主题切换图标）时，通过注入的上下文读取：

```js
const currentThemeId = window.__PLUGIN_CTX__.theme.getCurrent()
// 返回主题 id，如 'default-light'、'forest-light'、'ocean-light'、'sakura-light'
```

## 边界

- 主程序**不保证**插件硬编码颜色跟随主题；不遵循本契约的插件将保持固定配色，与主程序主题割裂。
- 本契约为"协商机制"：主程序提供令牌与查询能力，是否遵循由插件自行决定。
