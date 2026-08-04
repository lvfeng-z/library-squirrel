<script setup lang="ts">
import { computed } from 'vue'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// props
const props = defineProps<{
  // 操作目标:单任务传 [row]、批量传多行;由调用方决定(如 TaskDialog 标题栏按是否选中子任务切换)
  rows: TaskProgressTreeDTO[]
  buttonClicked: (rows: TaskProgressTreeDTO[], code: TaskOperationCodeEnum) => void
}>()

// 按钮禁用:无操作对象时禁用。显式绑定以覆盖外层 el-form 的 disabled
// (el-button 经 useFormDisabled 继承 form?.disabled;显式 false 因 ?? 逻辑生效,避免标题栏按钮在 VIEW 模式被表单禁用)
const disabled = computed(() => props.rows.length === 0)
</script>

<template>
  <el-button-group>
    <el-button
      size="small"
      :disabled="disabled"
      @click="buttonClicked(rows, TaskOperationCodeEnum.START)"
    >
      运行
    </el-button>
    <el-button
      size="small"
      :disabled="disabled"
      @click="buttonClicked(rows, TaskOperationCodeEnum.PAUSE)"
    >
      暂停
    </el-button>
    <el-button
      size="small"
      type="danger"
      :disabled="disabled"
      @click="buttonClicked(rows, TaskOperationCodeEnum.DELETE)"
    >
      删除
    </el-button>
  </el-button-group>
</template>
