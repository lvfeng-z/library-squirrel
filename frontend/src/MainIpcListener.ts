import { useTaskStore } from "@renderer/store/UseTaskStore";
import { ElMessageBox, ElNotification } from "element-plus";
import { useParentTaskStore } from "@renderer/store/UseParentTaskStore";
import ConfirmConfig from "@renderer/model/util/ConfirmConfig";
import GotoPageConfig from "@renderer/model/util/GotoPageConfig";
import { h } from "vue";
import NotifyConfig from "@renderer/model/util/NotifyConfig";
import { askGotoPage } from "@renderer/utils/PageUtil";
import { initSlotSyncListener } from "@renderer/composables/useSlotSyncListener";

// Helper function to check if value is null or undefined
function isNullish(val: any): boolean {
  return val === null || val === undefined;
}

export function initListener() {
  // 任务队列 - 使用 Wails Events
  window.wails.Events.On("taskStatus-setTask", (data: any) => {
    const taskList = data as TaskProgressDTO[];
    useTaskStore().setTask(taskList);
  });

  window.wails.Events.On("taskStatus-updateTask", (data: any) => {
    const taskList = data as TaskProgressDTO[];
    useTaskStore().updateTask(taskList);
  });

  window.wails.Events.On("taskStatus-updateSchedule", (data: any) => {
    const scheduleDTOList = data as TaskScheduleDTO[];
    useTaskStore().updateTaskSchedule(scheduleDTOList);
  });

  window.wails.Events.On("taskStatus-removeTask", (data: any) => {
    const ids = data as number[];
    useTaskStore().removeTask(ids);
  });

  // 父任务状态
  window.wails.Events.On(
    "parentTaskStatus-setParentTask",
    (data: any) => {
      const taskList = data as TaskProgressMapTreeDTO[];
      useParentTaskStore().setParentTask(taskList);
    }
  );

  window.wails.Events.On(
    "parentTaskStatus-updateParentTask",
    (data: any) => {
      const taskList = data as TaskProgressMapTreeDTO[];
      useParentTaskStore().updateParentTask(taskList);
    }
  );

  window.wails.Events.On("parentTaskStatus-updateSchedule", (data: any) => {
    const taskList = data as TaskScheduleDTO[];
    useParentTaskStore().updateParentTaskSchedule(taskList);
  });

  window.wails.Events.On("parentTaskStatus-removeParentTask", (data: any) => {
    const ids = data as number[];
    useParentTaskStore().removeParentTask(ids);
  });

  // 自定义确认弹窗 - 需要 Wails 绑定支持
  // window.wails.Events.On("custom-confirm", (confirmId: string, config: ConfirmConfig) => {
  //   ElMessageBox.confirm(config.msg, config.title, {
  //     confirmButtonText: config.confirmButtonText,
  //     cancelButtonText: config.cancelButtonText,
  //     type: config.type
  //   })
  //     .then(() => window.wails.Events.Emit("custom-confirm-echo", confirmId, true))
  //     .catch(() => window.wails.Events.Emit("custom-confirm-echo", confirmId, false));
  // });

  // 自定义通知
  window.wails.Events.On("custom-notify", (config: NotifyConfig) => {
    ElNotification({
      type: config.type,
      message: h(
        "span",
        {
          style: {
            display: "-webkit-box",
            "-webkit-box-orient": "vertical",
            "-webkit-line-clamp": isNullish(config.maxRow) ? 3 : config.maxRow,
            overflow: "hidden",
            "text-overflow": "ellipsis",
          },
        },
        config.msg
      ),
      duration: config.duration,
    });
  });

  // 页面跳转
  window.wails.Events.On("goto-page", (config: GotoPageConfig) => {
    askGotoPage(config);
  });

  // 初始化插槽同步监听器
  initSlotSyncListener();
}

// 类型定义
interface TaskProgressDTO {
  id?: number;
  taskName?: string;
  status?: number;
  total?: number;
  finished?: number;
}

interface TaskScheduleDTO {
  id?: number;
  status?: number;
  total?: number;
  finished?: number;
}

interface TaskProgressMapTreeDTO {
  id?: number;
  taskName?: string;
  status?: number;
  children?: TaskProgressMapTreeDTO[];
}
