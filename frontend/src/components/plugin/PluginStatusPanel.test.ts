import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, type Component } from 'vue'
import PluginStatusPanel from './PluginStatusPanel.vue'
import {
  ParticipationEvalStatus,
  ParticipationOverview,
  PluginStatusDTO
} from '@bindings/github.com/library-squirrel/backend/plugin/models'

// pluginApi 替身（vi.hoisted：vi.mock 工厂随 mock 提升先于顶层常量求值）
const { pluginGetStatusMock, pluginGetByPublicIdMock } = vi.hoisted(() => ({
  pluginGetStatusMock: vi.fn(),
  pluginGetByPublicIdMock: vi.fn()
}))
vi.mock('@renderer/apis/http', () => ({
  pluginApi: {
    pluginGetStatus: pluginGetStatusMock,
    pluginGetByPublicId: pluginGetByPublicIdMock
  }
}))
vi.mock('element-plus', () => ({
  ElMessage: { error: vi.fn() }
}))

// —— Element Plus 组件替身（渲染函数实现，不依赖运行时模板编译器）——

// el-tooltip 替身：把悬浮内容透出为 data-tooltip-content 供断言，触发器插槽照常渲染
const ElTooltipStub: Component = defineComponent({
  props: { content: String, placement: String },
  setup(props, { slots }) {
    return () =>
      h('span', { class: 'el-tooltip-stub', 'data-tooltip-content': props.content }, slots.default?.())
  }
})

const ElDescriptionsStub: Component = defineComponent({
  props: { title: String },
  setup(props, { slots }) {
    return () => h('div', { class: 'el-descriptions-stub', 'data-title': props.title }, slots.default?.())
  }
})

const ElDescriptionsItemStub: Component = defineComponent({
  props: { label: String },
  setup(props, { slots }) {
    return () => h('div', { class: 'el-descriptions-item-stub', 'data-label': props.label }, slots.default?.())
  }
})

const ElTagStub: Component = defineComponent({
  props: { type: String, size: String },
  setup(_props, { slots }) {
    return () => h('span', { class: 'el-tag-stub' }, slots.default?.())
  }
})

// buildStatus 组装面板状态载荷（participation 为 null = 插件未激活无会话）
function buildStatus(participation: ParticipationOverview | null): PluginStatusDTO {
  return new PluginStatusDTO({
    lifecycleState: 'active',
    isRunning: true,
    pid: 1234,
    activatedAt: 1780000000000,
    urlPatterns: [],
    participation
  })
}

// mountPanel 挂载面板并等待自取状态完成
function mountPanel(status: PluginStatusDTO) {
  pluginGetStatusMock.mockReset().mockResolvedValue({ data: status })
  pluginGetByPublicIdMock.mockReset().mockResolvedValue({ data: null })
  return mount(PluginStatusPanel, {
    props: { publicId: 'com.test.panel' },
    global: {
      directives: { loading: {} },
      stubs: {
        ElDescriptions: ElDescriptionsStub,
        ElDescriptionsItem: ElDescriptionsItemStub,
        ElTooltip: ElTooltipStub,
        ElTag: ElTagStub
      }
    }
  })
}

function overviewOf(
  entries: Array<{ point: string; id: string; active: boolean; reason?: string }>,
  evalStatus?: Partial<ParticipationEvalStatus>
): ParticipationOverview {
  return new ParticipationOverview({
    entries: entries.map((e) => ({ ...e, reason: e.reason ?? '' })),
    status: new ParticipationEvalStatus({
      hasResolver: true,
      lastEvalAt: 1780000005000,
      ...evalStatus
    })
  })
}

describe('PluginStatusPanel 声明条目参与度', () => {
  it('条目行内徽标：参与条目标「参与中」、被覆盖停用条目标「已停用」，按派生面分组展示', async () => {
    const wrapper = mountPanel(buildStatus(overviewOf([
      { point: 'workFetch', id: 'main', active: false },
      { point: 'frontendExtensions', id: 'menu', active: true }
    ])))

    await flushPromises()

    // 分组行（声明 = 清单条目，按派生面展示）
    const labels = wrapper.findAll('.el-descriptions-item-stub').map((n) => n.attributes('data-label'))
    expect(labels).toContain('作品拉取')
    expect(labels).toContain('前端扩展')

    // 徽标状态：参与中 / 已停用（StatusTag 真组件，文案经 StatusRegistry 登记）
    const tagTexts = wrapper.findAll('.status-tag').map((n) => n.text())
    expect(tagTexts).toContain('参与中')
    expect(tagTexts).toContain('已停用')

    // 条目 id 随徽标展示
    expect(wrapper.text()).toContain('menu')
    expect(wrapper.text()).toContain('main')
  })

  it('停用理由悬浮：被覆盖停用且 resolver 给出理由的条目以 tooltip 展示理由，参与条目不悬浮', async () => {
    const wrapper = mountPanel(buildStatus(overviewOf([
      { point: 'workFetch', id: 'main', active: false, reason: '用户关闭了高质量模式' },
      { point: 'frontendExtensions', id: 'menu', active: true }
    ])))

    await flushPromises()

    const tooltip = wrapper.find('.el-tooltip-stub')
    expect(tooltip.exists()).toBe(true)
    expect(tooltip.attributes('data-tooltip-content')).toBe('停用理由：用户关闭了高质量模式')
    // 悬浮容器内照常渲染条目徽标（触发器不因悬浮丢失）
    expect(tooltip.text()).toContain('main')

    // 参与条目不走悬浮（整个面板仅一个 tooltip，来自停用条目）
    expect(wrapper.findAll('.el-tooltip-stub')).toHaveLength(1)
  })

  it('resolver 降级态显著标注：最近求值失败时展示降级横幅与拒收提示，正常态不展示', async () => {
    const degraded = mountPanel(buildStatus(overviewOf(
      [{ point: 'frontendExtensions', id: 'menu', active: true }],
      { lastFailure: 'timeout', lastFailureMsg: '脚本执行超时', lastRejected: 2 }
    )))
    await flushPromises()

    const banner = degraded.find('[data-testid="resolver-degraded"]')
    expect(banner.exists()).toBe(true)
    // 失败分类映射为人读标签 + 原始信息 + 滞后提示
    expect(banner!.text()).toContain('执行超时')
    expect(banner!.text()).toContain('脚本执行超时')
    expect(banner!.text()).toContain('滞后')
    // 单条拒收软降级提示
    const rejectedNote = degraded.find('[data-testid="rejected-note"]')
    expect(rejectedNote.exists()).toBe(true)
    expect(rejectedNote!.text()).toContain('2 条')

    const healthy = mountPanel(buildStatus(overviewOf(
      [{ point: 'frontendExtensions', id: 'menu', active: true }],
      { lastFailure: '', lastFailureMsg: '' }
    )))
    await flushPromises()
    expect(healthy.find('[data-testid="resolver-degraded"]').exists()).toBe(false)
    expect(healthy.text()).toContain('正常')
  })

  it('插件未激活无会话：后端不下发参与度概要，面板不渲染该节', async () => {
    const wrapper = mountPanel(buildStatus(null))
    await flushPromises()

    expect(wrapper.text()).not.toContain('声明条目参与度')
    expect(wrapper.find('.el-tooltip-stub').exists()).toBe(false)
  })
})
