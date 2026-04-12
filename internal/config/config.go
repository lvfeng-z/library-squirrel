package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 应用配置（统一配置结构）
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	App      AppConfig      `mapstructure:"app"`
	Sites    []SiteConfig   `mapstructure:"sites"`
	Plugins  []PluginConfig `mapstructure:"plugins"`
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
	Level  string `mapstructure:"level"`  // 日志级别 (debug, info, warn, error)
	Format string `mapstructure:"format"` // 日志格式 (json, console)
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

var cfg *Config

// Load 加载配置
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// 设置默认值
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("database.path", "data.db")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("app.host", "127.0.0.1")
	viper.SetDefault("app.resourcePath", "./resources")
	viper.SetDefault("app.dataPath", "./data")

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
func LoadFromDir(dir string) (*Config, error) {
	configFile := filepath.Join(dir, "config.yaml")
	return Load(configFile)
}

// Get 获取全局配置
func Get() *Config {
	return cfg
}
