<script setup lang="ts">
</script>
<template>
  <div class="base-view">
    <div class="base-view-content">
      <slot />
    </div>
    <div class="base-view-dialog">
      <slot name="dialog" />
    </div>
  </div>
</template>

<style>
.base-view {
  display: flex;
  width: 100%;
  height: 100%;
  background: #e6e6e6;
  justify-content: center;
  align-items: center;
}
.base-view-dialog {
  position: absolute;
}
.base-view-content {
  height: 100%;
  width: 100%;
}
</style>
