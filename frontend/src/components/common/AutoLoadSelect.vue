<script setup lang="ts">
import IPage from '@renderer/model/util/IPage.ts'
import {Ref, ref} from 'vue'
import { SelectItem } from "@bindings/github.com//lvfeng-z/library-squirrel-plugin-sdk/dto"
import {arrayNotEmpty} from "@renderer/utils/CommonUtil.ts";
import {newPage} from "@renderer/utils/Pager.ts";
import lodash from "lodash";

// props
const props = withDefaults(
  defineProps<{
    load: (page: IPage<SelectItem>, input: string) => Promise<IPage<SelectItem>>
    pageSize?: number
  }>(),
  {
    pageSize: 10
  }
)

// model
const data = defineModel<string | number | null>('data')
const selectList = defineModel<SelectItem[]>('selectList', { default: [] })

// 变量
// el-select组件的实例
const select = ref()
const page: Ref<IPage<SelectItem>> = ref(newPage<SelectItem>({ pageSize: props.pageSize })) as Ref<IPage<SelectItem>>
// 方法
// 查询页
async function queryPage(newQuery: boolean, input: string) {
  // 新查询重置查询条件
  if (newQuery) {
    page.value = newPage<SelectItem>({ pageSize: props.pageSize })
    page.value.data = []
    selectList.value = page.value.data as SelectItem[]
  }
  //查询
  const tempPage = lodash.cloneDeep(page.value)
  tempPage.data = []
  tempPage.pageSize = props.pageSize
  const nextPage = await props.load(tempPage, input)

  // 没有新数据时，不再增加页码
  if (arrayNotEmpty(nextPage.data)) {
    page.value.pageNumber++
    page.value.pageCount = nextPage.pageCount
    page.value.dataCount = nextPage.dataCount
    page.value.data?.push(...nextPage.data)
  }
}
function focus() {
  select.value.focus()
}

// 暴露
defineExpose({ focus })
</script>

<template>
  <el-select
    ref="select"
    v-model="data"
    v-el-select-bottomed="(input: string) => queryPage(false, input)"
    :remote-method="(input: string) => queryPage(true, input)"
    remote
  >
    <slot name="default" :list="selectList" />
  </el-select>
</template>

<style scoped></style>
