<script setup lang="ts">
import { computed, Ref, ref } from 'vue'
import { arrayNotEmpty, notNullish } from '@renderer/utils/CommonUtil'
import RankAuthor from '@renderer/model/model/interface/RankAuthor.ts'
import {RankedLocalAuthor, RankedSiteAuthor} from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto";

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
// 作者
const authors = computed(() => {
  let noLocalAuthorList: RankedSiteAuthor[] = []
  if (arrayNotEmpty(props.siteAuthors)) {
    const localAuthors = arrayNotEmpty(props.localAuthors) ? props.localAuthors : []
    noLocalAuthorList = props.siteAuthors.filter(
      (siteAuthor) => !localAuthors.some((localAuthor) => siteAuthor.localAuthorId === localAuthor.id)
    )
  }
  let authorList: (RankedLocalAuthor | RankedSiteAuthor)[] = []
  if (arrayNotEmpty(props.localAuthors)) {
    authorList.push(...props.localAuthors)
  }
  if (arrayNotEmpty(noLocalAuthorList)) {
    authorList.push(...noLocalAuthorList)
  }
  return authorList
})
// 作者名称列表
const authorNames = computed(() => {
  let noLocalAuthorList: RankedSiteAuthor[] = []
  if (arrayNotEmpty(props.siteAuthors)) {
    const localAuthors = arrayNotEmpty(props.localAuthors) ? props.localAuthors : []
    noLocalAuthorList = props.siteAuthors.filter(
      (siteAuthor) => !localAuthors.some((localAuthor) => siteAuthor.localAuthorId === localAuthor.id)
    )
  }
  let nameList: string[] = []
  if (arrayNotEmpty(props.localAuthors)) {
    nameList.push(...props.localAuthors.map((author) => author.authorName).filter(notNullish))
  }
  if (arrayNotEmpty(noLocalAuthorList)) {
    nameList.push(...noLocalAuthorList.map((author) => author.authorName).filter(notNullish))
  }
  return nameList
})
// 当前查看的作者
const currentAuthor: Ref<RankAuthor | undefined> = computed(() => {
  return authors.value.find((tempAuthor) => tempAuthor.authorName === currentAuthorName.value)
})
// 当前选中的作者名称
const currentAuthorName: Ref<string> = ref(authorNames.value[0])
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
          {{ currentAuthor?.introduce }}
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
