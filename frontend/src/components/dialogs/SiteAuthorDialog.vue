<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { localAuthorApi, siteApi, siteAuthorApi } from '@renderer/apis/http'
import IPage from '@renderer/model/util/IPage.ts'
import {SelectItem, SiteAuthorDTO, SiteAuthorLocalRelateDTO} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import {SiteQueryDTO} from "@bindings/github.com/library-squirrel/wails/internal/site/models"
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model"
import {newPage} from "@renderer/utils/Pager.ts"

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
async function localAuthorQuerySelectItemPageAdapter(page: IPage<SelectItem>, input: string): Promise<IPage<SelectItem>> {
  const response = await localAuthorApi.localAuthorQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: { authorName: input }
  })
  if (!response.success || !response.data) {
    return newPage<SelectItem>()
  }
  return {
    pageNumber: response.data.pageNumber,
    pageSize: response.data.pageSize,
    pageCount: response.data.pageCount,
    dataCount: response.data.dataCount,
    currentCount: response.data.currentCount,
    data: response.data.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}

async function siteQuerySelectItemPageAdapter(page: IPage<SelectItem>, _input: string): Promise<IPage<SelectItem>> {
  const pageArg = new Page<SelectItem>({pageNumber: page.pageNumber, pageSize: page.pageSize})
  const response = await siteApi.siteQuerySelectItemPage(pageArg, new SiteQueryDTO({}))
  if (!response.success || !response.data) {
    return newPage<SelectItem>()
  }
  return {
    pageNumber: response.data.pageNumber,
    pageSize: response.data.pageSize,
    pageCount: response.data.pageCount,
    dataCount: response.data.dataCount,
    currentCount: response.data.currentCount,
    data: response.data.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    if (props.mode === DialogMode.NEW) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const authorDTO = new SiteAuthorDTO({
        authorName: tempFormData.siteAuthor?.authorName || null,
        introduce: tempFormData.siteAuthor?.introduce || null,
        siteId: tempFormData.siteAuthor?.siteId || null,
        fixedAuthorName: tempFormData.siteAuthor?.fixedAuthorName || null
      })
      const response = await apis.siteAuthorSave(authorDTO)
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
    if (props.mode === DialogMode.EDIT) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const authorDTO = new SiteAuthorDTO({
        id: tempFormData.siteAuthor?.id,
        authorName: tempFormData.siteAuthor?.authorName || null,
        introduce: tempFormData.siteAuthor?.introduce || null,
        localAuthorId: tempFormData.localAuthor?.id || null,
        siteId: tempFormData.siteAuthor?.siteId || null,
        fixedAuthorName: tempFormData.siteAuthor?.fixedAuthorName || null
      })
      const response = await apis.siteAuthorUpdateById(authorDTO)
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
            <el-input v-model="formData.siteAuthor!.authorName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="固定名称">
            <el-input v-model="formData.siteAuthor!.fixedAuthorName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="介绍">
            <el-input v-model="formData.siteAuthor!.introduce" type="textarea" autosize></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="本地作者">
            <auto-load-select
              v-model="formData.siteAuthor!.localAuthorId"
              :load="localAuthorQuerySelectItemPageAdapter"
              remote
              filterable
              clearable
            >
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.localAuthor)"
                  :hidden="true"
                  :value="formData.localAuthor!.id"
                  :label="formData.localAuthor!.authorName"
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
            <auto-load-select v-model="formData.siteAuthor!.siteId" :load="siteQuerySelectItemPageAdapter" remote filterable clearable>
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.site)"
                  :hidden="true"
                  :value="formData.site!.id"
                  :label="formData.site!.siteName"
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
            <el-date-picker v-model="formData.siteAuthor!.createTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker v-model="formData.siteAuthor!.updateTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped></style>
