<template>
  <div class="settings-view">
    <h2>设置</h2>
    <el-form label-width="120px">
      <el-form-item label="主题">
        <el-select v-model="theme" placeholder="请选择主题">
          <el-option label="浅色" value="light" />
          <el-option label="深色" value="dark" />
        </el-select>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";

const theme = ref("light");
</script>

<style scoped>
.settings-view {
  padding: 40px;
}
</style>
