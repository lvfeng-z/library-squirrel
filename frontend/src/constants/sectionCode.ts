// 资源 store_type(与后端 entity.StoreType* 常量一致)
export const StoreRole = {
  MAIN: 'main', // 主资源
  THUMBNAIL: 'thumbnail', // 缩略图/封面
  VIDEO_TRACK: 'videoTrack', // 视频轨(预留,多轨阶段启用)
  AUDIO_TRACK: 'audioTrack', // 音频轨(预留,多轨阶段启用)
  MERGED: 'merged' // 合并产物(预留,合并动作产出)
} as const

// 兜底资源角色集(task 未声明 involvedRoles 时展示的可选全集;用户不勾选任何板块时下发等价全量重执行)
export const ALL_STORE_ROLES: string[] = [
  StoreRole.MAIN,
  StoreRole.THUMBNAIL,
  StoreRole.VIDEO_TRACK,
  StoreRole.AUDIO_TRACK,
  StoreRole.MERGED
]

// 资源板块中文标签(操作栏勾选项展示)
export const StoreRoleLabels: Record<string, string> = {
  [StoreRole.MAIN]: '资源',
  [StoreRole.THUMBNAIL]: '缩略图',
  [StoreRole.VIDEO_TRACK]: '视频轨',
  [StoreRole.AUDIO_TRACK]: '音频轨',
  [StoreRole.MERGED]: '合并'
}
