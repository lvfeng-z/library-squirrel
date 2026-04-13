<template>
  <div class="main-page-wrapper">
    <h2>Library Squirrel</h2>
    <p>欢迎使用个人资源库管理系统</p>
    <el-card class="welcome-card">
      <template #header>
        <span>快速开始</span>
      </template>
      <div class="quick-actions">
        <el-button type="primary" @click="$router.push('/tag/local-tag')">
          管理标签
        </el-button>
        <el-button type="success" @click="$router.push('/task')">
          查看任务
        </el-button>
        <el-button @click="$router.push('/plugin')">
          管理插件
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
// Placeholder - Main page wrapper
</script>

<style scoped>
.main-page-wrapper {
  padding: 40px;
}

.welcome-card {
  max-width: 600px;
  margin-top: 20px;
}

.quick-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
</style>
