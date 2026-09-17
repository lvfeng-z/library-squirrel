/**
 * namespace 内置集的**前端加载兜底**：后端清单拉取失败时由 UseDimensionListStore 投影为候选行，
 * 正常路径的候选（含中文显示名、origin 分组、last_use 排序）以后端 tag_namespace 清单表为权威。
 * 内置集权威定义位于后端 `backend/base/constant/dimension.go`（启动投影进清单表），本表仅与其保持同步。
 * namespace 是开放字符串——清单/兜底集仅作选择器候选，未知 namespace 允许用户自行输入（el-select allow-create）。
 * artist 归 author 体系，不入 tag namespace。
 */

/** namespace 选择器选项 */
export interface NamespaceOption {
  /** namespace 值（开放字符串） */
  value: string
  /** 显示文案 */
  label: string
}

/** 内置 namespace 兜底集（与后端 BuiltinTagNamespaces 同步；仅后端拉取失败时启用） */
export const BUILTIN_NAMESPACES: NamespaceOption[] = [
  { value: 'language', label: '语言' },
  { value: 'character', label: '角色' },
  { value: 'parody', label: '原作' },
  { value: 'female', label: '女性' },
  { value: 'male', label: '男性' },
  { value: 'misc', label: '杂项' },
  { value: 'general', label: '通用' }
]
