package nats

import (
	"encoding/json"
	"fmt"
	"time"

	"nats-runner/internal/domain"

	natsgo "github.com/nats-io/nats.go"
)

// Connect establishes a NATS connection using settings from a ConnectionConfig.
func Connect(conn *domain.ConnectionConfig) (*natsgo.Conn, error) {
	opts, err := BuildOptions(conn)
	if err != nil {
		return nil, err
	}
	nc, err := natsgo.Connect(conn.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS at %s: %w", conn.URL, err)
	}
	return nc, nil
}

// Exec runs entry according to its mode and returns a human-readable result
// string for display (the pretty reply for "req", a confirmation line for
// "pub"/"js"). It performs no I/O of its own, so both the CLI and the TUI can
// reuse it and decide how to present the result.
func Exec(nc *natsgo.Conn, entry *domain.TemplateEntry, payload []byte, timeoutMs int) (string, error) {
	switch entry.Mode {
	case "req":
		return ExecReqReply(nc, entry.Subject, payload, timeoutMs)
	case "pub":
		return ExecPub(nc, entry.Subject, payload)
	case "js":
		return ExecJS(nc, entry, payload)
	default:
		return "", fmt.Errorf("unknown mode %q (expected req, pub, or js)", entry.Mode)
	}
}

// ExecReqReply sends a synchronous request and returns the pretty-printed reply.
func ExecReqReply(nc *natsgo.Conn, subject string, payload []byte, timeoutMs int) (string, error) {
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	msg, err := nc.Request(subject, payload, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("request to %q failed: %w", subject, err)
	}
	return prettyJSON(msg.Data), nil
}

// ExecPub publishes a fire-and-forget message and returns a confirmation string.
func ExecPub(nc *natsgo.Conn, subject string, payload []byte) (string, error) {
	if err := nc.Publish(subject, payload); err != nil {
		return "", fmt.Errorf("publish to %q failed: %w", subject, err)
	}
	return fmt.Sprintf("Published to: %s", subject), nil
}

// ExecJS publishes to JetStream, optionally auto-creating the stream first.
// It calls js.Publish() synchronously, waits for the server Ack, and returns a
// confirmation string including the assigned sequence number.
//
// Stream creation behaviour (when entry.Stream.Create == true):
//   - Stream does not exist              → creates it.
//   - Stream exists, config matches      → no-op (server handles silently).
//   - Stream exists, config conflicts    → server returns error; propagated as-is.
func ExecJS(nc *natsgo.Conn, entry *domain.TemplateEntry, payload []byte) (string, error) {
	js, err := nc.JetStream()
	if err != nil {
		return "", fmt.Errorf("failed to get JetStream context: %w", err)
	}

	if entry.Stream != nil && entry.Stream.Create {
		if err := ensureStream(js, entry.Stream); err != nil {
			return "", err
		}
	}

	ack, err := js.Publish(entry.Subject, payload)
	if err != nil {
		return "", fmt.Errorf("JetStream publish to %q failed: %w", entry.Subject, err)
	}
	return fmt.Sprintf("JetStream published to: %s (seq: %d)", entry.Subject, ack.Sequence), nil
}

// ensureStream calls js.AddStream with the given StreamConfig.
func ensureStream(js natsgo.JetStreamContext, s *domain.StreamConfig) error {
	storage := natsgo.FileStorage
	if s.Storage == "memory" {
		storage = natsgo.MemoryStorage
	}
	streamCfg := &natsgo.StreamConfig{
		Name:     s.Name,
		Subjects: s.Subjects,
		Storage:  storage,
	}
	if s.MaxAgeSeconds > 0 {
		streamCfg.MaxAge = time.Duration(s.MaxAgeSeconds) * time.Second
	}
	if s.MaxMsgs > 0 {
		streamCfg.MaxMsgs = s.MaxMsgs
	}

	if _, err := js.AddStream(streamCfg); err != nil {
		return fmt.Errorf("failed to ensure stream %q: %w", s.Name, err)
	}
	return nil
}

// prettyJSON formats raw bytes as indented JSON.
// Falls back to the original string if the data is not valid JSON.
func prettyJSON(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(out)
}
