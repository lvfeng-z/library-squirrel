package storageLocation

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/util"
)

// WorkDirProvider 资源库目录提供者接口（settings.Service 实现，实时读取——
// 用户可能中途改库，禁止启动期缓存快照）
type WorkDirProvider interface {
	GetWorkDir() string
}

// Service 存储位置服务（三目录契约的展示面：程序目录/用户数据目录/资源库目录）
type Service struct {
	workDirProvider WorkDirProvider
}

// NewService 创建存储位置服务
func NewService(workDirProvider WorkDirProvider) *Service {
	return &Service{workDirProvider: workDirProvider}
}

// StorageLocations 三目录绝对路径（设置页「存储位置」区块数据源）
type StorageLocations struct {
	ProgramDir string `json:"programDir"` // 程序目录：exe 所在目录，前端资源与捆绑插件随安装包分发，运行期只读
	DataDir    string `json:"dataDir"`    // 用户数据目录：配置/数据库/日志/插件等全部写输出的落盘根
	WorkDir    string `json:"workDir"`    // 资源库目录：settings 的 workdir（媒体文件/暂存/备份）；未配置为空串
}

// GetStorageLocations 返回三目录绝对路径。用户数据目录解析失败在启动期已属致命场景
// （NewApp fail-fast），此处仍优雅返回错误交由前端提示；资源库目录每次实时读 settings。
func (s *Service) GetStorageLocations(ctx context.Context) (*StorageLocations, error) {
	dataDir, err := util.DataDir()
	if err != nil {
		return nil, fmt.Errorf("解析用户数据目录失败: %w", err)
	}
	return &StorageLocations{
		ProgramDir: util.RootPath(),
		DataDir:    dataDir,
		WorkDir:    s.workDirProvider.GetWorkDir(),
	}, nil
}
