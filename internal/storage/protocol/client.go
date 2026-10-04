package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

var ErrObjectTooLarge = errors.New("object exceeds protocol size limit")

// Client performs one storage operation on one TCP connection.
type Client struct {
	conn      net.Conn
	requestID uint64
}

func NewClient(conn net.Conn, requestID uint64) (*Client, error) {
	if conn == nil || requestID == 0 {
		return nil, fmt.Errorf("%w: client needs a connection and nonzero request ID", ErrInvalidFrame)
	}
	return &Client{conn: conn, requestID: requestID}, nil
}

func (c *Client) send(typ FrameType, payload []byte) error {
	return WriteFrame(c.conn, Frame{Type: typ, RequestID: c.requestID, Payload: payload})
}

func (c *Client) next() (Frame, error) {
	frame, err := ReadFrame(c.conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Frame{}, io.ErrUnexpectedEOF
		}
		return Frame{}, err
	}
	if frame.RequestID != c.requestID {
		return Frame{}, fmt.Errorf("%w: response request ID differs", ErrInvalidFrame)
	}
	return frame, nil
}

func expectResult(frame Frame, payloadSize int) error {
	if frame.Type != FrameResult {
		return fmt.Errorf("%w: expected RESULT, got type %d", ErrInvalidFrame, frame.Type)
	}
	if frame.Status != StatusOK {
		return &StatusError{Code: frame.Status}
	}
	if len(frame.Payload) != payloadSize {
		return fmt.Errorf("%w: unexpected RESULT payload size", ErrInvalidFrame)
	}
	return nil
}

type putResponse struct {
	frame Frame
	err   error
}

func finishPut(response putResponse, allowSuccess bool) error {
	if response.err != nil {
		return response.err
	}
	if response.frame.Type != FrameResult {
		return fmt.Errorf("%w: expected final PUT RESULT", ErrInvalidFrame)
	}
	if !allowSuccess && response.frame.Status == StatusOK {
		return fmt.Errorf("%w: PUT succeeded before END", ErrInvalidFrame)
	}
	return expectResult(response.frame, 0)
}

func earlyPutError(responses <-chan putResponse, transportErr error) error {
	select {
	case response := <-responses:
		return finishPut(response, false)
	default:
		return transportErr
	}
}

func (c *Client) Put(key string, reader io.Reader) (result error) {
	defer func() {
		if result != nil {
			_ = c.conn.Close()
		}
	}()
	if reader == nil {
		return fmt.Errorf("%w: PUT reader is nil", ErrInvalidFrame)
	}
	keyPayload, err := EncodeKeyPayload(key)
	if err != nil {
		return err
	}
	if err := c.send(FramePutBegin, keyPayload); err != nil {
		return err
	}
	ready, err := c.next()
	if err != nil {
		return err
	}
	if ready.Type == FrameResult && ready.Status != StatusOK {
		return &StatusError{Code: ready.Status}
	}
	if ready.Type != FrameReady || ready.Status != StatusOK || len(ready.Payload) != 0 {
		return fmt.Errorf("%w: expected READY", ErrInvalidFrame)
	}
	responses := make(chan putResponse, 1)
	go func() {
		frame, err := c.next()
		responses <- putResponse{frame: frame, err: err}
		if err != nil || frame.Type != FrameResult || frame.Status != StatusOK {
			_ = c.conn.Close() // Stop a writer blocked after an early server failure.
		}
	}()

	buffer := make([]byte, MaxPayloadSize)
	var total uint64
	emptyReads := 0
	for {
		select {
		case response := <-responses:
			return finishPut(response, false)
		default:
		}
		n, readErr := reader.Read(buffer)
		if n < 0 || n > len(buffer) {
			return fmt.Errorf("%w: reader returned invalid byte count", ErrInvalidFrame)
		}
		if n > 0 {
			if uint64(n) > MaxObjectSize-total {
				return ErrObjectTooLarge
			}
			if err := c.send(FrameData, buffer[:n]); err != nil {
				return earlyPutError(responses, err)
			}
			total += uint64(n)
			emptyReads = 0
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return io.ErrNoProgress
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], total)
	if err := c.send(FrameEnd, count[:]); err != nil {
		return earlyPutError(responses, err)
	}
	return finishPut(<-responses, true)
}

// Get returns a streaming reader. The owner of Client must keep conn open until
// reading reaches END or the caller stops and closes the connection.
func (c *Client) Get(key string) (io.Reader, uint64, error) {
	keyPayload, err := EncodeKeyPayload(key)
	if err != nil {
		return nil, 0, err
	}
	if err := c.send(FrameGet, keyPayload); err != nil {
		return nil, 0, err
	}
	result, err := c.next()
	if err != nil {
		return nil, 0, err
	}
	if err := expectResult(result, 8); err != nil {
		return nil, 0, err
	}
	size := binary.BigEndian.Uint64(result.Payload)
	if size > MaxObjectSize {
		return nil, 0, fmt.Errorf("%w: GET size exceeds 1 GiB", ErrInvalidFrame)
	}
	return &getReader{client: c, expected: size}, size, nil
}

