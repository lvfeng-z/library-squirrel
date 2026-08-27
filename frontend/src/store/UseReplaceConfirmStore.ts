import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export interface DuplicateInfo {
  taskId: number
  taskName: string
  existingWorkId: number
  existingWorkName: string
  /** 将被覆盖的板块角色(行级交集后);null/undefined 表示行级信息不可得(查询失败/旧版本载荷) */
  conflictRoles?: string[] | null
}

export const useReplaceConfirmStore = defineStore('replaceConfirm', () => {
  const list = ref<DuplicateInfo[]>([])
  const loadingTaskIds = ref<Set<number>>(new Set())

  const visible = computed(() => list.value.length > 0)
  const totalCount = computed(() => list.value.length)

  function add(info: DuplicateInfo) {
    list.value.push(info)
  }

  function remove(taskId: number) {
    // 答复语义为任务粒度整体决策:一次答复清理该 taskId 的全部冲突行(同任务多冲突作品各占一行)
    list.value = list.value.filter((item) => item.taskId !== taskId)
    loadingTaskIds.value.delete(taskId)
  }

  function clear() {
    list.value.splice(0, list.value.length)
    loadingTaskIds.value.clear()
  }

  function setLoading(taskId: number, loading: boolean) {
    if (loading) {
      loadingTaskIds.value.add(taskId)
    } else {
      loadingTaskIds.value.delete(taskId)
    }
  }

  function isLoading(taskId: number): boolean {
    return loadingTaskIds.value.has(taskId)
  }

  return {
    list,
    visible,
    totalCount,
    loadingTaskIds,
    add,
    remove,
    clear,
    setLoading,
    isLoading
  }
})
