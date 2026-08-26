<script setup lang="ts">
import BaseView from './BaseView.vue'
import SearchTable from '@renderer/components/common/SearchTable.vue'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { h, onMounted, Ref, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { backupGovernanceApi } from '@renderer/apis/http'
import { Thead } from '@renderer/model/util/Thead.ts'
import { newPage } from '@renderer/utils/Pager.ts'
import { BackupDTO, BackupStatsDTO } from '@bindings/github.com/library-squirrel/backend/backupGovernance/models'
import type { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

// 引用方观察阈值（天）：最老引用年龄超过即高亮——与后端监视哨 Warn 阈值同值
const REFERENCER_OBSERVE_DAYS = 90

// onMounted
onMounted(() => {
  backupManageSearchTable.value.doSearch()
  loadStats()
})

// 变量
// 备份清单表组件实例
const backupManageSearchTable = ref()
// 分页参数
const page: Ref<Page<BackupDTO>> = ref(newPage<BackupDTO>())
// 引用态筛选（字符串模型映射到 null=全部 / true=有主 / false=无主）
const referencedFilter = ref('all')
// 占用统计（统计卡区数据面，服务端短 TTL 缓存）
const stats: Ref<BackupStatsDTO | null> = ref(null)
// 立即巡检执行中
const reconciliationRunning = ref(false)

// 备份清单的表头（大小列只展示不排序——大小无库列，服务端排序需全量 Stat）
const backupThead: Ref<Thead<BackupDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'fileName',
    title: '文件名',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'filePath',
    title: '保管路径',
    hide: false,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'fileSize',
    title: '大小',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (data) => h('span', formatBytes(data as number))
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createTime',
    title: '保管时间',
    hide: false,
    width: 200,
    headerAlign: 'center',
    dataAlign: 'center'
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'referenced',
    title: '状态',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    render: (data) => h(StatusTag, { status: data === true ? 'backup-referenced' : 'backup-orphaned' })
  })
])

