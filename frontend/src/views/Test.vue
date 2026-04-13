<script setup lang="ts">
import { h, Ref, ref } from 'vue'
import { ElMessageBox, ElTreeSelect } from 'element-plus'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'

const value = ref()
const value2 = ref(5)

const taskStatus = useTaskStore().tasks
const parentTaskStatus = useParentTaskStore().parentTasks
const notificationStore = useNotificationStore().notificationMap

const cacheData = [
  { value: 5, label: 'lazy load node5' },
  { value: 6, label: 'lazy load node5' }
]

const props = {
  label: 'label',
  children: 'children',
  isLeaf: 'isLeaf'
}

let id = 0

const load = (node, resolve) => {
  if (node.isLeaf) return resolve([])

  const r = [
    {
      value: ++id,
      label: `lazy load node${id}`
    },
    {
      value: ++id,
      label: `lazy load node${id}`,
      isLeaf: true
    }
  ]
  setTimeout(() => {
    resolve(r)
  }, 1)
}

function handleTest1() {
  elMessageBoxTest()
}

function handleTest2() {
  elMessageBoxTestItems.value.push({ context: '1' })
}

const elMessageBoxTestItems: Ref<{ context: string }[]> = ref([])
function elMessageBoxTest() {
  ElMessageBox.confirm(hFunc, 'msg', {
    confirmButtonText: 'confirmButtonText',
    cancelButtonText: 'cancelButtonText',
    type: 'warning'
  })
}
function testListWorkSetWithWorkByIds() {
  window.api.testListWorkSetWithWorkByIds([1])
}
const hFunc = () =>
  h('div', {}, [h('button', { onClick: handleTest2 }, 'addone'), ...elMessageBoxTestItems.value.map((item) => item.context)])
</script>

<template>
  <base-subpage>
    <el-button @click="handleTest1">test1</el-button>
    <el-button @click="handleTest2">test2</el-button>
    <div style="display: flex; flex-direction: row">
      <div style="width: 50%">
        <template v-for="item in parentTaskStatus.values()" :key="item.id">
          <span style="white-space: nowrap">
            {{ 'id: ' + item.id + ', taskName: ' + item.taskName + ', pid:' + item.pid + ', status: ' + item.status }}
          </span>
          <br />
        </template>
      </div>
      <div style="width: 50%">
        <template v-for="item in taskStatus.values()" :key="item.task.id">
          <span style="white-space: nowrap">
            {{
              'id: ' +
              item.task.id +
              ', taskName: ' +
              item.task.taskName +
              ', pid:' +
              item.task.pid +
              ', status: ' +
              item.task.status +
              ', finished: ' +
              item.task.finished +
              ', total: ' +
              item.task.total
            }}
          </span>
          <br />
        </template>
      </div>
    </div>
    <div style="display: flex; flex-direction: row">
      <div style="width: 50%">
        <template v-for="item in notificationStore.values()" :key="item.id">
          <span style="white-space: nowrap">
            {{ 'id: ' + item.id + ', title: ' + item.title + ', description:' + item.description }}
          </span>
          <br />
        </template>
      </div>
    </div>
    <button class="button">hover me</button>
    <div class="wrapper">
      <div class="content"></div>
    </div>
    <el-tree-select v-model="value" lazy :load="load" :props="props" style="width: 240px" />
    <el-divider />
    <el-tree-select v-model="value2" lazy :load="load" :props="props" :cache-data="cacheData" style="width: 240px" />
    <el-button @click="testListWorkSetWithWorkByIds">测试查询作品集DTO接口</el-button>
  </base-subpage>
</template>

<style scoped>
.wrapper {
  width: 0;
  overflow: hidden;
  transition: width 1s;
}

.content {
  width: 200px;
  height: 100px;
  background-color: #f0dc4e;
}

.button:hover ~ .wrapper {
  width: auto;
}
</style>
