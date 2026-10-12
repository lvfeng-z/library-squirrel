package externalapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// tokenEntropyBytes 随机 token 的熵字节数（URL 安全 base64 编码后为 43 字符）
const tokenEntropyBytes = 32

// bearerPrefix 鉴权头的方案前缀（Authorization: Bearer <token>）
const bearerPrefix = "Bearer "

// GenerateToken 随机生成外部接口 token（URL 安全 base64、无填充）。
// token 的防线定位：防误触、防跨用户、提供归属标识；同机同用户进程可读发现文件，
// 不构成对抗恶意进程的防线
func GenerateToken() (string, error) {
	buf := make([]byte, tokenEntropyBytes)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("生成随机 token 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// TokenAuthenticator token 鉴权器：内存单点比对——token 只存于本进程，更换即旧 token
// 立即失效（无宽限窗口）。单 token 语义：一把钥匙代表「外部侧」整体，不按调用方逐个
// 发证；操作记录中的客户端标签为调用方自报归属，非鉴别依据
type TokenAuthenticator struct {
	token atomic.Pointer[string]
}

// NewTokenAuthenticator 以指定 token 创建鉴权器
func NewTokenAuthenticator(token string) *TokenAuthenticator {
	a := &TokenAuthenticator{}
	a.token.Store(&token)
	return a
}

// Token 返回当前 token（发现文件写入与设置页展示用）
func (a *TokenAuthenticator) Token() string {
	if p := a.token.Load(); p != nil {
		return *p
	}
	return ""
}

// SetToken 更换 token；内存单点比对意味着旧 token 随即失效
func (a *TokenAuthenticator) SetToken(token string) {
	a.token.Store(&token)
}

// Authenticate 校验请求鉴权头：须为 Bearer 形态且与当前 token 常数时间比对一致。
// 无 token、非 Bearer 形态、伪 token 一律不放行（同归鉴权失败）
func (a *TokenAuthenticator) Authenticate(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		return false
	}
	presented := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	current := a.Token()
	if presented == "" || current == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(current)) == 1
}
