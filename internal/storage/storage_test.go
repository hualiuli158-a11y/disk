package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalStorageLifecycle(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}

	const key = "sha256/ab/cd/example"
	const content = "hello storage"

	if err := store.Save(context.Background(), key, strings.NewReader(content)); err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}

	reader, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		_ = reader.Close()
		t.Fatalf("读取文件失败: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("关闭文件失败: %v", err)
	}
	if string(data) != content {
		t.Fatalf("文件内容不一致: got %q, want %q", data, content)
	}

	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("删除文件失败: %v", err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("重复删除应当成功: %v", err)
	}
	if _, err := store.Open(context.Background(), key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("已删除的文件应当不存在，实际错误: %v", err)
	}
}

func TestLocalStorageRejectsUnsafeKeys(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}

	unsafeKeys := []string{
		"",
		"../secret",
		"folder/../secret",
		"folder//file",
		"/absolute/path",
		`C:\absolute\path`,
	}

	for _, key := range unsafeKeys {
		t.Run(key, func(t *testing.T) {
			if err := store.Save(context.Background(), key, strings.NewReader("data")); err == nil {
				t.Fatalf("不安全的 key %q 应当被拒绝", key)
			}
		})
	}
}

func TestLocalStorageHonorsCancelledContext(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("创建存储失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.Save(ctx, "cancelled/file", strings.NewReader("data"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际错误: %v", err)
	}
}
