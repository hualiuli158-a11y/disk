package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"net"
	"testing"
	"time"

	"disk/internal/storage/protocol"
)

func fakeNode(t *testing.T, handle func(net.Conn) error) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		defer listener.Close()
		connection, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer connection.Close()
		result <- handle(connection)
	}()
	return listener.Addr().String(), result
}

func TestRemoteStorageMapsNotFound(t *testing.T) {
	address, done := fakeNode(t, func(connection net.Conn) error {
		request, err := protocol.ReadFrame(connection)
		if err != nil {
			return err
		}
		return protocol.WriteFrame(connection, protocol.Frame{
			Type: protocol.FrameResult, RequestID: request.RequestID,
			Status: protocol.StatusNotFound,
		})
	})
	store, err := NewRemoteStorage(address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Open(context.Background(), "objects/missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("remote NOT_FOUND did not map to fs.ErrNotExist: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteStorageMapsTooLarge(t *testing.T) {
	address, done := fakeNode(t, func(connection net.Conn) error {
		request, err := protocol.ReadFrame(connection)
		if err != nil {
			return err
		}
		return protocol.WriteFrame(connection, protocol.Frame{
			Type: protocol.FrameResult, RequestID: request.RequestID,
			Status: protocol.StatusTooLarge,
		})
	})
	store, err := NewRemoteStorage(address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "objects/item", bytes.NewReader(nil)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("remote TOO_LARGE did not map to storage error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteReaderReportsTruncatedDownload(t *testing.T) {
	address, done := fakeNode(t, func(connection net.Conn) error {
		request, err := protocol.ReadFrame(connection)
		if err != nil {
			return err
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], 5)
		if err := protocol.WriteFrame(connection, protocol.Frame{
			Type: protocol.FrameResult, RequestID: request.RequestID, Payload: size[:],
		}); err != nil {
			return err
		}
		return protocol.WriteFrame(connection, protocol.Frame{
			Type: protocol.FrameData, RequestID: request.RequestID, Payload: []byte("hi"),
		})
	})
	store, err := NewRemoteStorage(address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(context.Background(), "objects/item")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if string(data) != "hi" || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated download data=%q err=%v", data, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteReadClosesOnContextCancellation(t *testing.T) {
	address, done := fakeNode(t, func(connection net.Conn) error {
		request, err := protocol.ReadFrame(connection)
		if err != nil {
			return err
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], 5)
		if err := protocol.WriteFrame(connection, protocol.Frame{
			Type: protocol.FrameResult, RequestID: request.RequestID, Payload: size[:],
		}); err != nil {
			return err
		}
		var unused [1]byte
		_, err = connection.Read(unused[:])
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	})
	store, err := NewRemoteStorage(address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader, err := store.Open(ctx, "objects/item")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	var data [8]byte
	_, err = reader.Read(data[:])
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reader returned %v", err)
	}
	_ = reader.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNewRemoteStorageRejectsInvalidSettings(t *testing.T) {
	for _, address := range []string{"", "not-a-host-port", ":9000", "127.0.0.1:abc", "127.0.0.1:0"} {
		if _, err := NewRemoteStorage(address, time.Second); err == nil {
			t.Errorf("accepted invalid address %q", address)
		}
	}
	if _, err := NewRemoteStorage("127.0.0.1:9000", 0); err == nil {
		t.Error("accepted nonpositive timeout")
	}
}
