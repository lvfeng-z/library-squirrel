<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, Ref, ref, watch } from 'vue'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TagBox from '../common/TagBox.vue'
import { LocalTagDTO, WorkSetDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import {
  SelectItem, WorkFullDTO, SiteTagFullDTO, ResourceFullDTO, RankedLocalAuthor, RankedSiteAuthor, SiteAuthorDTO
} from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import ApiUtil from '@renderer/utils/ApiUtil'
import ExchangeBox from '@renderer/components/common/ExchangeBox.vue'
import type { ReWorkTag } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { OriginType } from '@renderer/constants/OriginType.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import AuthorTag from '@renderer/components/common/AuthorTag.vue'
import AuthorRoleTag from '@renderer/components/common/AuthorRoleTag.vue'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { LocalTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/localTag/models'
import { SiteTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/siteTag/models'
import { LocalAuthorQueryDTO } from '@bindings/github.com/library-squirrel/backend/localAuthor/models'
import { SiteAuthorQueryDTO } from '@bindings/github.com/library-squirrel/backend/siteAuthor/models'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { localTagApi, siteTagApi, workApi, workSetApi, localAuthorApi, siteAuthorApi } from '@renderer/apis/http'
import { reWorkTagApi, reWorkAuthorApi } from '@renderer/apis/http'
import { resourceMerge, resourceMergeCancel } from '@renderer/apis/http/wrappers/resource'
import { getMergeState, markMergeStarted, clearMergeState } from '@renderer/composables/useMergeProgress'
import { useWorkLockConfirm } from '@renderer/composables/useWorkLockConfirm'
import { isResourceMergeable } from '@renderer/utils/ResourceUtil.ts'
import ResourceViewer from '@renderer/components/resource/ResourceViewer.vue'

// 作品详情弹窗：主体（ResourceViewer）+ 右侧功能栏 + 元数据 drawer + 标签编辑 drawer
const props = defineProps<{
  work: WorkFullDTO[]
  width?: string
}>()

// 弹窗开关
const state = defineModel<boolean>('state', { required: true })
const currentWorkIndex = defineModel<number>('currentWorkIndex', { required: true })

const emits = defineEmits(['openWorkSet'])

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})

// 接口
const apis = {
  localTagListByWorkId: localTagApi.localTagListByWorkId,
  localTagQuerySelectItemPageByWorkId: localTagApi.localTagQuerySelectItemPageByWorkId,
  siteTagQueryPageByWorkId: siteTagApi.siteTagQueryPageByWorkId,
  siteTagQuerySelectItemPageByWorkId: siteTagApi.siteTagQuerySelectItemPageByWorkId,
  reWorkTagLink: reWorkTagApi.reWorkTagLink,
  reWorkTagUnlink: reWorkTagApi.reWorkTagUnlink,
  reWorkTagUnlinkDimension: reWorkTagApi.reWorkTagUnlinkDimension,
  reWorkTagListByWorkId: reWorkTagApi.reWorkTagListByWorkId,
  workSoftDelete: workApi.workSoftDelete,
  workGetFullWorkInfoById: workApi.workGetFullWorkInfoById,
  workSetListByWorkId: workSetApi.workSetListByWorkId,
  localAuthorQuerySelectItemPage: localAuthorApi.localAuthorQuerySelectItemPage,
  siteAuthorQuerySelectItemPage: siteAuthorApi.siteAuthorQuerySelectItemPage,
  reWorkAuthorLink: reWorkAuthorApi.reWorkAuthorLink,
  reWorkAuthorUnlink: reWorkAuthorApi.reWorkAuthorUnlink,
  reWorkAuthorUnlinkDimension: reWorkAuthorApi.reWorkAuthorUnlinkDimension,
  reWorkAuthorListLocalAuthorsByWorkId: reWorkAuthorApi.reWorkAuthorListLocalAuthorsByWorkId,
  reWorkAuthorListSiteAuthorsByWorkId: reWorkAuthorApi.reWorkAuthorListSiteAuthorsByWorkId
}
// 作品分享拉取锁交互（删除命中锁时弹强制解锁确认）
const { isWorkLockedResponse, confirmWorkForceUnlock } = useWorkLockConfirm()
// ExchangeBox 组件实例
const localTagExchangeBox = ref()
const siteTagExchangeBox = ref()
// 作者编辑 ExchangeBox 组件实例（localAuthor/siteAuthor 各一）
const localAuthorExchangeBox = ref()
const siteAuthorExchangeBox = ref()
// 作品信息
const currentWorkFullInfo: Ref<WorkFullDTO> = computed(() => {
  const raw = props.work[currentWorkIndex.value]
  if (!raw) return new WorkFullDTO()
  if (!raw.work) raw.work = { id: 0, createTime: 0, updateTime: 0 } as any
  return raw
})
// 本地标签
const localTags: Ref<SegmentedTagItem[]> = ref([])
// 站点标签
const siteTags: Ref<SegmentedTagItem[]> = ref([])
// 作品已绑定 tag 的 namespace 映射（localTagId→ns 列表、siteTagId→ns 列表——同标签多 ns 关联
// 各占一行，值为全量 ns），供已绑定候选区/详情区展示 ns 子段与编辑确认时 diff 旧值
const workTagNs = ref<{ local: Map<number, string[]>; site: Map<number, string[]> }>({ local: new Map(), site: new Map() })
// 刷新作品已绑定 tag 的 namespace 映射（local/site 各一）；失败不阻断（仅无 ns 展示）
async function refreshWorkTagNs() {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) {
    workTagNs.value = { local: new Map(), site: new Map() }
    return
  }
  try {
    const response = await apis.reWorkTagListByWorkId(workId)
    const rels = ApiUtil.data<(ReWorkTag | null)[]>(response) ?? []
    const local = new Map<number, string[]>()
    const site = new Map<number, string[]>()
    for (const rel of rels) {
      // 空串=无 ns，不产生 ns 段
      if (!rel?.namespace) continue
      if (rel.tagType?.Int64 === OriginType.LOCAL && rel.localTagId?.Valid) {
        const list = local.get(rel.localTagId.Int64) ?? []
        list.push(rel.namespace)
        local.set(rel.localTagId.Int64, list)
      } else if (rel.tagType?.Int64 === OriginType.SITE && rel.siteTagId?.Valid) {
        const list = site.get(rel.siteTagId.Int64) ?? []
        list.push(rel.namespace)
        site.set(rel.siteTagId.Int64, list)
      }
    }
    workTagNs.value = { local, site }
  } catch {
    workTagNs.value = { local: new Map(), site: new Map() }
  }
}
// 作品集
const workSets: Ref<SegmentedTagItem[]> = ref([])
// 展示作者列表：本地作者全部 + 未绑定本地作者的站点作者（已绑定本地作者由本地作者代表展示）；
// 同作者多 role 的多条关联行按作者聚合为一个条目（AuthorTag 并列展示全部 role 段）；
// origin 标记来源供模板 key 区分（local/site 两表 id 空间重叠）
const displayAuthors = computed<{ author: RankedLocalAuthor | RankedSiteAuthor; origin: OriginType; roles: string[] }[]>(() => {
  const localAuthors = currentWorkFullInfo.value.localAuthors?.filter(notNullish) ?? []
  const siteAuthors = currentWorkFullInfo.value.siteAuthors?.filter(notNullish) ?? []
  const localAggregated = aggregateAuthorRoles(localAuthors)
  const localAuthorIds = new Set(localAggregated.map((entry) => entry.id))
  const siteAggregated = aggregateAuthorRoles(
    siteAuthors.filter((siteAuthor) => !localAuthorIds.has(siteAuthor.author.localAuthorId ?? Number.NaN))
  )
  return [
    ...localAggregated.map((entry) => ({ author: entry.author, roles: entry.roles, origin: OriginType.LOCAL })),
    ...siteAggregated.map((entry) => ({ author: entry.author, roles: entry.roles, origin: OriginType.SITE }))
  ]
})
// 同作者多 role 的关联行按作者聚合：author 取首行（同作者行仅 roleName/sortOrder 不同），roles 去重收集；
// 返回条目带非空 id（模板 key 与绑定索引的键）
function aggregateAuthorRoles<T extends RankedLocalAuthor | RankedSiteAuthor>(authors: T[]): { id: number; author: T; roles: string[] }[] {
  const byId = new Map<number, { id: number; author: T; roles: string[] }>()
  for (const entry of authors) {
    const id = entry.author.id
    if (isNullish(id)) continue
    const aggregated = byId.get(id) ?? { id, author: entry, roles: [] }
    if (entry.roleName && !aggregated.roles.includes(entry.roleName)) {
      aggregated.roles.push(entry.roleName)
    }
    byId.set(id, aggregated)
  }
  return [...byId.values()]
}
// 元数据抽屉开关
const drawerState: Ref<boolean> = ref(false)
// 标签编辑模式（编辑 drawer 内本地/站点 ExchangeBox 切换）
const localTagEdit: Ref<boolean> = ref(false)
const siteTagEdit: Ref<boolean> = ref(false)
// 标签编辑抽屉（独立于元数据抽屉：编辑时关元数据，关闭回元数据，避免 ExchangeBox 内嵌 descriptions 拥挤）
const editDrawerState: Ref<boolean> = ref(false)
// 本地标签查询参数
const localTagExchangeUpperSearchParams: Ref<LocalTagQueryDTO> = ref(new LocalTagQueryDTO())
const localTagExchangeLowerSearchParams: Ref<LocalTagQueryDTO> = ref(new LocalTagQueryDTO())
// 站点标签查询参数
const siteTagExchangeUpperSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
const siteTagExchangeLowerSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 作者编辑模式（作者编辑 drawer 内本地/站点 ExchangeBox 切换）
const localAuthorEdit: Ref<boolean> = ref(false)
const siteAuthorEdit: Ref<boolean> = ref(false)
// 作者编辑抽屉（独立于标签编辑抽屉：编辑时关元数据抽屉，关闭回元数据抽屉）
const authorDrawerState: Ref<boolean> = ref(false)
// 本地作者查询参数（upper=已绑定区前端过滤词，lower=候选区后端分页条件）
const localAuthorExchangeUpperSearchParams: Ref<LocalAuthorQueryDTO> = ref(new LocalAuthorQueryDTO())
const localAuthorExchangeLowerSearchParams: Ref<LocalAuthorQueryDTO> = ref(new LocalAuthorQueryDTO())
// 站点作者查询参数（upper=已绑定区前端过滤词，lower=候选区后端分页条件）
const siteAuthorExchangeUpperSearchParams: Ref<SiteAuthorQueryDTO> = ref(new SiteAuthorQueryDTO())
const siteAuthorExchangeLowerSearchParams: Ref<SiteAuthorQueryDTO> = ref(new SiteAuthorQueryDTO())
// 作品已绑定作者 id 集合（localAuthorId/siteAuthorId 各一），供候选区分页过滤已绑定项
const boundAuthorIds: Ref<{ local: Set<number>; site: Set<number> }> = ref({ local: new Set(), site: new Set() })
// 作品已绑定作者 role 索引（localAuthorId/siteAuthorId → 该作者全部关联级 role——同作者多 role
// 各占一行），供绑定确认时 diff 出被替换的旧 role 值
const workAuthorRoles: Ref<{ local: Map<number, string[]>; site: Map<number, string[]> }> = ref({ local: new Map(), site: new Map() })

