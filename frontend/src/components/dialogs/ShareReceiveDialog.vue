<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { shareReceive } from '@renderer/apis/http/wrappers/share'
import { useShareReceiveStore } from '@renderer/store/UseShareReceiveStore'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil.ts'
import { gotoPage } from '@renderer/utils/PageUtil'
import { PageEnum } from '@renderer/model/constant/PageEnum.ts'
import type { ShareReceiveResult } from '@bindings/github.com/library-squirrel/backend/share/models'

// 接收分享对话框（MainLayout 挂载，深链到达/手动入口共用）：
// 粘贴或预填分享链接 → 可选访问密码 → 创建 share-receive 父子任务树（父任务聚合 + 每作品一子任务；
// 成功态展示作品名列表——决策4 之①，进度/终态由任务面板承载）。
// 支持两种链接形态：library-squirrel://share/{中继}/{token}#k={密钥} 与
// https://{中继}/s/{token}#k={密钥}；完整校验在后端（错误信息在此内联展示）。

const receiveStore = useShareReceiveStore()

// 链接输入（打开时预填 initialLink——深链到达场景）
const link = ref('')
// 访问密码（分享设有密码时填写；空=无密码）
const password = ref('')
// 创建任务进行中（防重复触发）
const starting = ref(false)
// 前置错误（链接形态/密钥缺失等，后端校验返回）
const startError = ref('')
// 前置错误是否为自指拒绝（链接会话 token 命中本地分享记录=本实例自己分享的内容，
// 后端 share.ErrShareSelfReference 透传文案「不能接收自己分享的内容」，按文案包含匹配；
// true 时以警示态展示——属使用方式问题而非系统故障）
const isSelfReference = ref(false)
// 接收成功建树结果（非空=对话框切换为成功态：作品名列表 + 前往任务面板引导）
const result = ref<ShareReceiveResult | null>(null)

const visible = computed({
  get: (): boolean => receiveStore.visible,
  set: (v: boolean): void => {
    receiveStore.visible = v
  }
})

// 打开时复位表单并预填链接；深链到达可能在对话框挂载前（冷启动消费待处理链接）或
// 对话框已开时（运行中重复到达）发生，visible 无 false→true 跳变，故预填挂在
// initialLink 变化上并 immediate 覆盖挂载时已就位的链接
watch(visible, (v: boolean): void => {
  if (v) {
    password.value = ''
    startError.value = ''
    isSelfReference.value = false
    starting.value = false
    result.value = null
  }
})
watch(
  (): string => receiveStore.initialLink,
  (v: string): void => {
    if (receiveStore.visible && isNotBlank(v)) link.value = v
  },
  { immediate: true }
)

const canSubmit = computed((): boolean => isNotBlank(link.value) && !starting.value)

// 链接缺密钥即时提示（后端完整校验前的粗判：#k= fragment 缺失即无法解密——
// 落地页「打开应用」深链可能不带密钥，引导粘贴完整链接）
const missingKeyHint = computed((): boolean => {
  const l = link.value.trim()
  return l !== '' && !l.includes('#k=') && !l.includes('?k=')
})

// 启动拉取：创建并启动 share-receive 父子任务树，成功后对话框切成功态展示作品名列表
async function handleStart(): Promise<void> {
  if (starting.value || isBlank(link.value)) return
  starting.value = true
  startError.value = ''
  isSelfReference.value = false
  try {
    result.value = await shareReceive(link.value.trim(), password.value)
    ElMessage.success('已创建拉取任务')
  } catch (e) {
    const message = e instanceof Error ? e.message : '启动拉取失败'
    // 自指拒绝：本实例自己分享的内容不能在本机接收（不进入拉取流程）；
    // 跨设备接收自己的分享是合法场景、不受影响
    isSelfReference.value = message.includes('不能接收自己分享的内容')
    startError.value = message
  } finally {
    starting.value = false
  }
}

// 成功态：前往任务面板查看进度（父任务聚合 + 子任务作品名/状态/进度/操作）
async function goTaskPanel(): Promise<void> {
  visible.value = false
  result.value = null
  await gotoPage(PageEnum.TaskManage)
}

