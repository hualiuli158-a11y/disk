package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Storage 定义文件内容存储需要具备的最小能力。
// key 是相对于存储根目录的内部标识，不是用户上传时的原始文件名。
type Storage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

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
