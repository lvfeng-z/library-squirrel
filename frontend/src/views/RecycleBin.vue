<script setup lang="ts">
import BaseView from './BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import AutoLoadTagSelect from '@renderer/components/common/AutoLoadTagSelect.vue'
import { onMounted, Ref, ref, h } from 'vue'
import { ElMessage, ElMessageBox, ElPopover, ElImage } from 'element-plus'
import lodash from 'lodash'
import { recycleBinApi, siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import { searchQuerySearchConditionPage } from '@renderer/apis/http/wrappers/search'
import { setSearchTagColor } from '@renderer/utils/SearchTagColorUtil.js'
import { buildStoreUrl } from '@renderer/utils/UrlUtil.ts'
import { Thead } from '@renderer/model/util/Thead.ts'
import { newPage } from '@renderer/utils/Pager.ts'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { SearchCondition, SearchType, WorkSearchOperator, RecycleWorkDTO, RecycleStoreDTO, RecycleStorePageQuery, RecycleWorkSetDTO, RecycleWorkSetPageQuery } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { SearchConditionQuery } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { SelectItem } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { RecyclePageQuery } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import type { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil.ts'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import ApiUtil from '@renderer/utils/ApiUtil.ts'

// onMounted
onMounted(() => {
  recycleBinSearchTable.value.doSearch()
})

// 变量
// 当前 tab：works=作品条目（work 已删行聚合）/ stores=文件条目（store 已删行，work 不可达或存活）/
// workSets=作品集条目（work_set 已删行）
const activeTab = ref('works')
// 回收站数据表组件的实例（作品 tab）
const recycleBinSearchTable = ref()
// 文件条目表组件的实例（文件 tab；首次切入才查询）
const storeSearchTable = ref()
// 作品集条目表组件的实例（作品集 tab；首次切入才查询）
const workSetSearchTable = ref()
// 标签条 auto-load-tag-select 的实例（类型开关变化后重置候选）
const searchConditionBar = ref()
// 文件 tab 是否已加载过（lazy：首次切入触发一次查询）
let storeTabLoaded = false
// 作品集 tab 是否已加载过（lazy：首次切入触发一次查询）
let workSetTabLoaded = false
// 回收站分页参数（作品 tab）
const page: Ref<Page<RecycleWorkDTO>> = ref(newPage<RecycleWorkDTO>())
// 文件条目分页参数（文件 tab）
const storePage: Ref<Page<RecycleStoreDTO>> = ref(newPage<RecycleStoreDTO>())
// 作品集条目分页参数（作品集 tab）
const workSetPage: Ref<Page<RecycleWorkSetDTO>> = ref(newPage<RecycleWorkSetDTO>())
// 回收站查询参数（SearchCondition 条件体系 + 排序）
const query: Ref<RecyclePageQuery> = ref(new RecyclePageQuery({ conditions: [] }))
// 标签条已选条目（extraData 携带 type/id/namespace；disabled 态 = 排除该条件）
const selectedTagList: Ref<SegmentedTagItem[]> = ref([])
// 标签条自定义文本条目（作品名关键词标签）
const customTagList: Ref<SegmentedTagItem[]> = ref([])
// 标签条输入框文本（回车后进 customTagList）
const autoLoadInput: Ref<string | undefined> = ref(undefined)
// 候选条件的类型开关（空 = 全部四类，与作品搜索一致）
const searchConditionType: Ref<SearchType[]> = ref([])
// 时间范围控件值（Date 二元组，查询时转毫秒时间戳条件；null = 清空范围）
const deleteTimeRange = ref<[Date, Date] | null>(null)
const workCreateTimeRange = ref<[Date, Date] | null>(null)
const uploadTimeRange = ref<[Date, Date] | null>(null)
// 站点选择（SelectItem.value 为 string，组装条件时 Number 转换）
const siteIdSelected = ref<string | number | null>(null)

// —— 文件 tab 筛选（文件域条件体系，与作品 tab 的 SearchCondition 体系分轨） ——
// 文件名模糊
const storeFileName = ref('')
// 路径模糊
const storeFilePath = ref('')
// 所属作品名模糊（有主链；离链行天然不命中）
const storeWorkName = ref('')
// 媒体类型（null=不过滤；值与后端 dto.MediaType 枚举对齐：1 图片/2 视频/3 文档/4 音频）
const storeMediaType = ref<number | null>(null)
// 备份状态（'all'=不过滤；'backup'=有备份（backup_id>0）；'none'=无备份）
const storeBackupFilter = ref('all')
// 删除时间范围（Date 二元组；null = 不限）
const storeDeleteTimeRange = ref<[Date, Date] | null>(null)

// —— 作品集 tab 筛选（作品集域平铺条件体系，与作品 tab 的 SearchCondition 体系分轨） ——
// 名称模糊（站点名/昵称任一命中）
const workSetName = ref('')
// 站点选择（null=不限）
const workSetSiteIdSelected = ref<string | number | null>(null)
// 删除时间范围（Date 二元组；null = 不限）
const workSetDeleteTimeRange = ref<[Date, Date] | null>(null)

// 媒体类型筛选项（与后端 dto.MediaExtMapping 四类对齐）
const mediaTypeOptions = [
  { label: '图片', value: 1 },
  { label: '视频', value: 2 },
  { label: '文档', value: 3 },
  { label: '音频', value: 4 }
]
// 可预览的图片扩展名（含点小写；悬停预览判定用）
const PREVIEW_EXTS = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp']

// 回收站的表头（作品 tab）
const recycleBinThead: Ref<Thead<RecycleWorkDTO>[]> = ref([
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'workName',
    title: '作品名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    // 悬停弹出预览图（图片选择与作品卡片同优先级：缩略图优先、图片资源主图回退，由后端投影 previewPath 决定）；
    // show-after 与单元格溢出 tooltip 错峰，避免双气泡叠加
    render: (data, extraData) => {
      const row = extraData as RecycleWorkDTO
      const name = (data as string | null) ?? ''
      if (isNullish(row) || isBlank(row.previewPath)) {
        return h('span', name)
      }
      return h(
        ElPopover,
        { trigger: 'hover', placement: 'top', width: 'auto', showAfter: 400 },
        {
          reference: () => h('span', name),
          default: () =>
            h(ElImage, {
              src: buildStoreUrl(row.previewPath),
              fit: 'contain',
              style: 'max-width: 360px; max-height: 240px;'
            })
        }
      )
    }
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'siteName',
    title: '站点',
    hide: false,
    width: 120,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'authorNames',
    title: '作者',
    hide: false,
    width: 180,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createTime',
    title: '创建时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    sortable: 'custom',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'deleteTime',
    title: '删除时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    sortable: 'custom',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'expireDaysLeft',
    title: '剩余天数',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  })
])

