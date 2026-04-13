<script setup lang="ts">
import { ref, Ref } from 'vue'
import WorkCardItem from '@renderer/model/model/dto/WorkCardItem.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'
// props
const props = withDefaults(
  defineProps<{
    work: WorkCardItem
    popoverTrigger?: 'click' | 'hover' | 'focus' | 'contextmenu'
    useHandCursor?: boolean
    width?: string
  }>(),
  {
    popoverTrigger: 'click',
    useHandCursor: true,
    width: 'auto'
  }
)

// 方法
// 获取要展示的作品名称
function getWorkNameForDisplay(): string {
  if (isNotBlank(props.work.nickName)) {
    return props.work.nickName as string
  }
  if (isNotBlank(props.work.siteItemName)) {
    return props.work.siteItemName as string
  }
  return '?'
}
// cursor参数
const cursorParam: Ref<string> = ref(props.useHandCursor ? 'pointer' : 'default')
</script>

<template>
  <div class="author-info-container">
    <el-popover :trigger="popoverTrigger" :width="width" popper-class="author-info-popper">
      <template #reference>
        <el-text class="work-info-text">{{ getWorkNameForDisplay() }}</el-text>
      </template>
      <template #default>
        <div class="work-info-description">
          {{ props.work.description }}
        </div>
      </template>
    </el-popover>
  </div>
</template>

<style scoped>
.author-info-container {
  cursor: v-bind(cursorParam);
}
.work-info-text {
  width: 100%;
}
.work-info-description {
  max-height: 300px;
  overflow-y: scroll;
  text-overflow: ellipsis;
}
</style>