// 当前资源是否可合并（含视频轨+音频轨）
const mergeable: Ref<boolean> = computed(() => isResourceMergeable(currentWorkFullInfo.value.resource))
// 当前资源 id（合并状态索引）
const currentResourceId = computed(() => currentWorkFullInfo.value.resource?.id)
// 当前资源的合并状态（由 useMergeProgress 单例 Map 驱动，切换作品时自动跟随）
const mergeState = computed(() => getMergeState(currentResourceId.value))
const isMerging = computed(() => mergeState.value?.status === 'running')
// 合并按钮 title（鼠标悬停提示）：进行中显示百分比，否则"合并音视频轨"
const mergeButtonTitle = computed(() => {
  if (!isMerging.value) return '合并音视频轨'
  const percent = mergeState.value?.percent ?? -1
  return percent >= 0 ? `合并中 ${percent}%` : '合并中…'
})
// 侧栏可见的合并百分比标签（合并中显示；不定态显示 …）—— 工具栏为图标按钮，进度仅藏在 tooltip 不易发现，故补可见标签
const mergePercentLabel = computed(() => {
  const percent = mergeState.value?.percent ?? -1
  return percent >= 0 ? `${percent}%` : '…'
})
// 本视图发起的合并 resourceId：把 complete 收尾限定在本视图发起的合并上，避免切换作品时的误触发
const initiatedMergeId = ref<number | null>(null)

