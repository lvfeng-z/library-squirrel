import { defineStore } from 'pinia'
import { PendingUpgradeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { pluginApi } from '@renderer/apis/http'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { useMenuBadgeStore } from '@renderer/store/UseMenuBadgeStore.ts'

/** 「插件」菜单项 slotId（useBuiltinMenus 的 builtin-plugin 项）——检查更新红点的注册键 */
export const PLUGIN_MENU_SLOT_ID = 'builtin-plugin'

/** 待办类型：可答复（升级/跳过，计入红点） */
export const PENDING_KIND_AVAILABLE = 'available'
/** 待办类型：已因契约不兼容强制升级（只读告知） */
export const PENDING_KIND_FORCED = 'forced'
/** 待办类型：捆绑包安装失败（只读告知） */
export const PENDING_KIND_ERROR = 'error'

/**
 * 插件检查更新 store：承载启动期检测出的更新待办（后端内存态，随拉取同步）。
 * 「插件」菜单按钮红点读 badgeCount；插件管理页待更新区块读三个分类列表；
 * 操作（升级/跳过/重新提示）完成后由调用方调 refresh() 同步红点与区块
 */
export const usePluginUpdateStore = defineStore('pluginUpdate', {
  state: (): { pendingList: PendingUpgradeDTO[] } => {
    return { pendingList: [] }
  },
  getters: {
    /** 红点计数：仅可答复（available）项——红点语义为「有 N 个更新等你在管理页处理」 */
    badgeCount: (state): number => state.pendingList.filter((item) => item.kind === PENDING_KIND_AVAILABLE).length,
    /** 可答复待办（管理页展示升级/跳过按钮） */
    availableList: (state): PendingUpgradeDTO[] => state.pendingList.filter((item) => item.kind === PENDING_KIND_AVAILABLE),
    /** 已强制升级告知项（只读） */
    forcedList: (state): PendingUpgradeDTO[] => state.pendingList.filter((item) => item.kind === PENDING_KIND_FORCED),
    /** 捆绑包安装失败告知项（只读） */
    errorList: (state): PendingUpgradeDTO[] => state.pendingList.filter((item) => item.kind === PENDING_KIND_ERROR)
  },
  actions: {
    /** 拉取待办并同步列表（mounted 首拉与操作后刷新共用；失败保留旧列表仅告警，不打断宿主流程） */
    async refresh(): Promise<void> {
      try {
        const res = await pluginApi.pluginGetPendingUpgrades()
        this.pendingList = isNullish(res.data) ? [] : res.data
      } catch (e) {
        console.warn('[pluginUpdate] 拉取插件更新待办失败', e)
      } finally {
        // 同步菜单红点注册表（消费侧 DynamicSideMenu 经注册表解耦，不直连本 store）
        useMenuBadgeStore().setBadge(PLUGIN_MENU_SLOT_ID, this.badgeCount)
      }
    }
  }
})
