import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

/** 待修复变更类型：0=Move 1=Delete 2=Untracked 3=DirMove */
export const CHANGE_KIND = {
  Move: 0,
  Delete: 1,
  Untracked: 2,
  DirMove: 3
} as const

/** 变更所属监控域：0=store 资源文件域 1=backup 保管清单行域 */
export const CHANGE_DOMAIN = {
  Store: 0,
  Backup: 1
} as const

/** 可读名称（与后端 kindName 对齐；backup 域缺失/移动面向保管清单行，独立文案） */
export const CHANGE_KIND_NAME: Record<number, string> = {
  '0-0': '文件移动/重命名',
  '0-1': '文件删除',
  '0-2': '外部新增文件',
  '0-3': '目录移动/改名',
  '1-0': '备份文件移动/改名',
  '1-1': '备份文件缺失'
}

/** 域感知的类型可读名称（domain × kind；未知组合回退「未知」） */
export function changeKindName(domain: number, kind: number): string {
  return CHANGE_KIND_NAME[`${domain}-${kind}`] ?? '未知'
}

export interface ChangeInfo {
  /** 待修复 ID（后端 RepairManager 分配，0 表示仅通知未入队） */
  id: number
  /** 0=store 资源文件域 1=backup 保管清单行域 */
  domain: number
  /** 0=Move 1=Delete 2=Untracked 3=DirMove */
  kind: number
  kindName: string
  /** Move: 旧路径；Delete: 消失路径 */
  fromPath: string
  /** Move: 新路径 */
  toPath: string
  storeId: number
  /** backup 域条目：关联的保管清单行 ID；其余 0 */
  backupId: number
}

/**
 * 待修复文件变更确认 store。
 * 由 MainIpcListener 监听 'fsmonitor:change' 事件推入；ChangeConfirmDialog 消费。
 */
export const useChangeConfirmStore = defineStore('changeConfirm', () => {
  const list = ref<ChangeInfo[]>([])
  const loadingIds = ref<Set<number>>(new Set())

  const visible = computed(() => list.value.length > 0)
  const totalCount = computed(() => list.value.length)

  /** 入队一条待修复变更（id<=0 表示仅通知未入队，忽略；同 id 去重） */
  function add(info: ChangeInfo) {
    if (info.id <= 0) return
    if (list.value.some((item) => item.id === info.id)) return
    list.value.push(info)
  }

  function remove(id: number) {
    const idx = list.value.findIndex((item) => item.id === id)
    if (idx !== -1) {
      list.value.splice(idx, 1)
    }
    loadingIds.value.delete(id)
  }

  function clear() {
    list.value.splice(0, list.value.length)
    loadingIds.value.clear()
  }

  function setLoading(id: number, loading: boolean) {
    if (loading) {
      loadingIds.value.add(id)
    } else {
      loadingIds.value.delete(id)
    }
  }

  function isLoading(id: number): boolean {
    return loadingIds.value.has(id)
  }

  return {
    list,
    visible,
    totalCount,
    loadingIds,
    add,
    remove,
    clear,
    setLoading,
    isLoading
  }
})
