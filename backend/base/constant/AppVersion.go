package constant

// AppVersion 应用版本号——二进制自述场景（导出 manifest meta.appVersion 等）的版本来源。
// 默认值 0.0.1；构建期经 -ldflags "-X backend/base/constant.AppVersion=vX.Y.Z" 覆盖（var 而非 const，
// 链接期符号替换仅作用于字符串变量），未注入时回落默认值。
var AppVersion = "0.0.1"