// 监听合并状态：仅处理"本视图发起 + 当前在看 + running→terminal"的收尾（成功刷新作品信息 / 失败提示）
watch(() => mergeState.value?.status, (status, oldStatus) => {
  if (initiatedMergeId.value === null) return
  if (currentResourceId.value !== initiatedMergeId.value) return
  if (oldStatus !== 'running' || !status || status === 'running') return
  initiatedMergeId.value = null
  if (status === 'done') {
    ElMessage.success('合并完成')
    getWorkInfo()
  } else {
    ElMessage.warning(mergeState.value?.errMsg || '合并失败')
  }
})

// 启动当前资源音视频合并（异步：立即返回，进度与结果经事件推送）
async function handleMergeButtonClick() {
  const resourceId = currentResourceId.value
  if (!resourceId) return
  markMergeStarted(resourceId)
  initiatedMergeId.value = resourceId
  try {
    await resourceMerge(resourceId)
  } catch (e) {
    clearMergeState(resourceId)
    initiatedMergeId.value = null
    ElMessage.error(e instanceof Error ? e.message : '合并失败')
  }
}

// 取消当前资源的进行中合并
async function handleMergeCancelClick() {
  const resourceId = currentResourceId.value
  if (!resourceId) return
  try {
    await resourceMergeCancel(resourceId)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '取消合并失败')
  }
}

