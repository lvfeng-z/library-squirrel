<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import { shareCancelPublish, sharePublish } from '@renderer/apis/http/wrappers/share'
import { settingsGetSettings } from '@renderer/apis/http/wrappers/settings'
import { useShareStore } from '@renderer/store/UseShareStore'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil'
import { copyText } from '@renderer/utils/ClipboardUtil'

// model
// 弹窗开关（MainView 控制）
const state = defineModel<boolean>('state', { required: true })

// props
// 选中作品/作品集 id 列表（与导出同形态，前端透传给后端收集）
const props = defineProps<{
  workIds: number[]
  workSetIds: number[]
}>()

// —— 配置项（打开弹窗时复位） ——
// 落地页标题（空=默认「分享 N 个作品…」）
const title = ref('')
// 有效期模式：default=中继默认(7 天) / custom=自定义天数 / never=无限期
const expireMode = ref<'default' | 'custom' | 'never'>('default')
// 自定义天数（expireMode=custom 时生效）
const customDays = ref(7)
// 访问密码（空=无密码）
const password = ref('')
// 分享中继地址（设置页 shareSettings.relayAddress；弹窗内只读展示）
const relayAddress = ref('')

// —— 发布状态 ——
// 本次发布的 shareID（null=尚未发起）
const shareId = ref<string | null>(null)
// SharePublish IPC 进行中（防重复触发）
const starting = ref(false)
// 同步失败信息（空选择/中继未配置等前置错误）
const startError = ref('')

const shareStore = useShareStore()

const hasSelection = computed(() => arrayNotEmpty(props.workIds) || arrayNotEmpty(props.workSetIds))

// 是否处于配置阶段（有选择、未发起、无错误）
const inConfig = computed(() => hasSelection.value && !shareId.value && !starting.value && !startError.value)

// 发布过程态（share-events 驱动）
const publishing = computed(() => (shareId.value ? shareStore.publishings[shareId.value] : undefined))

// 会话运行态（registering 阶段起 state 事件持续推送；中继不可达时为 reconnecting）
const session = computed(() => (shareId.value ? shareStore.sessions[shareId.value] : undefined))

// 中继重连中（注册阶段连接失败无限退避重试，须向用户透出而非静默等待）
const relayReconnecting = computed(
  (): boolean => publishing.value?.phase === 'registering' && session.value?.state === 'reconnecting'
)

// 取消按钮可用性：发布运行中（收集/注册阶段）
const canCancel = computed(() => shareId.value !== null && publishing.value?.status === 'running')

// ExpireSeconds 映射：-1=中继默认；0=无限期；>0=自定义秒
const expireSeconds = computed<number>(() => {
  if (expireMode.value === 'never') return 0
  if (expireMode.value === 'custom') return Math.max(1, Math.floor(customDays.value)) * 86400
  return -1
})

async function loadRelayAddress(): Promise<void> {
  try {
    const res = await settingsGetSettings()
    if (!res.success || !res.data) return
    const ss = res.data.shareSettings as { relayAddress?: string } | undefined
    relayAddress.value = ss?.relayAddress ?? ''
  } catch (e) {
    console.warn('读取分享中继地址失败', e)
  }
}

async function handleStart(): Promise<void> {
  if (starting.value || shareId.value) return
  starting.value = true
  startError.value = ''
  try {
    const id = await sharePublish(props.workIds, props.workSetIds, {
      title: title.value,
      expireSeconds: expireSeconds.value,
      password: password.value
    })
    shareId.value = id
    shareStore.markPublishStarted(id)
  } catch (e) {
    startError.value = e instanceof Error ? e.message : String(e)
  } finally {
    starting.value = false
  }
}

async function handleCancel(): Promise<void> {
  if (!shareId.value) return
  try {
    await shareCancelPublish(shareId.value)
  } catch {
    // 取消失败（发布可能已结束）：静默，等待终态事件收尾
  }
}

