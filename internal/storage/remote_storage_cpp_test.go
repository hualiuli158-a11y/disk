package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRemoteStorageAgainstCppNode(t *testing.T) {
	address := os.Getenv("STORAGE_NODE_TEST_ADDRESS")
	if address == "" {
		t.Skip("set STORAGE_NODE_TEST_ADDRESS to run the C++ node integration test")
	}
	store, err := NewRemoteStorage(address, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	key := "objects/go-test/" + hex.EncodeToString(random[:])
	content := make([]byte, 150000)
	for i := range content {
		content[i] = byte(i)
	}
	content[0], content[len(content)-1] = 0, 0xff
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	if err := store.Save(ctx, key, bytes.NewReader(content)); err != nil {
		t.Fatalf("PUT against C++ node: %v", err)
	}
	reader, err := store.Open(ctx, key)
	if err != nil {
		t.Fatalf("GET against C++ node: %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, content) {
		t.Fatalf("GET data mismatch: read=%v close=%v got=%d want=%d", readErr, closeErr, len(got), len(content))
	}

	items, err := store.List(ctx, "objects")
	if err != nil {
		t.Fatalf("LIST against C++ node: %v", err)
	}
	found := false
	for _, item := range items {
		if item.Key == key && item.Size == int64(len(content)) && !item.ModifiedAt.IsZero() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("LIST omitted uploaded key %s", key)
	}
	for i := 0; i < 2; i++ {
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("DELETE attempt %d: %v", i+1, err)
		}
	}
	if _, err := store.Open(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleted object was still readable: %v", err)
	}

	emptyKey := "objects/go-test/empty-" + hex.EncodeToString(random[:])
	t.Cleanup(func() { _ = store.Delete(context.Background(), emptyKey) })
	if err := store.Save(ctx, emptyKey, strings.NewReader("")); err != nil {
		t.Fatalf("empty PUT: %v", err)
	}
	emptyReader, err := store.Open(ctx, emptyKey)
	if err != nil {
		t.Fatalf("empty GET: %v", err)
	}
	empty, readErr := io.ReadAll(emptyReader)
	_ = emptyReader.Close()
	if readErr != nil || len(empty) != 0 {
		t.Fatalf("empty GET returned %d bytes: %v", len(empty), readErr)
	}
}
