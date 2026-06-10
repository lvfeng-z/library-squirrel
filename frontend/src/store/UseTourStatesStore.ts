import { defineStore } from 'pinia'
import {type SettingChange, Settings} from "@bindings/github.com/library-squirrel/backend/settings";
import mitt, { Emitter } from 'mitt'
import ApiUtil from '@renderer/utils/ApiUtil.ts'
import { settingsGetSettings, settingsSaveSettings } from '@renderer/apis/http/wrappers/settings'

export const useTourStatesStore = defineStore('tourStates', {
  state: (): { tourStates: TourStates } => {
    const getSettings = async (): Promise<Settings> => {
      const response = await settingsGetSettings()
      if (ApiUtil.check(response)) {
        return ApiUtil.data(response) as Settings
      } else {
        throw new Error('获取设置失败')
      }
    }
    return { tourStates: new TourStates(getSettings, async (changes: SettingChange[]) => {
        await settingsSaveSettings(changes)
      })
    }
  }
})

export class TourStates {
  /**
   * 向导页菜单向导开关
   */
  guideMenuTour: boolean

  /**
   * 工作目录向导开关
   */
  workdirTour: boolean

  /**
   * 任务菜单向导开关
   */
  taskMenuTour: boolean

  /**
   * 任务向导开关
   */
  taskTour: boolean

  emitter: Emitter<{
    guideMenuTour: void
    workdirTour: void
    taskMenuTour: void
    taskTour: void
  }>

  settingGetter: () => Promise<Settings>
  settingSetter: (changes: SettingChange[]) => Promise<void>

  constructor(settingGetter: () => Promise<Settings>, settingSetter: (changes: SettingChange[]) => Promise<void>) {
    this.guideMenuTour = false
    this.workdirTour = false
    this.taskMenuTour = false
    this.taskTour = false
    this.emitter = mitt()
    this.settingGetter = settingGetter
    this.settingSetter = settingSetter
  }

  public getCallback(eventName: TourEvents) {
    this.emitter.emit(eventName)
  }

  public async startGuideTour(): Promise<void> {
    this.guideMenuTour = true
    return this.waitUserFinish('guideMenuTour')
  }

  public async startWorkdirTour(): Promise<void> {
    this.workdirTour = true
    return this.waitUserFinish('workdirTour')
  }

  public async startTaskTour(): Promise<void> {
    this.taskMenuTour = true
    // 等待用户完成向导
    await this.waitUserFinish('taskMenuTour')
    this.taskTour = true
  }

  private async waitUserFinish(eventName: TourEvents): Promise<void> {
    let tempResolve: () => void
    const waitFinish: Promise<void> = new Promise((resolve) => (tempResolve = resolve))
    this.emitter.on(eventName, () => tempResolve())
    return waitFinish
  }
}

export type TourEvents = 'guideMenuTour' | 'workdirTour' | 'taskMenuTour' | 'taskTour'