// 复制分享链接（含 fragment 密钥——链接即访问凭证，仅发给信任的收件人）
async function handleCopyLink(): Promise<void> {
  const link = publishing.value?.link
  if (!link) return
  if (await copyText(link)) {
    ElMessage.success('链接已复制')
  } else {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

function handleClose(): void {
  if (shareId.value) {
    shareStore.clearPublishing(shareId.value)
  }
  shareId.value = null
  startError.value = ''
  state.value = false
}

// 打开弹窗：复位配置并载入中继地址，等待用户点 [开始分享]
watch(state, (open) => {
  if (!open) return
  shareId.value = null
  startError.value = ''
  title.value = ''
  expireMode.value = 'default'
  customDays.value = 7
  password.value = ''
  relayAddress.value = ''
  void loadRelayAddress()
})
</script>

<template>
  <el-dialog
    v-model="state"
    title="分享"
    width="480px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
    append-to-body
  >
    <div v-if="starting" class="share-dialog-body">
      <div class="share-dialog-loading">
        <el-icon class="is-loading" :size="24"><Loading /></el-icon>
      </div>
      <div class="share-dialog-hint">正在准备分享…</div>
    </div>

    <div v-else-if="startError" class="share-dialog-body">
      <div class="share-dialog-status share-dialog-status-fail">
        分享启动失败
      </div>
      <div class="share-dialog-msg">{{ startError }}</div>
      <div class="share-dialog-footer">
        <el-button type="primary" @click="handleClose">
          关闭
        </el-button>
      </div>
    </div>

    <div v-else-if="publishing" class="share-dialog-body">
      <template v-if="publishing.status === 'running'">
        <div class="share-dialog-loading">
          <el-icon class="is-loading" :size="24"><Loading /></el-icon>
        </div>
        <div class="share-dialog-status">
          {{ publishing.phase === 'collecting' ? '正在收集分享数据…' : '正在注册到中继…' }}
        </div>
        <div v-if="relayReconnecting" class="share-dialog-warn">
          中继 {{ session?.relayAddress }} 连接失败，重连中——请检查分享中继设置与网络
        </div>
        <div class="share-dialog-footer">
          <el-button :disabled="!canCancel" @click="handleCancel">
            取消
          </el-button>
        </div>
      </template>
      <template v-else-if="publishing.status === 'success'">
        <div class="share-dialog-status share-dialog-status-done">
          分享已创建
        </div>
        <div class="share-dialog-detail">
          把链接发给收件人（链接含访问密钥，请勿公开传播）：
        </div>
        <div class="share-dialog-link">
          {{ publishing.link }}
        </div>
        <el-alert
          class="share-dialog-alert"
          type="info"
          :closable="false"
          show-icon
        >
          收件人仅能在本应用运行期间拉取：关闭本应用后链接临时失效，有效期内重新打开应用自动恢复分享；
          有效期到期后中继也会拒绝访问。
        </el-alert>
        <div class="share-dialog-footer">
          <el-button @click="handleCopyLink">
            复制链接
          </el-button>
          <el-button type="primary" @click="handleClose">
            完成
          </el-button>
        </div>
      </template>
      <template v-else>
        <div class="share-dialog-status share-dialog-status-fail">
          分享失败
        </div>
        <div class="share-dialog-msg">{{ publishing.errMsg || '分享失败' }}</div>
        <div class="share-dialog-footer">
          <el-button type="primary" @click="handleClose">
            关闭
          </el-button>
        </div>
      </template>
    </div>

    <div v-else-if="inConfig" class="share-dialog-body">
      <div class="share-dialog-config-label">
        标题
      </div>
      <el-input
        v-model="title"
        maxlength="200"
        placeholder="默认：分享 N 个作品/作品集"
        clearable
      />
      <div class="share-dialog-config-label">
        有效期
      </div>
      <el-radio-group v-model="expireMode">
        <el-radio-button value="default">
          默认 7 天
        </el-radio-button>
        <el-radio-button value="custom">
          自定义
        </el-radio-button>
        <el-radio-button value="never">
          无限期
        </el-radio-button>
      </el-radio-group>
      <div v-if="expireMode === 'custom'" class="share-dialog-config-row">
        <el-input-number
          v-model="customDays"
          :min="1"
          :max="3650"
          :step="1"
          step-strictly
        />
        <span class="share-dialog-hint-inline">天</span>
      </div>
      <div class="share-dialog-config-label">
        访问密码（可选）
      </div>
      <el-input
        v-model="password"
        placeholder="留空则任何人持链接可访问"
        show-password
        clearable
      />
      <div class="share-dialog-config-label">
        中继
      </div>
      <div class="share-dialog-relay">
        {{ relayAddress || '未配置（请在设置页配置分享中继地址）' }}
      </div>
      <el-alert
        class="share-dialog-alert"
        type="info"
        :closable="false"
        show-icon
      >
        分享内容端到端加密，中继无法读取；应用需保持运行收件人才能拉取，关闭后有效期内重开自动恢复。
      </el-alert>
      <div class="share-dialog-footer">
        <el-button @click="handleClose">
          取消
        </el-button>
        <el-button type="primary" @click="handleStart">
          开始分享
        </el-button>
      </div>
    </div>

    <div v-else-if="!hasSelection" class="share-dialog-body">
      <div class="share-dialog-status share-dialog-status-fail">
        未选择任何作品或作品集
      </div>
      <div class="share-dialog-footer">
        <el-button type="primary" @click="handleClose">
          关闭
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.share-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.share-dialog-status {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.share-dialog-status-done {
  color: var(--app-status-share-online-text);
}

.share-dialog-status-fail {
  color: var(--app-status-task-failed-text);
}

/* 阶段进行中的加载图标（收集/注册阶段无百分比语义，用旋转图标代替进度条） */
.share-dialog-loading {
  display: flex;
  justify-content: center;
  padding: 4px 0;
  color: var(--app-text-secondary);
}

/* 中继重连提示（注册阶段连接失败的透出） */
.share-dialog-warn {
  font-size: 13px;
  color: var(--app-status-warn-text);
  word-break: break-all;
}

.share-dialog-detail {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.share-dialog-msg {
  font-size: 13px;
  color: var(--app-status-task-failed-text);
  word-break: break-all;
}

.share-dialog-link {
  padding: 8px 10px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background-color: var(--app-fill-color-light);
  font-family: monospace;
  font-size: 12px;
  color: var(--app-text-primary);
  word-break: break-all;
  user-select: text;
}

.share-dialog-relay {
  font-size: 13px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.share-dialog-hint {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.share-dialog-hint-inline {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.share-dialog-config-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.share-dialog-config-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.share-dialog-alert {
  margin-top: 2px;
}

.share-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
</style>
