import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, type Component } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import lodash from 'lodash'
import Settings from './Settings.vue'
import { emptySettings } from '@renderer/model/util/Settings'

// —— wrapper 层替身（vi.hoisted：vi.mock 工厂随 mock 提升先于顶层常量求值）——
// settings wrapper 被 '@renderer/apis/http' 与主题/向导/工作目录状态 store 两条链消费，
// 两处 mock 共享同一组替身
const {
  settingsGetSettingsMock,
  setExternalApiEnabledMock,
  resetExternalApiTokenMock,
  externalApiListOpRecordsMock,
  messageBoxConfirmMock,
  elMessageMock
} = vi.hoisted(() => ({
  settingsGetSettingsMock: vi.fn(),
  settingsSaveSettingsMock: vi.fn(),
  settingsResetSettingsMock: vi.fn(),
  setExternalApiEnabledMock: vi.fn(),
  resetExternalApiTokenMock: vi.fn(),
  // 外部操作记录列表默认回空页（各用例挂载链都会触发一次列表加载）
  externalApiListOpRecordsMock: vi.fn().mockResolvedValue({ success: true, msg: '', data: { data: [], dataCount: 0 } }),
  messageBoxConfirmMock: vi.fn(),
  elMessageMock: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn()
  })
}))

vi.mock('@renderer/apis/http/wrappers/settings', () => ({
  settingsGetSettings: settingsGetSettingsMock,
  settingsSaveSettings: vi.fn(),
  settingsResetSettings: vi.fn(),
  settingsSetExternalApiEnabled: setExternalApiEnabledMock,
  settingsResetExternalApiToken: resetExternalApiTokenMock
}))

vi.mock('@renderer/apis/http/wrappers/externalapi', () => ({
  externalApiListOpRecords: externalApiListOpRecordsMock
}))

vi.mock('@renderer/apis/http', () => ({
  settingsApi: {
    settingsGetSettings: settingsGetSettingsMock,
    settingsSaveSettings: vi.fn(),
    settingsResetSettings: vi.fn(),
    settingsSetExternalApiEnabled: setExternalApiEnabledMock,
    settingsResetExternalApiToken: resetExternalApiTokenMock
  },
  externalApiApi: { externalApiListOpRecords: externalApiListOpRecordsMock },
  fileSysUtilApi: { fileSysUtilSelectDirectory: vi.fn() },
  fsmonitorApi: { fsmonitorGetAutoRepairPolicySchema: vi.fn().mockResolvedValue({ data: [] }) },
  workdirGuardApi: { workdirGuardGetInfo: vi.fn().mockResolvedValue({ data: null }) },
  stickyMemoryApi: { stickyMemoryListMemories: vi.fn().mockResolvedValue({ data: [] }) },
  pluginPreferenceApi: { pluginPreferenceListAll: vi.fn().mockResolvedValue({ data: [] }) },
  storageLocationApi: { storageLocationGetLocations: vi.fn().mockResolvedValue({ data: null }) },
  appLauncherApi: { appLauncherOpenAbsolutePath: vi.fn() }
}))

vi.mock('@renderer/apis/http/wrappers/share', () => ({
  shareProtocolStatus: vi.fn().mockResolvedValue(null),
  shareUnregisterProtocol: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: elMessageMock,
  ElMessageBox: { confirm: messageBoxConfirmMock }
}))

// —— Element Plus / 视图壳组件替身（渲染函数实现，不依赖运行时模板编译器）——

// 透传替身：渲染默认插槽，未解析的 el-* 组件全部由此兜住
function passthrough(name: string): Component {
  return defineComponent({
    name,
    setup(_, { slots }) {
      return () => h('div', { class: `${name}-stub` }, slots.default?.())
    }
  })
}

// el-switch 替身：按钮形态，点击 emit change/update:modelValue，当前值经 data-model-value 透出
const ElSwitchStub: Component = defineComponent({
  name: 'ElSwitch',
  props: {
    modelValue: { type: Boolean, default: false },
    activeText: String,
    inactiveText: String,
    inlinePrompt: Boolean,
    size: String,
    loading: Boolean,
    disabled: Boolean
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          class: 'el-switch-stub',
          'data-model-value': String(props.modelValue),
          'data-loading': String(props.loading),
          onClick: () => {
            const next = !props.modelValue
            emit('update:modelValue', next)
            emit('change', next)
          }
        },
        props.modelValue ? (props.activeText ?? '开') : (props.inactiveText ?? '关')
      )
  }
})

// el-button 替身：原生 button（点击监听经 attrs 落到根元素），按钮文案即插槽内容
const ElButtonStub: Component = defineComponent({
  name: 'ElButton',
  setup(_, { slots }) {
    return () => h('button', { type: 'button', class: 'el-button-stub' }, slots.default?.())
  }
})