// 文件条目的表头（文件 tab）
const storeThead: Ref<Thead<RecycleStoreDTO>[]> = ref([
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'fileName',
    title: '文件名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    // 图片行悬停弹出预览（软删期间文件在 backup/，/store/ 服务按行内 backup_id 兜底可访问）；
    // show-after 与单元格溢出 tooltip 错峰，避免双气泡叠加
    render: (data, extraData) => {
      const row = extraData as RecycleStoreDTO
      const name = (data as string | null) ?? ''
      const ext = row.filenameExtension?.toLowerCase() ?? ''
      if (isNullish(row) || isBlank(row.filePath) || !PREVIEW_EXTS.includes(ext)) {
        return h('span', name)
      }
      return h(
        ElPopover,
        { trigger: 'hover', placement: 'top', width: 'auto', showAfter: 400 },
        {
          reference: () => h('span', name),
          default: () =>
            h(ElImage, {
              src: buildStoreUrl(row.filePath),
              fit: 'contain',
              style: 'max-width: 360px; max-height: 240px;'
            })
        }
      )
    }
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'filePath',
    title: '路径',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'workName',
    title: '所属作品',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    // 离链行（挂载链断）无作品上下文，显示「—」
    render: (data, extraData) => {
      const row = extraData as RecycleStoreDTO
      const name = (data as string | null) ?? ''
      if (isNullish(row.workId)) {
        return h('span', '—')
      }
      return h('span', name)
    }
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'hasBackup',
    title: '状态',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    // 三态：可复原（有备份+挂载活作品，操作入口随替换/merge 软删化接通）＞已失效（无备份，文件已失）＞离链（挂载链断残迹）
    render: (data, extraData) => {
      const row = extraData as RecycleStoreDTO
      if (row.canRestore) {
        return h(StatusTag, { status: 'recycle-store-restorable' })
      }
      if (!row.hasBackup) {
        return h(StatusTag, { status: 'recycle-store-no-backup' })
      }
      return h(StatusTag, { status: 'recycle-store-orphan' })
    }
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'deleteTime',
    title: '删除时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'expireDaysLeft',
    title: '剩余天数',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    // 自动清理未启用时为 null，显示「—」
    render: (data) => h('span', isNullish(data) ? '—' : String(data))
  })
])

