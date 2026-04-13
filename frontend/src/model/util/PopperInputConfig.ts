import { VNode } from 'vue'
import { CommonInputConfig, ICommonInputConfig } from '@renderer/model/util/CommonInputConfig.ts'

export class PopperInputConfig extends CommonInputConfig implements IPopperInputConfig {
  popperRender?: (data?) => VNode

  constructor(config: ICommonInputConfig) {
    super(config)
  }
}

export interface IPopperInputConfig extends ICommonInputConfig {
  popperRender?: (data?) => VNode
}
