<template>
  <teleport to="#dialog-mount-point">
    <el-dialog
      v-model="visible"
      style="height: fit-content;"
      width="520px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
    >
      <template #header>
        <div class="explain-header">
          <el-tooltip content="本地导入插件请求解释路径段的含义，以便为作品填充作者、标签等信息" placement="bottom">
            <span>解释路径含义</span>
          </el-tooltip>
          <div class="explain-actions">
            <el-button type="success" icon="CirclePlus" @click="addMeaning">新增</el-button>
            <el-button type="primary" @click="confirm">确定</el-button>
            <el-button @click="cancel">取消</el-button>
          </div>
        </div>
      </template>
      <div class="explain-panel">
        <el-text class="explain-path-text">{{ question?.dirName }}</el-text>
        <el-scrollbar class="explain-scroll">
          <div class="explain-scroll-inner">
          <div v-for="(meaning, index) in meanings" :key="index" class="explain-row">
            <el-select v-model="meaning.type" class="explain-type-select" @change="onTypeChange(meaning, index)">
              <el-option v-for="opt in meaningTypes" :key="opt.value" :value="opt.value" :label="opt.label" />
            </el-select>
            <el-select
              v-if="isSelectType(meaning.type)"
              v-el-select-bottomed="() => loadPage(index)"
              v-model="meaning.id"
              class="explain-name-input"
              filterable
              remote
              clearable
              :remote-method="(q: string) => search(index, q)"
              :loading="getState(index).searchLoading"
              @change="onItemSelect(meaning, index)"
              @visible-change="(v: boolean) => onDropdownOpen(index, v)"
            >
              <el-option
                v-for="item in getState(index).options"
                :key="item.value"
                :value="item.value"
                :label="item.label"
              />
            </el-select>
            <el-input v-else v-model="meaning.name" class="explain-name-input" clearable />
            <el-button icon="Remove" @click="removeMeaning(index)" />
          </div>
          </div>
        </el-scrollbar>
      </div>
    </el-dialog>
  </teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

interface ClassifyQuestion {
  level: number
  dirName: string
}

interface PathMeaning {
  type: string
  id: string
  name: string
}

interface SelectOption {
  value: any
  label: string
}

interface SelectState {
  options: SelectOption[]
  pageNumber: number
  loading: boolean
  searchLoading: boolean
  hasMore: boolean
  currentQuery: string
}

const meaningTypes = [
  { value: 'localAuthor', label: '本地作者' },
  { value: 'siteAuthor', label: '站点作者' },
  { value: 'localTag', label: '本地标签' },
  { value: 'siteTag', label: '站点标签' },
  { value: 'workName', label: '作品名称' },
  { value: 'workSet', label: '作品集名称' },
  { value: 'site', label: '站点名称' },
  { value: 'unknown', label: '未知/无含义' }
]

const SELECT_TYPES = new Set(['localAuthor', 'localTag', 'site'])
const PAGE_SIZE = 10

// el-select 触底加载指令
const vElSelectBottomed = {
  mounted(el: any, binding: any) {
    const handleScroll = function (event: any) {
      const domTarget = event.target
      const scrollTop = domTarget.scrollTop
      const clientHeight = domTarget.clientHeight
      const scrollHeight = domTarget.scrollHeight
      if (scrollHeight <= clientHeight) return
      if (scrollHeight - scrollTop <= clientHeight + 0.5) {
        binding.value(el.querySelector('.el-select__input')?.value || '')
      }
    }
    const child = el.querySelector('.el-select__input')
    const id = child?.getAttribute('aria-controls')
    const popper = id ? document.getElementById(id) : null
    if (popper) {
      const selectWrapper = popper.parentElement
      if (selectWrapper) {
        selectWrapper.addEventListener('scroll', handleScroll)
        el.__ls_handleScroll = handleScroll
        el.__ls_scrollDom = selectWrapper
      }
    }
  },
  unmounted(el: any) {
    if (el.__ls_scrollDom) {
      el.__ls_scrollDom.removeEventListener('scroll', el.__ls_handleScroll)
    }
  }
}

const visible = ref(false)
const question = ref<ClassifyQuestion | null>(null)
const meanings = ref<PathMeaning[]>([])
const selectStates = ref<Record<number, SelectState>>({})

