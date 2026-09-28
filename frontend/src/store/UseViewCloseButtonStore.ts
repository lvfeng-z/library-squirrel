import { defineStore } from 'pinia'
import { settingsGetSettings, settingsSaveSettings } from '@renderer/apis/http/wrappers/settings'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import type { SettingChange } from '@bindings/github.com/library-squirrel/backend/settings/models'

/** 视图关闭按钮开关（应用外壳左上角圆形悬浮按钮，非主页视图时露出、点击返回主页） */
export const useViewCloseButtonStore = defineStore('viewCloseButton', {
  state: () => ({
    /** 是否显示视图关闭按钮（默认关，返回主页走侧栏菜单） */
    enabled: false,
  }),
  actions: {
    /** 应用启动时从设置加载开关状态 */
    async load() {
      const response = await settingsGetSettings()
      if (ApiUtil.check(response)) {
        const data = ApiUtil.data<{ appearance?: { viewCloseButtonEnabled?: boolean } }>(response)
        this.enabled = data?.appearance?.viewCloseButtonEnabled === true
      }
    },
    /** 切换开关：即时生效并按变更路径持久化（独立于设置页整表保存流程） */
    async setEnabled(v: boolean) {
      this.enabled = v
      const changes: SettingChange[] = [{ path: 'appearance.viewCloseButtonEnabled', value: v }]
      await settingsSaveSettings(changes)
    },
  },
})