// BaseView 壳替身：透出默认与 dialog 两个插槽
const BaseViewStub: Component = defineComponent({
  name: 'BaseView',
  setup(_, { slots }) {
    return () => h('div', { class: 'base-view-stub' }, [slots.default?.(), slots.dialog?.()])
  }
})

const stubComponents: Record<string, Component> = {
  BaseView: BaseViewStub,
  HighlightRing: passthrough('HighlightRing'),
  ElSwitch: ElSwitchStub,
  ElButton: ElButtonStub,
  ElAlert: passthrough('ElAlert'),
  ElAnchor: passthrough('ElAnchor'),
  ElAnchorLink: passthrough('ElAnchorLink'),
  ElCard: passthrough('ElCard'),
  ElCol: passthrough('ElCol'),
  ElContainer: passthrough('ElContainer'),
  ElDialog: passthrough('ElDialog'),
  ElFooter: passthrough('ElFooter'),
  ElInput: passthrough('ElInput'),
  ElInputNumber: passthrough('ElInputNumber'),
  ElMain: passthrough('ElMain'),
  ElOption: passthrough('ElOption'),
  ElPagination: passthrough('ElPagination'),
  ElRow: passthrough('ElRow'),
  ElScrollbar: passthrough('ElScrollbar'),
  ElSelect: passthrough('ElSelect'),
  ElTag: passthrough('ElTag'),
  ElText: passthrough('ElText'),
  ElTooltip: passthrough('ElTooltip')
}

// buildExternalApiSettings 设置载荷工厂（externalApi 节按用例覆写，其余为默认骨架）
function buildSettings(externalApi: { enabled: boolean; port?: number; token?: string }) {
  return lodash.cloneDeep({
    ...emptySettings,
    externalApi: {
      enabled: externalApi.enabled,
      port: externalApi.port ?? 0,
      token: externalApi.token ?? 'tok-display-123'
    }
  })
}

// mountSettings 挂载设置页并等待 onBeforeMount 的各加载链完成
async function mountSettings(externalApi: { enabled: boolean; port?: number; token?: string }) {
  settingsGetSettingsMock.mockReset().mockResolvedValue({ success: true, msg: '', data: buildSettings(externalApi) })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { render: () => null } }]
  })
  // 触发首次导航后再等就绪：未 push 时初始导航不发生，isReady 永不 resolve
  void router.push('/')
  await router.isReady()
  const wrapper = mount(Settings, {
    global: {
      plugins: [createPinia(), router],
      components: stubComponents
    }
  })
  await flushPromises()
  return wrapper
}

// externalApiSwitch 外部接口开关替身（区块内唯一定位）
function externalApiSwitch(wrapper: ReturnType<typeof mount>) {
  return wrapper.find('.settings-external-api-switch-item .el-switch-stub')
}

