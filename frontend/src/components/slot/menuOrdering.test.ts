import { describe, expect, it } from 'vitest'
import { applyUserOrder, buildSnapshotAfterMove, type OrderableMenuItem } from './menuOrdering'

// 构造静态排序后的一级菜单条目；order 传 undefined 表示该条目未声明静态排序号
function buildItems(entries: Array<[string, number | undefined]>): OrderableMenuItem[] {
  return entries.map(([slotId, order]) => ({ slotId, order }))
}

// 条目序列的 slotId 有序表（断言用）
function idsOf(items: OrderableMenuItem[]): string[] {
  return items.map((item) => item.slotId)
}

describe('applyUserOrder 用户序合成', () => {
  it('快照为空：原样返回，顺序与内容不变', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['builtin-tag', 20],
      ['com.plugin.a/gallery', 30]
    ])

    const result = applyUserOrder(items, [])

    expect(result).toBe(items)
    expect(idsOf(result)).toEqual(['builtin-main', 'builtin-tag', 'com.plugin.a/gallery'])
  })

  it('快照含失效 slotId：失效项丢弃，其余条目按快照序输出', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['builtin-tag', 20],
      ['com.plugin.a/gallery', 30]
    ])

    // 'com.plugin.removed/gone' 为插件卸载后残留在快照中的失效 id
    const result = applyUserOrder(items, ['com.plugin.a/gallery', 'com.plugin.removed/gone', 'builtin-main', 'builtin-tag'])

    expect(idsOf(result)).toEqual(['com.plugin.a/gallery', 'builtin-main', 'builtin-tag'])
  })

  it('快照全部失效：全部条目按静态序输出，等同未自定义', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['builtin-tag', 20],
      ['com.plugin.a/gallery', 30]
    ])

    const result = applyUserOrder(items, ['gone/a', 'gone/b'])

    expect(idsOf(result)).toEqual(['builtin-main', 'builtin-tag', 'com.plugin.a/gallery'])
  })

  it('新条目 order 小于已排列表首项：插到最前', () => {
    // 已排列表首项 order=30，新条目 order=5 严格更小
    const items = buildItems([
      ['new-5', 5],
      ['builtin-main', 10],
      ['com.plugin.a/gallery', 30]
    ])

    const result = applyUserOrder(items, ['com.plugin.a/gallery', 'builtin-main'])

    expect(idsOf(result)).toEqual(['new-5', 'com.plugin.a/gallery', 'builtin-main'])
  })

  it('新条目 order 落在区间：插到首个更大 order 条目之前', () => {
    // 已排列表 order 依次为 10、30，新条目 order=20 落在区间内
    const items = buildItems([
      ['builtin-main', 10],
      ['new-20', 20],
      ['com.plugin.a/gallery', 30]
    ])

    const result = applyUserOrder(items, ['builtin-main', 'com.plugin.a/gallery'])

    expect(idsOf(result)).toEqual(['builtin-main', 'new-20', 'com.plugin.a/gallery'])
  })

  it('新条目 order 大于全部既有条目：追加末尾', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['com.plugin.a/gallery', 30],
      ['new-40', 40]
    ])

    const result = applyUserOrder(items, ['com.plugin.a/gallery', 'builtin-main'])

    expect(idsOf(result)).toEqual(['com.plugin.a/gallery', 'builtin-main', 'new-40'])
  })

  it('新条目与既有条目 order 相等：排在全部同值条目之后', () => {
    // 已排列表含两个 order=20 条目，新条目 order 同为 20
    const items = buildItems([
      ['plugin-x/one', 20],
      ['plugin-x/two', 20],
      ['new-20', 20],
      ['com.plugin.a/gallery', 40]
    ])

    const result = applyUserOrder(items, ['plugin-x/two', 'plugin-x/one', 'com.plugin.a/gallery'])

    expect(idsOf(result)).toEqual(['plugin-x/two', 'plugin-x/one', 'new-20', 'com.plugin.a/gallery'])
  })

  it('多个新条目按静态序逐个插入：结果确定且可复现', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['new-20', 20],
      ['new-30', 30],
      ['com.plugin.a/gallery', 40]
    ])
    const snapshot = ['com.plugin.a/gallery', 'builtin-main']

    const first = applyUserOrder(items, snapshot)
    const second = applyUserOrder(items, snapshot)

    // 依次插入：20 插到 order=40 之前 → [new-20, gallery, main]；30 再插到 40 之前 → [new-20, new-30, gallery, main]
    expect(idsOf(first)).toEqual(['new-20', 'new-30', 'com.plugin.a/gallery', 'builtin-main'])
    expect(idsOf(second)).toEqual(idsOf(first))
  })

  it('未声明 order 的条目按 100 参与比较：等值时排在既有同值条目之后', () => {
    const items = buildItems([
      ['builtin-noorder', undefined],
      ['com.plugin.a/withorder', undefined]
    ])

    const result = applyUserOrder(items, ['com.plugin.a/withorder'])

    expect(idsOf(result)).toEqual(['com.plugin.a/withorder', 'builtin-noorder'])
  })

  it('不修改入参数组与快照', () => {
    const items = buildItems([
      ['builtin-main', 10],
      ['new-20', 20],
      ['com.plugin.a/gallery', 30]
    ])
    const snapshot = ['com.plugin.a/gallery', 'builtin-main']

    applyUserOrder(items, snapshot)

    expect(idsOf(items)).toEqual(['builtin-main', 'new-20', 'com.plugin.a/gallery'])
    expect(snapshot).toEqual(['com.plugin.a/gallery', 'builtin-main'])
  })
})

