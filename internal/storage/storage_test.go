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

	objects, err := store.List(context.Background(), "sha256")
	if err != nil {
		t.Fatalf("枚举文件失败: %v", err)
	}
	if len(objects) != 1 || objects[0].Key != key || objects[0].Size != int64(len(content)) || objects[0].ModifiedAt.IsZero() {
		t.Fatalf("枚举结果不正确: %+v", objects)
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

func TestLocalStoragePagesObjectsInKeyOrder(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pager := store.(Pager)
	for _, key := range []string{"objects/ff/last", "objects/00/first", "objects/ab/middle"} {
		if err := store.Save(context.Background(), key, strings.NewReader(key)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := pager.ListPage(context.Background(), "objects", "", 2)
	if err != nil || len(first) != 2 || first[0].Key != "objects/00/first" || first[1].Key != "objects/ab/middle" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	second, err := pager.ListPage(context.Background(), "objects", first[1].Key, 2)
	if err != nil || len(second) != 1 || second[0].Key != "objects/ff/last" {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
}

func TestLocalStoragePagesFileBeforeNeighborDirectory(t *testing.T) {
	store, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"objects/a/child", "objects/a.txt"} {
		if err := store.Save(context.Background(), key, strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
	}
	pager := store.(Pager)
	first, err := pager.ListPage(context.Background(), "objects", "", 1)
	if err != nil || len(first) != 1 || first[0].Key != "objects/a.txt" {
		t.Fatalf("first page=%v err=%v", first, err)
	}
	second, err := pager.ListPage(context.Background(), "objects", first[0].Key, 1)
	if err != nil || len(second) != 1 || second[0].Key != "objects/a/child" {
		t.Fatalf("second page=%v err=%v", second, err)
	}
}
