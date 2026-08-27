<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { shareRevoke } from '@renderer/apis/http/wrappers/share'
import { useShareStore } from '@renderer/store/UseShareStore'
import { useShareReceiveStore } from '@renderer/store/UseShareReceiveStore'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil'
import { copyText } from '@renderer/utils/ClipboardUtil'

// model
// 弹窗开关（MainView 控制）
const state = defineModel<boolean>('state', { required: true })

const shareStore = useShareStore()
// 撤销确认中的 shareId（行内按钮防重复）
const revokingId = ref('')

const sessions = computed(() => shareStore.sessionList)
const hasSessions = computed(() => arrayNotEmpty(sessions.value))

// 会话状态 → StatusTag key（tokens.css/StatusRegistry 已登记）
function statusKey(state: string): string {
  return `share-${state}`
}

// 有效期展示：0=无限期
function formatExpires(expiresAt: number): string {
  if (expiresAt <= 0) return '无限期'
  return new Date(expiresAt).toLocaleString()
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`
}

async function handleCopyLink(link: string): Promise<void> {
  if (await copyText(link)) {
    ElMessage.success('链接已复制')
  } else {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

async function handleRevoke(shareId: string): Promise<void> {
  if (revokingId.value) return
  revokingId.value = shareId
  try {
    await shareRevoke(shareId)
    ElMessage.success('已撤销分享')
    await shareStore.loadSessions()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '撤销失败')
  } finally {
    revokingId.value = ''
  }
}

// 打开接收分享对话框（粘贴他人分享的链接拉取入库；空链接入口，粘贴后创建任务）
function handleReceive(): void {
  state.value = false
  useShareReceiveStore().openWith('')
}

onMounted(() => {
  void shareStore.loadSessions()
})
</script>

<template>
  <el-dialog
    v-model="state"
    title="分享管理"
    width="640px"
    append-to-body
  >
    <div class="share-manage-body">
      <el-alert
        class="share-manage-alert"
        type="info"
        :closable="false"
        show-icon
      >
        分享在应用运行期间有效：关闭本应用后全部链接立即失效；链接含访问密钥，请勿公开传播；
        分享内容端到端加密，中继无法读取。
      </el-alert>

      <div v-if="!hasSessions" class="share-manage-empty">
        暂无分享会话——在主页多选作品/作品集后点击 [分享] 创建
      </div>

      <div
        v-for="s in sessions"
        v-else
        :key="s.shareId"
        class="share-manage-item"
      >
        <div class="share-manage-item-head">
          <span class="share-manage-item-title">{{ s.title }}</span>
          <status-tag :status="statusKey(s.state)" size="small" />
        </div>
        <div class="share-manage-item-meta">
          {{ s.workCount }} 个作品 · {{ s.fileCount }} 个文件（{{ formatBytes(s.totalBytes) }}）
          <template v-if="s.missingFiles > 0">
            · {{ s.missingFiles }} 个缺失
          </template>
          <template v-if="s.passwordProtected">
            · 密码保护
          </template>
          · 有效期至 {{ formatExpires(s.expiresAt) }}
        </div>
        <div class="share-manage-item-meta">
          已服务 {{ s.streamsServed }} 次拉取 · {{ formatBytes(s.bytesServed) }}
          <template v-if="s.state === 'failed' && s.errMsg">
            · {{ s.errMsg }}
          </template>
        </div>
        <div class="share-manage-item-link">
          {{ s.link || '（未在线）' }}
        </div>
        <div class="share-manage-item-actions">
          <el-button
            size="small"
            :disabled="!s.link"
            @click="handleCopyLink(s.link)"
          >
            复制链接
          </el-button>
          <el-button
            size="small"
            type="danger"
            class="tone-fail"
            :disabled="s.state === 'revoked' || s.state === 'expired'"
            :loading="revokingId === s.shareId"
            @click="handleRevoke(s.shareId)"
          >
            撤销
          </el-button>
        </div>
      </div>

      <div class="share-manage-receive">
        <el-button size="small" @click="handleReceive">
          接收分享（粘贴链接拉取入库）
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.share-manage-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 60vh;
  overflow-y: auto;
}

.share-manage-alert {
  flex-shrink: 0;
}

.share-manage-empty {
  font-size: 13px;
  color: var(--app-text-secondary);
  padding: 24px 0;
  text-align: center;
}

.share-manage-receive {
  display: flex;
  justify-content: center;
  padding-top: 4px;
}

.share-manage-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border: 1px solid var(--app-border-color-lighter);
  border-radius: var(--app-radius);
  background-color: var(--app-fill-color-lighter);
}

.share-manage-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.share-manage-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-manage-item-meta {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.share-manage-item-link {
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

.share-manage-item-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
