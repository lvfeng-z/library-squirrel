import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * QueryDTO
 * 作品
 */
export default class WorkQueryDTO extends BaseQueryDTO {
    /**
     * 作品来源站点id
     */
    siteId: number | undefined | null
    /**
     * 站点中作品的id
     */
    siteWorkId: string | undefined | null
    /**
     * 站点中作品的名称
     */
    siteWorkName: string | undefined | null
    /**
     * 站点中作品的作者id
     */
    siteAuthorId: string | undefined | null
    /**
     * 站点中作品的描述
     */
    siteWorkDescription: string | undefined | null
    /**
     * 站点中作品的上传时间
     */
    siteUploadTime: number | undefined | null
    /**
     * 站点中作品最后修改的时间
     */
    siteUpdateTime: number | undefined | null
    /**
     * 作品别称
     */
    nickName: string | undefined | null
    /**
     * 最后一次查看的时间
     */
    lastView: number | undefined | null

    constructor(workQueryDTO?: WorkQueryDTO) {
        super(workQueryDTO)
        if (notNullish(workQueryDTO)) {
            this.siteId = workQueryDTO.siteId
            this.siteWorkId = workQueryDTO.siteWorkId
            this.siteWorkName = workQueryDTO.siteWorkName
            this.siteAuthorId = workQueryDTO.siteAuthorId
            this.siteWorkDescription = workQueryDTO.siteWorkDescription
            this.siteUploadTime = workQueryDTO.siteUploadTime
            this.siteUpdateTime = workQueryDTO.siteUpdateTime
            this.nickName = workQueryDTO.nickName
            this.lastView = workQueryDTO.lastView
        }
    }
}
