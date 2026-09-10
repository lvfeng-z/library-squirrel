// 内置任务类型集合(task.task_type 内置取值域;与后端内置建树方 share-receive/export 一致)
// 插件任务的 taskType 为 'plugin-download',不属内置集合
export const BUILTIN_TASK_TYPES: Set<string> = new Set(['share-receive', 'export'])
