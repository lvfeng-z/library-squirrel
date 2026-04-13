<template>
  <div class="search-list">
    <el-row class="search-list-toolbar">
      <el-col :span="selectList ? 18 : 24">
        <el-input v-model="inputText" placeholder="请输入关键词进行搜索" prefix-icon="el-icon-search" @input="handleSearch" />
      </el-col>
      <el-col v-if="selectList" :span="6">
        <el-select v-model="selectListSelected" multiple filterable remote :remote-method="getSelectList">
          <el-option v-for="item in selectListItems" :key="item.value" :value="item.value" :label="item.label"> </el-option>
        </el-select>
      </el-col>
    </el-row>
    <div class="result-list rounded-borders">
      <el-checkbox-group v-if="multiSelect" v-model="checkboxSelected">
        <el-checkbox v-for="item in filteredItems" :key="item.value" :value="item.value" @change="selectionChange">
          {{ item.label }}
        </el-checkbox>
      </el-checkbox-group>

      <el-radio-group v-else v-model="radioSelected">
        <el-radio v-for="item in filteredItems" :key="item.value" :value="item.value" @change="selectionChange">
          {{ item.label }}
        </el-radio>
      </el-radio-group>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Ref, ref, UnwrapRef } from 'vue'
import SelectItem from '../../model/util/SelectItem'

const props = defineProps<{
  multiSelect: boolean
  selectList?: boolean
  inputKeyword: string
  selectKeyword?: string
  parentParams?: object
  searchApi: (args: object) => Promise<never>
  selectListSearchApi?: (args: object) => Promise<never>
}>()

const emits = defineEmits(['selectionChange'])

const inputText = ref('')
const checkboxSelected = ref([])
const radioSelected = ref(null)
const selectListSelected = ref([])
const selectListItems: Ref<UnwrapRef<SelectItem[]>> = ref([])
const filteredItems: Ref<UnwrapRef<SelectItem[]>> = ref([])

// 查询主列表
async function handleSearch() {
  // 输入框的参数
  const params = {}
  params[props.inputKeyword] = inputText.value

  // 下拉选择框的参数
  if (props.selectList && props.selectKeyword != undefined && props.selectKeyword.length > 0) {
    params[props.selectKeyword] = String(selectListSelected.value)
  }

  // 父组件传入的参数
  if (props.parentParams != undefined) {
    Object.keys(props.parentParams).forEach((prop) => {
      if (props.parentParams != undefined) {
        params[prop] = props.parentParams[prop]
      }
    })
  }

  // 调用接口
  filteredItems.value = await props.searchApi(params)
}

async function getSelectList(keyword: string) {
  const params = { keyword: keyword }
  if (props.selectListSearchApi != undefined) {
    selectListItems.value = await props.selectListSearchApi(params)
  }
}

function selectionChange() {
  if (props.multiSelect) {
    emits('selectionChange', checkboxSelected.value)
  } else {
    emits('selectionChange', radioSelected.value)
  }
}
</script>

<style scoped>
.search-list {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  width: 100%;
  height: 100%;
}

.search-list-toolbar {
  width: 100%;
}

.result-list {
  margin-top: 5px;
  width: 100%;
  height: 100%;
}
</style>
