/**
 * 侧边菜单用户手动排序（纯函数，组件与单测共用）。
 *
 * 用户自定义的顺序以「slotId 有序表」快照保存：快照只记顺序、不记条目内容。
 * 菜单条目集合会随插件启停增减，故渲染时要把快照与静态排序后的条目列表合成——
 * 快照中仍存在的条目按快照序输出，快照未覆盖的新条目按静态 order 插回，
 * 从而「插件新增菜单」与「插件卸载后的残留 slotId」都不会破坏用户已调好的顺序。
 */

/** 用户序快照：slotId 的有序表；空表表示用户未自定义 */
export type MenuOrderSnapshot = readonly string[]

// 可参与用户排序的菜单条目结构：slotId 为条目唯一标识（内置项为 builtin- 前缀，
// 插件项为 publicId/extensionId 复合键），order 为条目自身声明的静态排序号
export interface OrderableMenuItem {
  slotId: string
  order?: number
}

// 条目缺省静态排序号：未声明 order 的条目按 100 参与比较，与静态排序基准一致
const DEFAULT_ORDER = 100

// 读条目静态排序号
function orderOf(item: OrderableMenuItem): number {
  return item.order ?? DEFAULT_ORDER
}

/**
 * 按用户序快照重排一级菜单条目。
 *
 * 快照为空即用户未自定义，直接返回入参数组本身（不复制、顺序不变）。
 * 快照非空时按三步合成：
 * 1. 按快照序输出仍在条目列表中的条目——快照中已失效的 slotId 丢弃，重复 slotId 只输出一次；
 * 2. 快照未覆盖的条目（快照落定后新出现的条目）保持彼此间的静态序；
 * 3. 这些新条目逐个智能插入已排列表：插到首个静态 order 严格大于它的条目之前，没有则追加末尾；
 *    与既有条目 order 相等时排在这些同值条目之后，与静态排序的平局规则一致。
 *
 * @param items 静态排序后的一级菜单条目列表
 * @param snapshot 用户序快照（空表示未自定义）
 * @returns 合成后的新数组；入参数组与快照均不被修改
 */
export function applyUserOrder<T extends OrderableMenuItem>(
  items: T[],
  snapshot: MenuOrderSnapshot
): T[] {
  if (snapshot.length === 0) return items

  const itemBySlotId = new Map<string, T>()
  for (const item of items) {
    if (!itemBySlotId.has(item.slotId)) itemBySlotId.set(item.slotId, item)
  }

  const ordered: T[] = []
  const placed = new Set<string>()

  // 第一步：快照序中的存量条目
  for (const slotId of snapshot) {
    if (placed.has(slotId)) continue
    const item = itemBySlotId.get(slotId)
    if (!item) continue
    ordered.push(item)
    placed.add(slotId)
  }

  // 第二、三步：快照未覆盖的条目按静态序逐个智能插入
  for (const item of items) {
    if (placed.has(item.slotId)) continue
    const insertAt = ordered.findIndex((existing) => orderOf(existing) > orderOf(item))
    if (insertAt < 0) ordered.push(item)
    else ordered.splice(insertAt, 0, item)
    placed.add(item.slotId)
  }

  return ordered
}

/**
 * 拖拽落定后构建新的用户序快照。
 *
 * 语义不变量：结果 = 当前显示顺序（currentIds 为全量 slotId 表）+ 本次移动。
 * 因此快照为空时，一次拖动就把当前显示顺序全量固化为用户序（首次自定义）；
 * 快照非空且与当前显示顺序完全一致时以快照为基线（两者等价，省去重建）；
 * 两者不一致（当前列表出现了新增条目，或快照残留失效 slotId）时以当前显示顺序为基线，
 * 保证未拖动条目之间的相对关系与用户眼前所见一致。
 *
 * 移动语义：移除 fromIndex 处的元素后，把它插入结果数组的 toIndex 位置——
 * toIndex 即该元素在结果数组中的下标。下标越界按边界截断（toIndex 超出末尾即追加到末尾）；
 * fromIndex 越界时返回基线副本，不做移动。
 *
 * @param currentIds 当前界面上全量条目的 slotId 有序表
 * @param snapshot 拖拽前的用户序快照（空表示用户未自定义）
 * @param fromIndex 被拖动条目在 currentIds 中的下标
 * @param toIndex 拖动后该条目在结果快照中的下标
 * @returns 新的用户序快照；入参数组与快照均不被修改
 */
export function buildSnapshotAfterMove(
  currentIds: readonly string[],
  snapshot: MenuOrderSnapshot,
  fromIndex: number,
  toIndex: number
): string[] {
  const baseline = isSameSequence(snapshot, currentIds) ? [...snapshot] : [...currentIds]
  if (fromIndex < 0 || fromIndex >= baseline.length) return baseline

  const moved = baseline.splice(fromIndex, 1)[0]
  const target = Math.min(Math.max(toIndex, 0), baseline.length)
  baseline.splice(target, 0, moved)
  return baseline
}

// 两个 slotId 表是否同长同序
function isSameSequence(left: readonly string[], right: readonly string[]): boolean {
  if (left.length !== right.length) return false
  return left.every((slotId, index) => slotId === right[index])
}
