<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { localAuthorQuerySelectItemPageByName } from '@renderer/apis/LocalAuthorApi.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/SiteApi.ts'
import SiteAuthorLocalRelateDTO from '@renderer/model/model/dto/SiteAuthorLocalRelateDTO.ts'
import { localAuthorApi, siteAuthorApi } from '@renderer/apis/http'

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

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    if (props.mode === DialogMode.NEW) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.siteAuthorSave(tempFormData)
      if (ApiUtil.check(response)) {
        emits('requestSuccess')
        state.value = false
      }
      ApiUtil.msg(response)
    }
    if (props.mode === DialogMode.EDIT) {
      const tempFormData = lodash.cloneDeep(formData.value)
      const response = await apis.siteAuthorUpdateById(tempFormData)
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
              :load="localAuthorQuerySelectItemPageByName"
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
            <auto-load-select v-model="formData.siteId" :load="siteQuerySelectItemPageBySiteName" remote filterable clearable>
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
