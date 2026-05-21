<script setup lang="ts">
import { computed, h, nextTick, onBeforeMount, onBeforeUnmount, onMounted, Ref, ref, UnwrapRef } from 'vue'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TagBox from '../common/TagBox.vue'
import { SelectItem, WorkFullDTO, LocalTagDTO, SiteTagFullDTO, ResourceDTO, LocalAuthorDTO, SiteAuthorFullDTO, WorkSetDTO } from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import { Page } from "@bindings/github.com/library-squirrel/backend/base/model/models"
import ApiUtil from '@renderer/utils/ApiUtil'
import ExchangeBox from '@renderer/components/common/ExchangeBox.vue'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { OriginType } from '@renderer/constants/OriginType.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import AuthorInfo from '@renderer/components/common/AuthorInfo.vue'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { Picture } from '@element-plus/icons-vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import AutoHeightDialog from '@renderer/components/dialogs/AutoHeightDialog.vue'
import { LocalTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/localTag/models'
import { SiteTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/siteTag/models'
import RankedLocalAuthor from '@renderer/model/model/domain/RankedLocalAuthor.ts'
import RankedSiteAuthor from '@renderer/model/model/domain/RankedSiteAuthor.ts'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { localTagApi, siteTagApi, workApi, workSetApi } from '@renderer/apis/http'
import { reWorkTagApi } from '@renderer/apis/http'
import { appLauncherOpenImage } from '@renderer/apis/http/wrappers/appLauncher'

// props
const props = defineProps<{
  work: WorkFullDTO[]
  width?: string
}>()

// model
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })
const currentWorkIndex = defineModel<number>('currentWorkIndex', { required: true })

// 事件
const emits = defineEmits(['openWorkSet'])

// onBeforeMount
onBeforeMount(async () => {
})
// onMounted
onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

// onBeforeUnmount
onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})

// 变量
// 接口
const apis = {
  localTagListByWorkId: localTagApi.localTagListByWorkId,
  localTagQuerySelectItemPageByWorkId: localTagApi.localTagQuerySelectItemPageByWorkId,
  siteTagQueryPageByWorkId: siteTagApi.siteTagQueryPageByWorkId,
  siteTagQuerySelectItemPageByWorkId: siteTagApi.siteTagQuerySelectItemPageByWorkId,
  reWorkTagLink: reWorkTagApi.reWorkTagLink,
  reWorkTagUnlink: reWorkTagApi.reWorkTagUnlink,
  workDeleteWorkAndSurroundingData: workApi.workDeleteWorkAndSurroundingData,
  workGetFullWorkInfoById: workApi.workGetFullWorkInfoById,
  workSetListByWorkId: workSetApi.workSetListByWorkId
}
// 主要容器的实例
const infosRef = ref()
// localTag的ExchangeBox组件的实例
const localTagExchangeBox = ref()
// siteTag的ExchangeBox组件的实例
const siteTagExchangeBox = ref()
// 作品信息
const currentWorkFullInfo: Ref<WorkFullDTO> = computed(() => {
  const raw = props.work[currentWorkIndex.value]
  if (!raw) return new WorkFullDTO()
  // 确保嵌套结构初始化
  if (!raw.work) raw.work = { id: 0, createTime: 0, updateTime: 0 } as any
  return raw
})
// 本地标签
const localTags: Ref<SegmentedTagItem[]> = ref([])
// 本地标签
const siteTags: Ref<SegmentedTagItem[]> = ref([])
// 作品集
const workSets: Ref<SegmentedTagItem[]> = ref([])
// 本地标签编辑开关
const drawerState: Ref<boolean> = ref(false)
// 本地标签编辑开关
const localTagEdit: Ref<boolean> = ref(false)
// 本地标签编辑开关
const siteTagEdit: Ref<boolean> = ref(false)
// 本地标签的查询参数
const localTagExchangeUpperSearchParams: Ref<LocalTagQueryDTO> = ref(new LocalTagQueryDTO())
// 本地标签的查询参数
const localTagExchangeLowerSearchParams: Ref<LocalTagQueryDTO> = ref(new LocalTagQueryDTO())
// 站点标签的查询参数
const siteTagExchangeUpperSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())
// 站点标签的查询参数
const siteTagExchangeLowerSearchParams: Ref<SiteTagQueryDTO> = ref(new SiteTagQueryDTO())

// ===== 辅助：从 resources 数组中获取活跃资源 =====
function getActiveResource(info: WorkFullDTO): ResourceDTO | null | undefined {
  if (!info.resources?.length) return undefined
  return info.resources.find(r => r?.state === 1) ?? info.resources[0]
}

