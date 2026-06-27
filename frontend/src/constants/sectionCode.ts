// 资源 store_type(与后端 entity.StoreType* 常量一致)
export const StoreRole = {
  MAIN: 'main', // 主资源
  THUMBNAIL: 'thumbnail', // 缩略图/封面
  VIDEO_TRACK: 'videoTrack', // 视频轨(预留,多轨阶段启用)
  AUDIO_TRACK: 'audioTrack', // 音频轨(预留,多轨阶段启用)
  MERGED: 'merged' // 合并产物(预留,合并动作产出)
} as const

// 全集资源角色(用户不勾选任何资源板块时下发,等价完整资源重执行)
export const ALL_STORE_ROLES: string[] = [StoreRole.MAIN, StoreRole.THUMBNAIL]
