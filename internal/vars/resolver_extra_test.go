package vars

import (
	"strings"
	"testing"

	"nats-runner/internal/domain"
)

// C2: a function defined but NOT referenced by the body must not be executed.
func TestResolve_UnreferencedFunction_NotExecuted(t *testing.T) {
	called := map[string]int{}
	orig := runShellFn
	defer func() { runShellFn = orig }()
	runShellFn = func(cmd string) (string, error) {
		called[cmd]++
		return "x", nil
	}

	body := `{"a": "{{ .used }}"}`
	_, err := Resolve(body, ctx(nil, nil, nil, map[string]domain.FuncConfig{
		"used":   {Command: "used-cmd"},
		"unused": {Command: "unused-cmd"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called["used-cmd"] != 1 {
		t.Errorf("referenced function should run once, got %d", called["used-cmd"])
	}
	if called["unused-cmd"] != 0 {
		t.Errorf("unreferenced function must not run, got %d", called["unused-cmd"])
	}
}

// C1: a rendered payload that looks like JSON but is malformed should error.
func TestResolve_InvalidJSON_Errors(t *testing.T) {
	// An unescaped quote in the value breaks the surrounding JSON string.
	body := `{"desc": "{{ .desc }}"}`
	_, err := Resolve(body, ctx(map[string]string{"desc": `a"b`}, nil, nil, nil))
	if err == nil {
		t.Fatal("expected invalid-JSON error")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error should mention invalid JSON: %v", err)
	}
}

// C1: piping a free-text value through toJson produces valid JSON.
func TestResolve_ToJsonEscaping_Valid(t *testing.T) {
	body := `{"desc": {{ .desc | toJson }}}`
	out, err := Resolve(body, ctx(map[string]string{"desc": `a"b` + "\n"}, nil, nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `\"`) {
		t.Errorf("expected escaped quote in output: %s", out)
	}
}

// Non-JSON payloads are not subjected to JSON validation.
func TestResolve_NonJSON_NotValidated(t *testing.T) {
	body := `plain text {{ .x }}`
	out, err := Resolve(body, ctx(map[string]string{"x": `"unbalanced`}, nil, nil, nil))
	if err != nil {
		t.Fatalf("non-JSON body should not be validated: %v", err)
	}
	if !strings.Contains(out, "unbalanced") {
		t.Errorf("unexpected output: %s", out)
	}
}

// Preview mode stubs shell functions instead of executing them.
func TestResolve_PreviewMode_StubsFunctions(t *testing.T) {
	orig := runShellFn
	defer func() { runShellFn = orig }()
	runShellFn = func(cmd string) (string, error) {
		t.Fatalf("shell function must not run in preview mode")
		return "", nil
	}
	body := `{"v": "{{ .gen }}"}`
	out, err := Resolve(body, ResolveContext{
		Functions: map[string]domain.FuncConfig{"gen": {Command: "should-not-run"}},
		Preview:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<func:gen>") {
		t.Errorf("expected stubbed function value, got: %s", out)
	}
}

func TestReferencedVars(t *testing.T) {
	body := `{"a": "{{ .srp_type }}", "b": {{ now_ms }}, "c": "{{ uuid }}", "d": "{{ .desc | trim }}"}`
	got, err := ReferencedVars(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Builtins (now_ms, uuid) are function calls, not data vars, and excluded.
	want := []string{"desc", "srp_type"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}
