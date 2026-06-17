export enum TaskOperationCodeEnum {
  VIEW,
  START,
  PAUSE,
  RESUME,
  RETRY,
  CANCEL,
  DELETE,
  // 板块重执行（多选，仅终态可用；携带 sections 板块代码数组）
  REDOWNLOAD
}
