import WorkSet from '../entity/WorkSet.ts'
import {WorkFullDTO} from "@bindings/github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
import { isNullish } from '@renderer/utils/CommonUtil.ts'

export default class WorkSetWithWorkDTO {
  /**
   * 作品集
   */
  workSet: WorkSet

  /**
   * 作品集的作品列表
   */
  workList: WorkFullDTO[]

  constructor(workSet: WorkSet, workList?: WorkFullDTO[]) {
    this.workSet = workSet
    this.workList = isNullish(workList) ? [] : workList
  }
}
