<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { workSetCreate } from '@renderer/apis/http/wrappers/workSet'
import { isBlank } from '@renderer/utils/StringUtil.ts'

// model
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// emits
// created: 作品集已创建（上抛供持有方刷新作品集列表）
const emits = defineEmits<{
  created: []
}>()

// 变量
// 作品名称（必填，落本地昵称——本地手建集无站点来源）
const nickName = ref('')
// 本地描述（可选，与站点简介分离，重新抓取不会被覆盖）
const description = ref('')

// 名称必填校验
const canSubmit = computed(() => !isBlank(nickName.value))

// 方法
async function handleCreate() {
  if (!canSubmit.value) {
    ElMessage.warning('请输入作品集名称')
    return
  }
  const response = await workSetCreate({ nickName: nickName.value, description: description.value })
  if (!response.success) {
    ElMessage.error(response.msg || '创建失败')
    return
  }
  ElMessage.success('创建成功')
  nickName.value = ''
  description.value = ''
  state.value = false
  emits('created')
}
</script>

<template>
  <el-dialog
    v-model="state"
    title="新建作品集"
    width="420px"
    :close-on-click-modal="false"
  >
    <el-form label-position="top">
      <el-form-item label="名称" required>
        <el-input
          v-model="nickName"
          placeholder="请输入作品集名称"
          maxlength="200"
          @keyup.enter="handleCreate"
        />
      </el-form-item>
      <el-form-item label="本地描述">
        <el-input
          v-model="description"
          type="textarea"
          :rows="3"
          placeholder="请输入本地描述（可选，与站点简介分离，重新抓取不会被覆盖）"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="state = false">取消</el-button>
      <el-button
        type="primary"
        :disabled="!canSubmit"
        @click="handleCreate"
      >
        创建
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped></style>
