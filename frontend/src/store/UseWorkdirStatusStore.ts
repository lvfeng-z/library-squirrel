import { defineStore } from 'pinia'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { settingsGetSettings } from '@renderer/apis/http/wrappers/settings'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore.ts'
import type { Settings } from '@bindings/github.com/library-squirrel/backend/settings/models'

/**
 * 工作目录未配置状态 store（前端双通道策略承载方）：
 * - 初始判定走启动拉取（MainLayout 挂载时调 refresh）——后端不做启动期发射，
 *   避免事件早于前端 Events.On 注册即丢失；
 * - 运行期由后端统一发射口的 workdir:unconfigured 事件（MainIpcListener 转发
 *   onUnconfiguredEvent）升常驻横幅；
 * - 同源去重：首启向导会话内至多弹一次（wizardAttempted），横幅升格幂等，
 *   向导进行中不叠横幅（向导结束经 onTourEnded 补升）
 */
export const useWorkdirStatusStore = defineStore('workdirStatus', {
  state: (): {
    /** 工作目录是否已配置（拉取与事件双通道共同维护；默认 true——拉取失败不误弹横幅，运行期拒绝事件兜底） */
    configured: boolean
    /** 常驻横幅（「工作目录未配置，点击前往设置」）是否展示 */
    bannerVisible: boolean
    /** 本会话是否已弹过首启向导（会话内不重复弹） */
    wizardAttempted: boolean
  } => ({
    configured: true,
    bannerVisible: false,
    wizardAttempted: false
  }),
  actions: {
    /**
     * 拉取 settings 收敛未配置状态：启动水合（MainLayout 挂载）与设置保存/重置后
     * 刷新共用。未配置时按首启向导完成与否分流——未完成且本会话未弹过则直接打开
     * 向导（复用 first-time 定义，首步即工作目录输入），否则升常驻横幅；
     * 已配置则收起横幅
     */
    async refresh(): Promise<void> {
      try {
        const response = await settingsGetSettings()
        if (!ApiUtil.check(response)) return
        const settings = ApiUtil.data<Settings>(response)
        this.applyPulled(settings?.workdir ?? '', settings?.tour?.completed?.['first-time'] === true)
      } catch (e) {
        console.warn('[workdirStatus] 拉取设置失败', e)
      }
    },

    /** 运行期未配置事件入口（source=发现未配置的后端模块名）→ 升横幅引导配置 */
    onUnconfiguredEvent(source: string): void {
      this.configured = false
      if (this.bannerVisible) return
      if (useTourCenterStore().isActive) return
      this.bannerVisible = true
    },

    /** 向导结束（完成/跳过）时仍未配置 → 升横幅（首启向导被跳过时的补升入口） */
    onTourEnded(): void {
      if (!this.configured && !this.bannerVisible) {
        this.bannerVisible = true
      }
    },

    /** 按拉取结果分流：未配置时向导未完成且本会话未弹过 → 弹首启向导；否则升横幅 */
    applyPulled(workdir: string, firstTimeTourCompleted: boolean): void {
      if (!isBlank(workdir)) {
        this.configured = true
        this.bannerVisible = false
        return
      }
      this.configured = false
      if (!firstTimeTourCompleted && !this.wizardAttempted) {
        this.wizardAttempted = true
        void useTourCenterStore().start('first-time').catch((e) => console.warn('[workdirStatus] 打开首启向导失败', e))
        return
      }
      if (!useTourCenterStore().isActive) {
        this.bannerVisible = true
      }
    }
  }
})
