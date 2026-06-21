package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogger_WriteEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	log, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ts := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	log.WriteEntry(Entry{
		Timestamp:   ts,
		Action:      "req",
		Subject:     "demo.subject",
		Values:      map[string]any{"id": "42", "count": 1000000},
		RequestBody: `{"id": "42"}`,
		Reply:       `{"ok": true}`,
		DurationMs:  12.34,
		Error:       fmt.Errorf("boom"),
	})
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(data)
	for _, want := range []string{"req demo.subject", "[VALUES]", "id:42", `{"id": "42"}`, `{"ok": true}`, "12.34ms", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
	if filepath.Ext(log.Path()) != ".log" {
		t.Errorf("expected .log file, got %s", log.Path())
	}
}

// WriteEntry after Close must not panic (the file handle is nil).
func TestLogger_WriteAfterClose(t *testing.T) {
	t.Chdir(t.TempDir())
	log, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	log.WriteEntry(Entry{Action: "req"}) // must be a no-op, not a panic
}
