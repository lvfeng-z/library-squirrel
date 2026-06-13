import BaseAuthor from './BaseAuthor.ts'

export default interface RankAuthor extends BaseAuthor {
  /**
   * 作者角色（自由文本，如：画师、作曲、剪辑等）
   */
  roleName?: string | null
  /**
   * 排序权重
   */
  sortOrder?: number | null
}
