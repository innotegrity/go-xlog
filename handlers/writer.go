package handlers

import (
	"bufio"
	"io"
	"sync"
)

// atomicWriter is a goroutine-safe wrapper for a bufio.Writer.
//
// It ensures that Write and Flush calls are serialized, preventing race conditions between slog writing to the buffer
// and the Close or Flush functions flushing it on exit.
type atomicWriter struct {
	// unexported variables
	mu  sync.Mutex    // mutex for synchronization
	buf *bufio.Writer // underlying buffered writer
}

// newAtomicWriter creates a new [atomicWriter] object.
func newAtomicWriter(wr io.Writer, size int) *atomicWriter {
	return &atomicWriter{
		buf: bufio.NewWriterSize(wr, size),
	}
}

// Close securely closes the underlying buffer by simply flushing its contents.
func (aw *atomicWriter) Close() error {
	return aw.Flush()
}

// Flush securely flushes the underlying buffer.
func (aw *atomicWriter) Flush() error {
	aw.mu.Lock()
	defer aw.mu.Unlock()
	return aw.buf.Flush()
}

// Write implements the io.Writer interface.
//
// It locks the mutex to ensure only one goroutine can write to the buffer at a time.
func (aw *atomicWriter) Write(p []byte) (int, error) {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	// the underlying bufio.Writer.Write will handle buffering the entire byte slice 'p' (which is one full
	// JSON log line) and will not be interrupted by a flush or closing the writer
	return aw.buf.Write(p)
}
