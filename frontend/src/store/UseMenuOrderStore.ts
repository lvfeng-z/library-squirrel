import { defineStore } from 'pinia'
import { settingsGetSettings, settingsSaveSettings } from '@renderer/apis/http/wrappers/settings'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { applyUserOrder, buildSnapshotAfterMove, type OrderableMenuItem } from '@renderer/components/slot/menuOrdering'
import type { SettingChange, Settings } from '@bindings/github.com/library-squirrel/backend/settings/models'

/** 用户菜单序在设置文件中的变更路径（外观分区下的 slotId 有序表） */
const MENU_ORDER_PATH = 'appearance.menuOrder'

// 设置中读出的原始菜单序转本地快照：缺省/null/非数组一律视为未自定义，
// 表内非字符串项丢弃（快照只保 slotId 字符串）
function parseMenuOrder(raw: unknown): string[] {
  if (!Array.isArray(raw)) return []
  return raw.filter((slotId): slotId is string => typeof slotId === 'string')
}

// 以全量快照提交菜单序：走设置增量保存接口的单条变更（path=MENU_ORDER_PATH）；
// 失败时抛错，本地快照不回退——调用方据错误提示用户，界面顺序仍是用户操作的结果
async function saveMenuOrder(snapshot: readonly string[]): Promise<void> {
  const changes: SettingChange[] = [{ path: MENU_ORDER_PATH, value: [...snapshot] }]
  const response = await settingsSaveSettings(changes)
  if (!ApiUtil.check(response)) {
    throw new Error(response?.msg || '菜单顺序保存失败')
  }
}

/**
 * 侧边菜单用户手动排序的状态仓。
 *
 * 快照语义：slotId 有序表即全量顺序，空表表示用户未自定义（渲染回落条目静态 order）。
 * 加载只在应用启动时发生一次；拖拽落定与重置各自即时改本地快照并提交保存。
 */
export const useMenuOrderStore = defineStore('menuOrder', {
  state: () => ({
    /** 用户菜单序快照（空表=未自定义） */
    snapshot: [] as string[],
  }),
  actions: {
    /** 应用启动时从设置外观分区加载用户菜单序（读取失败保持未自定义态，不阻断启动） */
    async load() {
      const response = await settingsGetSettings()
      if (!ApiUtil.check(response)) return
      const data = ApiUtil.data<Settings>(response)
      this.snapshot = parseMenuOrder(data?.appearance.menuOrder)
    },
    /**
     * 拖拽落定：以当前显示顺序为基线合成新快照，即时写入本地并按变更路径持久化。
     * @returns 新的用户菜单序快照
     */
    async commitMove(currentIds: readonly string[], fromIndex: number, toIndex: number): Promise<string[]> {
      this.snapshot = buildSnapshotAfterMove(currentIds, this.snapshot, fromIndex, toIndex)
      await saveMenuOrder(this.snapshot)
      return this.snapshot
    },
    /** 恢复默认静态序：清空本地快照并提交空值 */
    async reset() {
      this.snapshot = []
      await saveMenuOrder(this.snapshot)
    },
  },
})

/**
 * 按当前用户菜单序重排一级菜单条目。
 *
 * 以模块函数而非 store 动作暴露：Pinia 的动作类型经 `_StoreWithActions` 的条件类型推断会
 * 把泛型参数退化为其约束（条目子类型信息丢失），模块函数保留 applyUserOrder 的泛型签名。
 */
export function reorderMenusByUserOrder<T extends OrderableMenuItem>(items: T[]): T[] {
  return applyUserOrder(items, useMenuOrderStore().snapshot)
}
