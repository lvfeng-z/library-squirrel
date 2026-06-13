<script setup lang="ts">
import { computed, Ref, ref } from 'vue'
import { arrayNotEmpty, notNullish } from '@renderer/utils/CommonUtil'
import { RankedLocalAuthor, RankedSiteAuthor } from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto"

// props
const props = withDefaults(
  defineProps<{
    localAuthors: RankedLocalAuthor[] | undefined | null
    siteAuthors: RankedSiteAuthor[] | undefined | null
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

// 变量
// 未绑定本地作者的站点作者列表（已绑定本地作者的站点作者由本地作者代表展示）
const noLocalAuthorList = computed<RankedSiteAuthor[]>(() => {
  if (!arrayNotEmpty(props.siteAuthors)) return []
  const localAuthors = arrayNotEmpty(props.localAuthors) ? props.localAuthors : []
  return props.siteAuthors.filter(
    (siteAuthor) => !localAuthors.some((localAuthor) => siteAuthor.author.localAuthorId === localAuthor.author.id)
  )
})
// 作者
const authors = computed<(RankedLocalAuthor | RankedSiteAuthor)[]>(() => {
  let authorList: (RankedLocalAuthor | RankedSiteAuthor)[] = []
  if (arrayNotEmpty(props.localAuthors)) {
    authorList.push(...props.localAuthors)
  }
  authorList.push(...noLocalAuthorList.value)
  return authorList
})
// 作者名称列表
const authorNames = computed<string[]>(() => {
  let nameList: string[] = []
  if (arrayNotEmpty(props.localAuthors)) {
    nameList.push(...props.localAuthors.map((author) => author.author.authorName).filter(notNullish))
  }
  nameList.push(...noLocalAuthorList.value.map((author) => author.author.authorName).filter(notNullish))
  return nameList
})
// 当前选中的作者名称
const currentAuthorName: Ref<string> = ref(authorNames.value[0])
// 当前查看的作者
const currentAuthor: Ref<(RankedLocalAuthor | RankedSiteAuthor) | undefined> = computed(() => {
  return authors.value.find((tempAuthor) => tempAuthor.author.authorName === currentAuthorName.value)
})
// cursor参数
const cursorParam: Ref<string> = ref(props.useHandCursor ? 'pointer' : 'default')
</script>

<template>
  <div class="author-info-container">
    <el-popover
      :trigger="popoverTrigger"
      :width="width"
      popper-class="author-info-popper"
    >
      <template #reference>
        <el-text class="author-info-text">
          {{ authorNames.join('、') }}
        </el-text>
      </template>
      <template #default>
        <el-segmented
          v-model="currentAuthorName"
          :options="authorNames"
        />
        <div class="author-info-introduce">
          {{ currentAuthor?.author?.introduce }}
        </div>
      </template>
    </el-popover>
  </div>
</template>

<style scoped>
.author-info-container {
  cursor: v-bind(cursorParam);
}
.author-info-text {
  width: 100%;
}
.author-info-introduce {
  max-height: 300px;
  overflow-y: scroll;
  text-overflow: ellipsis;
}
</style>
