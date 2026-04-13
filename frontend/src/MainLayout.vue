<template>
  <el-container class="main-layout">
    <el-aside width="200px" class="sidebar">
      <div class="logo">Library Squirrel</div>
      <el-menu
        :default-active="currentRoute"
        class="sidebar-menu"
        router
      >
        <template v-for="menu in menuItems" :key="menu.index">
          <el-sub-menu v-if="menu.children" :index="menu.index">
            <template #title>
              <el-icon v-if="menu.icon"><component :is="menu.icon" /></el-icon>
              <span>{{ menu.label }}</span>
            </template>
            <el-menu-item
              v-for="child in menu.children"
              :key="child.index"
              :index="child.index"
            >
              <el-icon v-if="child.icon"><component :is="child.icon" /></el-icon>
              <span>{{ child.label }}</span>
            </el-menu-item>
          </el-sub-menu>
          <el-menu-item v-else :index="menu.index">
            <el-icon v-if="menu.icon"><component :is="menu.icon" /></el-icon>
            <span>{{ menu.label }}</span>
          </el-menu-item>
        </template>
      </el-menu>
    </el-aside>
    <el-container>
      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useSlotRegistryStore } from "@renderer/store/SlotRegistryStore";
import { markRaw } from "vue";
import {
  HomeFilled,
  Discount,
  User,
  Star,
  List,
  Link,
  TakeawayBox,
  Setting,
  Guide,
  Coordinate,
} from "@element-plus/icons-vue";

const route = useRoute();
const slotStore = useSlotRegistryStore();

const currentRoute = computed(() => route.path);

// Built-in menu items
const menuItems = computed(() => {
  const items = [
    {
      index: "/",
      label: "主页",
      icon: markRaw(HomeFilled),
    },
    {
      index: "tag",
      label: "标签",
      icon: markRaw(Discount),
      children: [
        { index: "/tag/local-tag", label: "本地标签", icon: undefined as any },
        { index: "/tag/site-tag", label: "站点标签", icon: undefined as any },
      ],
    },
    {
      index: "author",
      label: "作者",
      icon: markRaw(User),
      children: [
        { index: "/author/local-author", label: "本地作者", icon: undefined as any },
        { index: "/author/site-author", label: "站点作者", icon: undefined as any },
      ],
    },
    {
      index: "/favorite",
      label: "收藏",
      icon: markRaw(Star),
    },
    {
      index: "/task",
      label: "任务",
      icon: markRaw(List),
    },
    {
      index: "site",
      label: "站点",
      icon: markRaw(Link),
      children: [
        { index: "/site/site-manage", label: "站点管理", icon: undefined as any },
        { index: "/site/site-browser", label: "站点浏览", icon: undefined as any },
      ],
    },
    {
      index: "/plugin",
      label: "插件",
      icon: markRaw(TakeawayBox),
    },
    {
      index: "/settings",
      label: "设置",
      icon: markRaw(Setting),
    },
    {
      index: "/guide",
      label: "向导",
      icon: markRaw(Guide),
    },
  ];

  // Merge with plugin menu slots
  const pluginMenus = slotStore.allMenuSlots;
  // Add plugin menus here if needed

  return items;
});
</script>

<style scoped>
.main-layout {
  height: 100vh;
}

.sidebar {
  background-color: #f5f7fa;
  border-right: 1px solid #e4e7ed;
}

.logo {
  padding: 20px;
  font-size: 18px;
  font-weight: bold;
  color: #409eff;
  text-align: center;
}

.sidebar-menu {
  border-right: none;
  background-color: transparent;
}

.main-content {
  background-color: #fff;
}
</style>
