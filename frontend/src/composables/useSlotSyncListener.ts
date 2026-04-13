import type { MenuSlotItem, SiteBrowserListSlotItem } from "@renderer/store/SlotRegistryStore";
import { useSlotRegistryStore } from "@renderer/store/SlotRegistryStore";
import type { EmbedSlot, PanelSlot, ViewSlot } from "@renderer/model/slot";
import { defineComponent } from "vue";
import { App } from "../../bindings/github.com/library-squirrel/wails";

/**
 * 初始化插槽同步监听器
 * 通过 Wails Events 监听 Go 后端的插槽注册/注销消息
 */
export function initSlotSyncListener() {
  const store = useSlotRegistryStore();

  // 处理插槽注册事件
  window.wails.Events.On("slot-register", (data: any) => {
    try {
      const config = data.slot;

      if (config.type === "view") {
        const slot = convertToViewSlot(config);
        store.registerViewSlot(slot);
      } else if (config.type === "menu") {
        const menuItem = convertToMenuSlot(config);
        store.registerMenuSlot(menuItem);
      } else if (config.type === "embed") {
        store.registerEmbedSlot(convertToEmbedSlot(config));
      } else if (config.type === "panel") {
        store.registerPanelSlot(convertToPanelSlot(config));

        if (config.replaceViewId) {
          store.replaceView(config.pluginPublicId, config.replaceViewId);
        }
      } else if (config.type === "siteBrowserList") {
        store.registerSiteBrowserSlot(convertToSiteBrowserListSlot(config));
      }
    } catch (error) {
      console.error("Failed to handle slot-register event:", error);
    }
  });

  // 处理插槽注销事件
  window.wails.Events.On("slot-unregister", (data: any) => {
    try {
      const slotId = data.slotId as string;
      const pluginId = data.pluginId as number;

      if (slotId) {
        store.unregisterViewSlot(slotId);
        store.unregisterMenuSlot(slotId);
        store.unregisterEmbedSlot(slotId);
        store.unregisterPanelSlot(slotId);
        store.unregisterSiteBrowserSlot(slotId);
      }
    } catch (error) {
      console.error("Failed to handle slot-unregister event:", error);
    }
  });

  // 处理批量插槽注册事件
  window.wails.Events.On("slot-batch-register", (data: any) => {
    try {
      const configs = data.slots;

      configs.forEach((config: any) => {
        if (config.type === "view") {
          store.registerViewSlot(convertToViewSlot(config));
        } else if (config.type === "menu") {
          store.registerMenuSlot(convertToMenuSlot(config));
        } else if (config.type === "embed") {
          store.registerEmbedSlot(convertToEmbedSlot(config));
        } else if (config.type === "panel") {
          store.registerPanelSlot(convertToPanelSlot(config));
        } else if (config.type === "siteBrowserList") {
          store.registerSiteBrowserSlot(convertToSiteBrowserListSlot(config));
        }
      });
    } catch (error) {
      console.error("Failed to handle slot-batch-register event:", error);
    }
  });

  // 同步所有已注册的插槽
  App.GetAllSlots()
    .then((slots: any[]) => {
      slots.forEach((config: any) => {
        if (config.type === "view") {
          store.registerViewSlot(convertToViewSlot(config));
        } else if (config.type === "menu") {
          store.registerMenuSlot(convertToMenuSlot(config));
        } else if (config.type === "embed") {
          store.registerEmbedSlot(convertToEmbedSlot(config));
        } else if (config.type === "panel") {
          store.registerPanelSlot(convertToPanelSlot(config));
        } else if (config.type === "siteBrowserList") {
          store.registerSiteBrowserSlot(convertToSiteBrowserListSlot(config));
        }
      });
    })
    .catch((err) => {
      console.error("Failed to sync slots:", err);
    });

  // 返回清理函数
  return () => {
    window.wails.Events.Off("slot-register");
    window.wails.Events.Off("slot-unregister");
    window.wails.Events.Off("slot-batch-register");
  };
}

// 简化的转换函数
function convertToViewSlot(config: any): ViewSlot {
  return {
    slotId: config.slotId,
    name: config.name,
    component: () => Promise.resolve(defineComponent({ template: "<div>Plugin View</div>" })),
    order: config.order ?? 100,
    isPlugin: true,
  };
}

function convertToEmbedSlot(config: any): EmbedSlot {
  return {
    slotId: config.slotId,
    name: config.name || "",
    position: config.position || "toolbar",
    component: () => Promise.resolve(defineComponent({ template: "<div>Embed</div>" })),
    order: config.order ?? 100,
  };
}

function convertToPanelSlot(config: any): PanelSlot {
  return {
    slotId: config.slotId,
    name: config.name || "",
    position: config.position || "left-sidebar",
    component: () => Promise.resolve(defineComponent({ template: "<div>Panel</div>" })),
    order: config.order ?? 100,
  };
}

function convertToMenuSlot(config: any): MenuSlotItem {
  return {
    slotId: config.slotId,
    index: config.pluginPublicId || config.slotId,
    label: config.name,
    order: config.order ?? 100,
    viewId: config.viewId,
  };
}

function convertToSiteBrowserListSlot(config: any): SiteBrowserListSlotItem {
  return {
    slotId: config.slotId,
    pluginId: config.pluginId || 0,
    pluginPublicId: config.pluginPublicId || "",
    name: config.name || "",
    order: config.order ?? 100,
    contributionId: config.contributionId || "",
    imagePath: config.imagePath || "",
  };
}
