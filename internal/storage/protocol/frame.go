package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	Magic                 = "ORST"
	Version        uint16 = 1
	HeaderSize            = 24
	MaxPayloadSize        = 64 * 1024
	MaxKeySize            = 500
	MaxObjectSize         = 1 << 30
	MaxListItems          = 100000
)

type FrameType uint16

const (
	FramePutBegin FrameType = 1
	FrameData     FrameType = 2
	FrameEnd      FrameType = 3
	FrameGet      FrameType = 4
	FrameDelete   FrameType = 5
	FrameList     FrameType = 6
	FrameReady    FrameType = 7
	FrameResult   FrameType = 8
	FrameListItem FrameType = 9
)

type Status uint32

const (
	StatusOK                 Status = 0
	StatusInvalidRequest     Status = 1
	StatusUnsupportedVersion Status = 2
	StatusNotFound           Status = 3
	StatusTooLarge           Status = 4
	StatusBusy               Status = 5
	StatusNoSpace            Status = 6
	StatusIOError            Status = 7
)

type Frame struct {
	Type      FrameType
	RequestID uint64
	Status    Status
	Payload   []byte
}

type StatusError struct{ Code Status }

func (e *StatusError) Error() string { return fmt.Sprintf("storage node status %d", e.Code) }

var ErrInvalidFrame = errors.New("invalid storage protocol frame")

func validType(typ FrameType) bool   { return typ >= FramePutBegin && typ <= FrameListItem }
func validStatus(status Status) bool { return status <= StatusIOError }

func ValidateKey(key string) error {
	if len(key) == 0 || len(key) > MaxKeySize || !utf8.ValidString(key) {
		return fmt.Errorf("%w: invalid key length or UTF-8", ErrInvalidFrame)
	}
	if strings.ContainsAny(key, "\\\x00") ||
		(len(key) >= 2 && key[1] == ':' &&
			((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= 'a' && key[0] <= 'z'))) {
		return fmt.Errorf("%w: forbidden key byte or drive prefix", ErrInvalidFrame)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%w: invalid key path segment", ErrInvalidFrame)
		}
	}
	return nil
}

func EncodeKeyPayload(key string) ([]byte, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	payload := make([]byte, 2+len(key))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(key)))
	copy(payload[2:], key)
	return payload, nil
}

func DecodeKeyPayload(payload []byte) (string, error) {
	if len(payload) < 2 {
		return "", fmt.Errorf("%w: short key payload", ErrInvalidFrame)
	}
	length := int(binary.BigEndian.Uint16(payload[:2]))
	if len(payload) != 2+length {
		return "", fmt.Errorf("%w: key length mismatch", ErrInvalidFrame)
	}
	key := string(payload[2:])
	return key, ValidateKey(key)
}

func validateFrame(frame Frame) error {
	if !validType(frame.Type) || !validStatus(frame.Status) || frame.RequestID == 0 || len(frame.Payload) > MaxPayloadSize {
		return fmt.Errorf("%w: invalid header field", ErrInvalidFrame)
	}
	if frame.Status != StatusOK && frame.Type != FrameResult && frame.Type != FrameEnd {
		return fmt.Errorf("%w: status on non-result frame", ErrInvalidFrame)
	}
	switch frame.Type {
	case FramePutBegin, FrameGet, FrameDelete, FrameList:
		_, err := DecodeKeyPayload(frame.Payload)
		return err
	case FrameData:
		if len(frame.Payload) == 0 {
			return fmt.Errorf("%w: empty DATA frame", ErrInvalidFrame)
		}
	case FrameEnd:
		if len(frame.Payload) != 4 && len(frame.Payload) != 8 {
			return fmt.Errorf("%w: invalid END count", ErrInvalidFrame)
		}
	case FrameReady:
		if len(frame.Payload) != 0 {
			return fmt.Errorf("%w: READY has a payload", ErrInvalidFrame)
		}
	case FrameResult:
		if (len(frame.Payload) != 0 && len(frame.Payload) != 8) || (frame.Status != StatusOK && len(frame.Payload) != 0) {
			return fmt.Errorf("%w: invalid RESULT payload", ErrInvalidFrame)
		}
	case FrameListItem:
		if len(frame.Payload) < 18 {
			return fmt.Errorf("%w: short LIST_ITEM", ErrInvalidFrame)
		}
		keyLength := int(binary.BigEndian.Uint16(frame.Payload[:2]))
		if len(frame.Payload) != 2+keyLength+16 {
			return fmt.Errorf("%w: LIST_ITEM length mismatch", ErrInvalidFrame)
		}
		if err := ValidateKey(string(frame.Payload[2 : 2+keyLength])); err != nil {
			return err
		}
		if binary.BigEndian.Uint64(frame.Payload[2+keyLength:10+keyLength]) > math.MaxInt64 {
			return fmt.Errorf("%w: LIST_ITEM size overflows int64", ErrInvalidFrame)
		}
	}
	return nil
}

func EncodeFrame(frame Frame) ([]byte, error) {
	if err := validateFrame(frame); err != nil {
		return nil, err
	}
	wire := make([]byte, HeaderSize+len(frame.Payload))
	copy(wire[:4], Magic)
	binary.BigEndian.PutUint16(wire[4:6], Version)
	binary.BigEndian.PutUint16(wire[6:8], uint16(frame.Type))
	binary.BigEndian.PutUint64(wire[8:16], frame.RequestID)
	binary.BigEndian.PutUint32(wire[16:20], uint32(len(frame.Payload)))
	binary.BigEndian.PutUint32(wire[20:24], uint32(frame.Status))
	copy(wire[HeaderSize:], frame.Payload)
	return wire, nil
}

func WriteFrame(w io.Writer, frame Frame) error {
	wire, err := EncodeFrame(frame)
	if err != nil {
		return err
	}
	for len(wire) > 0 {
		n, writeErr := w.Write(wire)
		if n < 0 || n > len(wire) {
			return fmt.Errorf("%w: invalid writer count", ErrInvalidFrame)
		}
		wire = wire[n:]
		if writeErr != nil {
			return writeErr
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func ReadFrame(r io.Reader) (Frame, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	if string(header[:4]) != Magic || binary.BigEndian.Uint16(header[4:6]) != Version {
		return Frame{}, fmt.Errorf("%w: magic or version mismatch", ErrInvalidFrame)
	}
	length := binary.BigEndian.Uint32(header[16:20])
	frame := Frame{
		Type:      FrameType(binary.BigEndian.Uint16(header[6:8])),
		RequestID: binary.BigEndian.Uint64(header[8:16]),
		Status:    Status(binary.BigEndian.Uint32(header[20:24])),
	}
	if length > MaxPayloadSize || !validType(frame.Type) || !validStatus(frame.Status) || frame.RequestID == 0 ||
		(frame.Status != StatusOK && frame.Type != FrameResult && frame.Type != FrameEnd) {
		return Frame{}, fmt.Errorf("%w: invalid header field", ErrInvalidFrame)
	}
	frame.Payload = make([]byte, int(length))
	if _, err := io.ReadFull(r, frame.Payload); err != nil {
		return Frame{}, err
	}
	return frame, validateFrame(frame)
}