// 方法
// 引用态筛选值 → 后端过滤参数（null=全部）
function resolveReferenced(): boolean | null {
  if (referencedFilter.value === 'referenced') return true
  if (referencedFilter.value === 'orphaned') return false
  return null
}
// 分页查询备份清单
async function backupQueryPageFn(p: Page<BackupDTO>): Promise<Page<BackupDTO> | undefined> {
  const response = await backupGovernanceApi.backupGovernancePageBackups(p, resolveReferenced())
  return response.data
}
// 引用态筛选变化后重新查询
function handleReferencedFilterChange() {
  backupManageSearchTable.value.doSearch()
}
// 加载占用统计
async function loadStats() {
  const response = await backupGovernanceApi.backupGovernanceGetBackupStats()
  stats.value = response.data
}
// 刷新清单与统计（删除/巡检后）
async function refreshAll() {
  await Promise.all([backupManageSearchTable.value.doSearch(), loadStats()])
}
// 立即巡检：手动触发一轮双向对账（与定时巡检互斥），完成后刷新
async function runReconciliation() {
  reconciliationRunning.value = true
  try {
    const response = await backupGovernanceApi.backupGovernanceRunReconciliationNow()
    const result = response.data
    ElMessage.success(`巡检完成：清理无主备份 ${result.orphansCleaned} 份，清除悬空引用 ${result.danglingRefsCleared} 条`)
    await refreshAll()
  } catch (e) {
    ElMessage.error((e as Error).message ?? '巡检失败')
  } finally {
    reconciliationRunning.value = false
  }
}
// 清理全部无主：圈定「无主且超保留期」（与治理正向判据同源，防误杀替换在途还原点/崩溃窗口新孤儿）
async function cleanExpiredOrphans() {
  const ids = stats.value?.expiredOrphanIds ?? []
  if (ids.length === 0) {
    ElMessage.info('当前没有超保留期的无主备份')
    return
  }
  try {
    await ElMessageBox.confirm(`将删除 ${ids.length} 份超保留期的无主备份（不可恢复），是否继续？`, '清理无主备份', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await backupGovernanceApi.backupGovernanceDeleteBackups(ids)
    ElMessage.success(`已清理 ${ids.length} 份无主备份`)
    await refreshAll()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，文件删除失败询问仅删记录，其余直接展示
    if (e instanceof Error) {
      await handleDeleteFailure(e, ids)
    }
  }
}
// 删除单份备份（单行删除不限年龄，页面有保管时间列可判）
async function deleteBackup(row: BackupDTO) {
  try {
    await ElMessageBox.confirm(`删除备份「${row.fileName}」后不可恢复，是否继续？`, '删除备份', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await backupGovernanceApi.backupGovernanceDeleteBackups([row.id])
    ElMessage.success('已删除')
    await refreshAll()
  } catch (e) {
    // 确认框取消为字符串 reject，静默；接口失败为 Error，文件删除失败询问仅删记录，其余直接展示
    if (e instanceof Error) {
      await handleDeleteFailure(e, [row.id])
    }
  }
}
// 删除备份文件失败（被占用/只读等，后端已保留记录）：询问「仅删除记录」还是放弃；
// 其余错误直接展示。仅删记录走独立降级入口（不动磁盘文件，留用户手动处理）
async function handleDeleteFailure(e: Error, ids: number[]) {
  if (!e.message.includes('删除备份文件失败')) {
    ElMessage.error(e.message)
    return
  }
  try {
    await ElMessageBox.confirm(`${e.message}。是否仅删除记录（磁盘文件保留，请手动处理）？`, '删除备份', {
      confirmButtonText: '仅删除记录',
      cancelButtonText: '放弃',
      type: 'warning'
    })
  } catch {
    return // 用户放弃：记录保留
  }
  try {
    await backupGovernanceApi.backupGovernanceDeleteBackupRecords(ids)
    ElMessage.success('已仅删除记录')
    await refreshAll()
  } catch (err) {
    ElMessage.error((err as Error).message ?? '仅删除记录失败')
  }
}
// 字节量格式化（B/KB/MB/GB，一位小数）
function formatBytes(bytes: number | null | undefined): string {
  if (isNullish(bytes) || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = -1
  do {
    value /= 1024
    unit++
  } while (value >= 1024 && unit < units.length - 1)
  return `${value.toFixed(1)} ${units[unit]}`
}
</script>

<template>
  <base-view>
    <template #default>
      <div class="backup-manage-container">
        <!-- 统计卡区：总占用 + 有主/无主拆分 + 按引用方分组（监视哨同源数据面） -->
        <div class="backup-manage-stats">
          <div class="backup-stats-card">
            <div class="backup-stats-card-label">总占用</div>
            <div class="backup-stats-card-value">{{ formatBytes(stats?.totalBytes ?? 0) }}</div>
            <div class="backup-stats-card-sub">{{ stats?.totalCount ?? 0 }} 份备份</div>
          </div>
          <div class="backup-stats-card">
            <div class="backup-stats-card-label">有主</div>
            <div class="backup-stats-card-value backup-stats-card-value--referenced">
              {{ formatBytes(stats?.referencedBytes ?? 0) }}
            </div>
            <div class="backup-stats-card-sub">{{ stats?.referencedCount ?? 0 }} 份 · 由回收站/插件流程管理</div>
          </div>
          <div class="backup-stats-card">
            <div class="backup-stats-card-label">无主</div>
            <div class="backup-stats-card-value">{{ formatBytes(stats?.orphanedBytes ?? 0) }}</div>
            <div class="backup-stats-card-sub">{{ stats?.orphanedCount ?? 0 }} 份 · 超保留期自动清理</div>
          </div>
          <div
            v-for="referencer in stats?.referencers ?? []"
            :key="referencer.name"
            class="backup-stats-card"
            :class="{ 'backup-stats-card--warn': referencer.oldestAgeDays > REFERENCER_OBSERVE_DAYS }"
          >
            <div class="backup-stats-card-label">{{ referencer.name }}引用</div>
            <div class="backup-stats-card-value">{{ formatBytes(referencer.totalBytes) }}</div>
            <div class="backup-stats-card-sub">
              {{ referencer.count }} 份 · 最老 {{ referencer.oldestAgeDays }} 天
            </div>
          </div>
        </div>
        <search-table
          ref="backupManageSearchTable"
          v-model:page="page"
          class="backup-manage-search-table"
          toolbar-radius="var(--app-radius)"
          data-radius="var(--app-radius)"
          data-key="id"
          :thead="backupThead"
          :search="backupQueryPageFn"
          :selectable="false"
          :multi-select="false"
          :custom-operation-button="true"
          :operation-width="100"
        >
          <template #toolbarMain>
            <div class="backup-manage-toolbar">
              <el-radio-group v-model="referencedFilter" @change="handleReferencedFilterChange">
                <el-radio-button value="all">全部</el-radio-button>
                <el-radio-button value="referenced">有主</el-radio-button>
                <el-radio-button value="orphaned">无主</el-radio-button>
              </el-radio-group>
              <el-button
                type="primary"
                :loading="reconciliationRunning"
                @click="runReconciliation"
              >
                立即巡检
              </el-button>
              <el-button
                type="danger"
                class="tone-fail"
                @click="cleanExpiredOrphans"
              >
                清理全部无主
              </el-button>
            </div>
          </template>
          <!-- 有主行删除禁用 + 行内提示（有主备份由回收站/插件流程管理） -->
          <template #customOperations="{ row }">
            <el-tooltip
              :disabled="!(row as BackupDTO).referenced"
              content="有主备份由回收站/插件流程管理"
              placement="top"
            >
              <span class="backup-manage-operation-wrapper">
                <el-button
                  size="small"
                  type="danger"
                  class="tone-fail"
                  :disabled="(row as BackupDTO).referenced"
                  @click="deleteBackup(row as BackupDTO)"
                >
                  删除
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </search-table>
      </div>
    </template>
  </base-view>
</template>

<style scoped>
.backup-manage-container {
  display: flex;
  flex-direction: column;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
  gap: 10px;
}

.backup-manage-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  flex-shrink: 0;
}

.backup-stats-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 160px;
  padding: 10px 14px;
  background-color: var(--app-bg-surface);
  border: 1px solid var(--app-border-color-lighter);
  border-radius: var(--app-radius);
}

/* 引用方最老年龄超观察阈值：整卡警示描边（生命周期清理路径可能缺失） */
.backup-stats-card--warn {
  border-color: var(--app-status-warn-border);
}

.backup-stats-card--warn .backup-stats-card-sub {
  color: var(--app-status-warn-text);
}

.backup-stats-card-label {
  color: var(--app-text-secondary);
  font-size: 12px;
}

.backup-stats-card-value {
  color: var(--app-text-primary);
  font-size: 18px;
  font-weight: bold;
}

.backup-stats-card-value--referenced {
  color: var(--app-status-done-text);
}

.backup-stats-card-sub {
  color: var(--app-text-secondary);
  font-size: 12px;
}

.backup-manage-search-table {
  flex: 1;
  min-height: 0;
  width: 100%;
}

.backup-manage-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

/* 禁用态按钮外层包一层：disabled button 不触发 tooltip 事件，靠 wrapper 承接 */
.backup-manage-operation-wrapper {
  display: inline-flex;
}
</style>
