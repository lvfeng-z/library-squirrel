package config

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	yaml "go.yaml.in/yaml/v3"
)

//go:embed default_config.yaml
var defaultConfigFS embed.FS

//go:embed locked_config.yaml
var lockedConfigFS embed.FS

// Config 应用配置（统一配置结构）
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	App      AppConfig      `mapstructure:"app"`
	Task     TaskConfig     `mapstructure:"task"`
	Sites    []SiteConfig   `mapstructure:"sites"`
	Plugins  []PluginConfig `mapstructure:"plugins"`
	// Locked 订死配置：随二进制分发的权威数据，由 loadLocked 独立加载（不走 viper 合并链，
	// 磁盘 config.yaml 无对应覆盖物）；mapstructure:"-" 显式排除出主 Unmarshal，防磁盘手写 locked 键弄脏单一来源
	Locked LockedConfig `mapstructure:"-"`
}

// LockedConfig 订死配置——随二进制分发的权威数据。加载走独立 go:embed + yaml.Unmarshal，
// 不参与 viper 两层合并（磁盘无覆盖物，防篡改由覆盖路径不存在保证）。
// 未来新增「随二进制分发的权威数据」一律进本文件并保持不进合并链
type LockedConfig struct {
	OfficialPlugins []OfficialPluginEntry `yaml:"officialPlugins"` // 官方插件指纹名单（构建管线累积维护，旧条目不删；空名单时官方判定全不命中）
}

// OfficialPluginEntry 官方插件指纹名单条目：以 (publicId, buildId) 定位、contentDigest 终裁
type OfficialPluginEntry struct {
	PublicID      string `yaml:"publicId"`      // 插件身份键（反向域名）
	BuildID       string `yaml:"buildId"`       // 构建身份标识（git describe 输出，同源码状态重构建同值）
	ContentDigest string `yaml:"contentDigest"` // 包内容摘要（文件级 sha256 聚合 hex，排除 zip 容器元数据）
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port int    `mapstructure:"port"` // HTTP 端口
	Mode string `mapstructure:"mode"` // Gin 模式 (debug, release, test)
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path string `mapstructure:"path"` // SQLite 数据库路径
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `mapstructure:"level"`      // 日志级别 (debug, info, warn, error)
	Format     string `mapstructure:"format"`     // 控制台日志格式 (json, console)
	MaxSize    int    `mapstructure:"maxSize"`    // 单个日志文件最大尺寸（MB），超过后轮转
	MaxBackups int    `mapstructure:"maxBackups"` // 保留的旧日志文件最大数量
	MaxAge     int    `mapstructure:"maxAge"`     // 保留旧日志文件的最大天数
	Compress   bool   `mapstructure:"compress"`   // 是否压缩轮转的旧日志文件
}

// AppConfig 应用配置
type AppConfig struct {
	Host         string `mapstructure:"host"`         // 监听地址
	ResourcePath string `mapstructure:"resourcePath"` // 资源文件路径
	DataPath     string `mapstructure:"dataPath"`     // 数据存储路径
}

// SiteConfig 站点配置
type SiteConfig struct {
	Name        string `mapstructure:"name"`        // 站点名称
	Description string `mapstructure:"description"` // 站点描述
}

// PluginConfig 插件配置
type PluginConfig struct {
	PackagePath string `mapstructure:"packagePath"` // 插件包路径
	PathType    string `mapstructure:"pathType"`    // 路径类型 (Relative, Absolute)
}

// TaskConfig 任务相关配置
type TaskConfig struct {
	UseSnapshotMode bool `mapstructure:"useSnapshotMode"` // 是否使用快照模式推送任务状态（默认 true）
}

var cfg *Config

// Load 加载配置
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// 设置默认值
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "release")
	viper.SetDefault("database.path", "database/database.db")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("app.host", "127.0.0.1")
	viper.SetDefault("app.resourcePath", "./resources")
	viper.SetDefault("app.dataPath", "./data")
	viper.SetDefault("task.useSnapshotMode", true)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// LoadFromDir 从指定目录加载配置
// 加载策略：嵌入的 default_config.yaml 作为基础层，磁盘上的 config.yaml 作为覆盖层
// 磁盘配置文件不存在时，仅使用嵌入的默认配置
func LoadFromDir(dir string) (*Config, error) {
	// 加载嵌入的默认配置作为基础层
	viper.SetConfigType("yaml")
	defaultData, err := defaultConfigFS.ReadFile("default_config.yaml")
	if err != nil {
		return nil, fmt.Errorf("读取嵌入的默认配置失败: %w", err)
	}
	if err := viper.MergeConfig(bytes.NewReader(defaultData)); err != nil {
		return nil, fmt.Errorf("解析默认配置失败: %w", err)
	}

	// 如果磁盘上存在 config.yaml，作为覆盖层合并
	configFile := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configFile); err == nil {
		viper.SetConfigFile(configFile)
		if err := viper.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("合并磁盘配置失败: %w", err)
		}
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.Locked = loadLocked()

	return cfg, nil
}

// parseLocked 解析订死配置内容。解析失败不拦启动：返回空名单（保守降级，官方判定全不命中）。
// config 包不依赖 logger（logger 反向依赖 config），降级告警走 stderr
func parseLocked(data []byte) LockedConfig {
	var locked LockedConfig
	if err := yaml.Unmarshal(data, &locked); err != nil {
		fmt.Fprintf(os.Stderr, "解析订死配置 locked_config.yaml 失败（空名单降级）: %v\n", err)
		return LockedConfig{}
	}
	return locked
}

// loadLocked 读取嵌入的订死配置文件并解析。go:embed 编译期内嵌、文件必然存在；
// 本函数是订死配置的唯一取值路径（恒不进 viper 合并链）
func loadLocked() LockedConfig {
	data, err := lockedConfigFS.ReadFile("locked_config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取嵌入订死配置失败（空名单降级）: %v\n", err)
		return LockedConfig{}
	}
	return parseLocked(data)
}

// Get 获取全局配置
func Get() *Config {
	return cfg
}
