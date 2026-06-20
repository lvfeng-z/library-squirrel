<script setup lang="ts">
import BaseView from './BaseView.vue'
import { onBeforeMount, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { recycleBinApi } from '@renderer/apis/http'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import type { RecycleItemDTO } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import type { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'

const items = ref<RecycleItemDTO[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const loading = ref(false)

onBeforeMount(() => {
  loadPage()
})

async function loadPage() {
  loading.value = true
  const response = await recycleBinApi.recycleBinPage(currentPage.value, pageSize.value)
  if (ApiUtil.check(response)) {
    const pageData = ApiUtil.data<Page<RecycleItemDTO>>(response)
    items.value = pageData?.data ?? []
    total.value = pageData?.dataCount ?? 0
  } else {
    ElMessage.error(response?.msg ?? '查询失败')
  }
  loading.value = false
}

async function handlePageChange(page: number) {
  currentPage.value = page
  await loadPage()
}

function formatTime(time: number): string {
  if (!time) return ''
  return new Date(time).toLocaleString()
}

async function restore(item: RecycleItemDTO) {
  const response = await recycleBinApi.recycleBinRestore(item.id, false)
  if (ApiUtil.check(response)) {
    ElMessage.success('复原成功')
    await loadPage()
    return
  }
  // 后端仅冲突时返回 ErrRestoreConflict（消息含“冲突”/“已存在”）；其他错误直接展示，不误弹覆盖框
  const failMsg = response?.msg ?? '复原失败'
  if (!failMsg.includes('冲突') && !failMsg.includes('已存在')) {
    ElMessage.error(failMsg)
    return
  }
  try {
    await ElMessageBox.confirm(`${failMsg}\n是否覆盖已存在的作品？`, '复原冲突', {
      confirmButtonText: '覆盖',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const overwriteResponse = await recycleBinApi.recycleBinRestore(item.id, true)
    if (ApiUtil.check(overwriteResponse)) {
      ElMessage.success('复原成功')
      await loadPage()
    } else {
      ElMessage.error(overwriteResponse?.msg ?? '复原失败')
    }
  } catch {
    // 用户取消
  }
}

async function purge(item: RecycleItemDTO) {
  try {
    await ElMessageBox.confirm('彻底删除后不可恢复，是否继续？', '彻底删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const response = await recycleBinApi.recycleBinPurge(item.id)
    if (ApiUtil.check(response)) {
      ElMessage.success('已彻底删除')
      await loadPage()
    } else {
      ElMessage.error(response?.msg ?? '删除失败')
    }
  } catch {
    // 用户取消
  }
}
</script>

<template>
  <base-view>
    <template #default>
      <el-table :data="items" v-loading="loading" style="width: 100%">
        <el-table-column label="作品名" prop="workName" show-overflow-tooltip />
        <el-table-column label="删除时间" width="200">
          <template #default="{ row }">
            {{ formatTime(row.deleteTime) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="restore(row)">复原</el-button>
            <el-button size="small" type="danger" @click="purge(row)">彻底删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        :current-page="currentPage"
        :page-size="pageSize"
        :total="total"
        layout="total, prev, pager, next"
        @current-change="handlePageChange"
      />
    </template>
  </base-view>
</template>
