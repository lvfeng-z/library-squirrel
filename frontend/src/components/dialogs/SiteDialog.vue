<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import {ElMessage} from 'element-plus'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { siteApi } from '@renderer/apis/http'
import { SiteDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'

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
// 表单数据
const formData = defineModel<SiteDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    try {
      if (props.mode === DialogMode.EDIT) {
        const tempFormData = lodash.cloneDeep(formData.value)
        const siteDTO = new SiteDTO({
          id: tempFormData.id,
          siteKey: tempFormData.siteKey,
          siteName: tempFormData.siteName ?? null,
          homepage: tempFormData.homepage ?? null
        })
        const response = await siteApi.siteUpdateById(siteDTO)
        ApiUtil.msg(response)
        emits('requestSuccess')
        state.value = false
      }
    } catch (e) {
      ElMessage.error((e as Error).message)
    }
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
    <template #header>
      <span style="font-size: 20px">站点</span>
    </template>
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="站点键">
            <el-input
              v-model="formData.siteKey"
              disabled
              placeholder="站点唯一身份键（SDK identity 注册表分配）"
            />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.siteName" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col :span="12">
          <el-form-item label="创建时间">
            <el-date-picker
              v-model="formData.createTime"
              type="datetime"
              value-format="x"
              disabled
            />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker
              v-model="formData.updateTime"
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

<style scoped></style>
