<template>
  <div class="test-view">
    <h2>测试页面</h2>
    <el-button @click="testNotification">测试通知</el-button>
    <el-button @click="testMessage">测试消息</el-button>
  </div>
</template>

<script setup lang="ts">
import { ElMessage } from "element-plus";

function testNotification() {
  ElMessage.success("这是一个测试通知");
}

function testMessage() {
  ElMessage.info("这是一个测试消息");
}
</script>

<style scoped>
.test-view {
  padding: 40px;
}
</style>
