import { describe, expect, it } from 'vitest'
import { resolveAttentionTarget, type AttentionMenuItem } from './menuAttention'

// 测试菜单树：内置分组（子项挂宿主路由名）+ 插件顶层项（viewId 为复合路由名），
// 覆盖真实菜单的两种形态；orphan 项无 viewId（纯分组占位）
function buildMenuTree(): AttentionMenuItem[] {
  return [
    {
      children: [{ viewId: 'mainPage' }, { viewId: 'settings' }]
    },
    { viewId: 'com.plugin.a/workbench' },
    { viewId: 'com.plugin.a/gallery' }
  ]
}

// parentNameOf 测试替身：路由表 name → meta.parent 的查表
function parentNameFromRecord(record: Record<string, string>): (name: string) => string | undefined {
  return (name: string) => record[name]
}

describe('resolveAttentionTarget 定位解析', () => {
  it('route.name 精确命中：返回 viewId 等于路由名的菜单项（含分组内子项）', () => {
    const items = buildMenuTree()

    // 顶层项精确命中
    expect(resolveAttentionTarget(items, 'com.plugin.a/workbench', parentNameFromRecord({})))
      .toBe(items[1])

    // 分组内子项精确命中
    expect(resolveAttentionTarget(items, 'settings', parentNameFromRecord({})))
      .toBe(items[0].children?.[1])
  })

  it('精确未中沿 meta.parent 上溯：返回最近的持有菜单项的祖先（跨层跳过无菜单项的中间层）', () => {
    const items = buildMenuTree()
    // 二级解析页 → 一级工作台（顶层菜单项）；中间隔一层无菜单项的页面时逐层上溯
    const parentOf = parentNameFromRecord({
      'com.plugin.a/workbench-parse': 'com.plugin.a/workbench'
    })
    expect(resolveAttentionTarget(items, 'com.plugin.a/workbench-parse', parentOf))
      .toBe(items[1])

    // 多级链：深层页 → 中间页（无菜单项）→ 工作台，命中链上最近的祖先项
    const deepParentOf = parentNameFromRecord({
      'deep-page': 'mid-page',
      'mid-page': 'com.plugin.a/workbench'
    })
    expect(resolveAttentionTarget(items, 'deep-page', deepParentOf))
      .toBe(items[1])
  })

  it('无命中返回 undefined：父链为空或链上无任何持有菜单项的祖先', () => {
    const items = buildMenuTree()

    // 路由名未命中且无父链
    expect(resolveAttentionTarget(items, 'nowhere', parentNameFromRecord({})))
      .toBeUndefined()

    // 父链存在但链上终点无菜单项
    const parentOf = parentNameFromRecord({ 'dangling-page': 'also-no-item' })
    expect(resolveAttentionTarget(items, 'dangling-page', parentOf))
      .toBeUndefined()

    // 路由名缺省直接无命中
    expect(resolveAttentionTarget(items, undefined, parentNameFromRecord({})))
      .toBeUndefined()
  })

  it('父链闭环自保护：成环链截断上溯并按无命中返回，不死循环', () => {
    const items = buildMenuTree()
    // a → b → a 闭环，两名字均无菜单项
    const cyclic = parentNameFromRecord({ a: 'b', b: 'a' })
    expect(resolveAttentionTarget(items, 'a', cyclic)).toBeUndefined()

    // 自指父（parent 指向自身）
    const selfRef = parentNameFromRecord({ a: 'a' })
    expect(resolveAttentionTarget(items, 'a', selfRef)).toBeUndefined()
  })

  it('精确命中优先于上溯：即使父链上有菜单项，精确匹配胜出', () => {
    const items = buildMenuTree()
    const parentOf = parentNameFromRecord({ 'com.plugin.a/gallery': 'com.plugin.a/workbench' })
    expect(resolveAttentionTarget(items, 'com.plugin.a/gallery', parentOf))
      .toBe(items[2])
  })
})
