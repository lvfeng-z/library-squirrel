// 资源 store_type(与后端 entity.StoreType* 常量一致)
export const StoreRole = {
  IMAGE: 'image', // 图片(image 资源主体;article 内嵌图多例)
  DOCUMENT: 'document', // 文档文件(article 正文 .md;document 原文件 .pdf/.docx)
  THUMBNAIL: 'thumbnail', // 缩略图/封面
  VIDEO_TRACK: 'videoTrack', // 视频轨
  AUDIO_TRACK: 'audioTrack', // 音频轨
  MERGED: 'merged' // 合并产物
} as const

// 兜底资源角色集(task 未声明 involvedRoles 时展示的可选全集;用户不勾选任何板块时下发等价全量重执行)
export const ALL_STORE_ROLES: string[] = [
  StoreRole.IMAGE,
  StoreRole.DOCUMENT,
  StoreRole.THUMBNAIL,
  StoreRole.VIDEO_TRACK,
  StoreRole.AUDIO_TRACK,
  StoreRole.MERGED
]

// 资源板块中文标签(操作栏勾选项展示)
export const StoreRoleLabels: Record<string, string> = {
  [StoreRole.IMAGE]: '图片',
  [StoreRole.DOCUMENT]: '文档',
  [StoreRole.THUMBNAIL]: '缩略图',
  [StoreRole.VIDEO_TRACK]: '视频轨',
  [StoreRole.AUDIO_TRACK]: '音频轨',
  [StoreRole.MERGED]: '合并'
}

// 资源类型(与后端 entity.ResourceType* 常量一致;前端按此分发渲染/外部打开)
export const ResourceType = {
  IMAGE: 'image', // 图片资源(单图/多图每子资源)
  VIDEO: 'video', // 视频资源(视频轨+音频轨,可合并)
  ARTICLE: 'article', // 图文紧密结合文档(正文 markdown + 内嵌图)
  DOCUMENT: 'document', // 现成文档原文件(pdf/docx/...)
  UNKNOWN: 'unknown' // 插件确实无法分类时声明
} as const