// 查询作品信息
async function getWorkInfo() {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  const response = await apis.workGetFullWorkInfoById(workId)
  if (ApiUtil.check(response)) {
    const temp = ApiUtil.data<WorkFullDTO>(response)
    if (notNullish(temp)) {
      copyIgnoreUndefined(currentWorkFullInfo.value, temp)
    } else {
      ElMessage({ type: 'error', message: '获取作品信息失败' })
    }
  }
}
// 刷新标签
function refreshTags() {
  const tempLocalTags = currentWorkFullInfo.value.localTags?.filter(notNullish).map(
    (localTag) => new SegmentedTagItem({
      value: localTag.id as number,
      label: localTag.localTagName as string,
      // 多 ns 关联聚合为 '/' 连接串，NamespaceTag 按清单逐段解析显示名
      extraData: { namespace: workTagNs.value.local.get(localTag.id as number)?.join('/') },
      disabled: false
    })
  )
  localTags.value = isNullish(tempLocalTags) ? [] : tempLocalTags
  const tempSiteTags = currentWorkFullInfo.value.siteTags?.filter(notNullish).map(
    (siteTag) => new SegmentedTagItem({
      value: (siteTag.siteTag?.id ?? 0) as number,
      label: (siteTag.siteTag?.siteTagName ?? '') as string,
      subLabels: [(isBlank(siteTag.site?.siteName) ? '?' : siteTag.site?.siteName) as string],
      extraData: { namespace: workTagNs.value.site.get((siteTag.siteTag?.id ?? 0) as number)?.join('/') },
      disabled: false
    })
  )
  siteTags.value = isNullish(tempSiteTags) ? [] : tempSiteTags
}
// 刷新作品集
async function refreshWorkSets() {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) {
    workSets.value = []
    return
  }
  const response = await apis.workSetListByWorkId(workId)
  if (ApiUtil.check(response)) {
    const data = ApiUtil.data<WorkSetDTO[]>(response)
    const tempWorkSets = data?.filter(notNullish).map(
      (ws) =>
        new SegmentedTagItem({
          value: ws.id as number,
          label: ws.siteWorkSetName ?? '',
          disabled: false
        })
    )
    workSets.value = isNullish(tempWorkSets) ? [] : tempWorkSets
  }
}
// 刷新作品（信息+标签+作品集）
async function refreshWorkInfo() {
  await getWorkInfo()
  await refreshWorkTagNs()
  refreshTags()
  await refreshWorkSets()
}
// 处理标签交换确认
async function handleTagExchangeConfirm(type: OriginType, upper: SelectItem[], lower: SelectItem[], isUpper?: boolean) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  if ((isNullish(isUpper) ? true : isUpper) && arrayNotEmpty(upper)) {
    // 绑定缓冲区（新增绑定 + 改 ns 重确认）：新值走 Link；改 ns 的旧值行走 UnlinkDimension——
    // 唯一键含 ns，仅 Link 新值会令旧值行残留。多 ns 聚合展示的 tag 编辑为单值 = 替换全部旧值。
    // 先 Link 后摘旧：Link 失败中止（缓冲区保留供重试），摘旧失败仅提示（新旧并存可重试编辑）
    const linkIds: number[] = []
    const linkNamespaces: string[] = []
    const unlinkDimIds: number[] = []
    const unlinkDimNamespaces: string[] = []
    const oldNsMap = OriginType.LOCAL === type ? workTagNs.value.local : workTagNs.value.site
    for (const item of upper) {
      const tagId = item.value as number
      const newNs = (item.extraData?.namespace as string) ?? ''
      linkIds.push(tagId)
      linkNamespaces.push(newNs)
      for (const oldNs of oldNsMap.get(tagId) ?? []) {
        if (oldNs !== newNs) {
          unlinkDimIds.push(tagId)
          unlinkDimNamespaces.push(oldNs)
        }
      }
    }
    const boundResponse: ApiResponse = await apis.reWorkTagLink(workId, type, linkIds, linkNamespaces)
    if (!ApiUtil.check(boundResponse)) {
      ApiUtil.msg(boundResponse)
      return
    }
    ApiUtil.msg(boundResponse)
    if (unlinkDimIds.length > 0) {
      const unlinkResponse: ApiResponse = await apis.reWorkTagUnlinkDimension(workId, type, unlinkDimIds, unlinkDimNamespaces)
      ApiUtil.msg(unlinkResponse)
    }
  }
  if ((isNullish(isUpper) ? true : !isUpper) && arrayNotEmpty(lower)) {
    // 解绑缓冲区：整标签移除（该标签全部 ns 行）
    const unboundIds = lower.map((item) => item.value)
    const unboundResponse: ApiResponse = await apis.reWorkTagUnlink(workId, type, unboundIds as number[])
    if (ApiUtil.check(unboundResponse)) {
      ApiUtil.msg(unboundResponse)
    }
  }
  await refreshWorkTagNs()
  if (OriginType.LOCAL === type) {
    localTagExchangeBox.value?.refreshData(isUpper)
  } else {
    siteTagExchangeBox.value?.refreshData(isUpper)
  }
  await updateWorkTags(type)
  refreshTags()
}
// 更新标签
async function updateWorkTags(type: OriginType) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  if (OriginType.LOCAL === type) {
    const response = await apis.localTagListByWorkId(workId)
    if (ApiUtil.check(response)) {
      currentWorkFullInfo.value.localTags = ApiUtil.data<LocalTagDTO[]>(response)
        ?.filter(notNullish)
        .map(lt => ({ id: lt.id, localTagName: lt.localTagName, baseLocalTagId: lt.baseLocalTagId, description: lt.description, lastUse: lt.lastUse, createTime: lt.createTime ?? 0, updateTime: lt.updateTime ?? 0 } as LocalTagDTO))
    }
  } else {
    const tempSiteTagPage = new Page<SiteTagFullDTO>()
    tempSiteTagPage.pageSize = 100
    const tempSiteTagQuery = new SiteTagQueryDTO()
    const response = await apis.siteTagQueryPageByWorkId(workId, tempSiteTagPage, tempSiteTagQuery)
    if (ApiUtil.check(response)) {
      const tempResultPage = ApiUtil.data<Page<SiteTagFullDTO>>(response)
      currentWorkFullInfo.value.siteTags = isNullish(tempResultPage?.data) ? [] : tempResultPage.data as unknown as (SiteTagFullDTO | null)[]
    }
  }
}
// 拉取当前作品已绑定作者索引（id 集合 + 每作者 role 列表，local/site 各一）：候选区分页过滤
// 已绑定项、绑定确认 diff 旧 role 值、upper 区 role 聚合展示共用同一数据
async function refreshAuthorBoundIds(type: OriginType) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) {
    boundAuthorIds.value = { local: new Set(), site: new Set() }
    workAuthorRoles.value = { local: new Map(), site: new Map() }
    return
  }
  if (OriginType.LOCAL === type) {
    const authors = (await apis.reWorkAuthorListLocalAuthorsByWorkId(workId)).data.filter(notNullish)
    writeAuthorIndex(OriginType.LOCAL, authors)
  } else {
    const authors = (await apis.reWorkAuthorListSiteAuthorsByWorkId(workId)).data.filter(notNullish)
    writeAuthorIndex(OriginType.SITE, authors)
  }
}
// 由关联行（同作者多 role 各一行）重建绑定索引：id 集合与每作者 role 列表
function writeAuthorIndex(type: OriginType, authors: (RankedLocalAuthor | RankedSiteAuthor)[]) {
  const aggregated = aggregateAuthorRoles(authors)
  const ids = new Set(aggregated.map((entry) => entry.id))
  const roles = new Map(aggregated.map((entry) => [entry.id, entry.roles]))
  if (OriginType.LOCAL === type) {
    boundAuthorIds.value.local = ids
    workAuthorRoles.value.local = roles
  } else {
    boundAuthorIds.value.site = ids
    workAuthorRoles.value.site = roles
  }
}
// 请求作品已绑定作者（upper）：作品作者量小，全量拉取伪分页单页返回；搜索为前端过滤；
// 同作者多 role 关联行聚合为一项（extraData.role 为 '/' 连接串，AuthorRoleTag 展示/编辑），
// 顺带重建绑定索引
async function requestWorkAuthorUpperPage(type: OriginType, page: IPage<SelectItem>): Promise<IPage<SelectItem>> {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return page
  let keyword = ''
  let siteId: number | undefined
  let items: SelectItem[] = []
  if (OriginType.LOCAL === type) {
    keyword = localAuthorExchangeUpperSearchParams.value.authorNameStr?.value ?? ''
    const authors = (await apis.reWorkAuthorListLocalAuthorsByWorkId(workId)).data.filter(notNullish)
    writeAuthorIndex(OriginType.LOCAL, authors)
    items = aggregateAuthorRoles(authors)
      .filter((entry) => !keyword || (entry.author.author.authorName ?? '').includes(keyword))
      .map((entry) => new SelectItem({
        value: entry.id,
        label: entry.author.author.authorName ?? '',
        extraData: { role: entry.roles.join('/') }
      }))
  } else {
    keyword = siteAuthorExchangeUpperSearchParams.value.authorName?.value ?? ''
    siteId = siteAuthorExchangeUpperSearchParams.value.siteId?.value ?? undefined
    const authors = (await apis.reWorkAuthorListSiteAuthorsByWorkId(workId)).data.filter(notNullish)
    writeAuthorIndex(OriginType.SITE, authors)
    items = aggregateAuthorRoles(authors)
      .filter((entry) => (!keyword || (entry.author.author.authorName ?? '').includes(keyword))
        && (isNullish(siteId) || entry.author.author.siteId === siteId))
      .map((entry) => new SelectItem({
        value: entry.id,
        label: entry.author.author.authorName ?? '',
        extraData: { role: entry.roles.join('/') }
      }))
  }
  const resultPage = new Page<SelectItem>()
  resultPage.pageNumber = 1
  resultPage.pageSize = items.length > 0 ? items.length : page.pageSize
  resultPage.pageCount = 1
  resultPage.dataCount = items.length
  resultPage.data = items
  return resultPage
}
// 请求作品未绑定本地作者候选（lower）：后端查询无作品维度绑定态过滤，已绑定项前端剔除
async function requestWorkLocalAuthorLowerPage(page: IPage<SelectItem>): Promise<IPage<SelectItem>> {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return page
  const bindingsPage = new Page<SelectItem>()
  bindingsPage.pageNumber = page.pageNumber
  bindingsPage.pageSize = page.pageSize
  const response = await apis.localAuthorQuerySelectItemPage(bindingsPage, localAuthorExchangeLowerSearchParams.value)
  const newPage = response.data
  newPage.data = (newPage.data ?? []).filter(
    (item) => notNullish(item) && !boundAuthorIds.value.local.has(item.value as number)
  )
  return newPage
}
// 请求作品未绑定站点作者候选（lower）：已绑定项前端剔除
async function requestWorkSiteAuthorLowerPage(page: IPage<SelectItem>): Promise<IPage<SelectItem>> {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return page
  const bindingsPage = new Page<SiteAuthorDTO>()
  bindingsPage.pageNumber = page.pageNumber
  bindingsPage.pageSize = page.pageSize
  const response = await apis.siteAuthorQuerySelectItemPage(bindingsPage, siteAuthorExchangeLowerSearchParams.value)
  const newPage = response.data
  newPage.data = (newPage.data ?? []).filter(
    (item) => notNullish(item) && !boundAuthorIds.value.site.has(item.value as number)
  )
  return newPage
}
// 处理作者exchangeBox确认交换事件
async function handleAuthorExchangeConfirm(type: OriginType, upper: SelectItem[], lower: SelectItem[], isUpper?: boolean) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  if ((isNullish(isUpper) ? true : isUpper) && arrayNotEmpty(upper)) {
    // 绑定缓冲区（新增绑定 + 改 role 重确认）：新值走 Link；改 role 的旧值行走 UnlinkDimension——
    // 唯一键含 role，仅 Link 新值会令旧值行残留。多 role 聚合展示的作者编辑为单值 = 替换全部旧值。
    // 先 Link 后摘旧：Link 失败中止（缓冲区保留供重试），摘旧失败仅提示（新旧并存可重试编辑）
    const linkIds: number[] = []
    const linkRoles: string[] = []
    const unlinkDimIds: number[] = []
    const unlinkDimRoles: string[] = []
    const oldRoleMap = OriginType.LOCAL === type ? workAuthorRoles.value.local : workAuthorRoles.value.site
    for (const item of upper) {
      const authorId = item.value as number
      const newRole = (item.extraData?.role as string) ?? ''
      linkIds.push(authorId)
      linkRoles.push(newRole)
      for (const oldRole of oldRoleMap.get(authorId) ?? []) {
        if (oldRole !== newRole) {
          unlinkDimIds.push(authorId)
          unlinkDimRoles.push(oldRole)
        }
      }
    }
    const boundResponse: ApiResponse = await apis.reWorkAuthorLink(workId, type, linkIds, linkRoles)
    if (!ApiUtil.check(boundResponse)) {
      ApiUtil.msg(boundResponse)
      return
    }
    ApiUtil.msg(boundResponse)
    if (unlinkDimIds.length > 0) {
      const unlinkResponse: ApiResponse = await apis.reWorkAuthorUnlinkDimension(workId, type, unlinkDimIds, unlinkDimRoles)
      ApiUtil.msg(unlinkResponse)
    }
    // 集合增量同步（覆盖 refreshData 重拉窗口，候选区过滤即时生效）
    for (const item of upper) {
      if (OriginType.LOCAL === type) {
        boundAuthorIds.value.local.add(item.value as number)
      } else {
        boundAuthorIds.value.site.add(item.value as number)
      }
    }
  }
  if ((isNullish(isUpper) ? true : !isUpper) && arrayNotEmpty(lower)) {
    // 解绑缓冲区：整作者移除（该作者全部 role 行）
    const unboundIds = lower.map((item) => item.value)
    const unboundResponse: ApiResponse = await apis.reWorkAuthorUnlink(workId, type, unboundIds as number[])
    if (ApiUtil.check(unboundResponse)) {
      ApiUtil.msg(unboundResponse)
    }
    for (const item of lower) {
      if (OriginType.LOCAL === type) {
        boundAuthorIds.value.local.delete(item.value as number)
      } else {
        boundAuthorIds.value.site.delete(item.value as number)
      }
    }
  }
  await updateWorkAuthors(type)
  if (OriginType.LOCAL === type) {
    localAuthorExchangeBox.value?.refreshData(isUpper)
  } else {
    siteAuthorExchangeBox.value?.refreshData(isUpper)
  }
}
// 更新作品作者数据（确认后重拉，作品作者区分段 tag 展示随之刷新）
async function updateWorkAuthors(type: OriginType) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  if (OriginType.LOCAL === type) {
    const response = await apis.reWorkAuthorListLocalAuthorsByWorkId(workId)
    if (ApiUtil.check(response)) {
      currentWorkFullInfo.value.localAuthors = ApiUtil.data<RankedLocalAuthor[]>(response)?.filter(notNullish)
    }
  } else {
    const response = await apis.reWorkAuthorListSiteAuthorsByWorkId(workId)
    if (ApiUtil.check(response)) {
      currentWorkFullInfo.value.siteAuthors = ApiUtil.data<RankedSiteAuthor[]>(response)?.filter(notNullish)
    }
  }
}
// 请求作品绑定的本地标签分页
async function requestWorkLocalTagPage(page: IPage<SelectItem>, bounded: boolean) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return page
  const query = new LocalTagQueryDTO()
  query.workId = { value: workId }
  const bindingsPage = new Page<SelectItem>()
  bindingsPage.pageNumber = page.pageNumber
  bindingsPage.pageSize = page.pageSize
  const response = await apis.localTagQuerySelectItemPageByWorkId(bindingsPage, query, bounded)
  if (ApiUtil.check(response)) {
    const newPage = ApiUtil.data<IPage<SelectItem>>(response)
    if (!isNullish(newPage)) {
      // namespace 值回写 tag 数据（多 ns 聚合为 '/' 连接串）；可编辑性由区域 prop 控制
      // （local/site ExchangeBox upperEditableNs=true 均开启绑定区编辑，候选区不可编辑）
      for (const item of newPage.data ?? []) {
        if (!item) continue
        item.extraData = { ...(item.extraData ?? {}), namespace: workTagNs.value.local.get(item.value as number)?.join('/') }
      }
      return newPage
    }
    return page
  } else {
    throw new Error()
  }
}
// 请求作品绑定的站点标签分页
async function requestWorkSiteTagPage(page: IPage<SelectItem>, bounded: boolean) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return page
  const query = new SiteTagQueryDTO()
  const bindingsPage = new Page<SelectItem>()
  bindingsPage.pageNumber = page.pageNumber
  bindingsPage.pageSize = page.pageSize
  const response = await apis.siteTagQuerySelectItemPageByWorkId(workId, bindingsPage, query, bounded)
  if (ApiUtil.check(response)) {
    const newPage = ApiUtil.data<IPage<SelectItem>>(response)
    if (!isNullish(newPage)) {
      // site tag ns 同为关联级值（workTagNs.site 取回，多 ns 聚合为 '/' 连接串），绑定区与 local 同样开放编辑
      for (const item of newPage.data ?? []) {
        if (!item) continue
        item.extraData = { ...(item.extraData ?? {}), namespace: workTagNs.value.site.get(item.value as number)?.join('/') }
      }
      return newPage
    }
    return page
  } else {
    throw new Error()
  }
}
// 切换当前作品
async function setCurrentWork(newIndex: number): Promise<void> {
  if (props.work.length <= newIndex) {
    currentWorkIndex.value = props.work.length - 1
    return
  }
  if (newIndex < 0) {
    currentWorkIndex.value = 0
    return
  }
  currentWorkIndex.value = newIndex
  await nextTick()
  return refreshWorkInfo()
}
// 键盘左右切换作品
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft') {
    setCurrentWork(currentWorkIndex.value - 1)
  } else if (event.key === 'ArrowRight') {
    setCurrentWork(currentWorkIndex.value + 1)
  }
}
// 删除作品
async function deleteWork() {
  const workId = currentWorkFullInfo.value.work?.id
  if (notNullish(workId)) {
    let response = await apis.workSoftDelete(workId!)
    // 作品正被分享拉取持有时后端拒绝软删：确认强制解锁后重试本次删除，取消则按原响应提示
    if (isWorkLockedResponse(response) && (await confirmWorkForceUnlock(workId!))) {
      response = await apis.workSoftDelete(workId!)
    }
    ApiUtil.msg(response)
  }
}
// 删除确认
function handleDeleteButtonClick() {
  ElMessageBox.confirm(
    h('div', {}, [h('span', null, '是否删除作品？'), h('br'), h('span', null, `${currentWorkFullInfo.value.work?.siteWorkName}`)]),
    '确认删除',
    {
      confirmButtonText: '删除',
      confirmButtonClass: 'el-button--danger',
      cancelButtonText: '取消'
    }
  )
    .then(() => deleteWork())
    .catch(() => ElMessage.warning({ message: '取消删除' }))
}
// 打开元数据抽屉（重置编辑模式）
function openDrawer() {
  localTagEdit.value = false
  siteTagEdit.value = false
  drawerState.value = true
}
// 进入标签编辑：关元数据抽屉，开编辑抽屉（ExchangeBox 独占，避免内嵌 descriptions 拥挤）
function openTagEdit(type: OriginType) {
  drawerState.value = false
  if (OriginType.LOCAL === type) {
    siteTagEdit.value = false
    localTagEdit.value = true
    editDrawerState.value = true
    nextTick(() => localTagExchangeBox.value?.refreshData())
  } else {
    localTagEdit.value = false
    siteTagEdit.value = true
    editDrawerState.value = true
    nextTick(() => siteTagExchangeBox.value?.refreshData())
  }
}
// 关闭编辑抽屉：重置编辑模式，回元数据抽屉
function closeEditDrawer() {
  editDrawerState.value = false
  localTagEdit.value = false
  siteTagEdit.value = false
  drawerState.value = true
}
// 进入作者编辑：关元数据抽屉，开作者编辑抽屉（对齐标签编辑抽屉结构）；先拉当前框已绑定集合，避免候选区分页混入已绑定作者
async function openAuthorEdit(type: OriginType) {
  drawerState.value = false
  if (OriginType.LOCAL === type) {
    siteAuthorEdit.value = false
    localAuthorEdit.value = true
  } else {
    localAuthorEdit.value = false
    siteAuthorEdit.value = true
  }
  await refreshAuthorBoundIds(type)
  authorDrawerState.value = true
  nextTick(() => {
    if (localAuthorEdit.value) localAuthorExchangeBox.value?.refreshData()
    if (siteAuthorEdit.value) siteAuthorExchangeBox.value?.refreshData()
  })
}
// 关闭作者编辑抽屉：重置编辑模式，回元数据抽屉
function closeAuthorDrawer() {
  authorDrawerState.value = false
  localAuthorEdit.value = false
  siteAuthorEdit.value = false
  drawerState.value = true
}
// 处理作品集标签点击
function handleWorkSetClicked(workSetTag: SegmentedTagItem) {
  emits('openWorkSet', workSetTag.value)
}
</script>