type getReader struct {
	client   *Client
	expected uint64
	received uint64
	pending  []byte
	done     bool
	failure  error
}

func (r *getReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if r.failure != nil {
		return 0, r.failure
	}
	if r.done {
		return 0, io.EOF
	}
	for len(r.pending) == 0 {
		frame, err := r.client.next()
		if err != nil {
			r.failure = err
			return 0, err
		}
		switch frame.Type {
		case FrameData:
			if uint64(len(frame.Payload)) > r.expected-r.received {
				r.failure = fmt.Errorf("%w: GET sent more bytes than declared", ErrInvalidFrame)
				return 0, r.failure
			}
			r.received += uint64(len(frame.Payload))
			r.pending = frame.Payload
		case FrameEnd:
			if len(frame.Payload) != 8 {
				r.failure = fmt.Errorf("%w: GET END needs 8-byte count", ErrInvalidFrame)
				return 0, r.failure
			}
			if frame.Status != StatusOK {
				r.failure = &StatusError{Code: frame.Status}
				return 0, r.failure
			}
			if binary.BigEndian.Uint64(frame.Payload) != r.received || r.received != r.expected {
				r.failure = io.ErrUnexpectedEOF
				return 0, r.failure
			}
			r.done = true
			return 0, io.EOF
		default:
			r.failure = fmt.Errorf("%w: unexpected GET response type %d", ErrInvalidFrame, frame.Type)
			return 0, r.failure
		}
	}
	n := copy(destination, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (c *Client) Delete(key string) error {
	keyPayload, err := EncodeKeyPayload(key)
	if err != nil {
		return err
	}
	if err := c.send(FrameDelete, keyPayload); err != nil {
		return err
	}
	result, err := c.next()
	if err != nil {
		return err
	}
	if result.Type == FrameResult && result.Status == StatusNotFound {
		return nil // Deleting an absent object is idempotent.
	}
	return expectResult(result, 0)
}

type ListItem struct {
	Key                  string
	Size                 int64
	ModifiedAtUnixMillis int64
}

func decodeListItem(payload []byte) (ListItem, error) {
	if len(payload) < 18 {
		return ListItem{}, fmt.Errorf("%w: short LIST_ITEM", ErrInvalidFrame)
	}
	keyLength := int(binary.BigEndian.Uint16(payload[:2]))
	if len(payload) != 2+keyLength+16 {
		return ListItem{}, fmt.Errorf("%w: LIST_ITEM length mismatch", ErrInvalidFrame)
	}
	key, err := DecodeKeyPayload(payload[:2+keyLength])
	if err != nil {
		return ListItem{}, err
	}
	return ListItem{
		Key:                  key,
		Size:                 int64(binary.BigEndian.Uint64(payload[2+keyLength : 10+keyLength])),
		ModifiedAtUnixMillis: int64(binary.BigEndian.Uint64(payload[10+keyLength:])),
	}, nil
}

func (c *Client) List(prefix string) ([]ListItem, error) {
	payload, err := EncodeKeyPayload(prefix)
	if err != nil {
		return nil, err
	}
	if err := c.send(FrameList, payload); err != nil {
		return nil, err
	}
	result, err := c.next()
	if err != nil {
		return nil, err
	}
	if err := expectResult(result, 0); err != nil {
		return nil, err
	}
	items := make([]ListItem, 0)
	for {
		frame, err := c.next()
		if err != nil {
			return nil, err
		}
		switch frame.Type {
		case FrameListItem:
			if len(items) >= MaxListItems {
				return nil, fmt.Errorf("%w: too many LIST_ITEM frames", ErrInvalidFrame)
			}
			item, err := decodeListItem(frame.Payload)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		case FrameEnd:
			if len(frame.Payload) != 4 {
				return nil, fmt.Errorf("%w: LIST END needs 4-byte count", ErrInvalidFrame)
			}
			if frame.Status != StatusOK {
				return nil, &StatusError{Code: frame.Status}
			}
			if binary.BigEndian.Uint32(frame.Payload) != uint32(len(items)) {
				return nil, fmt.Errorf("%w: LIST count mismatch", ErrInvalidFrame)
			}
			return items, nil
		default:
			return nil, fmt.Errorf("%w: unexpected LIST response type %d", ErrInvalidFrame, frame.Type)
		}
	}
}
