<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import {notNullish} from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { localTagApi, localTagQuerySelectItemPageByName } from '@renderer/apis/http'
import { LocalTagDTO, LocalTagWithBaseTagDTO } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import { ElMessage } from 'element-plus'

// props
const props = withDefaults(
  defineProps<{
    mode?: DialogMode
    submitEnabled?: boolean
  }>(),
  {
    mode: DialogMode.EDIT,
    submitEnabled: true
  }
)

// model
// 表单数据
const formData = defineModel<LocalTagWithBaseTagDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    try {
      const tempFormData = lodash.cloneDeep(formData.value)
      if (props.mode === DialogMode.NEW) {
        const response = await localTagApi.localTagSave(tempFormData.localTag ?? new LocalTagDTO())
        ApiUtil.msg(response)
      }
      if (props.mode === DialogMode.EDIT) {
        const response = await localTagApi.localTagUpdateById(tempFormData.localTag ?? new LocalTagDTO())
        ApiUtil.msg(response)
      }
      emits('requestSuccess')
      state.value = false
    } catch (e) {
      ElMessage.error((e as Error).message)
    }
  }
}
</script>

<template>
  <form-dialog v-model:form-data="formData" v-model:state="state" :mode="props.mode" @save-button-clicked="handleSaveButtonClicked">
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.localTag!.localTagName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="基础标签">
            <auto-load-select v-model="formData.localTag!.baseLocalTagId" :load="localTagQuerySelectItemPageByName" remote filterable clearable>
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.baseTag)"
                  :hidden="true"
                  :value="formData.baseTag.id"
                  :label="formData.baseTag.localTagName"
                ></el-option>
                <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
              </template>
            </auto-load-select>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col :span="12">
          <el-form-item label="创建时间">
            <el-date-picker v-model="formData.localTag!.createTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker v-model="formData.localTag!.updateTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped></style>
