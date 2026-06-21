// Package logger provides goroutine-safe structured log writing for nats-runner execution runs.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry holds the data for a single execution log record.
type Entry struct {
	Timestamp   time.Time
	Action      string
	Subject     string
	Values      map[string]any
	RequestBody string
	Reply       string
	DurationMs  float64
	Error       error
}

// Logger writes structured entries to a timestamped log file under logs/.
type Logger struct {
	path string
	file *os.File
	mu   sync.Mutex
}

// New creates the logs/ directory if needed, opens a new timestamped log file,
// and returns a ready-to-use Logger. The caller must call Close when done.
func New() (*Logger, error) {
	dir := "logs"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating logs dir: %w", err)
	}
	name := fmt.Sprintf("nats-runner-%s.log", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file %q: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &Logger{path: abs, file: f}, nil
}

// Path returns the absolute path of the log file.
func (l *Logger) Path() string { return l.path }

// Close flushes and closes the underlying log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// WriteEntry appends one structured entry to the log file. Safe for concurrent use.
func (l *Logger) WriteEntry(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	action := e.Action
	if action == "" {
		action = "?"
	}
	fmt.Fprintf(l.file, "[TIMESTAMP] %s\n", e.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"))
	fmt.Fprintf(l.file, "[ACTION]    %s %s\n", action, e.Subject)
	if len(e.Values) > 0 {
		fmt.Fprintf(l.file, "[VALUES]    %v\n", e.Values)
	}
	if e.RequestBody != "" {
		fmt.Fprintf(l.file, "[REQUEST]   %s\n", e.RequestBody)
	}
	if e.Reply != "" {
		fmt.Fprintf(l.file, "[REPLY]     %s\n", e.Reply)
	}
	fmt.Fprintf(l.file, "[DURATION]  %.2fms\n", e.DurationMs)
	if e.Error != nil {
		fmt.Fprintf(l.file, "[ERROR]     %s\n", e.Error)
	}
	fmt.Fprintln(l.file, "---")
}
