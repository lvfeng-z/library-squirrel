import { defineStore } from "pinia";
import { ref } from "vue";
import { useNotificationStore } from "@renderer/store/UseNotificationStore";
import { h } from "vue";

// Task status enum values
const TaskStatusEnum = {
  PROCESSING: 1,
  WAITING: 2,
  FINISHED: 3,
  FAILED: 4,
};

export const useParentTaskStore = defineStore("parentTask", () => {
  // state
  const parentTasks = ref(new Map<number, ParentTaskStoreObj>());

  // getters
  function getParentTask(taskId: number): any {
    return parentTasks.value.get(taskId)?.task;
  }

  // actions
  function setParentTask(taskList: any[]): void {
    const taskStatus: Map<number, ParentTaskStoreObj> = parentTasks.value;
    taskList.forEach((task: any) => {
      let notificationId: string | undefined;
      if (
        TaskStatusEnum.PROCESSING === task.status ||
        TaskStatusEnum.WAITING === task.status
      ) {
        const notificationItem = createNotificationItem(task);
        notificationId = useNotificationStore().add(notificationItem);
      }
      if (!task.id) {
        throw new Error("UseParentTaskStore: 赋值任务失败，任务id为空");
      }
      taskStatus.set(task.id, { task, notificationId });
    });
  }

  function updateParentTask(taskList: any[]): void {
    taskList.forEach((task: any) => {
      if (!task.id) {
        throw new Error("UseParentTaskStore: 更新任务失败，任务id为空");
      }
      const taskStoreObj = parentTasks.value.get(task.id);
      if (taskStoreObj) {
        if (task.status !== taskStoreObj.task.status) {
          if (taskStoreObj.notificationId) {
            if (task.status === TaskStatusEnum.FINISHED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: "success",
                msg: `任务【${taskStoreObj.task.taskName}】完成`,
              });
            } else if (task.status === TaskStatusEnum.FAILED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: "error",
                msg: `任务【${taskStoreObj.task.id}】失败`,
              });
            }
          }
          if (
            !taskStoreObj.notificationId &&
            (TaskStatusEnum.PROCESSING === task.status ||
              TaskStatusEnum.WAITING === task.status)
          ) {
            const notificationItem = createNotificationItem(taskStoreObj.task);
            taskStoreObj.notificationId =
              useNotificationStore().add(notificationItem);
          }
        }
        Object.assign(taskStoreObj.task, task);
      }
    });
  }

  function updateParentTaskSchedule(scheduleDTOList: any[]): void {
    scheduleDTOList.forEach((scheduleDTO: any) => {
      if (!scheduleDTO.id) {
        throw new Error("UseParentTaskStore: 更新任务进度失败，任务id为空");
      }
      const task = getParentTask(scheduleDTO.id);
      if (task) {
        task.status = scheduleDTO.status;
        task.total = scheduleDTO.total;
        task.finished = scheduleDTO.finished;
      }
    });
  }

  function removeParentTask(ids: number[]) {
    const taskStatus = parentTasks.value;
    if (ids && ids.length > 0) {
      ids.forEach((id) => taskStatus.delete(id));
    }
  }

  return {
    parentTasks,
    getParentTask,
    setParentTask,
    updateParentTask,
    updateParentTaskSchedule,
    removeParentTask,
  };
});

export type ParentTaskStoreObj = {
  task: any;
  notificationId: string | undefined;
};

function createNotificationItem(task: any): any {
  return {
    id: undefined,
    title: `任务【${task.taskName}】`,
    render: () => h("div", {}, "处理中"),
  };
}
