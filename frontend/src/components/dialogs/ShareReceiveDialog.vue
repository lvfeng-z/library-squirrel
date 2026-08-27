<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { shareReceive } from '@renderer/apis/http/wrappers/share'
import { useShareReceiveStore } from '@renderer/store/UseShareReceiveStore'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil'
import { askGotoPage } from '@renderer/utils/PageUtil'
import type GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { PageEnum } from '@renderer/model/constant/PageEnum.ts'

// 接收分享对话框（MainLayout 挂载，深链到达/手动入口共用）：
// 粘贴或预填分享链接 → 可选访问密码 → 创建 share-receive 任务（进度/终态由任务面板承载）。
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
    starting.value = false
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

// 启动拉取：创建并启动 share-receive 任务，成功后引导前往任务面板查看进度
async function handleStart(): Promise<void> {
  if (starting.value || isBlank(link.value)) return
  starting.value = true
  startError.value = ''
  try {
    await shareReceive(link.value.trim(), password.value)
    ElMessage.success('已创建拉取任务')
    visible.value = false
    askGotoPage({
      title: '拉取分享',
      content: '拉取任务已创建，是否前往任务面板查看进度？拉取完成后内容自动入库。',
      options: { confirmButtonText: '前往查看' },
      page: PageEnum.TaskManage
    } satisfies GotoPageConfig)
  } catch (e) {
    startError.value = e instanceof Error ? e.message : '启动拉取失败'
  } finally {
    starting.value = false
  }
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
        type="error"
        :closable="false"
        show-icon
      >
        {{ startError }}
      </el-alert>
    </div>

    <template #footer>
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
</style>
