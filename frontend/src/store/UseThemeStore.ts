import { defineStore } from 'pinia'
import { settingsGetSettings, settingsSaveSettings } from '@renderer/apis/http/wrappers/settings'
import { windowSetTitleBarColor } from '@renderer/apis/http/wrappers/window'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import type { SettingChange } from '@bindings/github.com/library-squirrel/backend/settings/models'
import { DEFAULT_THEME_ID, THEMES, type ThemeId, type ThemeMeta } from '@renderer/theme/themes'

/** 将主题 id 写入 <html data-theme>，触发 CSS 级联切换 */
function applyTheme(id: ThemeId): void {
  document.documentElement.setAttribute('data-theme', id)
}

/** 同步主窗口标题栏颜色（仅 Windows 11 生效，失败不影响主题切换） */
async function applyTitleBar(theme: ThemeMeta): Promise<void> {
  try {
    await windowSetTitleBarColor(theme.titleBar.bg, theme.titleBar.text)
  } catch {
    // 非 Windows 11 平台或句柄未就绪时静默失败
  }
}

/** 校验主题 id 是否在已启用清单中 */
function isKnownTheme(id: string | undefined): id is ThemeId {
  return !!id && THEMES.some((t) => t.id === id)
}

export const useThemeStore = defineStore('theme', {
  state: () => ({
    /** 当前主题 id */
    currentThemeId: DEFAULT_THEME_ID as ThemeId,
    /** 是否已完成首次加载与应用 */
    initialized: false,
  }),
  getters: {
    /** 当前主题元信息 */
    currentTheme(state): ThemeMeta {
      return THEMES.find((t) => t.id === state.currentThemeId) ?? THEMES[0]
    },
    /** 全部可选主题 */
    themeList(): ThemeMeta[] {
      return THEMES
    },
  },
  actions: {
    /** 应用启动时从设置加载并应用主题 */
    async load() {
      const response = await settingsGetSettings()
      if (ApiUtil.check(response)) {
        const data = ApiUtil.data<{ appearance?: { theme?: string } }>(response)
        const id = data?.appearance?.theme
        if (isKnownTheme(id)) {
          this.currentThemeId = id
        }
      }
      applyTheme(this.currentThemeId)
      void applyTitleBar(this.currentTheme)
      this.initialized = true
    },
    /** 切换主题：写入 DOM 并持久化 */
    async setTheme(id: ThemeId) {
      if (!isKnownTheme(id)) {
        throw new Error(`主题不存在: ${id}`)
      }
      this.currentThemeId = id
      applyTheme(id)
      void applyTitleBar(this.currentTheme)
      const changes: SettingChange[] = [{ path: 'appearance.theme', value: id }]
      await settingsSaveSettings(changes)
    },
  },
})
