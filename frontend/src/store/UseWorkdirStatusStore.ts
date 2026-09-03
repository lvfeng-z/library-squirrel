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
 * - 同源去重：首启欢迎弹窗会话内至多弹一次（welcomeAttempted），横幅升格幂等，
 *   向导进行中不叠横幅（向导结束经 onTourEnded 补升）
 */
export const useWorkdirStatusStore = defineStore('workdirStatus', {
  state: (): {
    /** 工作目录是否已配置（拉取与事件双通道共同维护；默认 true——拉取失败不误弹横幅，运行期拒绝事件兜底） */
    configured: boolean
    /** 常驻横幅（「工作目录未配置，点击前往设置」）是否展示 */
    bannerVisible: boolean
    /** 首启欢迎弹窗是否展示 */
    welcomeVisible: boolean
    /** 本会话是否已弹过首启欢迎弹窗（会话内不重复弹） */
    welcomeAttempted: boolean
  } => ({
    configured: true,
    bannerVisible: false,
    welcomeVisible: false,
    welcomeAttempted: false
  }),
  actions: {
    /**
     * 拉取 settings 收敛未配置状态：启动水合（MainLayout 挂载）与设置保存/重置后
     * 刷新共用。未配置时按欢迎弹窗看过与否分流——未看过且本会话未弹过则弹首启
     * 欢迎弹窗（两按钮分别导向新手向导与直接开始），否则升常驻横幅；
     * 已配置则收起横幅
     */
    async refresh(): Promise<void> {
      try {
        const response = await settingsGetSettings()
        if (!ApiUtil.check(response)) return
        const settings = ApiUtil.data<Settings>(response)
        this.applyPulled(settings?.workdir ?? '', settings?.tour?.completed?.['welcome-shown'] === true)
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

    /**
     * 欢迎弹窗关闭入口（弹窗两按钮共用）：关弹窗并持久化「已看过欢迎」——以用户
     * 主动关闭为凭证，显示瞬间不标记。welcome-shown 为非向导标记借住向导完成表
     * （completed 只被向导中心按注册表遍历，借住键不会出现在向导中心列表）。
     * 「开始使用」关闭且未配置时升常驻横幅；「查看新手向导」由调用方随后
     * start('workdir-setup') 接管，横幅交给向导结束链（onTourEnded）兜底
     */
    closeWelcome(action: 'start-using' | 'view-tour'): void {
      this.welcomeVisible = false
      useTourCenterStore().markCompleted('welcome-shown')
      if (action === 'start-using' && !this.configured && !this.bannerVisible) {
        this.bannerVisible = true
      }
    },

    /** 按拉取结果分流：未配置时欢迎未看过且本会话未弹过 → 弹首启欢迎弹窗；否则升横幅 */
    applyPulled(workdir: string, welcomeShown: boolean): void {
      if (!isBlank(workdir)) {
        this.configured = true
        this.bannerVisible = false
        return
      }
      this.configured = false
      if (!welcomeShown && !this.welcomeAttempted) {
        this.welcomeAttempted = true
        this.welcomeVisible = true
        return
      }
      if (!useTourCenterStore().isActive) {
        this.bannerVisible = true
      }
    }
  }
})
