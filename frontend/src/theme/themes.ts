/**
 * 主题元信息清单
 * 每套主题对应 styles/theme/theme-<id>.css 中的 html[data-theme="<id>"] 定义。
 * 阶段 0 仅启用「默认浅色」，其余 id 为预留（阶段 3 补充配色与 CSS 文件后再加入 THEMES）。
 */

export type ThemeId = 'default-light' | 'forest-light' | 'ocean-light' | 'sakura-light'

export interface ThemeMeta {
  /** 主题唯一标识，对应 html[data-theme] 属性值与 theme-<id>.css 文件名 */
  id: ThemeId
  /** 主题显示名称 */
  name: string
  /** 设置页选择器的预览色板 */
  swatch: {
    primary: string
    bg: string
    surface: string
  }
}

/** 默认主题 id（未配置或配置无效时使用） */
export const DEFAULT_THEME_ID: ThemeId = 'default-light'

/** 全部已启用主题清单 */
export const THEMES: ThemeMeta[] = [
  {
    id: 'default-light',
    name: '默认浅色',
    swatch: { primary: '#409eff', bg: '#fafafa', surface: '#ffffff' },
  },
  {
    id: 'forest-light',
    name: '森林绿',
    swatch: { primary: '#007038', bg: '#fafafa', surface: '#ffffff' },
  },
  {
    id: 'ocean-light',
    name: '海洋蓝',
    swatch: { primary: '#1ab7c7', bg: '#f4fbfc', surface: '#ffffff' },
  },
  {
    id: 'sakura-light',
    name: '樱花粉',
    swatch: { primary: '#ec6d8e', bg: '#fdf6f8', surface: '#ffffff' },
  },
]
