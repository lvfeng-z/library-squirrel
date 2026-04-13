import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { v4 } from "uuid";
import { ElNotification } from "element-plus";
import { h } from "vue";

export class NotificationItem {
  id?: string;
  title?: string;
  render?: () => any;
}

export const useNotificationStore = defineStore("notification", () => {
  // state
  const notificationMap = ref(new Map<string, NotificationItem>());
  const notificationList = ref<NotificationItem[]>([]);

  // getters
  const count = computed(() => notificationMap.value.size);

  // actions
  function add(notificationItem: NotificationItem): string {
    const id: string = v4();
    notificationItem.id = id;
    notificationMap.value.set(id, notificationItem);
    notificationList.value.push(notificationItem);
    return id;
  }

  function get(id: string): NotificationItem | undefined {
    return notificationMap.value.get(id);
  }

  function getRange(startIndex: number, endIndex?: number): NotificationItem[] {
    const listLength = notificationList.value.length;
    if (listLength === 0) {
      return [];
    }
    if (startIndex < 0) {
      startIndex = 0;
    } else if (startIndex >= listLength) {
      return [];
    }
    if (endIndex === undefined) {
      return notificationList.value.slice(startIndex);
    }
    if (endIndex < 0) {
      return [];
    } else if (endIndex >= listLength) {
      endIndex = listLength - 1;
    }
    if (startIndex > endIndex) {
      [startIndex, endIndex] = [endIndex, startIndex];
    }
    return notificationList.value.slice(startIndex, endIndex + 1);
  }

  function remove(
    id: string,
    notificationConfig?: {
      msg: string;
      type?: "error" | "primary" | "success" | "warning" | "info";
      duration?: number;
    }
  ): void {
    notificationMap.value.delete(id);
    const index = notificationList.value.findIndex(
      (notification) => notification.id === id
    );
    notificationList.value.splice(index, 1);
    if (notificationConfig) {
      const config = {
        msg: notificationConfig.msg,
        type: notificationConfig.type || "info",
        duration: notificationConfig.duration || 3000,
      };
      startNotify(config);
    }
  }

  return {
    notificationMap,
    notificationList,
    count,
    add,
    get,
    getRange,
    remove,
  };
});

type NotificationConfig = {
  msg: string;
  type: "error" | "primary" | "success" | "warning" | "info";
  duration: number;
};

const positon: boolean[] = [true, true];
const notificationBuffer: NotificationConfig[] = [];

const startNotify = (config: NotificationConfig) => {
  notificationBuffer.push(config);
  recursiveNotify();
};

const recursiveNotify = async () => {
  const index = positon.findIndex((item) => item);
  if (index === -1) {
    return;
  }
  positon[index] = false;
  if (index === 1) {
    await new Promise<void>((resolve) => setTimeout(() => resolve(), 300));
  }
  let config: NotificationConfig | undefined = undefined;
  let currentLength: number = 0;
  if (notificationBuffer.length > 0) {
    config = notificationBuffer[0];
    currentLength = notificationBuffer.length;
    notificationBuffer.length = 0;
  }
  if (config === undefined) {
    return;
  }
  const children = [h("i", {}, config.msg)];
  if (currentLength > 1) {
    children.push(
      h(
        "span",
        {
          style: {
            display: "inline-block",
            backgroundColor: "#f56c6c",
            color: "white",
            borderRadius: "10px",
            padding: "0 6px",
            fontSize: "12px",
            fontWeight: "bold",
            marginLeft: "8px",
            lineHeight: "18px",
            minWidth: "18px",
            textAlign: "center",
          },
        },
        currentLength + "+"
      )
    );
  }
  ElNotification({
    type: config.type,
    message: h("div", {}, children),
    duration: config.duration,
    offset: 80,
    onClose: () => {
      positon[index] = true;
      if (notificationBuffer.length > 0) {
        recursiveNotify();
      }
    },
  });
};
