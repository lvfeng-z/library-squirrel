<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import Site from '@renderer/model/model/entity/Site.ts'
import { siteApi } from '@renderer/apis/http'

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
const formData = defineModel<Site>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 变量
// 接口
const apis = {
  siteSave: siteApi.siteSave,
  siteUpdateById: siteApi.siteUpdateById
}

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    if (props.mode === DialogMode.NEW) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.siteSave({
        name: tempFormData.siteName ?? undefined,
        url: tempFormData.homepage ?? undefined
      })
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
    if (props.mode === DialogMode.EDIT) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.siteUpdateById({
        id: tempFormData.id ?? 0,
        name: tempFormData.siteName ?? undefined,
        url: tempFormData.homepage ?? undefined
      })
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
  }
}
</script>

<template>
  <form-dialog v-model:form-data="formData" v-model:state="state" :mode="props.mode" @save-button-clicked="handleSaveButtonClicked">
    <template #header>
      <span style="font-size: 20px">站点</span>
    </template>
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.siteName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="描述">
            <el-input v-model="formData.siteDescription" type="textarea" autosize></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col :span="12">
          <el-form-item label="创建时间">
            <el-date-picker v-model="formData.createTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker v-model="formData.updateTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped></style>
