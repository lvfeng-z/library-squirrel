<script setup lang="ts">
import { computed, Ref, ref, watch } from 'vue'
import { type PluginCandidate } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'

// model
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// props
const props = defineProps<{
  /** 候选清单，顺序即展示序——首位为默认选中项（由后端按复合全键字典序排定，此处不再重排） */
  candidates: PluginCandidate[]
  /** 引导文案（可选）：候选按站点分组逐个询问时由调用方点明本次问的是哪个站点，缺省用通用文案 */
  tip?: string
}>()

// 事件
const emits = defineEmits<{
  (e: 'confirm', candidate: PluginCandidate): void
  (e: 'cancel'): void
}>()

// 变量
// 选中项下标（候选清单内位置，不参与重排）
const selectedIndex: Ref<number> = ref(0)
// 确认关闭标记：确认已带出选择，随后的关闭事件不再上报取消
let confirmed = false

// 候选清单（剔除后端可空元素）
const candidateList = computed<PluginCandidate[]>(() => props.candidates.filter(notNullish))
// 当前选中项（确认后原样带回两键）
const selectedCandidate = computed<PluginCandidate | undefined>(() => candidateList.value[selectedIndex.value])

// 方法
// 候选展示名：插件名，缺名回落插件公开 ID
function candidateName(candidate: PluginCandidate): string {
  return isNotBlank(candidate.pluginName) ? candidate.pluginName : candidate.pluginPublicId
}

// 每次打开重置选中的候选（首位即默认选中项）与确认标记：连续询问多组时确认后随即再次打开，
// 关闭事件的到达可能落在再次打开之后，标记一律在打开时清零
watch(state, (visible) => {
  if (!visible) {
    return
  }
  selectedIndex.value = 0
  confirmed = false
})

// 确认：带出选中候选并关闭
function handleConfirm() {
  const candidate = selectedCandidate.value
  if (isNullish(candidate)) {
    return
  }
  confirmed = true
  state.value = false
  emits('confirm', candidate)
}

// 取消：仅关闭，由调用方收口（不重发）
function handleCancel() {
  state.value = false
}

// 关闭（取消按钮/右上角/ESC）：非确认路径一律上报取消
function handleClosed() {
  if (confirmed) {
    confirmed = false
    return
  }
  emits('cancel')
}
</script>

<template>
  <el-dialog
    v-model="state"
    title="选择处理插件"
    width="480px"
    :close-on-click-modal="false"
    @closed="handleClosed"
  >
    <div class="plugin-candidate-select-tip">
      {{ isNotBlank(props.tip) ? props.tip : '该操作可由多个插件处理，请选择本次使用的插件' }}
    </div>
    <el-radio-group
      v-model="selectedIndex"
      class="plugin-candidate-select-group"
    >
      <el-radio
        v-for="(candidate, index) in candidateList"
        :key="index"
        :value="index"
        class="plugin-candidate-select-item"
      >
        <span class="plugin-candidate-select-name">{{ candidateName(candidate) }}</span>
        <span
          v-if="isNotBlank(candidate.extensionId)"
          class="plugin-candidate-select-extension"
        >{{ candidate.extensionId }}</span>
      </el-radio>
    </el-radio-group>
    <template #footer>
      <el-button @click="handleCancel">
        取消
      </el-button>
      <el-button
        type="primary"
        :disabled="isNullish(selectedCandidate)"
        @click="handleConfirm"
      >
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.plugin-candidate-select-tip {
  margin-bottom: 12px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.plugin-candidate-select-group {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  width: 100%;
}

.plugin-candidate-select-item {
  width: 100%;
  height: auto;
  padding: 6px 0;
  margin-right: 0;
}

.plugin-candidate-select-name {
  color: var(--app-text-primary);
}

/* 扩展点标识：同插件多扩展点分列候选时供区分 */
.plugin-candidate-select-extension {
  margin-left: 8px;
  font-size: 12px;
  color: var(--app-text-secondary);
}
</style>
