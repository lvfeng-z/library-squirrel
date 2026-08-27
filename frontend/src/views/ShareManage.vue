<script setup lang="ts">
import { computed, h, onMounted, Ref, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseView from '@renderer/views/BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { Thead } from '@renderer/model/util/Thead.ts'
import { newPage } from '@renderer/utils/Pager.ts'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import type { ShareRecordDTO } from '@bindings/github.com/library-squirrel/backend/share/models'
import { shareDeleteRecord, shareRecords, shareRevoke } from '@renderer/apis/http/wrappers/share'
import { useShareStore } from '@renderer/store/UseShareStore'
import { useShareReceiveStore } from '@renderer/store/UseShareReceiveStore'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil'
import { copyText } from '@renderer/utils/ClipboardUtil'

/**
 * 分享管理视图：上区为进行中会话（运行态源=进程内会话，state 事件实时驱动），
 * 下区为历史分享记录（持久源=share_record 账本）。两源按 shareId 关联——active
 * 记录的状态列实时显示会话运行态（在线/重连中），无在驻会话时显示记录态「有效」。
 */

// onMounted
onMounted(() => {
  recordsSearchTable.value.doSearch()
  // 会话快照兜底（state 事件持续维护；视图进入时拉一次保证徽标/会话区即时）
  void useShareStore().loadSessions().catch((e) => console.warn('拉取分享会话失败', e))
})

// 变量
const shareStore = useShareStore()
// 历史记录表组件实例
const recordsSearchTable = ref()
// 分页参数（记录为全量查询，视图内本地分页）
const recordPage: Ref<Page<ShareRecordDTO>> = ref(newPage<ShareRecordDTO>())
// 记录全量缓存（SearchTable 每次查询刷新）
const allRecords: Ref<ShareRecordDTO[]> = ref([])
// 撤销确认中的 shareId（会话区按钮防重复）
const revokingId = ref('')
// 删除执行中的 shareId（记录行按钮防重复）
const deletingId = ref('')

// 会话终态判定（与 UseShareStore 后端状态机一致：revoked/expired/failed 不可逆）
function isTerminalSessionState(state: string): boolean {
  return state === 'revoked' || state === 'expired' || state === 'failed'
}

// 进行中会话（运行态：connecting/online/reconnecting；终态会话由历史记录区承载）
const liveSessions = computed(() =>
  shareStore.sessionList.filter((s) => !isTerminalSessionState(s.state))
)
const hasLiveSessions = computed(() => arrayNotEmpty(liveSessions.value))

// shareId → 在驻会话（记录状态列的运行态关联源）
const sessionByShareId = computed(() => {
  const map = new Map(shareStore.sessionList.map((s) => [s.shareId, s]))
  return map
})

// 记录状态列的 StatusTag key：active 记录优先显示在驻会话运行态，其余直接映射记录态
function recordStatusKey(record: ShareRecordDTO): string {
  if (record.state === 'active') {
    const session = sessionByShareId.value.get(record.shareId)
    if (session && !isTerminalSessionState(session.state)) {
      return `share-${session.state}`
    }
    return 'share-active'
  }
  return `share-${record.state}`
}

// 会话状态 → StatusTag key
function sessionStatusKey(state: string): string {
  return `share-${state}`
}

// 有效期展示：0=无限期
function formatExpires(expiresAt: number): string {
  if (expiresAt <= 0) return '无限期'
  return new Date(expiresAt).toLocaleString()
}

// 字节量格式化（B/KB/MB/GB，一位小数）
function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`
}

// 历史记录的表头
const recordsThead: Ref<Thead<ShareRecordDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'title',
    title: '标题',
    hide: false,
    minWidth: 160,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'workIds',
    title: '分享对象',
    hide: false,
    width: 120,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (_data, extraData) => {
      const record = extraData as ShareRecordDTO
      const parts: string[] = []
      if (record.workIds.length > 0) parts.push(`${record.workIds.length} 作品`)
      if (record.workSetIds.length > 0) parts.push(`${record.workSetIds.length} 作品集`)
      return h('span', parts.length > 0 ? parts.join(' · ') : '-')
    }
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'totalBytes',
    title: '内容大小',
    hide: false,
    width: 140,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (_data, extraData) => {
      const record = extraData as ShareRecordDTO
      const text = `${formatBytes(record.totalBytes)} · ${record.fileCount} 文件`
      return h('span', record.missingFiles > 0 ? `${text}（缺 ${record.missingFiles}）` : text)
    }
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'relayAddress',
    title: '中继',
    hide: false,
    minWidth: 140,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'expiresAt',
    title: '有效期至',
    hide: false,
    width: 170,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (data) => h('span', formatExpires(data as number))
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'state',
    title: '状态',
    hide: false,
    width: 90,
    headerAlign: 'center',
    dataAlign: 'center',
    // failed 记录把失败原因挂 tooltip（StatusTag 单根元素，title 透传到标签上）
    render: (_data, extraData) => {
      const record = extraData as ShareRecordDTO
      return h(StatusTag, {
        status: recordStatusKey(record),
        size: 'small',
        title: record.errMsg || undefined
      })
    }
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createdAt',
    title: '分享时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center'
  })
])

// 方法
// 拉取全量记录（每次查询刷新缓存；失败保留旧缓存不打断翻页）
async function loadAllRecords(): Promise<void> {
  allRecords.value = await shareRecords()
}

// 历史记录本地分页查询（后端为全量接口，视图内切片）
async function recordsQueryPageFn(p: Page<ShareRecordDTO>): Promise<Page<ShareRecordDTO>> {
  try {
    await loadAllRecords()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '查询分享记录失败')
  }
  const total = allRecords.value.length
  const slice = allRecords.value.slice((p.pageNumber - 1) * p.pageSize, p.pageNumber * p.pageSize)
  return newPage<ShareRecordDTO>({
    pageNumber: p.pageNumber,
    pageSize: p.pageSize,
    pageCount: Math.max(1, Math.ceil(total / p.pageSize)),
    dataCount: total,
    currentCount: slice.length,
    data: slice
  })
}

// 复制分享链接（含 fragment 密钥——链接即访问凭证，仅发给信任的收件人）
async function handleCopyLink(link: string): Promise<void> {
  if (!link) return
  if (await copyText(link)) {
    ElMessage.success('链接已复制')
  } else {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

// 撤销进行中的分享会话（在线即在中继即时生效；记录行随终态事件落 revoked）
async function handleRevoke(shareId: string): Promise<void> {
  if (revokingId.value) return
  revokingId.value = shareId
  try {
    await shareRevoke(shareId)
    ElMessage.success('已撤销分享')
    await refreshAll()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '撤销失败')
  } finally {
    revokingId.value = ''
  }
}

// 删除分享记录（物理删行；在驻会话后端先撤销——活跃分享删除即链接失效）
async function handleDeleteRecord(record: ShareRecordDTO): Promise<void> {
  if (deletingId.value) return
  const activeHint = record.state === 'active' ? '；该分享仍在有效期，删除后链接立即失效' : ''
  try {
    await ElMessageBox.confirm(`删除分享记录「${record.title}」后不可恢复${activeHint}，是否继续？`, '删除分享记录', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  deletingId.value = record.shareId
  try {
    await shareDeleteRecord(record.shareId)
    ElMessage.success('已删除分享记录')
    await refreshAll()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '删除分享记录失败')
  } finally {
    deletingId.value = ''
  }
}

// 打开接收分享对话框（粘贴他人分享的链接拉取入库；弹窗挂在 MainLayout）
function handleReceive(): void {
  useShareReceiveStore().openWith('')
}

// 刷新会话区与历史记录（撤销/删除后）
async function refreshAll(): Promise<void> {
  await Promise.all([recordsSearchTable.value.doSearch(), shareStore.loadSessions()])
}

// 记录行「复制链接」可用性：链接可重建且记录未失效（失效禁用）
function canCopyRecordLink(record: ShareRecordDTO): boolean {
  return record.state === 'active' && record.link !== ''
}

// 会话区「复制链接」可用性：链接未生成（注册中/重连中）时禁用
function canCopySessionLink(link: string): boolean {
  return link !== ''
}
</script>

<template>
  <base-view>
    <div class="share-manage-container">
      <el-alert
        class="share-manage-alert"
        type="info"
        :closable="false"
        show-icon
      >
        分享内容端到端加密，中继无法读取；关闭应用后链接临时失效，有效期内重新打开应用自动恢复；
        链接含访问密钥，请勿公开传播。
      </el-alert>

      <!-- 进行中会话区：运行态源（state 事件实时驱动）；终态历史由下方记录表承载 -->
      <div class="share-manage-sessions">
        <div class="share-manage-sessions-header">
          进行中的分享（{{ liveSessions.length }}）
        </div>
        <div v-if="!hasLiveSessions" class="share-manage-sessions-empty">
          当前没有进行中的分享会话——在主页多选作品/作品集后点击 [分享] 创建
        </div>
        <el-scrollbar v-else class="share-manage-sessions-list">
          <div
            v-for="s in liveSessions"
            :key="s.shareId"
            class="share-manage-session-item"
          >
            <div class="share-manage-session-item-head">
              <span class="share-manage-session-item-title">{{ s.title }}</span>
              <status-tag :status="sessionStatusKey(s.state)" size="small" />
            </div>
            <div class="share-manage-session-item-meta">
              {{ s.workCount }} 个作品 · {{ s.fileCount }} 个文件（{{ formatBytes(s.totalBytes) }}）
              <template v-if="s.missingFiles > 0">
                · {{ s.missingFiles }} 个缺失
              </template>
              <template v-if="s.passwordProtected">
                · 密码保护
              </template>
              · 有效期至 {{ formatExpires(s.expiresAt) }}
            </div>
            <div class="share-manage-session-item-meta">
              已服务 {{ s.streamsServed }} 次拉取 · {{ formatBytes(s.bytesServed) }}
              · {{ s.relayAddress }}
            </div>
            <div class="share-manage-session-item-link">
              {{ s.link || '（链接生成中）' }}
            </div>
            <div class="share-manage-session-item-actions">
              <el-button
                size="small"
                :disabled="!canCopySessionLink(s.link)"
                @click="handleCopyLink(s.link)"
              >
                复制链接
              </el-button>
              <el-button
                size="small"
                type="danger"
                class="tone-fail"
                :loading="revokingId === s.shareId"
                @click="handleRevoke(s.shareId)"
              >
                撤销
              </el-button>
            </div>
          </div>
        </el-scrollbar>
      </div>

      <!-- 历史分享记录表（持久源 share_record；本地分页） -->
      <search-table
        ref="recordsSearchTable"
        v-model:page="recordPage"
        class="share-manage-records-table"
        toolbar-radius="var(--app-radius)"
        data-key="shareId"
        :thead="recordsThead"
        :search="recordsQueryPageFn"
        :selectable="false"
        :multi-select="false"
        :custom-operation-button="true"
        :operation-width="180"
      >
        <template #toolbarMain>
          <div class="share-manage-toolbar">
            <el-button
              type="primary"
              @click="handleReceive"
            >
              接收分享（粘贴链接拉取入库）
            </el-button>
          </div>
        </template>
        <template #customOperations="{ row }">
          <div class="share-manage-record-operations">
            <el-button
              size="small"
              :disabled="!canCopyRecordLink(row as ShareRecordDTO)"
              @click="handleCopyLink((row as ShareRecordDTO).link)"
            >
              复制链接
            </el-button>
            <el-button
              size="small"
              type="danger"
              class="tone-fail"
              :loading="deletingId === (row as ShareRecordDTO).shareId"
              @click="handleDeleteRecord(row as ShareRecordDTO)"
            >
              删除记录
            </el-button>
          </div>
        </template>
      </search-table>
    </div>
  </base-view>
</template>

<style scoped>
.share-manage-container {
  display: flex;
  flex-direction: column;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
  gap: 10px;
}

.share-manage-alert {
  flex-shrink: 0;
}

/* 会话区：固定弹性上区（无会话时收缩为空态一行），多条会话内部滚动 */
.share-manage-sessions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex-shrink: 0;
  max-height: 40%;
  padding: 10px 12px;
  background-color: var(--app-bg-surface);
  border: 1px solid var(--app-border-color-lighter);
  border-radius: var(--app-radius);
}

.share-manage-sessions-header {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
  flex-shrink: 0;
}

.share-manage-sessions-empty {
  font-size: 13px;
  color: var(--app-text-secondary);
  padding: 4px 0;
}

.share-manage-sessions-list {
  flex: 1;
  min-height: 0;
}

.share-manage-session-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--app-border-color-lighter);
  border-radius: var(--app-radius);
  background-color: var(--app-fill-color-lighter);
}

.share-manage-session-item + .share-manage-session-item {
  margin-top: 8px;
}

.share-manage-session-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.share-manage-session-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-manage-session-item-meta {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.share-manage-session-item-link {
  padding: 6px 8px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background-color: var(--app-fill-color-light);
  font-family: monospace;
  font-size: 12px;
  color: var(--app-text-primary);
  word-break: break-all;
  user-select: text;
}

.share-manage-session-item-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.share-manage-records-table {
  flex: 1;
  min-height: 0;
  width: 100%;
}

.share-manage-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.share-manage-record-operations {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
