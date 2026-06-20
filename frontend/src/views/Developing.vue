<script setup lang="ts">
import BaseView from '@renderer/views/BaseView.vue'
import { HotWater } from '@element-plus/icons-vue'
</script>

<template>
  <base-view>
    <el-empty
      style="height: 100%; background-color: #fafafa"
      description=" "
    >
      <el-icon size="large">
        <HotWater />
      </el-icon>
      开发中...
    </el-empty>
  </base-view>
</template>

<style scoped></style>
