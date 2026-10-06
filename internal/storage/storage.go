package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Storage 定义文件内容存储需要具备的最小能力。
// key 是相对于存储根目录的内部标识，不是用户上传时的原始文件名。
type Storage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// Pager supports bounded object inventory scans. Pages are ordered by key;
// afterKey is exclusive and empty for the first page.
type Pager interface {
	ListPage(ctx context.Context, prefix, afterKey string, limit int) ([]ObjectInfo, error)
}

type ObjectInfo struct {
	Key        string
	Size       int64
	ModifiedAt time.Time
}

var ErrTooLarge = errors.New("object exceeds storage size limit")

// NewStorage 创建一个本地文件存储。
// root 是所有文件的存储根目录；目录不存在时会自动创建。
func NewStorage(root string) (Storage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("存储根目录不能为空")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析存储根目录: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o750); err != nil {
		return nil, fmt.Errorf("创建存储根目录: %w", err)
	}

	// 保存解析符号链接后的真实根目录，后续用它限制所有文件操作范围。
	realRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("解析存储根目录真实路径: %w", err)
	}

	return &local{root: realRoot}, nil
}

type local struct {
	root string
}

// Save 将内容原子地写入指定 key。
// 数据会先写入同目录的临时文件，完整写入后再替换目标文件，
// 避免程序中断时留下只有一部分内容的目标文件。
func (l *local) Save(ctx context.Context, key string, reader io.Reader) error {
	if reader == nil {
		return fmt.Errorf("保存文件 %q: reader 不能为空", key)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	target, err := l.resolve(key)
	if err != nil {
		return err
	}

	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("创建文件目录 %q: %w", key, err)
	}

	// MkdirAll 可能经过已有的符号链接，因此创建目录后再次检查真实路径。
	realDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return fmt.Errorf("解析文件目录 %q: %w", key, err)
	}
	if !l.contains(realDirectory) {
		return fmt.Errorf("无效的存储 key %q: 路径超出存储根目录", key)
	}
	target = filepath.Join(realDirectory, filepath.Base(target))

	temporary, err := os.CreateTemp(realDirectory, ".upload-*")
	if err != nil {
		return fmt.Errorf("创建临时文件 %q: %w", key, err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()

	if _, err := io.Copy(temporary, &contextReader{ctx: ctx, reader: reader}); err != nil {
		return fmt.Errorf("写入文件 %q: %w", key, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("同步文件 %q: %w", key, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时文件 %q: %w", key, err)
	}

	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("提交文件 %q: %w", key, err)
	}

	return nil
}

// Open 打开指定 key，并返回由调用方负责关闭的读取器。
func (l *local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	target, err := l.resolveExisting(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("打开文件 %q: %w", key, err)
	}

	return &contextReadCloser{ctx: ctx, file: file}, nil
}

// Delete 删除指定 key。目标不存在时也视为删除成功，使该操作保持幂等。
func (l *local) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	target, err := l.resolveExisting(key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除文件 %q: %w", key, err)
	}

	return nil
}

// List 返回 prefix 下的普通文件，不跟随符号链接。
func (l *local) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, err := l.resolve(prefix)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(base); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ObjectInfo{}, nil
		}
		return nil, fmt.Errorf("读取存储前缀 %q: %w", prefix, err)
	}

	objects := make([]ObjectInfo, 0)
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(l.root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("存储对象路径越界: %q", path)
		}
		objects = append(objects, ObjectInfo{
			Key: filepath.ToSlash(relative), Size: info.Size(), ModifiedAt: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("枚举存储前缀 %q: %w", prefix, err)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (l *local) ListPage(ctx context.Context, prefix, afterKey string, limit int) ([]ObjectInfo, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("list page limit must be between 1 and 1000")
	}
	if afterKey != "" {
		if err := validateKey(afterKey); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(afterKey, prefix+"/") {
			return nil, fmt.Errorf("list cursor is outside prefix")
		}
	}
	base, err := l.resolveExisting(prefix)
	if errors.Is(err, os.ErrNotExist) {
		return []ObjectInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	page := make([]ObjectInfo, 0, limit)
	var visit func(string) (bool, error)
	visit = func(directory string) (bool, error) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return false, err
		}
		// A directory sorts as its key plus "/". This preserves global key
		// order when a file such as a.txt sits beside directory a.
		sort.Slice(entries, func(i, j int) bool {
			orderKey := func(entry os.DirEntry) string {
				key := entry.Name()
				if entry.IsDir() {
					key += "/"
				}
				return key
			}
			return orderKey(entries[i]) < orderKey(entries[j])
		})
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".upload-") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			relative, err := filepath.Rel(l.root, path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				return false, fmt.Errorf("存储对象路径越界: %q", path)
			}
			key := filepath.ToSlash(relative)
			if entry.IsDir() {
				subtree := key + "/"
				if afterKey != "" && subtree <= afterKey && !strings.HasPrefix(afterKey, subtree) {
					continue
				}
				done, err := visit(path)
				if done || err != nil {
					return done, err
				}
				continue
			}
			if key <= afterKey {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return false, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			page = append(page, ObjectInfo{Key: key, Size: info.Size(), ModifiedAt: info.ModTime()})
			if len(page) == limit {
				return true, nil
			}
		}
		return false, nil
	}
	_, err = visit(base)
	if err != nil {
		return nil, fmt.Errorf("枚举存储分页 %q: %w", prefix, err)
	}
	return page, nil
}

// resolve 将存储 key 转换成根目录内的本地路径。
func (l *local) resolve(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	normalized := strings.ReplaceAll(key, "\\", "/")
	target := filepath.Join(l.root, filepath.FromSlash(normalized))
	if !l.contains(target) {
		return "", fmt.Errorf("无效的存储 key %q: 路径超出存储根目录", key)
	}

	return target, nil
}

// resolveExisting 在普通路径检查之外继续解析符号链接，防止借助符号链接越界。
func (l *local) resolveExisting(key string) (string, error) {
	target, err := l.resolve(key)
	if err != nil {
		return "", err
	}

	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("解析文件 %q: %w", key, err)
	}
	if !l.contains(realTarget) {
		return "", fmt.Errorf("无效的存储 key %q: 符号链接超出存储根目录", key)
	}

	return realTarget, nil
}

func (l *local) contains(target string) bool {
	relative, err := filepath.Rel(l.root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("存储 key 不能为空")
	}
	if strings.ContainsRune(key, '\x00') {
		return fmt.Errorf("无效的存储 key %q: 包含空字符", key)
	}
	if filepath.IsAbs(key) || filepath.VolumeName(key) != "" {
		return fmt.Errorf("无效的存储 key %q: 不允许使用绝对路径", key)
	}

	normalized := strings.ReplaceAll(key, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return fmt.Errorf("无效的存储 key %q: 不允许使用绝对路径", key)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("无效的存储 key %q: 包含非法路径片段", key)
		}
	}

	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

type contextReadCloser struct {
	ctx  context.Context
	file *os.File
}

func (r *contextReadCloser) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Read(buffer)
}

func (r *contextReadCloser) Close() error {
	return r.file.Close()
}
