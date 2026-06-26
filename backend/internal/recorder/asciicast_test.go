package recorder

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRecorderProducesValidAsciicast(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "session-1", 80, 24, 1700000000)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	r.Output([]byte("hello "))
	r.Output([]byte("world\n"))
	r.Resize(120, 40)
	r.Input([]byte("ls\n"))
	res := r.Close()

	if res.SizeBytes == 0 || res.SHA256 == "" {
		t.Fatalf("expected non-empty result, got %+v", res)
	}

	f, err := os.Open(res.Path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)

	// First line is the header object with version 2.
	if !sc.Scan() {
		t.Fatal("missing header line")
	}
	var hdr map[string]any
	if err := json.Unmarshal(sc.Bytes(), &hdr); err != nil {
		t.Fatalf("header not JSON: %v", err)
	}
	if v, _ := hdr["version"].(float64); v != 2 {
		t.Fatalf("expected asciicast version 2, got %v", hdr["version"])
	}

	// Remaining lines are [time, type, data] arrays.
	var events int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev []any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("event not JSON array: %v (%s)", err, line)
		}
		if len(ev) != 3 {
			t.Fatalf("event must have 3 fields, got %d", len(ev))
		}
		events++
	}
	if events < 4 {
		t.Fatalf("expected >=4 events, got %d", events)
	}
}
