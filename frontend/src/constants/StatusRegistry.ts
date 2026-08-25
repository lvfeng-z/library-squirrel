import { TaskStatusEnum } from './TaskStatusEnum'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

/** 状态类目：与 tokens.css 中 --app-status-{类目}-{语义} 命名对应 */
export type StatusCategory = 'task' | 'source' | 'toggle' | 'resource' | 'plugin' | 'backup' | 'recycle'

export interface StatusMeta {
  /** 状态唯一标识，与 tokens.css 的 --app-status-{key}-{bg|text|border} 令牌后缀严格一致 */
  key: string
  /** 默认显示文案 */
  label: string
  /** 所属类目 */
  category: StatusCategory
}

/** 状态注册表：status key → { label, category } 的单一真相源。
 *  新增状态时：先在 tokens.css 补齐 --app-status-{key}-{bg/text/border} 三档令牌，再在此登记。 */
export const STATUS_REGISTRY: Record<string, StatusMeta> = {
  // —— 任务状态 ——
  'task-created': { key: 'task-created', label: '已创建', category: 'task' },
  'task-processing': { key: 'task-processing', label: '进行中', category: 'task' },
  'task-waiting': { key: 'task-waiting', label: '等待中', category: 'task' },
  'task-pausing': { key: 'task-pausing', label: '暂停中', category: 'task' },
  'task-paused': { key: 'task-paused', label: '已暂停', category: 'task' },
  'task-stopping': { key: 'task-stopping', label: '停止中', category: 'task' },
  'task-completed': { key: 'task-completed', label: '完成', category: 'task' },
  'task-partly-finished': { key: 'task-partly-finished', label: '部分完成', category: 'task' },
  'task-failed': { key: 'task-failed', label: '失败', category: 'task' },
  'task-waiting-input': { key: 'task-waiting-input', label: '等待确认', category: 'task' },
  // —— 来源类型（作者/标签的 local/site）——
  'source-local': { key: 'source-local', label: '本地', category: 'source' },
  'source-site': { key: 'source-site', label: '站点', category: 'source' },
  // —— 开关/运行态 ——
  'toggle-enabled': { key: 'toggle-enabled', label: '启用', category: 'toggle' },
  'toggle-disabled': { key: 'toggle-disabled', label: '禁用', category: 'toggle' },
  // —— 资源/作品状态 ——
  'resource-downloaded': { key: 'resource-downloaded', label: '已下载', category: 'resource' },
  'resource-missing': { key: 'resource-missing', label: '缺失', category: 'resource' },
  'resource-damaged': { key: 'resource-damaged', label: '损坏', category: 'resource' },
  // —— 插件渠道/官方身份/信任（渠道：bundled=捆绑、local/url/marketplace=第三方渠道；官方身份：official=内容命中官方指纹名单；信任：trusted/unverified） ——
  'plugin-bundled': { key: 'plugin-bundled', label: '捆绑', category: 'plugin' },
  'plugin-local': { key: 'plugin-local', label: '本地', category: 'plugin' },
  'plugin-url': { key: 'plugin-url', label: '网络', category: 'plugin' },
  'plugin-marketplace': { key: 'plugin-marketplace', label: '市场', category: 'plugin' },
  'plugin-official': { key: 'plugin-official', label: '官方', category: 'plugin' },
  'plugin-unverified': { key: 'plugin-unverified', label: '未信任', category: 'plugin' },
  'plugin-trusted': { key: 'plugin-trusted', label: '已信任', category: 'plugin' },
  // —— 备份引用态（有主=被业务行引用，由回收站/插件流程管理；无主=可清理） ——
  'backup-referenced': { key: 'backup-referenced', label: '有主', category: 'backup' },
  'backup-orphaned': { key: 'backup-orphaned', label: '无主', category: 'backup' },
  // —— 回收站文件条目状态（可复原=有备份且挂载活作品，操作入口随替换/merge 软删化接通；
  //    无备份=外部裁决失效或备份缺失；离链=挂载链断的历史残迹） ——
  'recycle-store-restorable': { key: 'recycle-store-restorable', label: '可复原', category: 'recycle' },
  'recycle-store-no-backup': { key: 'recycle-store-no-backup', label: '已失效', category: 'recycle' },
  'recycle-store-orphan': { key: 'recycle-store-orphan', label: '离链', category: 'recycle' }
}

/** TaskStatusEnum(后端状态码) → 状态别名 key 映射 */
const TASK_STATUS_TO_KEY: Record<number, string> = {
  [TaskStatusEnum.CREATED]: 'task-created',
  [TaskStatusEnum.WAITING]: 'task-waiting',
  [TaskStatusEnum.PROCESSING]: 'task-processing',
  [TaskStatusEnum.PAUSING]: 'task-pausing',
  [TaskStatusEnum.PAUSED]: 'task-paused',
  [TaskStatusEnum.STOPPING]: 'task-stopping',
  [TaskStatusEnum.FINISHED]: 'task-completed',
  [TaskStatusEnum.FAILED]: 'task-failed',
  [TaskStatusEnum.PARTLY_FINISHED]: 'task-partly-finished',
  [TaskStatusEnum.WAITING_FOR_INPUT]: 'task-waiting-input'
}

/** 未知任务状态回退 key（对应原 statusTagTypeMap 中 invalidStatus=-1 → danger 的语义） */
export const TASK_STATUS_UNKNOWN_KEY = 'task-failed'

export function getStatusMeta(key: string): StatusMeta | undefined {
  return STATUS_REGISTRY[key]
}

export function getStatusLabel(key: string): string {
  return STATUS_REGISTRY[key]?.label ?? key
}

/** 后端任务状态码 → 状态别名 key；空值或未知码回退到 unknown key */
export function taskStatusToKey(status: number | null | undefined): string {
  if (isNullish(status)) {
    return TASK_STATUS_UNKNOWN_KEY
  }
  return TASK_STATUS_TO_KEY[status] ?? TASK_STATUS_UNKNOWN_KEY
}
