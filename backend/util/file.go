package util

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// RootPath 获取根目录
func RootPath() string {
	appEnv, ok := os.LookupEnv("APP_ENV")
	var rootPath string
	var err error
	if ok {
		switch appEnv {
		case "development":
			// 如果是 go run，则返回当前的源代码目录
			rootPath, err = os.Getwd()
			if err != nil {
				panic(err)
			}
		default:
			panic(errors.New("unknown APP_ENV environment: " + appEnv))
		}
	} else {
		execPath, err := os.Executable()
		if err != nil {
			panic(err)
		}
		rootPath = filepath.Dir(execPath)
		// 如果可执行文件在 bin 目录下，返回上级目录（项目根目录）
		if filepath.Base(rootPath) == "bin" {
			rootPath = filepath.Dir(rootPath)
		}
	}
	return rootPath
}

// ErrFileNotFound 文件不存在
var ErrFileNotFound = errors.New("file not found")

// ErrInvalidZip 无效的 ZIP 文件
var ErrInvalidZip = errors.New("invalid zip file")

// CreateDirIfNotExists 创建目录（如果不存在）
func CreateDirIfNotExists(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RemoveDir 删除目录（递归）
func RemoveDir(path string) error {
	if FileExists(path) {
		return os.RemoveAll(path)
	}
	return nil
}

// ExtractZip 解压 ZIP 文件到目标目录
func ExtractZip(zipPath string, destDir string) error {
	// 打开 ZIP 文件
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return ErrInvalidZip
	}
	defer reader.Close()

	// 创建目标目录
	if err := CreateDirIfNotExists(destDir); err != nil {
		return err
	}

	// 遍历并解压文件
	for _, file := range reader.File {
		if err := extractFile(file, destDir); err != nil {
			return err
		}
	}

	return nil
}

// extractFile 解压单个文件
func extractFile(file *zip.File, destDir string) error {
	// 构建目标路径
	filePath := filepath.Join(destDir, file.Name)

	// 检查文件路径是否为目录或包含不安全字符
	if file.FileInfo().IsDir() {
		return CreateDirIfNotExists(filePath)
	}

	// 确保父目录存在
	if err := CreateDirIfNotExists(filepath.Dir(filePath)); err != nil {
		return err
	}

	// 打开源文件
	srcFile, err := file.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// 复制内容
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

// CopyFile 复制文件
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// 确保目标目录存在
	if err := CreateDirIfNotExists(filepath.Dir(dst)); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// GetFileSize 获取文件大小
func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// JoinPath 安全拼接路径
func JoinPath(base string, paths ...string) string {
	result := base
	for _, p := range paths {
		result = filepath.Join(result, p)
	}
	return result
}
