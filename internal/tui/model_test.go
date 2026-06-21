package tui

import (
	"strings"
	"testing"

	"nats-runner/internal/domain"
)

func sampleModel() Model {
	m := Model{
		funcs:       map[string]domain.FuncConfig{"gen": {Command: "echo x"}},
		inputs:      map[string]string{},
		mergedVals:  map[string]any{},
		connName:    "metering",
		tmplPath:    "templates/srp.toml",
		entryName:   "create",
		countStr:    "1",
		intervalStr: "1s",
		tmplMap: map[string]domain.TemplateEntry{
			"create": {
				Subject:  "s.create",
				Mode:     "req",
				Defaults: map[string]string{"role": "member"},
				Body:     `{"id": "{{ .id }}", "role": "{{ .role }}", "g": "{{ .gen }}"}`,
			},
		},
	}
	m.rows = buildRows(m)
	return m
}

// T3: variable rows are discovered by scanning the body, not just defaults.
func TestBuildRows_ScansBodyForVars(t *testing.T) {
	m := sampleModel()
	got := map[string]string{}
	for _, r := range m.rows {
		if r.kind == kindVar {
			got[r.key] = r.source
		}
	}
	for _, k := range []string{"id", "role", "gen"} {
		if _, ok := got[k]; !ok {
			t.Errorf("expected var row for %q (referenced in body), rows: %v", k, got)
		}
	}
	if got["role"] != "defaults" {
		t.Errorf("role should resolve from defaults, got %q", got["role"])
	}
	if got["gen"] != "func" {
		t.Errorf("gen should resolve from func, got %q", got["gen"])
	}
	if got["id"] != "" {
		t.Errorf("id has no source and should need input, got %q", got["id"])
	}
}

// T3: only the unsatisfied variable is reported as missing.
func TestMissingVars(t *testing.T) {
	m := sampleModel()
	missing := m.missingVars()
	if len(missing) != 1 || missing[0] != "id" {
		t.Errorf("expected [id] missing, got %v", missing)
	}
	m.inputs["id"] = "42"
	m.rows = buildRows(m)
	if len(m.missingVars()) != 0 {
		t.Errorf("after filling id, nothing should be missing, got %v", m.missingVars())
	}
}

// T2: user inputs feed the preview (and, by the same ResolveContext, execution).
func TestPreviewPayload_UsesInputsAndStubsFuncs(t *testing.T) {
	m := sampleModel()
	m.inputs["id"] = "42"
	out, err := m.previewPayload()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"42"`) {
		t.Errorf("input id not reflected in preview: %s", out)
	}
	if !strings.Contains(out, "member") {
		t.Errorf("default role not reflected: %s", out)
	}
	if !strings.Contains(out, "<func:gen>") {
		t.Errorf("function should be stubbed in preview: %s", out)
	}
}

func TestBuildCLI(t *testing.T) {
	m := sampleModel()
	m.inputs["id"] = "42"
	cli := buildCLI(m)
	for _, want := range []string{"-c metering", "-t templates/srp.toml", "-n create", "id=42"} {
		if !strings.Contains(cli, want) {
			t.Errorf("CLI missing %q: %s", want, cli)
		}
	}
}

func TestWrapIndex(t *testing.T) {
	cases := []struct{ i, n, want int }{
		{0, 3, 0}, {1, 3, 1}, {3, 3, 0}, {-1, 3, 2}, {-2, 3, 1}, {5, 3, 2},
	}
	for _, c := range cases {
		if got := wrapIndex(c.i, c.n); got != c.want {
			t.Errorf("wrapIndex(%d,%d)=%d want %d", c.i, c.n, got, c.want)
		}
	}
}

// Inline ←/→ cycling changes the entry without opening a picker.
func TestCycleSelector_Entry(t *testing.T) {
	m := sampleModel()
	m.entryNames = []string{"create", "get", "list"}
	m.entryName = "create"
	m.rows = buildRows(m)
	for i, r := range m.rows {
		if r.kind == kindEntry {
			m.cursor = i
		}
	}
	m.cycleSelector(1)
	if m.entryName != "get" {
		t.Errorf("right should advance to 'get', got %q", m.entryName)
	}
	m.cycleSelector(-1)
	m.cycleSelector(-1)
	if m.entryName != "list" {
		t.Errorf("left past start should wrap to 'list', got %q", m.entryName)
	}
}

// startExec must refuse to run until prerequisites are met.
func TestStartExec_Validation(t *testing.T) {
	m := sampleModel() // connCfg is nil
	got, _ := m.startExec()
	gm := got.(Model)
	if gm.mode != modeForm {
		t.Errorf("should stay on form when connection missing")
	}
	if gm.status == "" {
		t.Errorf("expected a status message when connection missing")
	}

	// With a connection but a missing required var, still refuse.
	m.connCfg = &domain.ConnectionConfig{URL: "nats://x:4222"}
	got2, _ := m.startExec()
	gm2 := got2.(Model)
	if gm2.mode != modeForm || !strings.Contains(gm2.status, "id") {
		t.Errorf("expected refusal citing missing var id, got mode=%v status=%q", gm2.mode, gm2.status)
	}
}
