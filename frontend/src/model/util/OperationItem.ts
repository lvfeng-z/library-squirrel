// DataTable的操作栏按钮或下拉菜单按钮
export default interface OperationItem<RowType> {
  /**
   * 文本
   */
  label: string

  /**
   * 图标（el-icon）
   */
  icon: string

  /**
   * 编码（用于区分是哪个按钮被点击）
   */
  code: string

  /**
   * 按钮类型
   */
  buttonType?: 'primary' | 'warning' | 'success' | 'danger' | 'text' | 'info'

  /**
   * 操作按钮的显隐逻辑
   * @param row 行数据
   */
  rule?: (row: RowType) => boolean
}
