<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import SiteAuthorLocalRelateDTO from '@renderer/model/model/dto/SiteAuthorLocalRelateDTO.ts'
import { localAuthorApi, siteApi, siteAuthorApi } from '@renderer/apis/http'
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
const formData = defineModel<SiteAuthorLocalRelateDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 变量
// 接口
const apis = {
  localAuthorQuerySelectItemPage: localAuthorApi.localAuthorQuerySelectItemPage,
  siteAuthorSave: siteAuthorApi.siteAuthorSave,
  siteAuthorUpdateById: siteAuthorApi.siteAuthorUpdateById
}

// 适配器函数：将 bindings Page 转换为 IPage
async function localAuthorQuerySelectItemPageAdapter(page: IPage<unknown, SelectItem>, input: string): Promise<IPage<unknown, SelectItem>> {
  const response = await localAuthorApi.localAuthorQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: { authorName: input }
  })
  if (!response.success || !response.data) {
    return new Page<unknown, SelectItem>()
  }
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

async function siteQuerySelectItemPageAdapter(page: IPage<unknown, SelectItem>, _input: string): Promise<IPage<unknown, SelectItem>> {
  // 注意：siteName 过滤在 bindings 中未实现
  const response = await siteApi.siteQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize
  })
  if (!response.success || !response.data) {
    return new Page<unknown, SelectItem>()
  }
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
      const response = await apis.siteAuthorSave({
        authorName: tempFormData.authorName ?? undefined,
        introduce: tempFormData.introduce ?? undefined,
        siteId: tempFormData.siteId ?? undefined
      })
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
    if (props.mode === DialogMode.EDIT) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.siteAuthorUpdateById({
        id: tempFormData.id ?? 0,
        authorName: tempFormData.authorName ?? undefined,
        introduce: tempFormData.introduce ?? undefined,
        localAuthorId: tempFormData.localAuthor?.id ?? undefined
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
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.authorName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="固定名称">
            <el-input v-model="formData.fixedAuthorName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="介绍">
            <el-input v-model="formData.introduce" type="textarea" autosize></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="本地作者">
            <auto-load-select
              v-model="formData.localAuthorId"
              :load="localAuthorQuerySelectItemPageAdapter"
              remote
              filterable
              clearable
            >
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.localAuthor)"
                  :hidden="true"
                  :value="formData.localAuthor.id"
                  :label="formData.localAuthor.authorName"
                ></el-option>
                <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
              </template>
            </auto-load-select>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="站点">
            <auto-load-select v-model="formData.siteId" :load="siteQuerySelectItemPageAdapter" remote filterable clearable>
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.site)"
                  :hidden="true"
                  :value="formData.site.id"
                  :label="formData.site.siteName"
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
