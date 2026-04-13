export default class NotifyConfig {
  type?: "error" | "primary" | "success" | "warning" | "info";
  msg?: string;
  duration?: number;
  maxRow?: number;
}