// 作品集条目的表头（作品集 tab）
const workSetThead: Ref<Thead<RecycleWorkSetDTO>[]> = ref([
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'name',
    title: '作品集名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    // 站点名优先，空则昵称，均空显示「—」（本地手建集可能无名）
    render: (data, extraData) => {
      const row = extraData as RecycleWorkSetDTO
      const name = (data as string | null) ?? ''
      if (isBlank(name) && isBlank(row.nickName)) {
        return h('span', '—')
      }
      return h('span', isNotBlank(name) ? name : row.nickName)
    }
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'siteName',
    title: '站点',
    hide: false,
    width: 120,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'aliveMemberCount',
    title: '活成员数',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    // 已删成员不计（成员关联保留，作品复原后自动回位）
    render: (data) => h('span', String(data ?? 0))
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createTime',
    title: '创建时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'deleteTime',
    title: '删除时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'expireDaysLeft',
    title: '剩余天数',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    // 自动清理未启用时为 null，显示「—」
    render: (data) => h('span', isNullish(data) ? '—' : String(data))
  })
])

// 方法
// tab 切换：首次切入文件/作品集 tab 触发一次查询（后续保留筛选状态，由用户手动查询/翻页）
function handleTabChange(tab: string | number) {
  if (tab === 'stores' && !storeTabLoaded) {
    storeTabLoaded = true
    storeSearchTable.value?.doSearch()
  }
  if (tab === 'workSets' && !workSetTabLoaded) {
    workSetTabLoaded = true
    workSetSearchTable.value?.doSearch()
  }
}
// 查询标签/作者候选列表（类型开关经 types 下发，空 = 全部）
async function querySearchItemPage(p: IPage<SelectItem>, input?: string): Promise<IPage<SelectItem>> {
  const conditionQuery = new SearchConditionQuery({ keyword: isBlank(input) ? undefined : input })
  conditionQuery.types = lodash.cloneDeep(searchConditionType.value)
  const response = await searchQuerySearchConditionPage(newPage<SelectItem>({ pageNumber: p.pageNumber, pageSize: p.pageSize }), conditionQuery)
  const data = ApiUtil.data<Page<SelectItem>>(response)
  if (isNullish(data)) {
    ApiUtil.msg(response)
    throw new Error(response?.msg ?? '查询搜索条件失败')
  }
  return data
}
// 类型开关变化后重新加载候选
function handleConditionTypeChange() {
  searchConditionBar.value.newSearch()
}
// namespace 由已选 tag 的 ns 段编辑写入 extraData.namespace（local=用户点 ns 段设搜索 ns、site=站点自带固定 ns）；author 不带
// 空串视作未设：DB 空串落 NULL，无命中
function resolveSearchNamespace(extraData: { type: SearchType; namespace?: string }): string | undefined {
  if (extraData.type === SearchType.LocalTag || extraData.type === SearchType.SiteTag) {
    return extraData.namespace || undefined
  }
  return undefined
}
// 组装查询条件（与作品搜索同构：多标签组合 + 排除 + namespace + 关键词 + 站点 + 三组时间范围）
function buildConditions(): SearchCondition[] {
  const conditions: SearchCondition[] = []
  // 标签/作者多选条（disabled 态转 NotEqual 排除）
  selectedTagList.value.forEach((tag) => {
    if (isNullish(tag.extraData)) {
      return
    }
    const extraData = tag.extraData as { type: SearchType; id: number; namespace?: string }
    const operator = notNullish(tag.disabled) && tag.disabled ? WorkSearchOperator.NotEqual : undefined
    conditions.push(new SearchCondition({ type: extraData.type, value: extraData.id, operator: operator, namespace: resolveSearchNamespace(extraData) }))
  })
  // 自定义文本标签 → 作品名 LIKE
  customTagList.value.forEach((tag) => {
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: tag.value, operator: WorkSearchOperator.Like }))
  })
  // 输入框关键词 → 作品名 LIKE（不并推昵称条件：双 AND 会滤掉无昵称作品）
  if (isNotBlank(autoLoadInput.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksSiteName, value: autoLoadInput.value, operator: WorkSearchOperator.Like }))
  }
  // 站点
  if (!isNullish(siteIdSelected.value)) {
    conditions.push(new SearchCondition({ type: SearchType.Site, value: Number(siteIdSelected.value) }))
  }
  // 三组时间范围（起 gte / 止 lte 成对）
  if (!isNullish(deleteTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksDeleteTime, value: deleteTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksDeleteTime, value: deleteTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  if (!isNullish(workCreateTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksCreateTime, value: workCreateTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksCreateTime, value: workCreateTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  if (!isNullish(uploadTimeRange.value)) {
    conditions.push(new SearchCondition({ type: SearchType.WorksUploadTime, value: uploadTimeRange.value[0].getTime(), operator: WorkSearchOperator.GreaterOrEqual }))
    conditions.push(new SearchCondition({ type: SearchType.WorksUploadTime, value: uploadTimeRange.value[1].getTime(), operator: WorkSearchOperator.LessOrEqual }))
  }
  return conditions
}
// 分页查询回收站作品列表（作品 tab）
async function recycleBinQueryPageFn(p: Page<RecycleWorkDTO>): Promise<Page<RecycleWorkDTO> | undefined> {
  query.value.conditions = buildConditions()
  const response = await recycleBinApi.recycleBinPageWorks(p, query.value)
  return response.data
}
// 组装文件条目查询（文件域条件体系；改动后需手动点查询刷新）
function buildStoreQuery(): RecycleStorePageQuery {
  const backup = storeBackupFilter.value === 'all' ? null : storeBackupFilter.value === 'backup'
  return new RecycleStorePageQuery({
    fileName: storeFileName.value,
    filePath: storeFilePath.value,
    workName: storeWorkName.value,
    mediaType: storeMediaType.value,
    hasBackup: backup,
    deleteTimeFrom: isNullish(storeDeleteTimeRange.value) ? 0 : storeDeleteTimeRange.value[0].getTime(),
    deleteTimeTo: isNullish(storeDeleteTimeRange.value) ? 0 : storeDeleteTimeRange.value[1].getTime()
  })
}
// 分页查询回收站文件条目（文件 tab）
async function storeQueryPageFn(p: Page<RecycleStoreDTO>): Promise<Page<RecycleStoreDTO> | undefined> {
  const response = await recycleBinApi.recycleBinPageStores(p, buildStoreQuery())
  return response.data
}

// 组装作品集条目查询（作品集域平铺条件；改动后需手动点查询刷新）
function buildWorkSetQuery(): RecycleWorkSetPageQuery {
  return new RecycleWorkSetPageQuery({
    name: workSetName.value,
    siteId: isNullish(workSetSiteIdSelected.value) ? null : Number(workSetSiteIdSelected.value),
    deleteTimeFrom: isNullish(workSetDeleteTimeRange.value) ? 0 : workSetDeleteTimeRange.value[0].getTime(),
    deleteTimeTo: isNullish(workSetDeleteTimeRange.value) ? 0 : workSetDeleteTimeRange.value[1].getTime()
  })
}

// 分页查询回收站作品集条目（作品集 tab）
async function workSetQueryPageFn(p: Page<RecycleWorkSetDTO>): Promise<Page<RecycleWorkSetDTO> | undefined> {
  const response = await recycleBinApi.recycleBinPageWorkSets(p, buildWorkSetQuery())
  return response.data
}
// 处理列头排序变化（后端排序；取消排序时回落默认 deleteTime DESC）
function handleSortChange(sortData: { column: unknown; prop: string; order: 'ascending' | 'descending' | null }) {
  if (isNullish(sortData.order)) {
    query.value.sortBy = ''
    query.value.sortOrder = ''
  } else {
    query.value.sortBy = sortData.prop === 'createTime' ? 'createTime' : 'deleteTime'
    query.value.sortOrder = sortData.order === 'ascending' ? 'asc' : 'desc'
  }
  recycleBinSearchTable.value.doSearch()
}
// 复原回收站条目（失败消息含「冲突/已存在」时弹覆盖确认，确认后 overwrite=true 重试）
async function restore(item: RecycleWorkDTO) {
  try {
    await recycleBinApi.recycleBinRestoreWork(item.id, false)
    ElMessage.success('复原成功')
    await recycleBinSearchTable.value.doSearch()
  } catch (e) {
    const failMsg = (e as Error).message ?? '复原失败'
    if (!failMsg.includes('冲突') && !failMsg.includes('已存在')) {
      ElMessage.error(failMsg)
      return
    }
    try {
      await ElMessageBox.confirm(`${failMsg}\n是否覆盖已存在的作品？`, '复原冲突', {
        confirmButtonText: '覆盖',
        cancelButtonText: '取消',
        type: 'warning'
      })
      await recycleBinApi.recycleBinRestoreWork(item.id, true)
      ElMessage.success('复原成功')
      await recycleBinSearchTable.value.doSearch()
    } catch (e2) {
      // ElMessageBox 取消为字符串 reject，静默；二次失败为 Error，展示
      if (e2 instanceof Error) {
        ElMessage.error(e2.message)
      }
    }
  }
}
// 彻底删除回收站作品条目
async function purge(item: RecycleWorkDTO) {
  try {
    await ElMessageBox.confirm('彻底删除后不可恢复，是否继续？', '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await recycleBinApi.recycleBinPurgeWork(item.id)
    ElMessage.success('已彻底删除')
    await recycleBinSearchTable.value.doSearch()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，文件删除失败询问仅删记录，其余直接展示
    if (e instanceof Error) {
      await handlePurgeFileFailure(
        e,
        () => recycleBinApi.recycleBinPurgeWorkRecords(item.id),
        () => recycleBinSearchTable.value.doSearch()
      )
    }
  }
}
// 彻底删除回收站文件条目（条目单位=store 行，连同其引用的备份一并清理）
async function purgeStore(item: RecycleStoreDTO) {
  try {
    await ElMessageBox.confirm(`彻底删除文件条目后不可恢复（含其引用的备份），是否继续？\n${item.filePath}`, '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await recycleBinApi.recycleBinPurgeStore(item.id)
    ElMessage.success('已彻底删除')
    await storeSearchTable.value.doSearch()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，文件删除失败询问仅删记录，其余直接展示
    if (e instanceof Error) {
      await handlePurgeFileFailure(
        e,
        () => recycleBinApi.recycleBinPurgeStoreRecords(item.id),
        () => storeSearchTable.value.doSearch()
      )
    }
  }
}
// 彻底删除时文件删除失败（被占用/只读等，后端已保留记录）：询问「仅删除记录」还是放弃；
// 其余错误直接展示。仅删记录走独立降级入口（不动磁盘文件，留用户手动处理）
async function handlePurgeFileFailure(e: Error, recordOnlyFn: () => Promise<unknown>, refreshFn: () => Promise<void>) {
  if (!e.message.includes('文件删除失败')) {
    ElMessage.error(e.message)
    return
  }
  try {
    await ElMessageBox.confirm(`${e.message}。是否仅删除记录（磁盘文件保留，请手动处理）？`, '彻底删除', {
      confirmButtonText: '仅删除记录',
      cancelButtonText: '放弃',
      type: 'warning'
    })
  } catch {
    return // 用户放弃：记录保留
  }
  try {
    await recordOnlyFn()
    ElMessage.success('已仅删除记录')
    await refreshFn()
  } catch (err) {
    ElMessage.error((err as Error).message ?? '仅删除记录失败')
  }
}
// 复原按钮禁用原因（不可复原条目的 tooltip 说明）
function storeRestoreDisabledReason(item: RecycleStoreDTO): string {
  if (!item.hasBackup) {
    return '该条目无备份（已失效或备份缺失），不可复原'
  }
  return '该条目所属作品不可达（已删除或挂载缺失），不可复原'
}
// 复原文件条目（版本回滚置换：备份还原为当前版本，当前版本转入回收站）
async function restoreStore(item: RecycleStoreDTO) {
  try {
    await ElMessageBox.confirm(
      `将把该版本还原为当前版本，当前生效版本会转入回收站（可再复原），是否继续？\n${item.filePath}`,
      '复原文件',
      {
        confirmButtonText: '复原',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    await recycleBinApi.recycleBinRestoreStore(item.id)
    ElMessage.success('复原成功')
    await storeSearchTable.value.doSearch()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，展示
    if (e instanceof Error) {
      ElMessage.error(e.message)
    }
  }
}

// 复原作品集条目（失败消息含「冲突/已存在」时弹覆盖确认，确认后 overwrite=true 重试——照作品 tab 形态）
async function restoreWorkSet(item: RecycleWorkSetDTO) {
  try {
    await recycleBinApi.recycleBinRestoreWorkSet(item.id, false)
    ElMessage.success('复原成功')
    await workSetSearchTable.value.doSearch()
  } catch (e) {
    const failMsg = (e as Error).message ?? '复原失败'
    if (!failMsg.includes('冲突') && !failMsg.includes('已存在')) {
      ElMessage.error(failMsg)
      return
    }
    try {
      await ElMessageBox.confirm(`${failMsg}\n是否覆盖已存在的作品集？`, '复原冲突', {
        confirmButtonText: '覆盖',
        cancelButtonText: '取消',
        type: 'warning'
      })
      await recycleBinApi.recycleBinRestoreWorkSet(item.id, true)
      ElMessage.success('复原成功')
      await workSetSearchTable.value.doSearch()
    } catch (e2) {
      // ElMessageBox 取消为字符串 reject，静默；二次失败为 Error，展示
      if (e2 instanceof Error) {
        ElMessage.error(e2.message)
      }
    }
  }
}

// 彻底删除回收站作品集条目（级联清成员与父子关联行）
async function purgeWorkSet(item: RecycleWorkSetDTO) {
  try {
    await ElMessageBox.confirm('彻底删除后不可恢复（作品集及其成员、层级关联一并清除），是否继续？', '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await recycleBinApi.recycleBinPurgeWorkSet(item.id)
    ElMessage.success('已彻底删除')
    await workSetSearchTable.value.doSearch()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，展示
    if (e instanceof Error) {
      ElMessage.error(e.message)
    }
  }
}
</script>

<template>
  <base-view>
    <template #default>
      <div class="recycle-bin-container">
        <el-tabs v-model="activeTab" class="recycle-bin-tabs" @tab-change="handleTabChange">
          <!-- 作品 tab：work 已删行聚合（SearchCondition 条件体系） -->
          <el-tab-pane label="作品" name="works">
            <search-table
              ref="recycleBinSearchTable"
              v-model:page="page"
              class="recycle-bin-search-table"
              toolbar-radius="var(--app-radius)"
              data-radius="var(--app-radius)"
              data-key="id"
              :thead="recycleBinThead"
              :search="recycleBinQueryPageFn"
              :selectable="false"
              :multi-select="false"
              :custom-operation-button="true"
              :operation-width="200"
              @sort-change="handleSortChange"
            >
              <template #toolbarMain>
                <div class="recycle-bin-tag-select">
                  <auto-load-tag-select
                    ref="searchConditionBar"
                    v-model:selected-data="selectedTagList"
                    v-model:custom-data="customTagList"
                    v-model:input="autoLoadInput"
                    :load="querySearchItemPage"
                    :page-size="40"
                    :color-resolver="setSearchTagColor"
                    tags-gap="10px"
                    max-height="300px"
                    min-height="33px"
                  >
                    <template #left>
                      <el-checkbox-group
                        v-model="searchConditionType"
                        class="recycle-bin-tag-type-checkbox-group"
                        @change="handleConditionTypeChange"
                      >
                        <el-checkbox :value="SearchType.LocalTag">本地标签</el-checkbox>
                        <el-checkbox :value="SearchType.SiteTag">站点标签</el-checkbox>
                        <el-checkbox :value="SearchType.LocalAuthor">本地作者</el-checkbox>
                        <el-checkbox :value="SearchType.SiteAuthor">站点作者</el-checkbox>
                      </el-checkbox-group>
                    </template>
                  </auto-load-tag-select>
                </div>
              </template>
              <!-- 次要筛选收进搜索按钮「更多选项」折叠面板，工具栏只保留标签条 -->
              <template #toolbarDropdown>
                <div class="recycle-bin-advanced-filter">
                  <auto-load-select
                    v-model:data="siteIdSelected"
                    class="recycle-bin-filter-select"
                    :load="siteQuerySelectItemPageBySiteName"
                    placeholder="选择站点"
                    remote
                    filterable
                    clearable
                  >
                    <template #default="{ list }">
                      <el-option
                        v-for="item in list"
                        :key="item.value"
                        :value="item.value"
                        :label="item.label"
                      />
                    </template>
                  </auto-load-select>
                  <el-date-picker
                    v-model="deleteTimeRange"
                    class="recycle-bin-filter-range"
                    type="datetimerange"
                    start-placeholder="删除时间起"
                    end-placeholder="删除时间止"
                  />
                  <el-date-picker
                    v-model="workCreateTimeRange"
                    class="recycle-bin-filter-range"
                    type="datetimerange"
                    start-placeholder="创建时间起"
                    end-placeholder="创建时间止"
                  />
                  <el-date-picker
                    v-model="uploadTimeRange"
                    class="recycle-bin-filter-range"
                    type="datetimerange"
                    start-placeholder="上传时间起"
                    end-placeholder="上传时间止"
                  />
                </div>
              </template>
              <!-- 破坏性按钮需保真 danger+tone-fail 红色形态，内置 operationButton 下拉项无 danger 样式，故走自定义操作列 -->
              <template #customOperations="{ row }">
                <el-button size="small" type="primary" @click="restore(row as RecycleWorkDTO)">复原</el-button>
                <el-button size="small" type="danger" class="tone-fail" @click="purge(row as RecycleWorkDTO)">彻底删除</el-button>
              </template>
            </search-table>
          </el-tab-pane>
          <!-- 文件 tab：persistent_store 已删行（文件域条件体系，与作品 tab 分轨） -->
          <el-tab-pane label="文件" name="stores">
            <search-table
              ref="storeSearchTable"
              v-model:page="storePage"
              class="recycle-bin-search-table"
              toolbar-radius="var(--app-radius)"
              data-radius="var(--app-radius)"
              data-key="id"
              :thead="storeThead"
              :search="storeQueryPageFn"
              :selectable="false"
              :multi-select="false"
              :custom-operation-button="true"
              :operation-width="160"
            >
              <template #toolbarMain>
                <div class="recycle-store-filter">
                  <el-input
                    v-model="storeFileName"
                    class="recycle-store-filter-input"
                    placeholder="文件名"
                    clearable
                  />
                  <el-input
                    v-model="storeFilePath"
                    class="recycle-store-filter-input"
                    placeholder="路径"
                    clearable
                  />
                  <el-input
                    v-model="storeWorkName"
                    class="recycle-store-filter-input"
                    placeholder="所属作品名"
                    clearable
                  />
                  <el-select
                    v-model="storeMediaType"
                    class="recycle-store-filter-select"
                    placeholder="类型"
                    clearable
                  >
                    <el-option
                      v-for="opt in mediaTypeOptions"
                      :key="opt.value"
                      :value="opt.value"
                      :label="opt.label"
                    />
                  </el-select>
                  <el-select
                    v-model="storeBackupFilter"
                    class="recycle-store-filter-select"
                    placeholder="备份状态"
                  >
                    <el-option value="all" label="全部" />
                    <el-option value="backup" label="有备份" />
                    <el-option value="none" label="无备份" />
                  </el-select>
                  <el-button type="primary" @click="storeSearchTable?.doSearch()">查询</el-button>
                </div>
              </template>
              <template #toolbarDropdown>
                <div class="recycle-bin-advanced-filter">
                  <el-date-picker
                    v-model="storeDeleteTimeRange"
                    class="recycle-bin-filter-range"
                    type="datetimerange"
                    start-placeholder="删除时间起"
                    end-placeholder="删除时间止"
                  />
                </div>
              </template>
              <template #customOperations="{ row }">
                <el-tooltip
                  :disabled="(row as RecycleStoreDTO).canRestore"
                  :content="storeRestoreDisabledReason(row as RecycleStoreDTO)"
                  placement="top"
                >
                  <span class="recycle-store-restore-wrap">
                    <el-button
                      size="small"
                      :disabled="!(row as RecycleStoreDTO).canRestore"
                      @click="restoreStore(row as RecycleStoreDTO)"
                    >复原</el-button>
                  </span>
                </el-tooltip>
                <el-button size="small" type="danger" class="tone-fail" @click="purgeStore(row as RecycleStoreDTO)">彻底删除</el-button>
              </template>
            </search-table>
          </el-tab-pane>
          <!-- 作品集 tab：work_set 已删行（作品集域平铺条件体系） -->
          <el-tab-pane label="作品集" name="workSets">
            <search-table
              ref="workSetSearchTable"
              v-model:page="workSetPage"
              class="recycle-bin-search-table"
              toolbar-radius="var(--app-radius)"
              data-radius="var(--app-radius)"
              data-key="id"
              :thead="workSetThead"
              :search="workSetQueryPageFn"
              :selectable="false"
              :multi-select="false"
              :custom-operation-button="true"
              :operation-width="200"
            >
              <template #toolbarMain>
                <div class="recycle-store-filter">
                  <el-input
                    v-model="workSetName"
                    class="recycle-store-filter-input"
                    placeholder="作品集名"
                    clearable
                  />
                  <auto-load-select
                    v-model:data="workSetSiteIdSelected"
                    class="recycle-bin-filter-select"
                    :load="siteQuerySelectItemPageBySiteName"
                    placeholder="选择站点"
                    remote
                    filterable
                    clearable
                  >
                    <template #default="{ list }">
                      <el-option
                        v-for="item in list"
                        :key="item.value"
                        :value="item.value"
                        :label="item.label"
                      />
                    </template>
                  </auto-load-select>
                  <el-button type="primary" @click="workSetSearchTable?.doSearch()">查询</el-button>
                </div>
              </template>
              <template #toolbarDropdown>
                <div class="recycle-bin-advanced-filter">
                  <el-date-picker
                    v-model="workSetDeleteTimeRange"
                    class="recycle-bin-filter-range"
                    type="datetimerange"
                    start-placeholder="删除时间起"
                    end-placeholder="删除时间止"
                  />
                </div>
              </template>
              <template #customOperations="{ row }">
                <el-button size="small" type="primary" @click="restoreWorkSet(row as RecycleWorkSetDTO)">复原</el-button>
                <el-button size="small" type="danger" class="tone-fail" @click="purgeWorkSet(row as RecycleWorkSetDTO)">彻底删除</el-button>
              </template>
            </search-table>
          </el-tab-pane>
        </el-tabs>
      </div>
    </template>
  </base-view>
</template>

<style scoped>
.recycle-bin-container {
  display: flex;
  flex-direction: row;
  justify-content: center;
  align-items: center;
  /* 容器不带底色：一体感由 SearchTable 自身的工具栏面与数据面（含分页面）连成的卡片承担；间距纯 margin（总边距 10px 不变） */
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}

/* 页内双 tab（作品/文件）：tab 头占固有高度，内容区撑满剩余高度 */
.recycle-bin-tabs {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.recycle-bin-tabs > :deep(.el-tabs__header) {
  margin-bottom: 8px;
  flex-shrink: 0;
}

.recycle-bin-tabs > :deep(.el-tabs__content) {
  flex: 1;
  min-height: 0;
}

.recycle-bin-tabs > :deep(.el-tabs__content) > .el-tab-pane {
  height: 100%;
}

.recycle-bin-search-table {
  height: 100%;
  width: 100%;
}

/* 占满工具栏内容列整行；组件内部 wrapper 经 :deep 撑满包裹 div，
   气泡宽度（el-popover :width 由 resizeObserver 实测 wrapper）随之满宽 */
.recycle-bin-tag-select {
  width: 100%;
}
.recycle-bin-tag-select :deep(.auto-load-tag-select-main) {
  width: 100%;
}

.recycle-bin-tag-type-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* 「更多选项」折叠面板内的次要筛选区：自由折行，超出走面板滚动条 */
.recycle-bin-advanced-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px;
}

.recycle-bin-filter-select {
  width: 200px;
  flex-shrink: 0;
}

.recycle-bin-filter-range {
  width: 350px;
  flex-shrink: 0;
}

/* 文件 tab 工具栏筛选区：自由折行 */
.recycle-store-filter {  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.recycle-store-filter-input {
  width: 180px;
  flex-shrink: 0;
}

.recycle-store-filter-select {
  width: 120px;
  flex-shrink: 0;
}

/* 复原按钮外包裹：禁用态按钮不触发鼠标事件，tooltip 须挂在外层元素上 */
.recycle-store-restore-wrap {
  display: inline-flex;
  margin-right: 8px;
}
</style>
