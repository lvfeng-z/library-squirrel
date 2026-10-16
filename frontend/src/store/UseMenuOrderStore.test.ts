import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { reorderMenusByUserOrder, useMenuOrderStore } from './UseMenuOrderStore.ts'
import type { OrderableMenuItem } from '@renderer/components/slot/menuOrdering'

// —— wrapper 层替身（vi.hoisted：vi.mock 工厂随 mock 提升先于顶层常量求值）——
const { settingsGetSettingsMock, settingsSaveSettingsMock } = vi.hoisted(() => ({
  settingsGetSettingsMock: vi.fn(),
  settingsSaveSettingsMock: vi.fn()
}))

vi.mock('@renderer/apis/http/wrappers/settings', () => ({
  settingsGetSettings: settingsGetSettingsMock,
  settingsSaveSettings: settingsSaveSettingsMock
}))

// 设置读取响应：只带用例关心的外观分区
function settingsResponse(appearance: unknown) {
  return { success: true, msg: '', data: { appearance } }
}

// 构造静态排序后的一级菜单条目；order 传 undefined 表示该条目未声明静态排序号
function buildItems(entries: Array<[string, number | undefined]>): OrderableMenuItem[] {
  return entries.map(([slotId, order]) => ({ slotId, order }))
}

// 条目序列的 slotId 有序表（断言用）
function idsOf(items: OrderableMenuItem[]): string[] {
  return items.map((item) => item.slotId)
}

describe('UseMenuOrderStore 启动加载', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    settingsGetSettingsMock.mockReset()
    settingsSaveSettingsMock.mockReset()
    settingsSaveSettingsMock.mockResolvedValue({ success: true, msg: '', data: true })
  })

  it('外观分区无 menuOrder 字段：未自定义态（空快照），条目保持静态序', async () => {
    settingsGetSettingsMock.mockResolvedValue(settingsResponse({ theme: 'default-light' }))
    const store = useMenuOrderStore()
    const items = buildItems([
      ['builtin-main', 10],
      ['builtin-tag', 20]
    ])

    await store.load()

    expect(store.snapshot).toEqual([])
    // 空快照下纯函数原样返回入参（未自定义即静态序）
    expect(reorderMenusByUserOrder(items)).toBe(items)
  })

  it('menuOrder 为 null 或非数组：一律按未自定义处理', async () => {
    const store = useMenuOrderStore()

    settingsGetSettingsMock.mockResolvedValue(settingsResponse({ menuOrder: null }))
    await store.load()
    expect(store.snapshot).toEqual([])

    settingsGetSettingsMock.mockResolvedValue(settingsResponse({ menuOrder: 'builtin-main' }))
    await store.load()
    expect(store.snapshot).toEqual([])
  })

  it('解析已有 menuOrder 全量表并据此重排条目', async () => {
    settingsGetSettingsMock.mockResolvedValue(
      settingsResponse({ menuOrder: ['builtin-tag', 'builtin-main', 'com.plugin.a/gallery'] })
    )
    const store = useMenuOrderStore()
    const items = buildItems([
      ['builtin-main', 10],
      ['builtin-tag', 20],
      ['com.plugin.a/gallery', 30]
    ])

    await store.load()

    expect(store.snapshot).toEqual(['builtin-tag', 'builtin-main', 'com.plugin.a/gallery'])
    expect(idsOf(reorderMenusByUserOrder(items))).toEqual(['builtin-tag', 'builtin-main', 'com.plugin.a/gallery'])
  })

  it('表内非字符串项丢弃，只保留 slotId 字符串', async () => {
    settingsGetSettingsMock.mockResolvedValue(
      settingsResponse({ menuOrder: ['builtin-tag', 7, null, { slotId: 'x' }, 'builtin-main'] })
    )
    const store = useMenuOrderStore()

    await store.load()

    expect(store.snapshot).toEqual(['builtin-tag', 'builtin-main'])
  })

  it('读取失败：保持未自定义态且不抛错', async () => {
    settingsGetSettingsMock.mockResolvedValue({ success: false, msg: '读取失败' })
    const store = useMenuOrderStore()

    await store.load()

    expect(store.snapshot).toEqual([])
    expect(settingsSaveSettingsMock).not.toHaveBeenCalled()
  })
})

describe('UseMenuOrderStore 拖拽落定持久化', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    settingsGetSettingsMock.mockReset()
    settingsSaveSettingsMock.mockReset()
    settingsSaveSettingsMock.mockResolvedValue({ success: true, msg: '', data: true })
  })

  it('首次拖动：按变更路径提交全量 slotId 有序表', async () => {
    const store = useMenuOrderStore()
    const currentIds = ['builtin-main', 'builtin-tag', 'com.plugin.a/gallery']

    const snapshot = await store.commitMove(currentIds, 0, 2)

    expect(settingsSaveSettingsMock).toHaveBeenCalledTimes(1)
    expect(settingsSaveSettingsMock).toHaveBeenCalledWith([
      { path: 'appearance.menuOrder', value: ['builtin-tag', 'com.plugin.a/gallery', 'builtin-main'] }
    ])
    expect(snapshot).toEqual(['builtin-tag', 'com.plugin.a/gallery', 'builtin-main'])
    expect(store.snapshot).toEqual(['builtin-tag', 'com.plugin.a/gallery', 'builtin-main'])
  })

  it('已有快照上再拖动：以当前显示顺序为基线，提交的仍是全量表', async () => {
    settingsGetSettingsMock.mockResolvedValue(
      settingsResponse({ menuOrder: ['builtin-tag', 'builtin-main'] })
    )
    const store = useMenuOrderStore()
    await store.load()

    // 快照落定后新增了 builtin-new：基线取当前显示顺序，未拖动条目相对关系不变
    const snapshot = await store.commitMove(
      ['builtin-tag', 'builtin-main', 'builtin-new'],
      2,
      0
    )

    expect(settingsSaveSettingsMock).toHaveBeenCalledWith([
      { path: 'appearance.menuOrder', value: ['builtin-new', 'builtin-tag', 'builtin-main'] }
    ])
    expect(snapshot).toEqual(['builtin-new', 'builtin-tag', 'builtin-main'])
  })

  it('保存失败：抛错上报，本地快照保留本次拖拽结果', async () => {
    settingsSaveSettingsMock.mockResolvedValue({ success: false, msg: '设置写入失败' })
    const store = useMenuOrderStore()

    await expect(
      store.commitMove(['builtin-main', 'builtin-tag'], 0, 1)
    ).rejects.toThrow('设置写入失败')
    expect(store.snapshot).toEqual(['builtin-tag', 'builtin-main'])
  })
})

describe('UseMenuOrderStore 重置', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    settingsGetSettingsMock.mockReset()
    settingsSaveSettingsMock.mockReset()
    settingsSaveSettingsMock.mockResolvedValue({ success: true, msg: '', data: true })
  })

  it('清空本地快照并提交空值（恢复默认静态序）', async () => {
    settingsGetSettingsMock.mockResolvedValue(
      settingsResponse({ menuOrder: ['builtin-tag', 'builtin-main'] })
    )
    const store = useMenuOrderStore()
    await store.load()
    expect(store.snapshot).toEqual(['builtin-tag', 'builtin-main'])

    await store.reset()

    expect(store.snapshot).toEqual([])
    expect(settingsSaveSettingsMock).toHaveBeenCalledWith([
      { path: 'appearance.menuOrder', value: [] }
    ])
  })
})
