package grpcstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"disk/internal/storage/grpcstore/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type memoryServer struct {
	pb.UnimplementedObjectStorageServer
	mu       sync.Mutex
	objects  map[string][]byte
	truncate bool
}

func (s *memoryServer) Put(stream grpc.ClientStreamingServer[pb.PutRequest, pb.PutReply]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if _, ok := first.Payload.(*pb.PutRequest_Key); !ok {
		return status.Error(codes.InvalidArgument, "missing key")
	}
	var data []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, ok := chunk.Payload.(*pb.PutRequest_Data); !ok {
			return status.Error(codes.InvalidArgument, "unexpected key")
		}
		if len(chunk.GetData()) > chunkSize {
			return status.Error(codes.OutOfRange, "large chunk")
		}
		data = append(data, chunk.GetData()...)
	}
	s.mu.Lock()
	s.objects[first.GetKey()] = data
	s.mu.Unlock()
	return stream.SendAndClose(&pb.PutReply{Size: uint64(len(data))})
}

func (s *memoryServer) Get(request *pb.GetRequest, stream grpc.ServerStreamingServer[pb.GetReply]) error {
	s.mu.Lock()
	data, exists := s.objects[request.Key]
	truncate := s.truncate
	s.mu.Unlock()
	if !exists {
		return status.Error(codes.NotFound, "missing")
	}
	size := uint64(len(data))
	if truncate {
		size++
	}
	if err := stream.Send(&pb.GetReply{Payload: &pb.GetReply_Size{Size: size}}); err != nil {
		return err
	}
	if len(data) > 0 {
		return stream.Send(&pb.GetReply{Payload: &pb.GetReply_Data{Data: data}})
	}
	return nil
}

func (s *memoryServer) Delete(_ context.Context, request *pb.DeleteRequest) (*pb.DeleteReply, error) {
	s.mu.Lock()
	delete(s.objects, request.Key)
	s.mu.Unlock()
	return &pb.DeleteReply{}, nil
}

func (s *memoryServer) List(request *pb.ListRequest, stream grpc.ServerStreamingServer[pb.ListReply]) error {
	s.mu.Lock()
	items := make(map[string]int)
	for key, data := range s.objects {
		if strings.HasPrefix(key, request.Prefix+"/") {
			items[key] = len(data)
		}
	}
	s.mu.Unlock()
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := stream.Send(&pb.ListReply{Key: key, Size: uint64(items[key]), ModifiedAtUnixMs: 123456789}); err != nil {
			return err
		}
	}
	return nil
}

func newTestStore(t *testing.T) (*Store, *memoryServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	backend := &memoryServer{objects: make(map[string][]byte)}
	pb.RegisterObjectStorageServer(server, backend)
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(); server.Stop(); _ = listener.Close() })
	return &Store{client: pb.NewObjectStorageClient(conn), conn: conn, timeout: 5 * time.Second}, backend
}

func TestStreamedLifecycle(t *testing.T) {
	store, _ := newTestStore(t)
	data := bytes.Repeat([]byte{0, 1, 2, 255}, 50000)
	for _, item := range []struct {
		key   string
		value []byte
	}{
		{"objects/full", data}, {"objects/empty", nil},
	} {
		if err := store.Save(context.Background(), item.key, bytes.NewReader(item.value)); err != nil {
			t.Fatal(err)
		}
		reader, err := store.Open(context.Background(), item.key)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil || !bytes.Equal(got, item.value) {
			t.Fatalf("Open(%q): %v, data match=%v", item.key, err, bytes.Equal(got, item.value))
		}
	}
	objects, err := store.List(context.Background(), "objects")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0].Key != "objects/empty" || objects[0].Size != 0 ||
		objects[1].Key != "objects/full" || objects[1].Size != int64(len(data)) ||
		objects[1].ModifiedAt.IsZero() {
		t.Fatalf("invalid gRPC listing: %+v", objects)
	}
	if err := store.Delete(context.Background(), "objects/full"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "objects/full"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), "objects/full"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing object: %v", err)
	}
}

func TestGetDetectsTruncatedObject(t *testing.T) {
	store, server := newTestStore(t)
	server.mu.Lock()
	server.objects["objects/truncated"] = []byte("abc")
	server.truncate = true
	server.mu.Unlock()
	reader, err := store.Open(context.Background(), "objects/truncated")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, err = io.ReadAll(reader)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected truncated stream error, got %v", err)
	}
}

// Run against a grpc_storage_node started with a disposable storage root.
func TestCppServerInteroperability(t *testing.T) {
	address := os.Getenv("STORAGE_TEST_GRPC_ADDRESS")
	if address == "" {
		t.Skip("set STORAGE_TEST_GRPC_ADDRESS to a disposable C++ server")
	}
	store, err := New(address, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := "objects/integration-" + time.Now().UTC().Format("20060102150405.000000000")
	data := bytes.Repeat([]byte{0, 1, 255, 42}, 40000)
	if err := store.Save(context.Background(), key, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	defer store.Delete(context.Background(), key)
	reader, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("C++ download: %v, data match=%v", err, bytes.Equal(got, data))
	}
	objects, err := store.List(context.Background(), "objects")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, object := range objects {
		if object.Key == key && object.Size == int64(len(data)) && !object.ModifiedAt.IsZero() {
			found = true
		}
	}
	if !found {
		t.Fatalf("C++ listing omitted uploaded key %q", key)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("C++ missing object: %v", err)
	}
}
