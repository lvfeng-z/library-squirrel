package model

// SiteBrowser 站点浏览器接口
type SiteBrowser interface {
	// Open 打开浏览器
	Open() error
	// Close 关闭浏览器
	Close() error
}
