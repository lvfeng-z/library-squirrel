package persistentStore

import (
	"context"
	"errors"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"

	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
)

var (
	ErrRelPathEmpty  = errors.New("存储路径不能为空")
	ErrFileNameEmpty = errors.New("文件名不能为空")
	ErrFileDataEmpty = errors.New("文件数据不能为空")
)

// Handler 文件持久存储 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建文件持久存储 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// StoreFile 前端上传文件
func (h *Handler) StoreFile(ctx context.Context, relPath string, fileName string, fileData []byte) *model.ApiResponse[int64] {
	if strings.TrimSpace(relPath) == "" {
		return model.HandleError[int64](ErrRelPathEmpty)
	}
	if strings.TrimSpace(fileName) == "" {
		return model.HandleError[int64](ErrFileNameEmpty)
	}
	if len(fileData) == 0 {
		return model.HandleError[int64](ErrFileDataEmpty)
	}

	id, err := h.svc.Store(ctx, relPath, fileName, strings.NewReader(string(fileData)))
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(id)
}

// GetById 获取文件记录
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.PersistentStoreDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.PersistentStoreDTO](err)
	}
	return model.Success(dto2.NewPersistentStoreDTO(result))
}

// Delete 删除文件记录及文件
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}
