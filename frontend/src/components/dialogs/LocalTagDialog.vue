<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import LocalTagDTO from '@renderer/model/model/dto/LocalTagDTO.ts'
import { localTagApi } from '@renderer/apis/http'
import IPage from '@renderer/model/util/IPage.ts'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import Page from '@renderer/model/util/Page.ts'

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
const formData = defineModel<LocalTagDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 变量
// 接口
const apis = {
  localTagSave: localTagApi.localTagSave,
  localTagUpdateById: localTagApi.localTagUpdateById,
  localTagQuerySelectItemPage: localTagApi.localTagQuerySelectItemPage,
  localTagGetTree: localTagApi.localTagGetTree,
  localTagGetById: localTagApi.localTagGetById
}

// 适配器函数：将 bindings 的 Page<SelectItem, LocalTagQueryDTO> 转换为 IPage
async function localTagQuerySelectItemPageAdapter(page: IPage<unknown, SelectItem>, input: string): Promise<IPage<unknown, SelectItem>> {
  const response = await localTagApi.localTagQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: { localTagName: input }
  })
  if (!response.success || !response.data) {
    return new Page<unknown, SelectItem>()
  }
  // 将 bindings Page 转换为 IPage
  return {
    pageNumber: response.data.pageNumber,
    pageSize: response.data.pageSize,
    pageCount: response.data.pageCount,
    dataCount: response.data.dataCount,
    currentCount: response.data.currentCount,
    query: response.data.query,
    data: response.data.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    if (props.mode === DialogMode.NEW) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.localTagSave({
        localTagName: tempFormData.localTagName ?? undefined,
        baseLocalTagId: tempFormData.baseLocalTagId ?? undefined
      })
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
    if (props.mode === DialogMode.EDIT) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.localTagUpdateById({
        id: tempFormData.id ?? 0,
        localTagName: tempFormData.localTagName ?? undefined,
        baseLocalTagId: tempFormData.baseLocalTagId ?? undefined
      })
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
  }
}
// async function load(node, resolve) {
//   if (node.isLeaf) {
//     return resolve([])
//   }
//   const baseTagTreeResponse = await apis.localTagGetTree(node.data.id)
//   if (ApiUtil.check(baseTagTreeResponse)) {
//     const children = ApiUtil.data<TreeSelectNode[]>(baseTagTreeResponse)
//     children?.forEach((child) => {
//       child.isLeaf = Boolean(child.isLeaf)
//       if (formData.value.id === child.id) {
//         child.disabled = true
//       }
//     })
//     resolve(children)
//   } else {
//     return resolve([])
//   }
// }
</script>

<template>
  <form-dialog v-model:form-data="formData" v-model:state="state" :mode="props.mode" @save-button-clicked="handleSaveButtonClicked">
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.localTagName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="基础标签">
            <auto-load-select v-model="formData.baseLocalTagId" :load="localTagQuerySelectItemPageAdapter" remote filterable clearable>
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