// 成功态：关闭（用户稍后自行到任务面板查看）
function closeAfterSuccess(): void {
  visible.value = false
  result.value = null
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="接收分享"
    width="560px"
    append-to-body
  >
    <div class="share-receive-body">
      <template v-if="result">
        <!-- 成功态：作品名列表（决策4 之①）——收件人创建任务即知分享内容 -->
        <el-alert
          class="share-receive-alert"
          type="success"
          :closable="false"
          show-icon
        >
          已创建拉取任务，共 {{ result.workCount }} 个作品。每个作品一个子任务，进度与结果在任务面板查看。
        </el-alert>
        <div class="share-receive-result-list">
          <div
            v-if="arrayNotEmpty(result.workNames)"
            class="share-receive-result-list-inner"
          >
            <div
              v-for="(name, index) in result.workNames"
              :key="index"
              class="share-receive-result-item"
            >
              <span class="share-receive-result-index">{{ index + 1 }}</span>
              <span class="share-receive-result-name">{{ name }}</span>
            </div>
          </div>
          <div v-else class="share-receive-result-empty">
            未获取到作品名
          </div>
        </div>
      </template>
      <template v-else>
        <el-alert
          class="share-receive-alert"
          type="info"
          :closable="false"
          show-icon
        >
          粘贴完整分享链接（含 #k= 解密密钥）；拉取需要分享方保持应用在线，
          内容端到端加密、中继无法读取。进度与结果在任务面板查看。
        </el-alert>

        <el-input
          v-model="link"
          type="textarea"
          :rows="3"
          placeholder="library-squirrel://share/{中继}/{token}#k={密钥} 或 https://{中继}/s/{token}#k={密钥}"
        />

        <el-alert
          v-if="missingKeyHint"
          class="share-receive-alert"
          type="warning"
          :closable="false"
          show-icon
        >
          链接缺少解密密钥（#k=…），无法解密内容——请向分享方索取完整链接（含 #k= 部分）后粘贴
        </el-alert>

        <el-input
          v-model="password"
          type="password"
          placeholder="访问密码（分享未设密码时留空）"
          show-password
          clearable
        />

        <el-alert
          v-if="startError"
          class="share-receive-alert"
          :type="isSelfReference ? 'warning' : 'error'"
          :closable="false"
          show-icon
        >
          {{ isSelfReference ? `${startError}（该分享由本机创建）` : startError }}
        </el-alert>
      </template>
    </div>

    <template #footer>
      <template v-if="result">
        <el-button @click="closeAfterSuccess">
          关闭
        </el-button>
        <el-button
          type="primary"
          @click="goTaskPanel"
        >
          前往任务面板查看进度
        </el-button>
      </template>
      <template v-else>
        <el-button @click="visible = false">
          取消
        </el-button>
        <el-button
          type="primary"
          :loading="starting"
          :disabled="!canSubmit"
          @click="handleStart"
        >
          开始拉取
        </el-button>
      </template>
    </template>
  </el-dialog>
</template>

<style scoped>
.share-receive-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.share-receive-alert {
  flex-shrink: 0;
}

.share-receive-result-list {
  max-height: 260px;
  overflow-x: hidden;
  overflow-y: auto;
  border: 1px solid var(--app-border-color-lighter);
  border-radius: var(--app-radius-sm);
  flex-shrink: 1;
}

.share-receive-result-list-inner {
  padding: 4px 0;
}

.share-receive-result-item {
  display: flex;
  flex-direction: row;
  align-items: center;
  padding: 6px 12px;
}

.share-receive-result-item + .share-receive-result-item {
  border-top: 1px solid var(--app-border-color-lighter);
}

.share-receive-result-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  width: 22px;
  height: 22px;
  margin-right: 10px;
  background-color: var(--app-fill-color-dark);
  border-radius: var(--app-radius-sm);
  flex-shrink: 0;
}

.share-receive-result-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.share-receive-result-empty {
  padding: 16px;
  text-align: center;
  color: var(--app-text-secondary);
}
</style>
