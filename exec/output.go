package exec

import (
	"bytes"
	"io"
	"sync"
)

// multiWriter writes to multiple writers simultaneously while capturing all output.
// It's similar to io.MultiWriter but provides a way to retrieve the captured output.
type multiWriter struct {
	writers []io.Writer
	mu      sync.Mutex
}

// newMultiWriter creates a new multiWriter that writes to all provided writers.
func newMultiWriter(writers ...io.Writer) *multiWriter {
	return &multiWriter{
		writers: writers,
	}
}

// Write writes data to all underlying writers.
func (mw *multiWriter) Write(p []byte) (n int, err error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

// outputCapture captures output while optionally streaming it to another writer.
type outputCapture struct {
	buffer     *bytes.Buffer
	passthrough io.Writer
	mu         sync.Mutex
}

// newOutputCapture creates a new output capture.
// If passthrough is non-nil, output will be written to it in addition to being captured.
func newOutputCapture(passthrough io.Writer) *outputCapture {
	return &outputCapture{
		buffer:     &bytes.Buffer{},
		passthrough: passthrough,
	}
}

// Writer returns an io.Writer that captures output.
func (oc *outputCapture) Writer() io.Writer {
	if oc.passthrough != nil {
		return newMultiWriter(oc.buffer, oc.passthrough)
	}
	return oc.buffer
}

// String returns the captured output as a string.
func (oc *outputCapture) String() string {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	return oc.buffer.String()
}

// combinedWriter combines stdout and stderr into a single output stream.
type combinedWriter struct {
	buffer *bytes.Buffer
	mu     sync.Mutex
}

// newCombinedWriter creates a new combined writer.
func newCombinedWriter() *combinedWriter {
	return &combinedWriter{
		buffer: &bytes.Buffer{},
	}
}

// Write writes data to the combined buffer.
func (cw *combinedWriter) Write(p []byte) (n int, err error) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.buffer.Write(p)
}

// String returns the combined output as a string.
func (cw *combinedWriter) String() string {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.buffer.String()
}
