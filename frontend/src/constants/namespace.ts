/**
 * namespace 内置已知集（tag 关联级 namespace 维度的常见取值）。
 * namespace 是开放字符串——此集合仅作 namespace 选择器的快捷选项，未知 namespace 允许用户自行输入（el-select allow-create）。
 * artist 归 author 体系，不入 tag namespace。
 */

/** namespace 选择器选项 */
export interface NamespaceOption {
  /** namespace 值（开放字符串） */
  value: string
  /** 显示文案 */
  label: string
}

/** 内置 namespace 快捷选项（常见取值） */
export const BUILTIN_NAMESPACES: NamespaceOption[] = [
  { value: 'language', label: '语言' },
  { value: 'character', label: '角色' },
  { value: 'parody', label: '原作' },
  { value: 'female', label: '女性' },
  { value: 'male', label: '男性' },
  { value: 'misc', label: '杂项' },
  { value: 'general', label: '通用' }
]
