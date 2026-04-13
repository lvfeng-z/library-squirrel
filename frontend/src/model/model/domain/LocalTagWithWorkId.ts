import LocalTag from '../entity/LocalTag.ts'

export default class LocalTagWithWorkId extends LocalTag {
  /**
   * 作品id
   */
  workId: number | undefined | null
}
