<script setup lang="ts">
import { ref } from 'vue'
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import AvatarThumb from '@renderer/components/common/AvatarThumb.vue'
import { authorInfoApi, fileSysUtilApi, localAuthorApi } from '@renderer/apis/http'
import { LocalAuthorFullDTO } from "@bindings/github.com/library-squirrel/backend/base/model/dto"
import { ElMessage, ElMessageBox } from 'element-plus'
import { arrayNotEmpty, isNullish } from '@renderer/utils/CommonUtil.ts'

// props
const props = withDefaults(
  defineProps<{
    mode: DialogMode
    submitEnabled?: boolean
  }>(),
  {
    submitEnabled: true
  }
)

// model
// 表单数据（宿主侧展示 DTO：SDK 实体 DTO + 头像展示路径）
const formData = defineModel<LocalAuthorFullDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 变量
// 头像上传/移除进行中（按钮 loading）
const avatarOpLoading = ref(false)
// 头像图片后端接受的格式白名单（与后端 SetLocalAuthorAvatar 白名单一致）
const AVATAR_FILE_FILTER = [{ DisplayName: '图片', Pattern: '*.jpg;*.jpeg;*.png;*.gif;*.webp;*.bmp' }]

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    try {
      const tempFormData = lodash.cloneDeep(formData.value)
      if (props.mode === DialogMode.NEW) {
        const response = await localAuthorApi.localAuthorSave(tempFormData.author)
        ApiUtil.msg(response)
      }
      if (props.mode === DialogMode.EDIT) {
        const response = await localAuthorApi.localAuthorUpdateById(tempFormData.author)
        ApiUtil.msg(response)
      }
      emits('requestSuccess')
      state.value = false
    } catch (e) {
      ElMessage.error((e as Error).message)
    }
  }
}
// 处理上传头像按钮点击事件：文件选择对话框取本地图片绝对路径，交后端拷入工作目录并四调用入库
// （换头像形态后端先删旧，失败不伤现有头像）
async function handleUploadAvatarClicked() {
  const id = formData.value.author?.id
  if (isNullish(id) || id <= 0) {
    ElMessage.warning('新建作者须先保存，才能设置头像')
    return
  }
  const response = await fileSysUtilApi.fileSysUtilSelectFile('选择头像图片', undefined, AVATAR_FILE_FILTER)
  if (!ApiUtil.check(response)) {
    return
  }
  const selectResult = ApiUtil.data<{ canceled: boolean; filePaths: string[] }>(response)
  if (isNullish(selectResult) || selectResult.canceled || !arrayNotEmpty(selectResult.filePaths)) {
    return
  }
  avatarOpLoading.value = true
  try {
    await authorInfoApi.authorInfoSetLocalAuthorAvatar(id, selectResult.filePaths[0])
    ElMessage.success('头像已更新')
    emits('requestSuccess')
    await refreshAvatarOnly()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    avatarOpLoading.value = false
  }
}
// 处理移除头像按钮点击事件（显式破坏操作：头像文件从库中删除，二次确认）
async function handleRemoveAvatarClicked() {
  const id = formData.value.author?.id
  if (isNullish(id) || id <= 0) {
    return
  }
  const confirmed = await ElMessageBox.confirm(
    '移除后头像文件将从库中删除（不可恢复），是否继续？',
    '移除头像',
    { confirmButtonText: '移除', cancelButtonText: '取消', type: 'warning' }
  ).then(() => true).catch(() => false)
  if (!confirmed) {
    return
  }
  avatarOpLoading.value = true
  try {
    await authorInfoApi.authorInfoRemoveLocalAuthorAvatar(id)
    ElMessage.success('头像已移除')
    emits('requestSuccess')
    await refreshAvatarOnly()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    avatarOpLoading.value = false
  }
}
// 头像操作成功后按 ID 回查最新展示 DTO，仅同步头像路径字段——不整对象覆盖表单，
// 避免冲掉用户未保存的名称/介绍编辑
async function refreshAvatarOnly() {
  const id = formData.value.author?.id
  if (isNullish(id)) {
    return
  }
  try {
    const response = await localAuthorApi.localAuthorGetById(id)
    formData.value.avatarFilePath = response.data?.avatarFilePath ?? null
  } catch (e) {
    // 回查失败不阻断流程：表格刷新后仍会带来最新头像
    console.warn('回查本地作者头像失败', e)
  }
}
</script>

<template>
  <form-dialog
    v-model:form-data="formData"
    v-model:state="state"
    :mode="props.mode"
    @save-button-clicked="handleSaveButtonClicked"
  >
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="头像">
            <!-- 本地作者头像大图：经 /store/ 通道展示，无头像/加载失败由组件降级为占位图标 -->
            <div class="local-author-dialog-avatar">
              <avatar-thumb
                :file-path="formData.avatarFilePath"
                :size="80"
              />
              <div
                v-if="props.mode === DialogMode.EDIT"
                class="local-author-dialog-avatar-actions"
              >
                <el-button
                  size="small"
                  :loading="avatarOpLoading"
                  @click="handleUploadAvatarClicked"
                >
                  上传头像
                </el-button>
                <el-button
                  size="small"
                  type="danger"
                  class="tone-fail"
                  :loading="avatarOpLoading"
                  :disabled="isNullish(formData.avatarFilePath)"
                  @click="handleRemoveAvatarClicked"
                >
                  移除头像
                </el-button>
              </div>
            </div>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.author.authorName" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="介绍">
            <el-input
              v-model="formData.author.introduce"
              type="textarea"
              autosize
            />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col :span="12">
          <el-form-item label="创建时间">
            <el-date-picker
              v-model="formData.author.createTime"
              type="datetime"
              value-format="x"
              disabled
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker
              v-model="formData.author.updateTime"
              type="datetime"
              value-format="x"
              disabled
            />
          </el-form-item>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped>
.local-author-dialog-avatar {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.local-author-dialog-avatar-actions {
  display: flex;
  gap: 8px;
}
</style>
