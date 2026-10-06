package grpcstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strings"
	"time"

	"disk/internal/storage"
	"disk/internal/storage/grpcstore/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const chunkSize = 64 * 1024
const maxObjectSize = 1 << 30

type Store struct {
	client  pb.ObjectStorageClient
	conn    *grpc.ClientConn
	timeout time.Duration
}

var _ storage.Storage = (*Store)(nil)

// New connects to a trusted internal gRPC endpoint. The channel is shared by
// operations and must be closed when the application shuts down.
func New(address string, timeout time.Duration) (*Store, error) {
	if strings.TrimSpace(address) == "" || timeout <= 0 {
		return nil, fmt.Errorf("gRPC storage requires address and positive timeout")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create gRPC storage connection: %w", err)
	}
	return &Store{client: pb.NewObjectStorageClient(conn), conn: conn, timeout: timeout}, nil
}

func (s *Store) Close() error { return s.conn.Close() }

func (s *Store) Save(ctx context.Context, key string, reader io.Reader) error {
	if reader == nil {
		return errors.New("storage reader is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	stream, err := s.client.Put(ctx)
	if err != nil {
		return mapError(err)
	}
	if err := sendPut(stream, &pb.PutRequest{Payload: &pb.PutRequest_Key{Key: key}}); err != nil {
		return err
	}
	buffer := make([]byte, chunkSize)
	var total uint64
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			emptyReads = 0
			if uint64(n) > maxObjectSize-total {
				return storage.ErrTooLarge
			}
			if err := sendPut(stream, &pb.PutRequest{Payload: &pb.PutRequest_Data{Data: buffer[:n]}}); err != nil {
				return err
			}
			total += uint64(n)
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return io.ErrNoProgress
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		return mapError(err)
	}
	if reply.Size != total {
		return fmt.Errorf("gRPC storage size mismatch: sent %d, committed %d: %w", total, reply.Size, io.ErrUnexpectedEOF)
	}
	return nil
}

func sendPut(stream pb.ObjectStorage_PutClient, message *pb.PutRequest) error {
	if err := stream.Send(message); err != nil {
		// Send may report EOF after the server has already returned a useful
		// status such as InvalidArgument or OutOfRange.
		_, finalErr := stream.CloseAndRecv()
		if finalErr != nil {
			return mapError(finalErr)
		}
		return mapError(err)
	}
	return nil
}

func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	stream, err := s.client.Get(ctx, &pb.GetRequest{Key: key})
	if err != nil {
		cancel()
		return nil, mapError(err)
	}
	header, err := stream.Recv()
	if err != nil {
		cancel()
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, mapError(err)
	}
	if _, ok := header.GetPayload().(*pb.GetReply_Size); !ok || header.GetSize() > maxObjectSize {
		cancel()
		return nil, fmt.Errorf("invalid gRPC get header: %w", io.ErrUnexpectedEOF)
	}
	return &objectReader{stream: stream, cancel: cancel, remaining: header.GetSize()}, nil
}

type objectReader struct {
	stream    grpc.ServerStreamingClient[pb.GetReply]
	cancel    context.CancelFunc
	remaining uint64
	pending   []byte
	done      bool
	closed    bool
}

func (r *objectReader) Read(out []byte) (int, error) {
	if len(out) == 0 {
		return 0, nil
	}
	if r.closed {
		return 0, fs.ErrClosed
	}
	for len(r.pending) == 0 && !r.done {
		message, err := r.stream.Recv()
		if err == io.EOF {
			r.done = true
			r.cancel()
			if r.remaining != 0 {
				return 0, io.ErrUnexpectedEOF
			}
			return 0, io.EOF
		}
		if err != nil {
			r.done = true
			r.cancel()
			return 0, mapError(err)
		}
		if _, ok := message.GetPayload().(*pb.GetReply_Data); !ok || len(message.GetData()) == 0 || uint64(len(message.GetData())) > r.remaining {
			r.done = true
			r.cancel()
			return 0, fmt.Errorf("invalid gRPC data chunk: %w", io.ErrUnexpectedEOF)
		}
		r.pending = message.GetData()
	}
	if r.done {
		return 0, io.EOF
	}
	n := copy(out, r.pending)
	r.pending = r.pending[n:]
	r.remaining -= uint64(n)
	return n, nil
}

func (r *objectReader) Close() error {
	if !r.closed {
		r.closed = true
		r.cancel()
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	_, err := s.client.Delete(ctx, &pb.DeleteRequest{Key: key})
	return mapError(err)
}

func (s *Store) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	stream, err := s.client.List(ctx, &pb.ListRequest{Prefix: prefix})
	if err != nil {
		return nil, mapError(err)
	}
	objects := make([]storage.ObjectInfo, 0)
	for {
		item, err := stream.Recv()
		if err == io.EOF {
			return objects, nil
		}
		if err != nil {
			return nil, mapError(err)
		}
		if len(objects) >= 100000 {
			return nil, storage.ErrTooLarge
		}
		if item.Size > math.MaxInt64 {
			return nil, fmt.Errorf("listed object size exceeds int64: %w", storage.ErrTooLarge)
		}
		objects = append(objects, storage.ObjectInfo{
			Key: item.Key, Size: int64(item.Size),
			ModifiedAt: time.UnixMilli(item.ModifiedAtUnixMs).UTC(),
		})
	}
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.NotFound:
		return fmt.Errorf("object not found: %w", fs.ErrNotExist)
	case codes.OutOfRange:
		return fmt.Errorf("object too large: %w", storage.ErrTooLarge)
	case codes.Canceled:
		return fmt.Errorf("gRPC request cancelled: %w", context.Canceled)
	case codes.DeadlineExceeded:
		return fmt.Errorf("gRPC request timed out: %w", context.DeadlineExceeded)
	default:
		return err
	}
}