describe('Settings 外部接口区块', () => {
  it('挂载渲染：开关反映启用态，启用时展示 token 与端口行，未启用时 token 行不渲染', async () => {
    const wrapper = await mountSettings({ enabled: true, token: 'tok-display-123' })

    const switchStub = externalApiSwitch(wrapper)
    expect(switchStub.exists()).toBe(true)
    expect(switchStub.attributes('data-model-value')).toBe('true')
    expect(switchStub.text()).toContain('开')

    expect(wrapper.find('.settings-external-api-token-item').exists()).toBe(true)
    expect(wrapper.find('.settings-external-api-token').text()).toBe('tok-display-123')

    wrapper.unmount()

    const offWrapper = await mountSettings({ enabled: false })
    expect(externalApiSwitch(offWrapper).attributes('data-model-value')).toBe('false')
    expect(offWrapper.find('.settings-external-api-token-item').exists()).toBe(false)
  })

  it('切换开关：以新值调用外部接口开关控制方法，成功后开关态随响应刷新', async () => {
    const wrapper = await mountSettings({ enabled: false })
    setExternalApiEnabledMock
      .mockReset()
      .mockResolvedValueOnce({ success: true, msg: '', data: { running: true, addr: '127.0.0.1:45678', lastError: '' } })
      .mockResolvedValueOnce({ success: true, msg: '', data: { running: false, addr: '', lastError: '' } })

    const switchStub = externalApiSwitch(wrapper)
    expect(switchStub.attributes('data-model-value')).toBe('false')

    await switchStub.trigger('click')
    await flushPromises()
    expect(setExternalApiEnabledMock).toHaveBeenCalledWith(true)
    expect(switchStub.attributes('data-model-value')).toBe('true')

    await switchStub.trigger('click')
    await flushPromises()
    expect(setExternalApiEnabledMock).toHaveBeenCalledWith(false)
    expect(switchStub.attributes('data-model-value')).toBe('false')
  })

  it('开启降级：监听未随开启启动时按状态载荷提示，开关态保持已开启', async () => {
    const wrapper = await mountSettings({ enabled: false })
    setExternalApiEnabledMock.mockReset().mockResolvedValue({
      success: true,
      msg: '',
      data: { running: false, addr: '', lastError: '外部接口监听启动失败: 端口被占用' }
    })

    await externalApiSwitch(wrapper).trigger('click')
    await flushPromises()

    expect(setExternalApiEnabledMock).toHaveBeenCalledWith(true)
    expect(externalApiSwitch(wrapper).attributes('data-model-value')).toBe('true')
    // 降级提示走 ElMessage 函数形态（对象参数携带 type: 'warning'）
    expect(elMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'warning', message: expect.stringContaining('端口被占用') })
    )
  })

  it('重置 token：确认后调用控制方法，展示随响应新 token 刷新', async () => {
    const wrapper = await mountSettings({ enabled: true, token: 'tok-old' })
    resetExternalApiTokenMock.mockReset().mockResolvedValue({ success: true, msg: '', data: 'tok-new-456' })
    messageBoxConfirmMock.mockReset().mockResolvedValue('confirm')

    expect(wrapper.find('.settings-external-api-token').text()).toBe('tok-old')

    const resetButton = wrapper
      .findAll('.settings-external-api-token-item .el-button-stub')
      .find((b) => b.text() === '重置')
    expect(resetButton).toBeDefined()
    await resetButton!.trigger('click')
    await flushPromises()

    expect(messageBoxConfirmMock).toHaveBeenCalled()
    expect(resetExternalApiTokenMock).toHaveBeenCalled()
    expect(wrapper.find('.settings-external-api-token').text()).toBe('tok-new-456')
  })

  // —— 外部操作记录只读列表区块 ——

  // 记录行载荷工厂（与后端分页页对象的 data/dataCount 形态一致）
  function buildOpRecordPage(rows: Array<Record<string, unknown>>, total: number) {
    return {
      pageNumber: 1,
      pageSize: 10,
      pageCount: total > 10 ? 2 : 1,
      dataCount: total,
      currentCount: rows.length,
      data: rows
    }
  }

  it('外部操作记录列表：挂载渲染记录行（操作/成败/耗时/自报标签/脱敏参数），首屏按第 1 页读取', async () => {
    externalApiListOpRecordsMock
      .mockReset()
      .mockResolvedValue({ success: true, msg: '', data: buildOpRecordPage(
        [
          {
            id: 2,
            createTime: 1760000000000,
            updateTime: 1760000000000,
            operation: 'taskManager.confirmReplace',
            params: '{"taskId":42,"action":"replace"}',
            success: true,
            clientLabel: 'dsh-agent',
            durationMs: 8
          },
          {
            id: 1,
            createTime: 1759999000000,
            updateTime: 1759999000000,
            operation: 'task.createByURL',
            params: '{"url":"https://www.pixiv.net/artworks/123456 ?…（含查询串 31 字符）"}',
            success: false,
            clientLabel: '',
            durationMs: 120
          }
        ],
        2
      ) })

    const wrapper = await mountSettings({ enabled: true })
    await flushPromises()

    expect(externalApiListOpRecordsMock).toHaveBeenCalledWith(1, 10)

    const rows = wrapper.findAll('.settings-external-api-record-row')
    expect(rows.length).toBe(2)

    expect(rows[0].text()).toContain('taskManager.confirmReplace')
    expect(rows[0].text()).toContain('成功')
    expect(rows[0].text()).toContain('8 ms')
    expect(rows[0].text()).toContain('dsh-agent')

    expect(rows[1].text()).toContain('task.createByURL')
    expect(rows[1].text()).toContain('失败')
    expect(rows[1].text()).toContain('未自报')
    expect(rows[1].find('.settings-external-api-record-failed').exists()).toBe(true)
    // 脱敏参数摘要纯展示（URL 查询串已不落记录）
    expect(rows[1].find('.settings-external-api-record-params').text()).toContain('www.pixiv.net/artworks/123456')
  })

  it('外部操作记录列表：空账表渲染占位文案；记录区块不随接口关闭隐藏（历史可回看）', async () => {
    externalApiListOpRecordsMock.mockReset().mockResolvedValue({ success: true, msg: '', data: buildOpRecordPage([], 0) })

    const wrapper = await mountSettings({ enabled: false })
    await flushPromises()

    expect(wrapper.find('.settings-external-api-records-item').exists()).toBe(true)
    expect(wrapper.findAll('.settings-external-api-record-row').length).toBe(0)
    expect(wrapper.find('.settings-external-api-record-empty').text()).toContain('暂无外部调用记录')
  })
})
