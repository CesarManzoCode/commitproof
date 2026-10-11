// Package history implements the run-directory formats of CONTRACT §5.4 and
// §6.1: canonical, hash-chained JSONL streams (history.jsonl,
// observations.jsonl), their verification, and the directory seal.
//
// It is imported by commitproof (writing) and by cpcheck/cpgate (verifying),
// so it must not depend on runner, cluster, workload, observation or
// Multigres packages (CONTRACT §5.3).
package history

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
)

// ZeroHash is the prev value of line 0.
var ZeroHash = strings.Repeat("0", 64)

// Event types of §6.1.
const (
	TypeInvoke  = "invoke"
	TypeOK      = "ok"
	TypeFail    = "fail"
	TypeInfo    = "info"
	TypeNemesis = "nemesis"
	TypeMarker  = "marker"
	TypeObs     = "obs"
)

// Writer appends canonical, hash-chained lines. It assigns i, prev and t
// (nanoseconds since origin, monotonic) under one lock, so lines are totally
// ordered and t is non-decreasing (ADR-0018: a single writer per stream).
type Writer struct {
	mu     sync.Mutex
	f      *os.File
	w      *bufio.Writer
	i      int64
	prev   string
	origin time.Time
	closed bool
	err    error
}

// Create opens a new stream file. origin must carry a monotonic reading
// (time.Now()); t is measured from it.
func Create(path string, origin time.Time) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("history: create %s: %w", path, err)
	}
	return &Writer{f: f, w: bufio.NewWriterSize(f, 1<<16), prev: ZeroHash, origin: origin}, nil
}

// Append writes one event. fields must not contain i, prev or t. It returns
// the line index and the t assigned.
func (w *Writer) Append(fields map[string]any) (int64, int64, error) {
	for _, k := range []string{"i", "prev", "t"} {
		if _, ok := fields[k]; ok {
			return 0, 0, fmt.Errorf("history: field %q is assigned by the writer", k)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, 0, fmt.Errorf("history: append after close")
	}
	if w.err != nil {
		return 0, 0, w.err
	}
	t := int64(time.Since(w.origin))
	ev := make(map[string]any, len(fields)+3)
	for k, v := range fields {
		ev[k] = v
	}
	ev["i"] = w.i
	ev["prev"] = w.prev
	ev["t"] = t
	line, err := canonjson.Marshal(ev)
	if err != nil {
		return 0, 0, fmt.Errorf("history: line %d: %w", w.i, err)
	}
	if _, err := w.w.Write(append(line, '\n')); err != nil {
		w.err = fmt.Errorf("history: write line %d: %w", w.i, err)
		return 0, 0, w.err
	}
	sum := sha256.Sum256(line)
	w.prev = hex.EncodeToString(sum[:])
	i := w.i
	w.i++
	return i, t, nil
}

// Len is the number of lines written so far.
func (w *Writer) Len() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.i
}

// Flush pushes buffered lines to the file.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	return w.w.Flush()
}

// Close flushes, fsyncs and closes. Further appends fail.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.w.Flush(); err != nil {
		w.f.Close()
		return fmt.Errorf("history: flush: %w", err)
	}
	if err := w.f.Sync(); err != nil {
		w.f.Close()
		return fmt.Errorf("history: fsync: %w", err)
	}
	return w.f.Close()
}
