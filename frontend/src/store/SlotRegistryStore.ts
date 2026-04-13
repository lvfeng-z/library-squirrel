import { defineStore } from "pinia";
import { ref, computed } from "vue";
import type { ViewSlot, EmbedSlot, PanelSlot } from "@renderer/model/slot";
import type { RouteRecordRaw } from "vue-router";
import type { Router } from "vue-router";

// 菜单项类型
export interface MenuSlotItem {
  slotId: string;
  index: string;
  icon?: unknown;
  label: string;
  order?: number;
  children?: MenuSlotItem[];
  // 如果是叶子菜单项，指向对应的视图
  viewId?: string;
  // 如果是叶子菜单项，指向对应的页面状态
  pageStateKey?: string;
}

// 站点浏览器列表插槽项类型
export interface SiteBrowserListSlotItem {
  slotId: string;
  pluginId: number;
  pluginPublicId: string;
  name: string;
  order?: number;
  contributionId: string;
  imagePath: string;
}

// Router 实例管理
let routerInstance: Router | null = null;

/**
 * 设置 Router 实例
 * 在应用启动时调用
 */
export function setRouterInstance(router: Router) {
  routerInstance = router;
}

/**
 * 获取当前的 Router 实例
 */
export function getRouterInstance(): Router | null {
  return routerInstance;
}

export const useSlotRegistryStore = defineStore("slotRegistry", () => {
  // state
  const viewSlots = ref(new Map<string, ViewSlot>());
  const embedSlots = ref(new Map<string, EmbedSlot>());
  const panelSlots = ref(new Map<string, PanelSlot>());
  const menuSlots = ref(new Map<string, MenuSlotItem>());
  const siteBrowserSlots = ref(new Map<string, SiteBrowserListSlotItem>());
  const activeViewId = ref<string | null>(null);
  const replacedViewId = ref<string | null>(null);

  // getters
  const allViewSlots = computed(() => {
    return Array.from(viewSlots.value.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  });

  const activeView = computed(() => {
    if (!activeViewId.value) return null;
    return viewSlots.value.get(activeViewId.value) || null;
  });

  const embedSlotsByPosition = computed(() => (position: string) => {
    return Array.from(embedSlots.value.values()).filter((slot) => slot.position === position);
  });

  const panelSlotsByPosition = computed(() => (position: string) => {
    return Array.from(panelSlots.value.values()).filter((slot) => slot.position === position);
  });

  const allMenuSlots = computed(() => {
    return Array.from(menuSlots.value.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  });

  const allSiteBrowserSlots = computed(() => {
    return Array.from(siteBrowserSlots.value.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  });

  const routeConfigs = computed((): RouteRecordRaw[] => {
    const routes: RouteRecordRaw[] = [];

    viewSlots.value.forEach((slot) => {
      routes.push({
        path: `/${slot.slotId}`,
        name: slot.slotId,
        component: slot.component,
        meta: {
          title: slot.name,
          order: slot.order ?? 100,
        },
      });
    });

    return routes.sort((a, b) => ((a.meta?.order as number) ?? 100) - ((b.meta?.order as number) ?? 100));
  });

  // actions
  function registerViewSlot(slot: ViewSlot) {
    viewSlots.value.set(slot.slotId, slot);

    // 如果是插件视图且 router 可用，自动添加路由
    if (slot.isPlugin && routerInstance) {
      routerInstance.addRoute("MainLayout", {
        path: slot.slotId,
        name: slot.slotId,
        component: slot.component,
        meta: { title: slot.name, order: slot.order ?? 100, isPlugin: true },
      });
    }
  }

  function registerViewSlotWithRoute(slot: ViewSlot) {
    registerViewSlot(slot);

    if (routerInstance) {
      routerInstance.addRoute("MainLayout", {
        path: slot.slotId,
        name: slot.slotId,
        component: slot.component,
        meta: { title: slot.name, order: slot.order ?? 100 },
      });
    }
  }

  function unregisterViewSlot(id: string) {
    const slot = viewSlots.value.get(id);
    if (slot?.isPlugin && routerInstance) {
      routerInstance.removeRoute(id);
    }

    if (activeViewId.value === id) {
      activeViewId.value = null;
    }
    viewSlots.value.delete(id);
  }

  function registerEmbedSlot(slot: EmbedSlot) {
    embedSlots.value.set(slot.slotId, slot);
  }

  function unregisterEmbedSlot(id: string) {
    embedSlots.value.delete(id);
  }

  function registerPanelSlot(slot: PanelSlot) {
    panelSlots.value.set(slot.slotId, slot);
  }

  function unregisterPanelSlot(id: string) {
    panelSlots.value.delete(id);
  }

  function switchView(viewId: string): boolean {
    const slot = viewSlots.value.get(viewId);
    if (slot) {
      activeViewId.value = viewId;
      return true;
    }
    return false;
  }

  function clearActiveView() {
    activeViewId.value = null;
  }

  function replaceView(panelSlotId: string, originalViewId: string) {
    replacedViewId.value = originalViewId;
    switchView(panelSlotId);
  }

  function restoreView() {
    if (replacedViewId.value) {
      switchView(replacedViewId.value);
      replacedViewId.value = null;
    }
  }

  function registerMenuSlot(item: MenuSlotItem) {
    menuSlots.value.set(item.slotId, item);
  }

  function registerMenuSlots(items: MenuSlotItem[]) {
    items.forEach((item) => {
      menuSlots.value.set(item.slotId, item);
    });
  }

  function unregisterMenuSlot(id: string) {
    menuSlots.value.delete(id);
  }

  function registerSiteBrowserSlot(item: SiteBrowserListSlotItem) {
    siteBrowserSlots.value.set(item.slotId, item);
  }

  function registerSiteBrowserSlots(items: SiteBrowserListSlotItem[]) {
    items.forEach((item) => {
      siteBrowserSlots.value.set(item.slotId, item);
    });
  }

  function unregisterSiteBrowserSlot(id: string) {
    siteBrowserSlots.value.delete(id);
  }

  function reset() {
    viewSlots.value.clear();
    embedSlots.value.clear();
    panelSlots.value.clear();
    menuSlots.value.clear();
    siteBrowserSlots.value.clear();
    activeViewId.value = null;
    replacedViewId.value = null;
  }

  return {
    // state
    viewSlots,
    embedSlots,
    panelSlots,
    menuSlots,
    siteBrowserSlots,
    activeViewId,
    replacedViewId,
    // getters
    allViewSlots,
    activeView,
    embedSlotsByPosition,
    panelSlotsByPosition,
    allMenuSlots,
    allSiteBrowserSlots,
    routeConfigs,
    // actions
    registerViewSlot,
    registerViewSlotWithRoute,
    unregisterViewSlot,
    registerEmbedSlot,
    unregisterEmbedSlot,
    registerPanelSlot,
    unregisterPanelSlot,
    switchView,
    clearActiveView,
    replaceView,
    restoreView,
    registerMenuSlot,
    registerMenuSlots,
    unregisterMenuSlot,
    registerSiteBrowserSlot,
    registerSiteBrowserSlots,
    unregisterSiteBrowserSlot,
    reset,
  };
});
