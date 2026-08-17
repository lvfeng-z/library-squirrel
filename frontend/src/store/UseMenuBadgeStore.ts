import { defineStore } from 'pinia'

/**
 * 菜单红点注册表（通用机制）：菜单项 slotId → 计数。
 * 消费侧 DynamicSideMenu 对任意菜单项按 slotId 查表渲染 el-badge（0 隐藏），不感知业务来源；
 * 生产侧任何模块调 setBadge 写入（如 UsePluginUpdateStore 写 builtin-plugin 的可升级数）。
 * 键即菜单项 slotId——builtin 项为 useBuiltinMenus 的 slotId，插件菜单项为其复合键
 * （pluginPublicId/extensionId）：渲染层天然兼容插件插槽，插件侧生产链路（SDK API+事件通道）延后，
 * 未来接入时只需在前端把插件计数写入本表
 */
export const useMenuBadgeStore = defineStore('menuBadge', {
  state: (): { badgeMap: Record<string, number> } => {
    return { badgeMap: {} }
  },
  getters: {
    /** 按菜单项 slotId 取红点计数（未注册为 0，0 即隐藏） */
    badgeOf: (state) => (slotId: string): number => state.badgeMap[slotId] ?? 0
  },
  actions: {
    /** 写入/更新某菜单项红点计数 */
    setBadge(slotId: string, count: number): void {
      this.badgeMap[slotId] = count
    }
  }
})
