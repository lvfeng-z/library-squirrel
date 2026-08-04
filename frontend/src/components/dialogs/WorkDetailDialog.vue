<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TagBox from '../common/TagBox.vue'
import { LocalTagDTO, WorkSetDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import {
  SelectItem, WorkFullDTO, SiteTagFullDTO, ResourceFullDTO
} from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import ApiUtil from '@renderer/utils/ApiUtil'
import ExchangeBox from '@renderer/components/common/ExchangeBox.vue'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import IPage from '@renderer/model/util/IPage.ts'
import { OriginType } from '@renderer/constants/OriginType.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import AuthorInfo from '@renderer/components/common/AuthorInfo.vue'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { LocalTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/localTag/models'
import { SiteTagQueryDTO } from '@bindings/github.com/library-squirrel/backend/siteTag/models'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { localTagApi, siteTagApi, workApi, workSetApi } from '@renderer/apis/http'
import { reWorkTagApi } from '@renderer/apis/http'
import { resourceMerge } from '@renderer/apis/http/wrappers/resource'
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
  workSoftDelete: workApi.workSoftDelete,
  workGetFullWorkInfoById: workApi.workGetFullWorkInfoById,
  workSetListByWorkId: workSetApi.workSetListByWorkId
}
// ExchangeBox 组件实例
const localTagExchangeBox = ref()
const siteTagExchangeBox = ref()
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
// 作品集
const workSets: Ref<SegmentedTagItem[]> = ref([])
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

// 当前资源是否可合并（含视频轨+音频轨）
const mergeable: Ref<boolean> = computed(() => isResourceMergeable(currentWorkFullInfo.value.resource))
// 合并进行中
const merging: Ref<boolean> = ref(false)

// 合并当前资源音视频轨，成功后刷新作品信息
async function handleMergeButtonClick() {
  const resourceId = currentWorkFullInfo.value.resource?.id
  if (!resourceId) return
  merging.value = true
  try {
    await resourceMerge(resourceId)
    ElMessage.success('合并完成')
    await getWorkInfo()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '合并失败')
  } finally {
    merging.value = false
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
    (localTag) =>
      new SegmentedTagItem({
        value: localTag.id as number,
        label: localTag.localTagName as string,
        disabled: false
      })
  )
  localTags.value = isNullish(tempLocalTags) ? [] : tempLocalTags
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
// 刷新作品（信息+标签+作品集）
async function refreshWorkInfo() {
  await getWorkInfo()
  refreshTags()
  await refreshWorkSets()
}
// 处理标签交换确认
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
    localTagExchangeBox.value?.refreshData(isUpper)
  } else {
    siteTagExchangeBox.value?.refreshData(isUpper)
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
    return isNullish(newPage) ? page : newPage
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
    return isNullish(newPage) ? page : newPage
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
    const response = await apis.workSoftDelete(workId!)
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
            title="合并音视频轨"
            :loading="merging"
            @click="handleMergeButtonClick"
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
              <el-descriptions-item label="作者">
                <author-info
                  :local-authors="currentWorkFullInfo.localAuthors?.filter(notNullish)"
                  :site-authors="currentWorkFullInfo.siteAuthors?.filter(notNullish)"
                />
              </el-descriptions-item>
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
            @upper-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
            @lower-confirm="(upper: SelectItem[], lower: SelectItem[]) => handleTagExchangeConfirm(OriginType.SITE, upper, lower)"
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
.work-detail-drawer-scrollbar {
  padding: 12px;
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
