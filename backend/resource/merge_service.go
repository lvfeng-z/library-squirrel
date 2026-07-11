package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"

	"gorm.io/gorm"
)

// 合并业务错误定义
var (
	// ErrMergeUnavailable 合并功能不可用（系统未安装 ffmpeg）。
	ErrMergeUnavailable = errors.New("合并功能不可用：系统未安装 ffmpeg")
	// ErrVideoTrackNotFound 该资源缺少视频轨。
	ErrVideoTrackNotFound = errors.New("该资源缺少视频轨，无法合并")
	// ErrAudioTrackNotFound 该资源缺少音频轨。
	ErrAudioTrackNotFound = errors.New("该资源缺少音频轨，无法合并")
)

// MergeResult 合并产物信息
type MergeResult struct {
	// MergedStoreID 合并产物的 PersistentStore ID
	MergedStoreID int64 `json:"mergedStoreId"`
}

// Merger 文件合并能力（由 merge.FFmpegMuxer 实现）。
// 输入输出均为文件绝对路径，不感知 store/resource。
type Merger interface {
	MergeRemux(ctx context.Context, videoPath, audioPath, outPath string) error
}

// StoreOps store 落盘/查询/删除/路径原语（由 persistentStore.Service 实现）。
type StoreOps interface {
	GetById(ctx context.Context, id int64) (*domain.PersistentStore, error)
	GetAbsPath(store *domain.PersistentStore) string
	StoreFromFile(ctx context.Context, relPath, fileName, srcAbsPath string) (int64, error)
	Delete(ctx context.Context, id int64, backup bool) (int64, error)
	BuildVariantPath(sourceRelPath, suffix string) string
}

// MergeSettingsReader 读合并策略（由 settings.Service 实现）。
type MergeSettingsReader interface {
	GetMergeStrategy() string
}

// Transactor 事务执行器（事务 DB 通过 ctx 传递，repository 经 dbFromCtx 感知）。
type Transactor interface {
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// MergeService 音视频合并业务编排：取 resource 的 videoTrack/audioTrack → 调合并 →
// 落产物 PersistentStore(merged) → 挂 resource_store。合并能力由 Merger 提供，
// store 落盘/路径由 StoreOps 提供，本服务只做编排。
type MergeService struct {
	resourceStoreRepo *ResourceStoreRepository
	merger            Merger
	storeOps          StoreOps
	settings          MergeSettingsReader
	tx                Transactor
}

// NewMergeService 创建合并业务服务。merger 为 nil 时合并功能不可用（调用返回 ErrMergeUnavailable）。
func NewMergeService(resourceStoreRepo *ResourceStoreRepository, merger Merger, storeOps StoreOps, settings MergeSettingsReader, tx Transactor) *MergeService {
	return &MergeService{
		resourceStoreRepo: resourceStoreRepo,
		merger:            merger,
		storeOps:          storeOps,
		settings:          settings,
		tx:                tx,
	}
}

// MergeResource 对指定 Resource 执行音视频合并（用户主动触发）。
// 产物为新 PersistentStore(store_type=merged) 挂回该 Resource；mergeStrategy=overwrite 时另删原轨道。
func (s *MergeService) MergeResource(ctx context.Context, resourceId int64) (*MergeResult, error) {
	if s.merger == nil {
		return nil, ErrMergeUnavailable
	}

	// 取 videoTrack/audioTrack store（缺轨返回明确中文错误）
	videoRS, err := s.resourceStoreRepo.GetByType(ctx, resourceId, domain.StoreTypeVideoTrack)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVideoTrackNotFound
		}
		return nil, fmt.Errorf("查询视频轨失败: %w", err)
	}
	audioRS, err := s.resourceStoreRepo.GetByType(ctx, resourceId, domain.StoreTypeAudioTrack)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAudioTrackNotFound
		}
		return nil, fmt.Errorf("查询音频轨失败: %w", err)
	}

	// 取源文件绝对路径
	videoPS, err := s.storeOps.GetById(ctx, videoRS.StoreID)
	if err != nil {
		return nil, fmt.Errorf("加载视频轨 store 失败: %w", err)
	}
	audioPS, err := s.storeOps.GetById(ctx, audioRS.StoreID)
	if err != nil {
		return nil, fmt.Errorf("加载音频轨 store 失败: %w", err)
	}
	videoAbs := s.storeOps.GetAbsPath(videoPS)
	audioAbs := s.storeOps.GetAbsPath(audioPS)

	// ffmpeg 输出到临时文件（唯一名，跟随源视频容器扩展）
	videoExt := filepath.Ext(videoPS.FilePath.String)
	if videoExt == "" {
		videoExt = ".mp4"
	}
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("ls-merge-%d-%d%s", resourceId, time.Now().UnixNano(), videoExt))

	// 合并（muxer 自处理超时/失败/产物残留）
	if err := s.merger.MergeRemux(ctx, videoAbs, audioAbs, tmpOut); err != nil {
		return nil, err
	}

	// 产物相对路径（源视频轨旁、文件名加 _merged）与展示文件名
	mergedRelPath := s.storeOps.BuildVariantPath(videoPS.FilePath.String, "_merged")
	mergedFileName := buildMergedFileName(videoPS.FileName.String, videoExt)

	// 落盘 + 建 PersistentStore；临时文件已复制为产物，随之清除
	mergedPsId, err := s.storeOps.StoreFromFile(ctx, mergedRelPath, mergedFileName, tmpOut)
	_ = os.Remove(tmpOut)
	if err != nil {
		return nil, fmt.Errorf("落盘合并产物失败: %w", err)
	}

	// 事务内挂 resource_store(merged)；失败补偿删产物 store
	if err := s.tx.ExecInTransaction(ctx, func(txCtx context.Context) error {
		rs := domain.NewResourceStore()
		rs.ResourceID = resourceId
		rs.StoreType = domain.StoreTypeMerged
		rs.Generation = domain.GenerationDownloaded
		rs.StoreID = mergedPsId
		return s.resourceStoreRepo.Save(txCtx, rs)
	}); err != nil {
		if _, derr := s.storeOps.Delete(ctx, mergedPsId, false); derr != nil {
			return nil, fmt.Errorf("挂载合并产物失败且补偿删除产物 store 也失败，挂载错误: %w", err)
		}
		return nil, fmt.Errorf("挂载合并产物失败: %w", err)
	}

	// overwrite：删原轨道 store 及文件，并清理 resource_store 关联
	if s.settings.GetMergeStrategy() == settings.MergeStrategyOverwrite {
		if _, err := s.storeOps.Delete(ctx, videoPS.GetID(), true); err != nil {
			return nil, fmt.Errorf("删除原视频轨失败: %w", err)
		}
		if _, err := s.storeOps.Delete(ctx, audioPS.GetID(), true); err != nil {
			return nil, fmt.Errorf("删除原音频轨失败: %w", err)
		}
		if err := s.resourceStoreRepo.DeleteByResourceIdAndTypes(ctx, resourceId, []string{domain.StoreTypeVideoTrack, domain.StoreTypeAudioTrack}); err != nil {
			return nil, fmt.Errorf("清理原轨道关联失败: %w", err)
		}
	}

	return &MergeResult{MergedStoreID: mergedPsId}, nil
}

// buildMergedFileName 由源文件名构造合并产物展示名（源名去扩展 + _merged + 扩展）。
func buildMergedFileName(srcFileName, ext string) string {
	base := strings.TrimSuffix(srcFileName, filepath.Ext(srcFileName))
	if base == "" {
		base = "merged"
	}
	return base + "_merged" + ext
}
