// Package recorder writes terminal sessions in the asciicast v2 format, which is
// directly replayable in the browser (xterm.js / asciinema player).
package recorder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Recorder writes one asciicast v2 file. It is safe for concurrent writes from
// the input/output relay goroutines.
type Recorder struct {
	mu     sync.Mutex
	f      *os.File
	hasher hash.Hash
	start  time.Time
	path   string
	bytes  int64
	closed bool
}

// Header is the asciicast v2 first line.
type header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Env       map[string]string `json:"env,omitempty"`
}

// New creates a recording file under dir named by id and writes the header.
func New(dir, id string, cols, rows int, startUnix int64) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".cast")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	r := &Recorder{f: f, start: time.Unix(startUnix, 0), path: path, hasher: sha256.New()}
	h := header{Version: 2, Width: cols, Height: rows, Timestamp: startUnix,
		Env: map[string]string{"TERM": "xterm-256color"}}
	line, _ := json.Marshal(h)
	r.writeLine(line)
	return r, nil
}

// Output records terminal output bytes ("o" event).
func (r *Recorder) Output(b []byte) { r.event("o", string(b)) }

// Input records terminal input bytes ("i" event), useful for keystroke audit.
func (r *Recorder) Input(b []byte) { r.event("i", string(b)) }

// Resize records a terminal resize ("r" event) as "COLSxROWS".
func (r *Recorder) Resize(cols, rows int) { r.event("r", fmt.Sprintf("%dx%d", cols, rows)) }

func (r *Recorder) event(kind, data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	elapsed := time.Since(r.start).Seconds()
	rec := []any{elapsed, kind, data}
	line, _ := json.Marshal(rec)
	r.writeLineLocked(line)
}

func (r *Recorder) writeLine(line []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeLineLocked(line)
}

func (r *Recorder) writeLineLocked(line []byte) {
	line = append(line, '\n')
	n, _ := r.f.Write(line)
	_, _ = r.hasher.Write(line)
	r.bytes += int64(n)
}

// Result is the finalized recording metadata.
type Result struct {
	Path       string
	SizeBytes  int64
	DurationMS int64
	SHA256     string
}

// Close finalizes the recording and returns its metadata.
func (r *Recorder) Close() Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Result{Path: r.path, SizeBytes: r.bytes}
	}
	r.closed = true
	_ = r.f.Sync()
	_ = r.f.Close()
	sum := r.hasher.Sum(nil)
	return Result{
		Path:       r.path,
		SizeBytes:  r.bytes,
		DurationMS: time.Since(r.start).Milliseconds(),
		SHA256:     hex.EncodeToString(sum),
	}
}

// Path returns the recording file path.
func (r *Recorder) Path() string { return r.path }