function isSelectType(type: string): boolean {
  return SELECT_TYPES.has(type)
}

function getState(index: number): SelectState {
  if (!selectStates.value[index]) {
    selectStates.value[index] = {
      options: [],
      pageNumber: 1,
      loading: false,
      searchLoading: false,
      hasMore: true,
      currentQuery: ''
    }
  }
  return selectStates.value[index]
}

function getLoadFn(type: string): ((page: any, input: string) => Promise<any>) | null {
  const apis = (window as any).__PLUGIN_CTX__?.custom?.apis
  if (!apis) return null
  switch (type) {
    case 'localAuthor': return apis.localAuthorApi.localAuthorQuerySelectItemPageByName
    case 'localTag': return apis.localTagApi.localTagQuerySelectItemPageByName
    case 'site': return apis.siteApi.siteQuerySelectItemPageBySiteName
    default: return null
  }
}

async function loadPage(index: number) {
  const meaning = meanings.value[index]
  if (!meaning) return
  const state = getState(index)
  const loadFn = getLoadFn(meaning.type)
  if (!loadFn || state.loading) return

  state.loading = true
  try {
    const result = await loadFn({ pageNumber: state.pageNumber, pageSize: PAGE_SIZE }, state.currentQuery)
    const items: any[] = result?.data || []
    if (items.length > 0) {
      const newOpts = items
        .filter((item: any) => item != null)
        .map((item: any) => ({ value: item.value, label: item.label }))
      state.options = [...state.options, ...newOpts]
      state.pageNumber++
    }
    state.hasMore = items.length >= PAGE_SIZE
  } finally {
    state.loading = false
  }
}

function search(index: number, query: string) {
  const state = getState(index)
  state.currentQuery = query
  state.pageNumber = 1
  state.options = []
  state.hasMore = true
  state.searchLoading = true
  loadPage(index).finally(() => { state.searchLoading = false })
}

function onDropdownOpen(index: number, visible: boolean) {
  if (!visible) return
  const state = getState(index)
  if (state.options.length === 0 && !state.loading) {
    state.pageNumber = 1
    state.currentQuery = ''
    loadPage(index)
  }
}

function onItemSelect(meaning: PathMeaning, index: number) {
  const state = getState(index)
  const opt = state.options.find((o: SelectOption) => String(o.value) === String(meaning.id))
  meaning.name = opt?.label || ''
}

function addMeaning() {
  meanings.value.push({ type: 'unknown', id: '', name: question.value?.dirName || '' })
}

function removeMeaning(index: number) {
  meanings.value.splice(index, 1)
  delete selectStates.value[index]
}

function onTypeChange(meaning: PathMeaning, index: number) {
  meaning.id = ''
  meaning.name = isSelectType(meaning.type) ? '' : (question.value?.dirName || '')
  delete selectStates.value[index]
}

function handleRequest(data: ClassifyQuestion) {
  question.value = data
  meanings.value = [{ type: 'unknown', id: '', name: data.dirName }]
  selectStates.value = {}
  visible.value = true
}

function confirm() {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    meanings: meanings.value.map(m => ({ ...m, id: m.id != null ? String(m.id) : '' }))
  })
  visible.value = false
  question.value = null
}

function cancel() {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    cancel: true
  })
  visible.value = false
  question.value = null
}

let offFn: (() => void) | null = null

onMounted(() => {
  console.log('[ClassifyPanel] 已挂载，开始监听 classify:request 事件')
  offFn = Events.On('plugin:local-import:classify:request', (event: any) => {
    console.log('[ClassifyPanel] 收到分类请求:', event.data)
    handleRequest(event.data)
  })
})

onUnmounted(() => {
  if (offFn) offFn()
})
</script>

<style scoped>
.explain-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.explain-actions {
  display: flex;
  gap: 8px;
}
.explain-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.explain-path-text {
  font-size: 14px;
  color: #606266;
  word-break: break-all;
}
.explain-scroll :deep(.el-scrollbar__wrap) {
  max-height: 300px;
}
.explain-scroll-inner {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.explain-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.explain-type-select {
  width: 140px;
  flex-shrink: 0;
}
.explain-name-input {
  flex: 1;
}
</style>
