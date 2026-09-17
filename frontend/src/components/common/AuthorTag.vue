<script setup lang="ts">
import { computed } from 'vue'
import SegmentedTag from '@renderer/components/common/SegmentedTag.vue'
import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { RankedLocalAuthor, RankedSiteAuthor } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// AuthorTag：单个作者的分段 tag 展示（主段作者名 + 第二段关联级角色），
// hover 任一 tag 弹 popover 显示该作者简介。主段/角色段底色由 block 模式
// neutral 令牌分节（main=neutral-bg、sub=neutral-bg-strong），随主题令牌变化。
const props = defineProps<{
  /** 单个作者（本地/站点统一形态：均含 authorName/introduce 与关联级 roleName） */
  author: RankedLocalAuthor | RankedSiteAuthor
}>()

// 分段 tag 数据：主段作者名；role 非空时追加第二段（无角色只显主段）
const tagItem = computed<SegmentedTagItem>(() => {
  const roleName = props.author.roleName ?? ''
  return new SegmentedTagItem({
    value: props.author.author.id,
    label: props.author.author.authorName ?? '',
    disabled: false,
    ...(roleName ? { subLabels: [roleName] } : {})
  })
})
</script>

<template>
  <el-popover
    trigger="hover"
    :width="300"
    popper-class="author-tag-popper"
  >
    <template #reference>
      <segmented-tag
        :item="tagItem"
        :closeable="false"
        variant="block"
      />
    </template>
    <template #default>
      <div class="author-tag-introduce">
        <div class="author-tag-introduce-name">
          {{ props.author.author.authorName }}
        </div>
        <div class="author-tag-introduce-text">
          {{ props.author.author.introduce }}
        </div>
      </div>
    </template>
  </el-popover>
</template>

<style scoped>
.author-tag-introduce-name {
  font-weight: bold;
  margin-bottom: 6px;
}
.author-tag-introduce-text {
  max-height: 300px;
  overflow-y: auto;
  text-overflow: ellipsis;
  white-space: pre-wrap;
}
</style>
