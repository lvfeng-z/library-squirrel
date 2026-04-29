<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import {PluginDTO} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import { ElMessage } from 'element-plus'
import { pluginApi } from '@renderer/apis/http'

// props
const props = withDefaults(
  defineProps<{
    mode?: DialogMode
    submitEnabled?: boolean
  }>(),
  {
    mode: DialogMode.VIEW,
    submitEnabled: true
  }
)

// model
// 表单数据
const formData = defineModel<PluginDTO>('formData', { required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 变量

// 方法
// 重新安装
async function reInstall(pluginPublicId: string | undefined | null) {
  if (!pluginPublicId) {
    ElMessage({ type: 'error', message: '插件ID不能为空' })
    return
  }
  try {
    await pluginApi.pluginReinstall(pluginPublicId)
    ElMessage({ type: 'success', message: '修复完成' })
  } catch (e) {
    ElMessage({ type: 'error', message: `修复失败，${(e as Error).message}` })
  }
}
// 卸载
async function unInstall(pluginPublicId: string | undefined | null) {
  if (!pluginPublicId) {
    ElMessage({ type: 'error', message: '插件ID不能为空' })
    return
  }
  try {
    await pluginApi.pluginUnInstall(pluginPublicId)
    ElMessage({ type: 'success', message: '已卸载' })
  } catch (e) {
    ElMessage({ type: 'error', message: `卸载失败，${(e as Error).message}` })
  }
}
</script>

<template>
  <form-dialog v-model:form-data="formData" v-model:state="state" :mode="props.mode">
    <template #header>
      <span style="font-size: 20px">站点</span>
    </template>
    <template #form>
      <el-row>
        <el-col>
          <el-form-item label="名称">
            <el-input v-model="formData.name"></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="描述">
            <el-input v-model="formData.description" type="textarea" autosize></el-input>
          </el-form-item>
        </el-col>
      </el-row>
      <el-row>
        <el-col>
          <el-form-item label="更新日志">
            <el-input v-model="formData.changelog" type="textarea" autosize></el-input>
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
    <template #footer>
      <el-row>
        <el-col :span="3">
          <el-button type="primary" @click="reInstall(formData.publicId)">修复</el-button>
        </el-col>
        <el-col :span="3">
          <el-button type="danger" @click="unInstall(formData.publicId)">卸载</el-button>
        </el-col>
        <el-col :span="3">
          <el-button @click="state = false">取消</el-button>
        </el-col>
      </el-row>
    </template>
  </form-dialog>
</template>

<style scoped></style>
