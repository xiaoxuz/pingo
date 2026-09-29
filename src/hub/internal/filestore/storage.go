package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrFileTooLarge = errors.New("file too large")
	ErrFileNotFound = errors.New("file not found")
)

type FileStore interface {
	Save(filename string, reader io.Reader) (storageKey string, size int64, err error)
	Get(storageKey string) (reader io.ReadCloser, size int64, filename string, err error)
	Delete(storageKey string) error
}

type LocalFileStore struct {
	basePath    string
	maxFileSize int64
}

func NewLocalFileStore(basePath string, maxFileSizeStr string) (*LocalFileStore, error) {
	maxSize := parseSize(maxFileSizeStr)
	if maxSize == 0 {
		maxSize = 50 * 1024 * 1024 // 50MB
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}

	return &LocalFileStore{
		basePath:    basePath,
		maxFileSize: maxSize,
	}, nil
}

func (s *LocalFileStore) Save(filename string, reader io.Reader) (string, int64, error) {
	// 生成唯一文件名
	hash := sha256.New()
	ext := filepath.Ext(filename)
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	safeName := timestamp + "_" + sanitizeFilename(strings.TrimSuffix(filename, ext)) + ext
	storageKey := filepath.Join("files", safeName)

	fullPath := filepath.Join(s.basePath, safeName)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", 0, fmt.Errorf("create dir: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// 限流写入，限制最大大小
	limited := io.LimitReader(reader, s.maxFileSize+1)
	size, err := io.Copy(io.MultiWriter(f, hash), limited)
	if err != nil {
		os.Remove(fullPath)
		return "", 0, fmt.Errorf("write file: %w", err)
	}

	if size > s.maxFileSize {
		os.Remove(fullPath)
		return "", 0, ErrFileTooLarge
	}

	_ = hex.EncodeToString(hash.Sum(nil))

	return storageKey, size, nil
}

func (s *LocalFileStore) Get(storageKey string) (io.ReadCloser, int64, string, error) {
	// storageKey 格式: files/filename
	filename := strings.TrimPrefix(storageKey, "files/")
	filename = filepath.Base(filename) // 防止路径穿越
	fullPath := filepath.Join(s.basePath, filename)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, "", ErrFileNotFound
		}
		return nil, 0, "", err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, "", err
	}

	// 从文件名提取原始名称
	origName := extractOriginalName(filename)

	return f, info.Size(), origName, nil
}

func (s *LocalFileStore) Delete(storageKey string) error {
	filename := strings.TrimPrefix(storageKey, "files/")
	filename = filepath.Base(filename)
	fullPath := filepath.Join(s.basePath, filename)

	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func parseSize(sizeStr string) int64 {
	sizeStr = strings.ToLower(strings.TrimSpace(sizeStr))
	if sizeStr == "" {
		return 0
	}

	multiplier := int64(1)
	if strings.HasSuffix(sizeStr, "gb") {
		multiplier = 1024 * 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "gb")
	} else if strings.HasSuffix(sizeStr, "mb") {
		multiplier = 1024 * 1024
		sizeStr = strings.TrimSuffix(sizeStr, "mb")
	} else if strings.HasSuffix(sizeStr, "kb") {
		multiplier = 1024
		sizeStr = strings.TrimSuffix(sizeStr, "kb")
	}

	var num int64
	fmt.Sscanf(sizeStr, "%d", &num)
	return num * multiplier
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"..", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}

func extractOriginalName(filename string) string {
	// 格式: timestamp_name.ext
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return filename
}
