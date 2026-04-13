import AdmZip from 'adm-zip'
import { PluginManifest } from '../../../main-go/plugin/types/PluginManifest.ts'
import { ActivationConfig } from '../../../main-go/plugin/types/ActivationTypes.ts'

export default class PluginInstallDTO {
  /**
   * 公开id
   */
  publicId: string

  /**
   * 作者
   */
  author: string

  /**
   * 名称
   */
  name: string

  /**
   * 版本
   */
  version: string

  /**
   * 加载配置
   */
  activation: ActivationConfig

  /**
   * 入口文件名
   */
  entryFile: string

  /**
   * 安装包路径
   */
  packagePath: string

  /**
   * 安装包
   */
  package: AdmZip

  constructor(src: PluginInstallDTOConstr) {
    this.publicId = src.id
    this.author = src.author
    this.name = src.name
    this.version = src.version
    this.activation = src.activation
    this.entryFile = src.entryFile
    this.packagePath = src.packagePath
    this.package = src.package
  }
}

interface PluginInstallDTOConstr extends PluginManifest {
  /**
   * 安装包路径
   */
  packagePath: string

  /**
   * 安装包
   */
  package: AdmZip
}
