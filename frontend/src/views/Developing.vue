<script setup lang="ts">
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import { HotWater } from '@element-plus/icons-vue'
</script>

<template>
  <base-subpage>
    <el-empty
      style="height: 100%; background-color: #fafafa"
      description=" "
    >
      <el-icon size="large">
        <HotWater />
      </el-icon>
      开发中...
    </el-empty>
  </base-subpage>
</template>

<style scoped></style>
