package grpcstore

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"disk/internal/storage/grpcstore/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type memoryServer struct {
	pb.UnimplementedObjectStorageServer
	mu       sync.Mutex
	objects  map[string][]byte
	truncate bool
	health   *health.Server
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("source read failed") }

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
	sent := uint32(0)
	for _, key := range keys {
		if key <= request.AfterKey {
			continue
		}
		if request.Limit > 0 && sent >= request.Limit {
			break
		}
		if err := stream.Send(&pb.ListReply{Key: key, Size: uint64(items[key]), ModifiedAtUnixMs: 123456789}); err != nil {
			return err
		}
		sent++
	}
	return nil
}

func newTestStore(t *testing.T) (*Store, *memoryServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	backend := &memoryServer{objects: make(map[string][]byte)}
	pb.RegisterObjectStorageServer(server, backend)
	backend.health = health.NewServer()
	backend.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, backend.health)
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
	if err := store.Check(context.Background()); err != nil {
		t.Fatalf("healthy node rejected: %v", err)
	}
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
	firstPage, err := store.ListPage(context.Background(), "objects", "", 1)
	if err != nil || len(firstPage) != 1 || firstPage[0].Key != "objects/empty" {
		t.Fatalf("first gRPC page=%+v err=%v", firstPage, err)
	}
	secondPage, err := store.ListPage(context.Background(), "objects", firstPage[0].Key, 1)
	if err != nil || len(secondPage) != 1 || secondPage[0].Key != "objects/full" {
		t.Fatalf("second gRPC page=%+v err=%v", secondPage, err)
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

func TestHealthCheckDetectsUnavailableNode(t *testing.T) {
	store, backend := newTestStore(t)
	backend.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	if err := store.Check(context.Background()); err == nil {
		t.Fatal("unhealthy node reported ready")
	}
}

func TestPlaintextRequiresLoopback(t *testing.T) {
	if _, err := New("192.0.2.10:50051", time.Second); err == nil {
		t.Fatal("plaintext connection to non-loopback address was accepted")
	}
	if _, err := NewWithTLS("192.0.2.10:50051", time.Second, TLSOptions{CAFile: "unused.pem"}); err == nil || !strings.Contains(err.Error(), "client certificate") {
		t.Fatalf("remote connection without mTLS was accepted: %v", err)
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
	tlsOptions := TLSOptions{
		CAFile:         os.Getenv("STORAGE_TEST_GRPC_TLS_CA"),
		ServerName:     os.Getenv("STORAGE_TEST_GRPC_TLS_SERVER_NAME"),
		ClientCertFile: os.Getenv("STORAGE_TEST_GRPC_TLS_CLIENT_CERT"),
		ClientKeyFile:  os.Getenv("STORAGE_TEST_GRPC_TLS_CLIENT_KEY"),
	}
	if binary := os.Getenv("STORAGE_TEST_GRPC_BINARY"); binary != "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = listener.Addr().String()
		_ = listener.Close()
		args := []string{address, t.TempDir()}
		if os.Getenv("STORAGE_TEST_GRPC_TLS") == "1" {
			ca, caKey, caPEM, _ := testCertificate(t, nil, nil, 11, true, x509.ExtKeyUsageAny)
			_, _, serverPEM, serverKey := testCertificate(t, ca, caKey, 12, false, x509.ExtKeyUsageServerAuth)
			_, _, clientPEM, clientKey := testCertificate(t, ca, caKey, 13, false, x509.ExtKeyUsageClientAuth)
			certDir := t.TempDir()
			write := func(name string, contents []byte) string {
				path := filepath.Join(certDir, name)
				if err := os.WriteFile(path, contents, 0600); err != nil {
					t.Fatal(err)
				}
				return path
			}
			caPath := write("ca.pem", caPEM)
			args = append(args, write("server.pem", serverPEM), write("server.key", serverKey), caPath)
			tlsOptions = TLSOptions{CAFile: caPath, ServerName: "localhost",
				ClientCertFile: write("client.pem", clientPEM), ClientKeyFile: write("client.key", clientKey)}
		}
		command := exec.Command(binary, args...)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = command.Process.Signal(os.Interrupt)
			finished := make(chan struct{})
			go func() { _ = command.Wait(); close(finished) }()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				_ = command.Process.Kill()
				<-finished
			}
		})
	}
	if address == "" {
		t.Skip("set STORAGE_TEST_GRPC_BINARY or STORAGE_TEST_GRPC_ADDRESS")
	}
	store, err := NewWithTLS(address, 10*time.Second, tlsOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var healthErr error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		healthErr = store.Check(context.Background())
		if healthErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if healthErr != nil {
		t.Fatalf("C++ health check failed: %v", healthErr)
	}
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
	page, err := store.ListPage(context.Background(), "objects", "", 1)
	if err != nil || len(page) != 1 || page[0].Key != key {
		t.Fatalf("C++ first page=%+v err=%v", page, err)
	}
	nextPage, err := store.ListPage(context.Background(), "objects", key, 1)
	if err != nil || len(nextPage) != 0 {
		t.Fatalf("C++ last page=%+v err=%v", nextPage, err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("C++ missing object: %v", err)
	}
	emptyKey := key + "-empty"
	if err := store.Save(context.Background(), emptyKey, bytes.NewReader(nil)); err != nil {
		t.Fatalf("C++ empty upload: %v", err)
	}
	defer store.Delete(context.Background(), emptyKey)
	emptyReader, err := store.Open(context.Background(), emptyKey)
	if err != nil {
		t.Fatalf("C++ empty download: %v", err)
	}
	emptyData, err := io.ReadAll(emptyReader)
	_ = emptyReader.Close()
	if err != nil || len(emptyData) != 0 {
		t.Fatalf("C++ empty object: %d bytes, %v", len(emptyData), err)
	}
	interruptedKey := key + "-interrupted"
	if err := store.Save(context.Background(), interruptedKey,
		io.MultiReader(bytes.NewReader([]byte("partial")), failingReader{})); err == nil {
		t.Fatal("interrupted C++ upload unexpectedly succeeded")
	}
	if _, err := store.Open(context.Background(), interruptedKey); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("interrupted C++ upload became visible: %v", err)
	}
}
