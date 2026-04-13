import Resource from '../entity/Resource.ts'

/**
 * 资源+作品集id
 */
export default class ResourceWithWorkSetId extends Resource {
  /**
   * 作品集id
   */
  workSetId: number | undefined | null
}
