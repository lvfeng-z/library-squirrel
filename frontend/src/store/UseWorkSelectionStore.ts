import { defineStore } from 'pinia'

/**
 * 主页作品/作品集多选选择集 store：跨页（含跨搜索、跨 tab）保持选中 id 集合。
 * 存 id 而非行对象——大库全选时选择集可达万级，行对象引用会让内存与失效追踪失控。
 * 作品与作品集两个集合独立维护——选择只作用于作品/作品集；标签/作者是作品的附加数据、
 * 恒完整连带导出、不存在选择概念。
 *
 * 同步语义：网格的 checkedChange 只携带「当前已加载项」中的勾选 id，已选但已离开可见集的项
 * （新搜索重置列表、翻页移出）不在其中。sync* 以网格当前可见 id 集为界：可见集外的已选项保留，
 * 可见集内的勾选以本次上报为准——选择集在新搜索/翻页后仍完整。
 */
export const useWorkSelectionStore = defineStore('workSelection', {
  state: (): { workIds: number[]; workSetIds: number[] } => {
    return { workIds: [], workSetIds: [] }
  },
  getters: {
    /** 选中作品数 */
    workCount: (state): number => state.workIds.length,
    /** 选中作品集数 */
    workSetCount: (state): number => state.workSetIds.length,
    /** 选中总数（作品 + 作品集），操作栏「已选 N 项」的数据源 */
    totalCount: (state): number => state.workIds.length + state.workSetIds.length,
    /** 是否有任何选择（操作栏出现/隐藏开关） */
    hasSelection: (state): boolean => state.workIds.length + state.workSetIds.length > 0
  },
  actions: {
    /** 同步作品勾选：ids=网格本次勾选、visibleWorkIds=网格当前已加载项 id 集（保留不在可见集的已选项） */
    syncWorkIds(ids: number[], visibleWorkIds: number[]): void {
      const visibleSet = new Set(visibleWorkIds)
      const retained = this.workIds.filter((id) => !visibleSet.has(id))
      this.workIds = Array.from(new Set([...retained, ...ids]))
    },
    /** 同步作品集勾选：语义同 syncWorkIds */
    syncWorkSetIds(ids: number[], visibleWorkSetIds: number[]): void {
      const visibleSet = new Set(visibleWorkSetIds)
      const retained = this.workSetIds.filter((id) => !visibleSet.has(id))
      this.workSetIds = Array.from(new Set([...retained, ...ids]))
    },
    /** 清空全部选择（作品 + 作品集）；网格经 checkedIds 置空联动取消勾选 */
    clear(): void {
      this.workIds = []
      this.workSetIds = []
    }
  }
})
