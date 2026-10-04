package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) { return w.Buffer.Write(p[:1]) }

func TestReferenceGETVector(t *testing.T) {
	payload, err := EncodeKeyPayload("objects")
	if err != nil {
		t.Fatal(err)
	}
	wire, err := EncodeFrame(Frame{Type: FrameGet, RequestID: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	// docs/storage-protocol-v1.md: GET("objects"), request ID 1, 33 bytes.
	want := []byte{
		0x4f, 0x52, 0x53, 0x54, 0x00, 0x01, 0x00, 0x04,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x07, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x73,
	}
	if !bytes.Equal(wire, want) {
		t.Fatalf("wire differs from C++ reference: %x", wire)
	}
	decoded, err := ReadFrame(&oneByteReader{data: want})
	if err != nil || decoded.Type != FrameGet || decoded.RequestID != 1 {
		t.Fatalf("read fragmented reference: frame=%+v err=%v", decoded, err)
	}
	key, err := DecodeKeyPayload(decoded.Payload)
	if err != nil || key != "objects" {
		t.Fatalf("decoded key=%q err=%v", key, err)
	}
}

func TestBinaryFrameAndPartialWrites(t *testing.T) {
	data := []byte{0, 0xff, 'x'}
	var writer shortWriter
	if err := WriteFrame(&writer, Frame{Type: FrameData, RequestID: 37, Payload: data}); err != nil {
		t.Fatal(err)
	}
	frame, err := ReadFrame(&oneByteReader{data: writer.Bytes()})
	if err != nil || frame.Type != FrameData || frame.RequestID != 37 || !bytes.Equal(frame.Payload, data) {
		t.Fatalf("binary frame=%+v err=%v", frame, err)
	}
}

func TestRejectMalformedAndTruncatedFrames(t *testing.T) {
	valid, err := EncodeFrame(Frame{Type: FrameGet, RequestID: 1, Payload: mustKey(t, "objects")})
	if err != nil {
		t.Fatal(err)
	}
	changes := []func([]byte){
		func(v []byte) { v[0] = 'X' },
		func(v []byte) { v[5] = 2 },
		func(v []byte) { v[7] = 0 },
		func(v []byte) { v[15] = 0 },
		func(v []byte) { v[17] = 1 }, // payload becomes 65545, rejected before allocation
		func(v []byte) { v[23] = byte(StatusNotFound) },
		func(v []byte) { v[25] = 8 },
		func(v []byte) { v[26] = 0xff },
	}
	for i, change := range changes {
		wire := bytes.Clone(valid)
		change(wire)
		if _, err := ReadFrame(bytes.NewReader(wire)); !errors.Is(err, ErrInvalidFrame) {
			t.Errorf("case %d: expected invalid frame, got %v", i, err)
		}
	}
	for _, length := range []int{1, HeaderSize - 1, len(valid) - 1} {
		if _, err := ReadFrame(bytes.NewReader(valid[:length])); err == nil {
			t.Errorf("accepted truncated frame of length %d", length)
		}
	}
	for _, key := range []string{"", "../escape", "/absolute", "a//b", "C:/drive", string([]byte{0xff})} {
		if _, err := EncodeKeyPayload(key); !errors.Is(err, ErrInvalidFrame) {
			t.Errorf("accepted invalid key %q: %v", key, err)
		}
	}
	if _, err := EncodeFrame(Frame{Type: FrameData, RequestID: 1, Payload: make([]byte, MaxPayloadSize+1)}); !errors.Is(err, ErrInvalidFrame) {
		t.Errorf("accepted oversized DATA: %v", err)
	}
}

func mustKey(t *testing.T, key string) []byte {
	t.Helper()
	payload, err := EncodeKeyPayload(key)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
