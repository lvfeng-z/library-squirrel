import { defineStore } from "pinia";
import { ref } from "vue";

export const useTaskStore = defineStore("task", () => {
  // state
  const tasks = ref(new Map<number, any>());

  // actions
  function getTask(taskId: number): any {
    return tasks.value.get(taskId);
  }

  function setTask(taskList: any[]): void {
    const taskStatus: Map<number, any> = tasks.value;
    taskList.forEach((task: any) => {
      if (task.id) {
        taskStatus.set(task.id, { task, notificationId: undefined });
      }
    });
  }

  function hasTask(taskId: number): boolean {
    return tasks.value.has(taskId);
  }

  function updateTask(taskList: any[]): void {
    taskList.forEach((task: any) => {
      if (task.id) {
        const taskStoreObj = tasks.value.get(task.id);
        if (taskStoreObj) {
          Object.assign(taskStoreObj.task, task);
        }
      }
    });
  }

  function updateTaskSchedule(scheduleDTOList: any[]): void {
    scheduleDTOList.forEach((scheduleDTO: any) => {
      if (scheduleDTO.id) {
        const task = getTask(scheduleDTO.id);
        if (task) {
          task.status = scheduleDTO.status;
          task.total = scheduleDTO.total;
          task.finished = scheduleDTO.finished;
        }
      }
    });
  }

  function removeTask(ids: number[]) {
    const taskStatus = tasks.value;
    if (ids && ids.length > 0) {
      ids.forEach((id) => taskStatus.delete(id));
    }
  }

  return {
    tasks,
    getTask,
    setTask,
    hasTask,
    updateTask,
    updateTaskSchedule,
    removeTask,
  };
});