// ===== 辅助：将 BindingsLocalAuthorDTO 转换为 RankedLocalAuthor（适配 AuthorInfo 组件）=====
function toRankedLocalAuthor(dto: LocalAuthorDTO | null): RankedLocalAuthor | null {
  if (!dto) return null
  const author = new RankedLocalAuthor()
  author.id = dto.id
  author.authorName = dto.authorName ?? undefined
  author.introduce = dto.introduce ?? undefined
  author.lastUse = dto.lastUse ?? undefined
  author.authorRank = undefined
  return author
}

// ===== 辅助：将 BindingsSiteAuthorFullDTO 转换为 RankedSiteAuthor（适配 AuthorInfo 组件）=====
function toRankedSiteAuthor(dto: SiteAuthorFullDTO | null): RankedSiteAuthor | null {
  if (!dto || !dto.siteAuthor) return null
  const author = new RankedSiteAuthor()
  author.id = dto.siteAuthor.id
  author.authorName = dto.siteAuthor.authorName ?? undefined
  author.introduce = dto.siteAuthor.introduce ?? undefined
  author.localAuthorId = dto.siteAuthor.localAuthorId ?? undefined
  author.authorRank = undefined
  return author
}

// 方法
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
      ElMessage({
        type: 'error',
        message: '获取作品信息失败'
      })
    }
  }
}
// 刷新标签
function refreshTags() {
  // 本地标签
  const tempLocalTags = currentWorkFullInfo.value.localTags?.filter(notNullish).map(
    (localTag) =>
      new SegmentedTagItem({
        value: localTag.id as number,
        label: localTag.localTagName as string,
        disabled: false
      })
  )
  localTags.value = isNullish(tempLocalTags) ? [] : tempLocalTags
  // 站点标签
  const tempSiteTags = currentWorkFullInfo.value.siteTags?.filter(notNullish).map(
    (siteTag) =>
      new SegmentedTagItem({
        value: (siteTag.siteTag?.id ?? 0) as number,
        label: (siteTag.siteTag?.siteTagName ?? '') as string,
        subLabels: [(isBlank(siteTag.site?.siteName) ? '?' : siteTag.site?.siteName) as string],
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
// 刷新作品
async function refreshWorkInfo() {
  await getWorkInfo()
  refreshTags()
  await refreshWorkSets()
}
// 处理本地标签exchangeBox确认交换事件
async function handleTagExchangeConfirm(type: OriginType, upper: SelectItem[], lower: SelectItem[], isUpper?: boolean) {
  const workId = currentWorkFullInfo.value.work?.id
  if (!workId) return
  if (isNullish(isUpper) ? true : isUpper) {
    const boundIds = upper.map((item) => item.value)
    const boundResponse: ApiResponse = await apis.reWorkTagLink(workId, type, boundIds as number[])
    if (ApiUtil.check(boundResponse)) {
      ApiUtil.msg(boundResponse)
    }
  }
  if (isNullish(isUpper) ? true : !isUpper) {
    const unboundIds = lower.map((item) => item.value)
    const unboundResponse: ApiResponse = await apis.reWorkTagUnlink(workId, type, unboundIds as number[])
    if (ApiUtil.check(unboundResponse)) {
      ApiUtil.msg(unboundResponse)
    }
  }
  if (OriginType.LOCAL === type) {
    localTagExchangeBox.value.refreshData(isUpper)
  } else {
    siteTagExchangeBox.value.refreshData(isUpper)
  }
  updateWorkTags(type)
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
// 请求作品绑定的本地标签接口的函数
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
    return isNullish(newPage) ? page : newPage
  } else {
    throw new Error()
  }
}
// 请求作品绑定的站点标签接口的函数
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
    return isNullish(newPage) ? page : newPage
  } else {
    throw new Error()
  }
}
// 处理图片点击事件
function handlePictureClicked() {
  const resource = getActiveResource(currentWorkFullInfo.value)
  if (notNullish(resource?.filePath)) {
    appLauncherOpenImage(resource!.filePath!)
  } else {
    ElMessage({
      type: 'error',
      message: '无法打开图片，获取资源路径失败'
    })
  }
}
// 打开本地标签和站点标签的抽屉面板
function openDrawer(isLocal: boolean) {
  drawerState.value = true
  if (isLocal) {
    siteTagEdit.value = false
    localTagEdit.value = true
  } else {
    localTagEdit.value = false
    siteTagEdit.value = true
  }
}
// 处理抽屉面板打开事件
function handleDrawerOpen() {
  if (localTagEdit.value) {
    localTagExchangeBox.value.refreshData()
  }
  if (siteTagEdit.value) {
    siteTagExchangeBox.value.refreshData()
  }
}
// 处理删除按钮点击事件
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
// 处理键盘按下事件
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
    const response = await apis.workDeleteWorkAndSurroundingData(workId!)
    ApiUtil.msg(response)
  }
}
// 处理作品集标签被点击事件
function handleWorkSetClicked(workSetTag: SegmentedTagItem) {
  emits('openWorkSet', workSetTag.value)
}
</script>
<template>
  <auto-height-dialog v-model:state="state" :width="props.width" @open="refreshWorkInfo">
    <template #header>
      <span class="work-dialog-work-name">
        {{ isBlank(currentWorkFullInfo.work?.nickName) ? currentWorkFullInfo.work?.siteWorkName : currentWorkFullInfo.work?.nickName }}
      </span>
    </template>
    <div class="work-dialog-container">
      <el-image
        class="work-dialog-image"
        fit="contain"
        :src="`/resource/${getActiveResource(currentWorkFullInfo)?.filePath}`"
        @click="handlePictureClicked"
      >
        <template #error>
          <div class="work-dialog-image-error">
            <el-icon class="work-dialog-image-error-icon"><Picture /></el-icon>
          </div>
        </template>
      </el-image>
      <el-scrollbar class="work-dialog-scrollbar">
        <el-descriptions ref="infosRef" class="work-dialog-info" direction="horizontal" :column="1">
          <el-descriptions-item label="作者">
            <author-info
              id="author"
              :local-authors="currentWorkFullInfo.localAuthors?.filter(notNullish).map(toRankedLocalAuthor).filter(notNullish)"
              :site-authors="currentWorkFullInfo.siteAuthors?.filter(notNullish).map(toRankedSiteAuthor).filter(notNullish)"
            />
          </el-descriptions-item>
          <el-descriptions-item>
            <div id="description">
              {{ currentWorkFullInfo.work?.siteWorkDescription }}
            </div>
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label>
              <span>本地标签 </span>
              <el-button @click="openDrawer(true)"> 编辑 </el-button>
            </template>
            <tag-box id="localTag" v-model:data="localTags" />
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label>
              <span>站点标签 </span>
              <el-button @click="openDrawer(false)"> 编辑 </el-button>
            </template>
            <tag-box id="siteTag" v-model:data="siteTags" />
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label>
              <span>作品集 </span>
            </template>
            <tag-box id="workSet" v-model:data="workSets" @tag-clicked="handleWorkSetClicked" />
          </el-descriptions-item>
          <el-descriptions-item label="站点">
            <span id="site">{{ currentWorkFullInfo.site?.siteName }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-scrollbar>
      <el-anchor :container="infosRef?.parentElement?.parentElement" direction="vertical" type="default" :offset="30" @click="(e: MouseEvent) => e.preventDefault()">
        <el-anchor-link href="#author" title="作者" />
        <el-anchor-link href="#description" title="简介" />
        <el-anchor-link href="#localTag" title="本地标签" />
        <el-anchor-link href="#siteTag" title="站点标签" />
        <el-anchor-link href="#site" title="站点" />
      </el-anchor>
      <el-drawer v-model="drawerState" size="45%" :with-header="false" :open-delay="1" @open="handleDrawerOpen">
        <exchange-box
          v-if="localTagEdit"
          ref="localTagExchangeBox"
          v-model:upper-search-params="localTagExchangeUpperSearchParams"
          v-model:lower-search-params="localTagExchangeLowerSearchParams"
          class="work-dialog-tag-exchange-box"
          :upper-load="(_page: IPage<SelectItem>) => requestWorkLocalTagPage(_page, true)"
          :lower-load="(_page: IPage<SelectItem>) => requestWorkLocalTagPage(_page, false)"
          :search-button-disabled="false"
          tags-gap="10px"
          @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower, true)"
          @lower-confirm="
            (upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower, false)
          "
          @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.LOCAL, upper, lower)"
        >
          <template #upperToolbarMain>
            <el-input v-model="localTagExchangeUpperSearchParams.localTagName.value" placeholder="输入本地标签名称" clearable />
          </template>
          <template #lowerToolbarMain>
            <el-input v-model="localTagExchangeLowerSearchParams.localTagName.value" placeholder="输入本地标签名称" clearable />
          </template>
          <template #upperTitle>
            <div class="work-dialog-tag-exchange-box-title">
              <span class="work-dialog-tag-exchange-box-title-text">已绑定</span>
            </div>
          </template>
          <template #lowerTitle>
            <div class="work-dialog-tag-exchange-box-title">
              <span class="work-dialog-tag-exchange-box-title-text">未绑定</span>
            </div>
          </template>
        </exchange-box>
        <exchange-box
          v-if="siteTagEdit"
          ref="siteTagExchangeBox"
          v-model:upper-search-params="siteTagExchangeUpperSearchParams"
          v-model:lower-search-params="siteTagExchangeLowerSearchParams"
          class="work-dialog-tag-exchange-box"
          :upper-load="(_page) => requestWorkSiteTagPage(_page, true)"
          :lower-load="(_page) => requestWorkSiteTagPage(_page, false)"
          :search-button-disabled="false"
          tags-gap="10px"
          @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
          @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
          @all-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
        >
          <template #upperToolbarMain>
            <el-row class="work-dialog-search-bar">
              <el-col :span="18">
                <el-input v-model="siteTagExchangeUpperSearchParams.siteTagName.value" placeholder="输入站点标签名称" clearable />
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
                    <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                  </template>
                </auto-load-select>
              </el-col>
            </el-row>
          </template>
          <template #lowerToolbarMain>
            <el-row class="work-dialog-search-bar">
              <el-col :span="18">
                <el-input v-model="siteTagExchangeLowerSearchParams.siteTagName.value" placeholder="输入站点标签名称" clearable />
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
                    <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                  </template>
                </auto-load-select>
              </el-col>
            </el-row>
          </template>
          <template #upperTitle>
            <div class="work-dialog-tag-exchange-box-title">
              <span class="work-dialog-tag-exchange-box-title-text">已绑定</span>
            </div>
          </template>
          <template #lowerTitle>
            <div class="work-dialog-tag-exchange-box-title">
              <span class="work-dialog-tag-exchange-box-title-text">未绑定</span>
            </div>
          </template>
        </exchange-box>
      </el-drawer>
    </div>
    <template #footer>
      <div class="work-dialog-footer-buttons">
        <el-button type="danger" icon="delete" @click="handleDeleteButtonClick">删除</el-button>
        <el-button-group class="work-dialog-footer-buttons-group" size="large">
          <el-button icon="back" @click="setCurrentWork(currentWorkIndex - 1)" />
          <el-button icon="right" @click="setCurrentWork(currentWorkIndex + 1)" />
        </el-button-group>
        <el-dropdown title="作品集" placement="top-end">
          <el-button type="primary" icon="Files">作品集</el-button>
          <template #dropdown>
            <template v-for="workSet in workSets" :key="workSet.value">
              <el-dropdown-item @click="handleWorkSetClicked(workSet)">{{ workSet.label }}</el-dropdown-item>
            </template>
          </template>
        </el-dropdown>
      </div>
    </template>
  </auto-height-dialog>
