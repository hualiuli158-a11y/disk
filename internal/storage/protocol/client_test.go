package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
)

func pipeClient(t *testing.T, id uint64) (*Client, net.Conn, net.Conn) {
	t.Helper()
	left, right := net.Pipe()
	client, err := NewClient(left, id)
	if err != nil {
		t.Fatal(err)
	}
	return client, left, right
}

func TestPutStreamsBytesAndWaitsForCommit(t *testing.T) {
	client, left, right := pipeClient(t, 42)
	defer left.Close()
	defer right.Close()
	content := make([]byte, MaxPayloadSize+3)
	content[0], content[len(content)-1] = 0, 0xff
	serverDone := make(chan error, 1)
	go func() {
		first, err := ReadFrame(right)
		if err != nil {
			serverDone <- err
			return
		}
		key, err := DecodeKeyPayload(first.Payload)
		if err != nil || first.Type != FramePutBegin || first.RequestID != 42 || key != "objects/aa/item" {
			serverDone <- errors.New("wrong PUT_BEGIN")
			return
		}
		if err := WriteFrame(right, Frame{Type: FrameReady, RequestID: 42}); err != nil {
			serverDone <- err
			return
		}
		var received []byte
		for {
			frame, err := ReadFrame(right)
			if err != nil {
				serverDone <- err
				return
			}
			if frame.Type == FrameData {
				received = append(received, frame.Payload...)
				continue
			}
			if frame.Type != FrameEnd || binary.BigEndian.Uint64(frame.Payload) != uint64(len(received)) ||
				!bytes.Equal(received, content) {
				serverDone <- errors.New("PUT data or END count differs")
				return
			}
			serverDone <- WriteFrame(right, Frame{Type: FrameResult, RequestID: 42})
			return
		}
	}()
	if err := client.Put("objects/aa/item", bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestGetReadsUntilValidEnd(t *testing.T) {
	client, left, right := pipeClient(t, 43)
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = ReadFrame(right)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], 5)
		_ = WriteFrame(right, Frame{Type: FrameResult, RequestID: 43, Payload: size[:]})
		_ = WriteFrame(right, Frame{Type: FrameData, RequestID: 43, Payload: []byte("hello")})
		_ = WriteFrame(right, Frame{Type: FrameEnd, RequestID: 43, Payload: size[:]})
	}()
	reader, size, err := client.Get("objects/item")
	if err != nil || size != 5 {
		t.Fatalf("GET start size=%d err=%v", size, err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "hello" {
		t.Fatalf("GET data=%q err=%v", data, err)
	}
}

func TestGetRejectsTruncation(t *testing.T) {
	client, left, right := pipeClient(t, 44)
	defer left.Close()
	defer right.Close()
	go func() {
		defer right.Close()
		_, _ = ReadFrame(right)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], 5)
		_ = WriteFrame(right, Frame{Type: FrameResult, RequestID: 44, Payload: size[:]})
		_ = WriteFrame(right, Frame{Type: FrameData, RequestID: 44, Payload: []byte("hi")})
	}()
	reader, _, err := client.Get("objects/item")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if !errors.Is(err, io.ErrUnexpectedEOF) || string(data) != "hi" {
		t.Fatalf("truncated GET data=%q err=%v", data, err)
	}
}

func TestDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	client, left, right := pipeClient(t, 45)
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = ReadFrame(right)
		_ = WriteFrame(right, Frame{Type: FrameResult, RequestID: 45, Status: StatusNotFound})
	}()
	if err := client.Delete("objects/missing"); err != nil {
		t.Fatal(err)
	}
}

func TestPutPreservesEarlyServerStatus(t *testing.T) {
	client, left, right := pipeClient(t, 47)
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = ReadFrame(right)
		_ = WriteFrame(right, Frame{Type: FrameReady, RequestID: 47})
		_ = WriteFrame(right, Frame{Type: FrameResult, RequestID: 47, Status: StatusNoSpace})
	}()
	err := client.Put("objects/item", bytes.NewReader(make([]byte, MaxPayloadSize)))
	var status *StatusError
	if !errors.As(err, &status) || status.Code != StatusNoSpace {
		t.Fatalf("early NO_SPACE status was lost: %v", err)
	}
}

func TestListDiscardsPartialResponseOnError(t *testing.T) {
	client, left, right := pipeClient(t, 46)
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = ReadFrame(right)
		_ = WriteFrame(right, Frame{Type: FrameResult, RequestID: 46})
		key, _ := EncodeKeyPayload("objects/aa/item")
		var item [16]byte
		binary.BigEndian.PutUint64(item[:8], 9)
		binary.BigEndian.PutUint64(item[8:], 1234)
		_ = WriteFrame(right, Frame{Type: FrameListItem, RequestID: 46, Payload: append(key, item[:]...)})
		_ = WriteFrame(right, Frame{Type: FrameEnd, RequestID: 46, Status: StatusIOError,
			Payload: []byte{0, 0, 0, 1}})
	}()
	items, err := client.List("objects")
	var status *StatusError
	if len(items) != 0 || !errors.As(err, &status) || status.Code != StatusIOError {
		t.Fatalf("partial LIST items=%v err=%v", items, err)
	}
}