describe('buildSnapshotAfterMove 拖拽落定快照', () => {
  it('空快照 + 一次移动 = 全量固化', () => {
    const currentIds = ['builtin-main', 'builtin-tag', 'com.plugin.a/gallery']
    const snapshot: string[] = []

    const result = buildSnapshotAfterMove(currentIds, snapshot, 0, 2)

    // 首项拖到末位；快照长度与全量条目一致（未拖动条目一并固化）
    expect(result).toEqual(['builtin-tag', 'com.plugin.a/gallery', 'builtin-main'])
    expect(snapshot).toEqual([])
    expect(currentIds).toEqual(['builtin-main', 'builtin-tag', 'com.plugin.a/gallery'])
  })

  it('空快照 + 原地移动：顺序不变但快照仍全量固化', () => {
    const currentIds = ['builtin-main', 'builtin-tag', 'com.plugin.a/gallery']

    const result = buildSnapshotAfterMove(currentIds, [], 1, 1)

    expect(result).toEqual(['builtin-main', 'builtin-tag', 'com.plugin.a/gallery'])
  })

  it('快照与当前显示顺序一致：以快照为基线移动', () => {
    const currentIds = ['com.plugin.a/gallery', 'builtin-main', 'builtin-tag']
    const snapshot = ['com.plugin.a/gallery', 'builtin-main', 'builtin-tag']

    const result = buildSnapshotAfterMove(currentIds, snapshot, 0, 2)

    expect(result).toEqual(['builtin-main', 'builtin-tag', 'com.plugin.a/gallery'])
    expect(snapshot).toEqual(['com.plugin.a/gallery', 'builtin-main', 'builtin-tag'])
  })

  it('当前列表含新增条目：以当前显示顺序为基线，未拖动条目相对关系保持所见', () => {
    const currentIds = ['com.plugin.a/gallery', 'builtin-main', 'builtin-new']
    // 快照落定于新增条目出现之前，未包含 builtin-new
    const snapshot = ['com.plugin.a/gallery', 'builtin-main']

    const result = buildSnapshotAfterMove(currentIds, snapshot, 2, 0)

    expect(result).toEqual(['builtin-new', 'com.plugin.a/gallery', 'builtin-main'])
  })

  it('下标越界按边界截断：toIndex 超出末尾即追加，fromIndex 越界不移动', () => {
    const currentIds = ['builtin-main', 'builtin-tag', 'com.plugin.a/gallery']

    expect(buildSnapshotAfterMove(currentIds, [], 0, 9))
      .toEqual(['builtin-tag', 'com.plugin.a/gallery', 'builtin-main'])
    expect(buildSnapshotAfterMove(currentIds, [], -1, 0)).toEqual(currentIds)
  })
})