</template>

<style scoped>
.work-dialog-work-name {
  text-overflow: ellipsis;
  white-space: nowrap;
}
.work-dialog-container {
  display: flex;
  flex-direction: row;
}
.work-dialog-scrollbar {
  flex-grow: 1;
}
.work-dialog-image {
  max-height: 65vh;
  max-width: 60%;
  margin-right: 10px;
  cursor: pointer;
  transition-duration: 0.3s;
}
.work-dialog-image:hover {
  background-color: rgb(166.2, 168.6, 173.4, 10%);
  filter: drop-shadow(0 0 10px rgba(0, 0, 0, 0.2));
}
.work-dialog-image-error {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background-color: var(--el-fill-color-dark);
  width: 200px;
}
.work-dialog-image-error-icon {
  color: var(--el-text-color-secondary);
  scale: 2;
}
.work-dialog-info {
  margin-right: 10px;
}
.work-dialog-tag-exchange-box {
  height: 100%;
}
.work-dialog-tag-exchange-box-title {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--el-border-color);
  border-radius: 5px;
}
.work-dialog-tag-exchange-box-title-text {
  text-align: center;
  writing-mode: vertical-lr;
  color: var(--el-text-color-regular);
}
.work-dialog-search-bar {
  flex-grow: 1;
}
.work-dialog-footer-buttons {
  display: flex;
  align-items: center;
  position: relative;
  width: 100%;
}
.work-dialog-footer-buttons .work-dialog-footer-buttons-group {
  margin: 0 auto;
}
</style>
