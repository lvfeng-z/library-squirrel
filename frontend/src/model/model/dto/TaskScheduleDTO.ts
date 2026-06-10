import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 后端 taskScheduleDTO（progress_pusher.go）的前端映射。
 *
 * 后端通过 Events.Emit("task-events"/"parent-events", {type: "updateSchedule"|"updateParentSchedule", data: []*taskScheduleDTO})
 * 推送增量进度，taskScheduleDTO 字段为非指针值类型，Go 零值不会序列化到 JSON 中，
 * 因此缺失字段在前端为 undefined，用于实现部分更新语义（Store 中通过 notNullish 判断跳过）。
 */
export default class TaskScheduleDTO {
  id: number | undefined | null
  total: number | undefined | null
  finished: number | undefined | null

  constructor(data?: any) {
    if (notNullish(data)) {
      this.id = data.id
      this.total = data.total
      this.finished = data.finished
    }
  }
}
