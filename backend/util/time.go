package util

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// GetCurrentTimestamp 获取当前时间戳（毫秒）
func GetCurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// GetCurrentTimestampSeconds 获取当前时间戳（秒）
func GetCurrentTimestampSeconds() int64 {
	return time.Now().Unix()
}

// GenerateUUID 生成随机UUID
func GenerateUUID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}
