<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import ApiUtil from '@renderer/utils/ApiUtil'
import lodash from 'lodash'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import {isNullish, notNullish} from '@renderer/utils/CommonUtil.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { localTagQuerySelectItemPageByName, siteApi, siteTagApi } from '@renderer/apis/http'
import IPage from '@renderer/model/util/IPage.ts'
import {
  SelectItem,
  SiteDTO,
  SiteTagDTO,
  SiteTagLocalRelateDTO
} from "@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto"
import {SiteQueryDTO} from "@bindings/github.com/library-squirrel/backend/site"
import {QueryAttribute} from "@bindings/github.com/library-squirrel/backend/base/query"
import {isBlank} from "@renderer/utils/StringUtil.ts"
import {ElMessage} from 'element-plus'

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
const formData = defineModel<SiteTagLocalRelateDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 事件
const emits = defineEmits(['requestSuccess'])

// 适配器函数：站点查询
async function siteQuerySelectItemPageAdapter(page: IPage<SelectItem>, input: string): Promise<IPage<SelectItem>> {
  const query = new SiteQueryDTO({siteName: isBlank(input) ? undefined : new QueryAttribute({value: input})})
  const response = await siteApi.siteQuerySelectItemPage(page, query)
  return response.data
}

// 方法
// 处理保存按钮点击事件
async function handleSaveButtonClicked() {
  if (props.submitEnabled) {
    try {
      const tempFormData = lodash.cloneDeep(formData.value)
      if (isNullish(tempFormData.siteTag)) {
        return
      }
      if (props.mode === DialogMode.NEW) {
        const response = await siteTagApi.siteTagSave(tempFormData.siteTag)
        ApiUtil.msg(response)
      }
      if (props.mode === DialogMode.EDIT) {
        const response = await siteTagApi.siteTagUpdateById(tempFormData.siteTag)
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
            <el-input v-model="formData.siteTag!.siteTagName"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="描述">
            <el-input v-model="formData.siteTag!.description" type="textarea"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="本地标签">
            <auto-load-select v-model:data="formData.siteTag!.localTagId" :load="localTagQuerySelectItemPageByName" remote filterable clearable>
              <template #default="{ list }">
                <el-option
                  v-if="notNullish(formData.localTag)"
                  :hidden="true"
                  :value="formData.localTag.id"
                  :label="formData.localTag.localTagName"
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
            <auto-load-select v-model:data="formData.siteTag!.siteId" :load="siteQuerySelectItemPageAdapter" remote filterable clearable>
              <template #default="{ list }">
                <el-option
                  :value="formData.site?.id"
                  :label="formData.site?.siteName"
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
            <el-date-picker v-model="formData.siteTag!.createTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="修改时间">
            <el-date-picker v-model="formData.siteTag!.updateTime" type="datetime" value-format="x" disabled></el-date-picker>
          </el-form-item>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped></style>
