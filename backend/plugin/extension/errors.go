package extension

import "errors"

// 扩展点注册中心错误
var (
	// ErrExtensionNotFound 扩展点不存在
	ErrExtensionNotFound = errors.New("extension not found")
	// ErrExtensionAlreadyExists 扩展点已存在
	ErrExtensionAlreadyExists = errors.New("extension already exists")
	// ErrPluginAlreadyLoaded 插件进程已加载（同名插件已有活跃进程条目）
	ErrPluginAlreadyLoaded = errors.New("plugin already loaded")
)
