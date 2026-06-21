package vars

import (
	"fmt"
	"strings"
	"testing"

	"nats-runner/internal/domain"
)

// ── helpers ────────────────────────────────────────────────────────────────

func ctx(cli map[string]string, vals map[string]any, defs map[string]string, fns map[string]domain.FuncConfig) ResolveContext {
	return ResolveContext{CLIParams: cli, MergedVals: vals, Defaults: defs, Functions: fns}
}

// ── data-layer tests ───────────────────────────────────────────────────────

func TestResolve_CLIParams(t *testing.T) {
	body := `{"id": "{{ .id }}", "name": "{{ .name }}"}`
	result, err := Resolve(body, ctx(
		map[string]string{"id": "123", "name": "Jack"},
		nil, nil, nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"123"`) || !strings.Contains(result, `"Jack"`) {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestResolve_CLIOverridesDefaults(t *testing.T) {
	body := `{"role": "{{ .role }}"}`
	result, err := Resolve(body, ctx(
		map[string]string{"role": "admin"},
		nil,
		map[string]string{"role": "member"},
		nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "admin") {
		t.Errorf("expected 'admin' but got: %s", result)
	}
}

func TestResolve_CLIOverridesMergedVals(t *testing.T) {
	body := `{"type": "{{ .srp_type }}"}`
	result, err := Resolve(body, ctx(
		map[string]string{"srp_type": "override"},
		map[string]any{"srp_type": "from-values"},
		nil, nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "override") {
		t.Errorf("expected 'override': %s", result)
	}
}

func TestResolve_MergedValsOverrideDefaults(t *testing.T) {
	body := `{"type": "{{ .srp_type }}"}`
	result, err := Resolve(body, ctx(
		nil,
		map[string]any{"srp_type": "from-values"},
		map[string]string{"srp_type": "from-defaults"},
		nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "from-values") {
		t.Errorf("expected 'from-values': %s", result)
	}
}

func TestResolve_Defaults(t *testing.T) {
	body := `{"status": "{{ .status }}"}`
	result, err := Resolve(body, ctx(
		nil, nil,
		map[string]string{"status": "active"},
		nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "active") {
		t.Errorf("unexpected result: %s", result)
	}
}

// ── function (shell) tests ─────────────────────────────────────────────────

func TestResolve_FunctionCache_SameVarRunsOnce(t *testing.T) {
	callCount := 0
	orig := runShellFn
	defer func() { runShellFn = orig }()
	runShellFn = func(cmd string) (string, error) {
		callCount++
		return "fixed-uuid", nil
	}

	// Shell-function result is stored in data map → both {{ .uuid }} read the same value.
	body := `{"a": "{{ .uuid }}", "b": "{{ .uuid }}"}`
	result, err := Resolve(body, ctx(
		nil, nil, nil,
		map[string]domain.FuncConfig{"uuid": {Command: "uuidgen"}},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected function to run once, got %d times", callCount)
	}
	if strings.Count(result, "fixed-uuid") != 2 {
		t.Errorf("expected both occurrences to be replaced: %s", result)
	}
}

func TestResolve_DifferentVarsSameCommand_RunsIndependently(t *testing.T) {
	callCount := 0
	orig := runShellFn
	defer func() { runShellFn = orig }()
	runShellFn = func(cmd string) (string, error) {
		callCount++
		return strings.Repeat("x", callCount), nil
	}

	body := `{"a": "{{ .uuid }}", "b": "{{ .uuid2 }}"}`
	_, err := Resolve(body, ctx(
		nil, nil, nil,
		map[string]domain.FuncConfig{
			"uuid":  {Command: "uuidgen"},
			"uuid2": {Command: "uuidgen"},
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 function calls (one per variable name), got %d", callCount)
	}
}

func TestResolve_FunctionFailure(t *testing.T) {
	orig := runShellFn
	defer func() { runShellFn = orig }()
	runShellFn = func(cmd string) (string, error) {
		return "", fmt.Errorf("exit status 1")
	}

	body := `{"user": "{{ .random_user }}"}`
	_, err := Resolve(body, ctx(
		nil, nil, nil,
		map[string]domain.FuncConfig{"random_user": {Command: "shuf -n 1 /nonexistent"}},
	))
	if err == nil {
		t.Fatal("expected error when function fails")
	}
	if !strings.Contains(err.Error(), "random_user") {
		t.Errorf("error should mention the function name: %v", err)
	}
}

// ── missing key ────────────────────────────────────────────────────────────

func TestResolve_UnresolvedError(t *testing.T) {
	body := `{"id": "{{ .missing }}"}`
	_, err := Resolve(body, ResolveContext{})
	if err == nil {
		t.Fatal("expected error for unresolved variable")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention the variable name: %v", err)
	}
}

// ── builtin tests ──────────────────────────────────────────────────────────

func TestResolve_Builtins(t *testing.T) {
	for _, name := range []string{"now", "now_ms", "now_iso", "uuid"} {
		body := `{"ts": "{{ ` + name + ` }}"}`
		result, err := Resolve(body, ResolveContext{})
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", name, err)
		}
		if strings.Contains(result, "{{") {
			t.Errorf("builtin %s was not replaced: %s", name, result)
		}
	}
}

func TestResolve_UUIDBuiltin_Format(t *testing.T) {
	body := `{"id": "{{ uuid }}"}`
	result, err := Resolve(body, ResolveContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	start := strings.Index(result, `"id": "`) + 7
	end := strings.LastIndex(result, `"`)
	got := result[start:end]

	parts := strings.Split(got, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d: %q", len(parts), got)
	}
	lengths := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != lengths[i] {
			t.Errorf("part %d: expected length %d, got %d: %q", i, lengths[i], len(p), p)
		}
	}
	if got[14] != '4' {
		t.Errorf("expected version nibble '4', got %q", got[14])
	}
}

func TestResolve_UUIDBuiltin_UniquePerResolve(t *testing.T) {
	body := `{"id": "{{ uuid }}"}`
	r1, _ := Resolve(body, ResolveContext{})
	r2, _ := Resolve(body, ResolveContext{})
	if r1 == r2 {
		t.Errorf("two separate Resolve calls should produce different UUIDs: %s", r1)
	}
}

func TestResolve_UUIDBuiltin_EachOccurrenceIsValid(t *testing.T) {
	// Both {{ uuid }} occurrences should be replaced (no unreplaced placeholders).
	body := `{"a": "{{ uuid }}", "b": "{{ uuid }}"}`
	result, err := Resolve(body, ResolveContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "{{") {
		t.Errorf("uuid placeholder not replaced: %s", result)
	}
}

// ── toJson pipe ────────────────────────────────────────────────────────────

func TestResolve_ToJsonPipe(t *testing.T) {
	body := `{"metrics": {{ .metrics | toJson }}}`
	result, err := Resolve(body, ctx(
		nil,
		map[string]any{"metrics": []any{
			map[string]any{"field": "tag", "type": "sum"},
		}},
		nil, nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"field"`) {
		t.Errorf("expected JSON array in result: %s", result)
	}
}

// ── misc ───────────────────────────────────────────────────────────────────

func TestResolve_NoPlaceholders(t *testing.T) {
	body := `{"static": "value"}`
	result, err := Resolve(body, ResolveContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != body {
		t.Errorf("body with no placeholders should be unchanged: %s", result)
	}
}

func TestResolve_ConditionalBlock(t *testing.T) {
	body := `{{ if .enabled }}on{{ else }}off{{ end }}`
	result, err := Resolve(body, ctx(nil, map[string]any{"enabled": true}, nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "on" {
		t.Errorf("expected 'on', got %q", result)
	}
}
