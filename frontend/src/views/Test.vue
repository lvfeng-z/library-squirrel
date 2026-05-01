<script setup lang="ts">
import { ref } from 'vue'
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import {getPropByPath} from "@renderer/utils/ObjectUtil.ts";

const treeData = ref([
    {
      p1: {
        id: 1,
        value: 1,
        hasChildren: true
      },
      p2: {
        value: 1.1
      },
      p3: {
        children: [{
          p1: {
            id: 11,
            value: 2
          },
          p2: {
            value: 2.1
          }
        }]
      },
      hasChildren: true,
      children: [{
        p1: {
          id: 11,
          value: 2
        },
        p2: {
          value: 2.1
        }
      }]
    },
    {
      p1: {
        id: 2,
        value: 2,
        hasChildren: true
      },
      p2: {
        value: 2.1
      },
      p3: {
        children: [{
          p1: {
            id: 22,
            value: 2
          },
          p2: {
            value: 2.1
          }
        }]
      },
      hasChildren: true,
      children: [{
        p1: {
          id: 22,
          value: 2
        },
        p2: {
          value: 2.1
        }
      }]
    },
    {
      p1: {
        id: 3,
        value: 3,
        hasChildren: true
      },
      p2: {
        value: 3.1
      },
      p3: {
        children: [{
          p1: {
            id: 33,
            value: 2
          },
          p2: {
            value: 2.1
          }
        }]
      },
      hasChildren: true,
      children: [{
        p1: {
          id: 33,
          value: 2
        },
        p2: {
          value: 2.1
        }
      }]
    },
    {
      p1: {
        id: 4,
        value: 4,
        hasChildren: true
      },
      p2: {
        value: 4.1
      },
      p3: {
        children: [{
          p1: {
            id: 44,
            value: 2
          },
          p2: {
            value: 2.1
          }
        }]
      },
      hasChildren: true,
      children: [{
        p1: {
          id: 44,
          value: 2
        },
        p2: {
          value: 2.1
        }
      }]
    }
])
const treeProps1 = ref({hasChildren: 'hasChildren', children: 'children'})
const treeProps2 = ref({hasChildren: 'hasChildren', children: 'children'})
</script>

<template>
  <base-subpage>
    <el-table row-key="p1.id" :data="treeData" :tree-props="treeProps2">
      <el-table-column>
        <template #default="{ row }">
          {{row}}
        </template>
      </el-table-column>
    </el-table>
  </base-subpage>
</template>

<style scoped>
</style>
