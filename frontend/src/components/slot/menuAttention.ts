/**
 * 侧边菜单跳转定位解析（纯函数，组件与单测共用）：
 * 给定菜单树与当前路由名，解析应被滚动/脉冲提示的菜单项——
 * 先按 route.name 精确匹配菜单项 viewId，未中则沿路由 meta.parent 链上溯
 * （紧邻父级在前），每上溯一层即在该层路由名上重试精确匹配，命中最近的持有菜单项的祖先。
 * 父名的取法由调用方以 parentNameOf 回调提供（从路由表读 meta.parent），本模块不接触 router。
 */

// 可参与定位解析的菜单项结构：viewId 即关联视图的路由名，children 为分组子项
export interface AttentionMenuItem {
  viewId?: string
  children?: AttentionMenuItem[]
}

// 父链上溯步数上限：截断成环的 meta.parent 链，深度超过常规页面层级即视为异常数据
const MAX_PARENT_HOPS = 32

/**
 * 解析跳转定位目标菜单项。
 * @param menuItems 菜单树（任意层级）
 * @param routeName 当前路由名（空值直接无命中）
 * @param parentNameOf 路由名 → 其父页面路由名（无父或路由不在场返回 undefined）
 * @returns 命中的菜单项（精确命中优先，其次最近的持有菜单项的祖先）；无命中返回 undefined
 */
export function resolveAttentionTarget<T extends AttentionMenuItem>(
  menuItems: T[],
  routeName: string | null | undefined,
  parentNameOf: (name: string) => string | null | undefined
): T | undefined {
  if (!routeName) return undefined

  const exact = findItemByViewId(menuItems, routeName)
  if (exact) return exact

  // 上溯：seen 集阻断闭环链（a→b→a 形态在第二次抵达已见名字时止步）
  const seen = new Set<string>([routeName])
  let cursor: string | undefined = routeName
  for (let hops = 0; hops < MAX_PARENT_HOPS; hops++) {
    const parent = parentNameOf(cursor)
    if (!parent || seen.has(parent)) break
    seen.add(parent)
    const ancestor = findItemByViewId(menuItems, parent)
    if (ancestor) return ancestor
    cursor = parent
  }
  return undefined
}

// 深度优先查找 viewId 对应的菜单项（先本层后子层，同层按数组序）
function findItemByViewId<T extends AttentionMenuItem>(items: T[], viewId: string): T | undefined {
  for (const item of items) {
    if (item.viewId === viewId) return item
    if (item.children?.length) {
      const matched = findItemByViewId(item.children, viewId)
      // children 在接口上声明为非泛型基型，递归返回值经此收窄回 T：命中项必取自入参数组，运行期即为 T
      if (matched) return matched as T
    }
  }
  return undefined
}