<template>
  <teleport to="#dialog-mount-point">
    <el-dialog
      v-model="state"
      :width="props.width"
      class="work-detail-dialog"
      style="margin: auto"
      destroy-on-close
      @open="refreshWorkInfo"
    >
      <template #header>
        <span class="work-detail-work-name">
          {{ isBlank(currentWorkFullInfo.work?.nickName) ? currentWorkFullInfo.work?.siteWorkName : currentWorkFullInfo.work?.nickName }}
        </span>
      </template>
      <div class="work-detail-container">
        <!-- 主体：作品资源展示（ResourceViewer，按 ResourceType 分发 / 插件渲染器覆盖） -->
        <div class="work-detail-main">
          <ResourceViewer
            :resource="currentWorkFullInfo.resource ?? new ResourceFullDTO()"
            :work="currentWorkFullInfo"
          />
        </div>
        <!-- 右侧功能栏 -->
        <div class="work-detail-sidebar">
          <el-button
              icon="Document"
              title="详情"
              @click="openDrawer"
          />
          <el-button
              icon="back"
              title="上一作品"
              @click="setCurrentWork(currentWorkIndex - 1)"
          />
          <el-button
              icon="right"
              title="下一作品"
              @click="setCurrentWork(currentWorkIndex + 1)"
          />
          <el-dropdown
              title="作品集"
              placement="left"
          >
            <el-button
                type="primary"
                icon="Files"
            />
            <template #dropdown>
              <template
                  v-for="workSet in workSets"
                  :key="workSet.value"
              >
                <el-dropdown-item @click="handleWorkSetClicked(workSet)">
                  {{ workSet.label }}
                </el-dropdown-item>
              </template>
            </template>
          </el-dropdown>
          <el-button
            type="danger"
            class="tone-fail"
            icon="delete"
            title="删除"
            @click="handleDeleteButtonClick"
          />
          <el-button
            v-if="mergeable"
            type="primary"
            icon="MagicStick"
            :title="mergeButtonTitle"
            :loading="isMerging"
            @click="handleMergeButtonClick"
          />
          <span
            v-if="mergeable && isMerging"
            class="work-detail-merge-pct"
          >{{ mergePercentLabel }}</span>
          <el-button
            v-if="mergeable && isMerging"
            type="warning"
            icon="Close"
            title="取消合并"
            @click="handleMergeCancelClick"
          />
        </div>
        <!-- 元数据抽屉：作者/简介/站点/作品集 + 标签 TagBox（点编辑弹出独立编辑抽屉） -->
        <el-drawer
          v-model="drawerState"
          size="40%"
          :with-header="false"
          :open-delay="1"
        >
          <el-scrollbar class="work-detail-drawer-scrollbar">
            <el-descriptions
              direction="horizontal"
              :column="1"
              border
            >
              <el-descriptions-item>
                <template #label>
                  <span>作者 </span>
                  <el-button
                    size="small"
                    @click="openAuthorEdit(OriginType.LOCAL)"
                  >
                    编辑本地
                  </el-button>
                  <el-button
                    size="small"
                    @click="openAuthorEdit(OriginType.SITE)"
                  >
                    编辑站点
                  </el-button>
                </template>
                <div class="work-detail-author-tags">
                  <author-tag
                    v-for="displayAuthor in displayAuthors"
                    :key="`${displayAuthor.origin}-${displayAuthor.author.author.id}`"
                    :author="displayAuthor.author"
                    :roles="displayAuthor.roles"
                  />
                </div>              </el-descriptions-item>
              <el-descriptions-item label="简介">
                <div>{{ currentWorkFullInfo.work?.siteWorkDescription }}</div>
              </el-descriptions-item>
              <el-descriptions-item label="站点">
                <span>{{ currentWorkFullInfo.site?.siteName }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="作品集">
                <tag-box
                  :data="workSets"
                  @tag-clicked="handleWorkSetClicked"
                />
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label>
                  <span>本地标签 </span>
                  <el-button
                    size="small"
                    @click="openTagEdit(OriginType.LOCAL)"
                  >
                    编辑
                  </el-button>
                </template>
                <tag-box :data="localTags" />
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label>
                  <span>站点标签 </span>
                  <el-button
                    size="small"
                    @click="openTagEdit(OriginType.SITE)"
                  >
                    编辑
                  </el-button>
                </template>
                <tag-box :data="siteTags" />
              </el-descriptions-item>
            </el-descriptions>
          </el-scrollbar>
        </el-drawer>
        <!-- 标签编辑抽屉：ExchangeBox 独占（本地/站点互斥），关闭回元数据抽屉 -->
        <el-drawer
          v-model="editDrawerState"
          size="55%"
          :with-header="false"
          @close="closeEditDrawer"
        >
          <exchange-box
            v-if="localTagEdit"
            ref="localTagExchangeBox"
            v-model:upper-search-params="localTagExchangeUpperSearchParams"
            v-model:lower-search-params="localTagExchangeLowerSearchParams"
            class="work-detail-tag-exchange-box"
            :upper-load="(_page: IPage<SelectItem>) => requestWorkLocalTagPage(_page, true)"
            :lower-load="(_page: IPage<SelectItem>) => requestWorkLocalTagPage(_page, false)"
            :search-button-disabled="false"
            tags-gap="10px"
            :upper-editable-ns="true"
            @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower, true)"
            @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower, false)"
            @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-input
                v-model="localTagExchangeUpperSearchParams.localTagName.value"
                placeholder="输入本地标签名称"
                clearable
              />
            </template>
            <template #lowerToolbarMain>
              <el-input
                v-model="localTagExchangeLowerSearchParams.localTagName.value"
                placeholder="输入本地标签名称"
                clearable
              />
            </template>
            <template #upperTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">已绑定</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">未绑定</span>
              </div>
            </template>
          </exchange-box>
          <exchange-box
            v-else-if="siteTagEdit"
            ref="siteTagExchangeBox"
            v-model:upper-search-params="siteTagExchangeUpperSearchParams"
            v-model:lower-search-params="siteTagExchangeLowerSearchParams"
            class="work-detail-tag-exchange-box"
            :upper-load="(_page) => requestWorkSiteTagPage(_page, true)"
            :lower-load="(_page) => requestWorkSiteTagPage(_page, false)"
            :search-button-disabled="false"
            tags-gap="10px"
            :upper-editable-ns="true"
            @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower, true)"
            @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower, false)"
            @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-row class="work-detail-search-bar">
                <el-col :span="18">
                  <el-input
                    v-model="siteTagExchangeUpperSearchParams.siteTagName.value"
                    placeholder="输入站点标签名称"
                    clearable
                  />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="siteTagExchangeUpperSearchParams.siteId.value"
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
                </el-col>
              </el-row>
            </template>
            <template #lowerToolbarMain>
              <el-row class="work-detail-search-bar">
                <el-col :span="18">
                  <el-input
                    v-model="siteTagExchangeLowerSearchParams.siteTagName.value"
                    placeholder="输入站点标签名称"
                    clearable
                  />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="siteTagExchangeLowerSearchParams.siteId.value"
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
                </el-col>
              </el-row>
            </template>
            <template #upperTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">已绑定</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">未绑定</span>
              </div>
            </template>
          </exchange-box>
        </el-drawer>
        <!-- 作者编辑抽屉：ExchangeBox 独占（本地/站点互斥），关闭回元数据抽屉；确认挂联走 reWorkAuthor Link/Unlink -->
        <el-drawer
          v-model="authorDrawerState"
          size="55%"
          :with-header="false"
          @close="closeAuthorDrawer"
        >
          <exchange-box
            v-if="localAuthorEdit"
            ref="localAuthorExchangeBox"
            v-model:upper-search-params="localAuthorExchangeUpperSearchParams"
            v-model:lower-search-params="localAuthorExchangeLowerSearchParams"
            class="work-detail-tag-exchange-box"
            :upper-load="(_page: IPage<SelectItem>) => requestWorkAuthorUpperPage(OriginType.LOCAL, _page)"
            :lower-load="(_page: IPage<SelectItem>) => requestWorkLocalAuthorLowerPage(_page)"
            :search-button-disabled="false"
            tags-gap="10px"
            :upper-editable-ns="true"
            :tag-component="AuthorRoleTag"
            @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.LOCAL, upper, lower, true)"
            @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.LOCAL, upper, lower, false)"
            @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.LOCAL, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-input
                v-model="localAuthorExchangeUpperSearchParams.authorNameStr.value"
                placeholder="输入本地作者名称"
                clearable
              />
            </template>
            <template #lowerToolbarMain>
              <el-input
                v-model="localAuthorExchangeLowerSearchParams.authorNameStr.value"
                placeholder="输入本地作者名称"
                clearable
              />
            </template>
            <template #upperTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">已绑定</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">未绑定</span>
              </div>
            </template>
          </exchange-box>
          <exchange-box
            v-else-if="siteAuthorEdit"
            ref="siteAuthorExchangeBox"
            v-model:upper-search-params="siteAuthorExchangeUpperSearchParams"
            v-model:lower-search-params="siteAuthorExchangeLowerSearchParams"
            class="work-detail-tag-exchange-box"
            :upper-load="(_page: IPage<SelectItem>) => requestWorkAuthorUpperPage(OriginType.SITE, _page)"
            :lower-load="(_page: IPage<SelectItem>) => requestWorkSiteAuthorLowerPage(_page)"
            :search-button-disabled="false"
            tags-gap="10px"
            :upper-editable-ns="true"
            :tag-component="AuthorRoleTag"
            @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.SITE, upper, lower, true)"
            @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.SITE, upper, lower, false)"
            @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleAuthorExchangeConfirm(OriginType.SITE, upper, lower)"
          >
            <template #upperToolbarMain>
              <el-row class="work-detail-search-bar">
                <el-col :span="18">
                  <el-input
                    v-model="siteAuthorExchangeUpperSearchParams.authorName.value"
                    placeholder="输入站点作者名称"
                    clearable
                  />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="siteAuthorExchangeUpperSearchParams.siteId.value"
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
                </el-col>
              </el-row>
            </template>
            <template #lowerToolbarMain>
              <el-row class="work-detail-search-bar">
                <el-col :span="18">
                  <el-input
                    v-model="siteAuthorExchangeLowerSearchParams.authorName.value"
                    placeholder="输入站点作者名称"
                    clearable
                  />
                </el-col>
                <el-col :span="6">
                  <auto-load-select
                    v-model="siteAuthorExchangeLowerSearchParams.siteId.value"
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
                </el-col>
              </el-row>
            </template>
            <template #upperTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">已绑定</span>
              </div>
            </template>
            <template #lowerTitle>
              <div class="work-detail-tag-exchange-box-title">
                <span class="work-detail-tag-exchange-box-title-text">未绑定</span>
              </div>
            </template>
          </exchange-box>
        </el-drawer>
      </div>
    </el-dialog>
  </teleport>
</template>

<style scoped>
.work-detail-work-name {
  display: block;
  width: 100%;
  text-align: center;
  color: var(--app-text-primary);
  font-size: 18px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
  overflow: hidden;
}
.work-detail-container {
  display: flex;
  flex-direction: row;
  height: 80vh;
}
.work-detail-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.work-detail-sidebar {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 8px 4px;
  width: 56px;
  border-left: 1px solid var(--app-border-color);
}
.work-detail-sidebar .el-button {
  margin-left: 0;
}
.work-detail-merge-pct {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
  line-height: 1;
}
.work-detail-drawer-scrollbar {
  padding: 12px;
}
.work-detail-author-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
}
.work-detail-tag-exchange-box {
  height: 100%;
}
.work-detail-tag-exchange-box-title {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--app-border-color);
  border-radius: var(--app-radius);
}
.work-detail-tag-exchange-box-title-text {
  text-align: center;
  writing-mode: vertical-lr;
  color: var(--app-text-regular);
}
.work-detail-search-bar {
  flex-grow: 1;
}
</style>
