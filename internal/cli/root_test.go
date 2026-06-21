package cli

import (
	"testing"
	"time"
)

func TestParseParams(t *testing.T) {
	got := parseParams([]string{"id=42", "name=Jack", "json={\"a\":1}", "empty="})
	want := map[string]string{"id": "42", "name": "Jack", "json": `{"a":1}`, "empty": ""}
	if len(got) != len(want) {
		t.Fatalf("got %d params, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("param %q: got %q want %q", k, got[k], v)
		}
	}
}

func TestParseInterval(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"60s", 60 * time.Second, false},
		{"500ms", 500 * time.Millisecond, false},
		{"nonsense", 0, true},
	}
	for _, c := range cases {
		got, err := parseInterval(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseInterval(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseInterval(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseInterval(%q): got %v want %v", c.in, got, c.want)
		}
	}
}
