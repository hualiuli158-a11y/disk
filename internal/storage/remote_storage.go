package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"disk/internal/storage/protocol"
)

var (
	ErrTooLarge = errors.New("remote object exceeds size limit")
	ErrBusy     = errors.New("remote storage is busy")
	ErrNoSpace  = errors.New("remote storage has no space")
)

// RemoteStorage implements Storage by using one TCP connection per operation.
type RemoteStorage struct {
	address string
	timeout time.Duration
	nextID  atomic.Uint64
}

var _ Storage = (*RemoteStorage)(nil)

func NewRemoteStorage(address string, timeout time.Duration) (*RemoteStorage, error) {
	if strings.TrimSpace(address) == "" || timeout <= 0 {
		return nil, fmt.Errorf("remote address and positive timeout are required")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return nil, fmt.Errorf("remote address must be host:port: %q", address)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("remote address has invalid port: %q", address)
	}
	return &RemoteStorage{address: address, timeout: timeout}, nil
}

type operationConnection struct {
	conn   net.Conn
	ctx    context.Context
	cancel context.CancelFunc
	stop   chan struct{}
	once   sync.Once
}

func (o *operationConnection) Close() error {
	var closeErr error
	o.once.Do(func() {
		close(o.stop)
		closeErr = o.conn.Close()
		o.cancel()
	})
	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (o *operationConnection) normalize(err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := o.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (s *RemoteStorage) dial(ctx context.Context) (*operationConnection, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}
	opCtx, cancel := context.WithTimeout(ctx, s.timeout)
	dialTimeout := 5 * time.Second
	if s.timeout < dialTimeout {
		dialTimeout = s.timeout
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(opCtx, "tcp", s.address)
	if err != nil {
		ctxErr := opCtx.Err()
		cancel()
		if ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	deadline, _ := opCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		cancel()
		return nil, err
	}
	op := &operationConnection{conn: conn, ctx: opCtx, cancel: cancel, stop: make(chan struct{})}
	go func() {
		select {
		case <-opCtx.Done():
			_ = conn.Close()
		case <-op.stop:
		}
	}()
	return op, nil
}

func (s *RemoteStorage) client(conn net.Conn) (*protocol.Client, error) {
	id := s.nextID.Add(1)
	if id == 0 {
		id = s.nextID.Add(1)
	}
	return protocol.NewClient(conn, id)
}

func mapRemoteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, protocol.ErrObjectTooLarge) {
		return fmt.Errorf("%w: %v", ErrTooLarge, err)
	}
	var status *protocol.StatusError
	if errors.As(err, &status) {
		switch status.Code {
		case protocol.StatusNotFound:
			return fmt.Errorf("%w: %v", fs.ErrNotExist, err)
		case protocol.StatusTooLarge:
			return fmt.Errorf("%w: %v", ErrTooLarge, err)
		case protocol.StatusBusy:
			return fmt.Errorf("%w: %v", ErrBusy, err)
		case protocol.StatusNoSpace:
			return fmt.Errorf("%w: %v", ErrNoSpace, err)
		}
	}
	return err
}

func (s *RemoteStorage) Save(ctx context.Context, key string, reader io.Reader) error {
	op, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer op.Close()
	client, err := s.client(op.conn)
	if err != nil {
		return err
	}
	return mapRemoteError(op.normalize(client.Put(key, reader)))
}

type remoteReadCloser struct {
	reader io.Reader
	op     *operationConnection
	done   atomic.Bool
	closed atomic.Bool
}

func (r *remoteReadCloser) Read(destination []byte) (int, error) {
	if r.done.Load() {
		return 0, io.EOF
	}
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	n, err := r.reader.Read(destination)
	if errors.Is(err, io.EOF) {
		r.done.Store(true)
		_ = r.op.Close()
		return n, io.EOF
	}
	if err != nil {
		mapped := mapRemoteError(r.op.normalize(err))
		r.closed.Store(true)
		_ = r.op.Close()
		return n, mapped
	}
	return n, nil
}

func (r *remoteReadCloser) Close() error {
	r.closed.Store(true)
	return r.op.Close()
}

func (s *RemoteStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	op, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	client, err := s.client(op.conn)
	if err != nil {
		_ = op.Close()
		return nil, err
	}
	reader, _, err := client.Get(key)
	if err != nil {
		mapped := mapRemoteError(op.normalize(err))
		_ = op.Close()
		return nil, mapped
	}
	return &remoteReadCloser{reader: reader, op: op}, nil
}

func (s *RemoteStorage) Delete(ctx context.Context, key string) error {
	op, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer op.Close()
	client, err := s.client(op.conn)
	if err != nil {
		return err
	}
	return mapRemoteError(op.normalize(client.Delete(key)))
}

func (s *RemoteStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	op, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer op.Close()
	client, err := s.client(op.conn)
	if err != nil {
		return nil, err
	}
	items, err := client.List(prefix)
	if err != nil {
		return nil, mapRemoteError(op.normalize(err))
	}
	objects := make([]ObjectInfo, 0, len(items))
	for _, item := range items {
		objects = append(objects, ObjectInfo{
			Key: item.Key, Size: item.Size,
			ModifiedAt: time.UnixMilli(item.ModifiedAtUnixMillis).UTC(),
		})
	}
	return objects, nil
}
