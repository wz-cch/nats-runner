// Package vars implements the five-level variable resolution engine
// backed by Go text/template.
package vars

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"nats-runner/internal/domain"
)

// runShellFn is a package-level variable to allow replacement in tests.
var runShellFn = func(cmd string) (string, error) {
	var stdout, stderr bytes.Buffer
	c := exec.Command("sh", "-c", cmd)
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("exit error: %v; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ResolveContext encapsulates all data sources for the five-layer resolution.
// Priority order (lowest → highest):
//  1. builtins (FuncMap: uuid, now, now_ms, now_iso, toJson, trim)
//  2. Functions — shell results, executed once per referenced key, cached in data map
//  3. Defaults  — static strings from template entry
//  4. MergedVals — loaded from --values files
//  5. CLIParams — highest priority, key=value from command line / TUI inputs
//
// When Preview is true, shell functions are NOT executed; their values are
// replaced with a "<func:NAME>" placeholder so the payload can be rendered
// cheaply and without side effects (used by the interactive preview pane).
type ResolveContext struct {
	CLIParams  map[string]string
	MergedVals map[string]any
	Defaults   map[string]string
	Functions  map[string]domain.FuncConfig
	Preview    bool
}

// Resolve renders body through Go text/template using the five-layer priority.
//
// Syntax conventions:
//   - Data variables (from any layer): {{ .key }}
//   - Built-in functions:              {{ uuid }}, {{ now }}, {{ now_ms }}, {{ now_iso }}
//   - Pipe helpers:                    {{ .arr | toJson }}, {{ .s | trim }}
//
// A missing data key causes an error (missingkey=error). When the rendered
// output looks like JSON, it is validated and a clear error is returned if it
// is malformed (commonly an unescaped string value — use {{ .field | toJson }}).
func Resolve(body string, ctx ResolveContext) (string, error) {
	tmpl, err := template.New("body").
		Funcs(buildFuncMap()).
		Option("missingkey=error").
		Parse(body)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	referenced := referencedFields(tmpl)
	data, err := buildDataMap(ctx, referenced)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template render error: %w", err)
	}
	out := buf.String()

	if looksLikeJSON(out) && !json.Valid([]byte(out)) {
		return "", fmt.Errorf(
			"rendered payload is not valid JSON; check that string values are escaped "+
				"(e.g. use {{ .field | toJson }} for free-text fields):\n%s", out)
	}
	return out, nil
}

// ReferencedVars returns the sorted, unique data-variable names ({{ .key }})
// referenced by body. Built-in function calls ({{ uuid }} etc.) are not data
// variables and are excluded. Used by the TUI to discover which inputs a
// template entry needs.
func ReferencedVars(body string) ([]string, error) {
	tmpl, err := template.New("body").Funcs(buildFuncMap()).Parse(body)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}
	set := referencedFields(tmpl)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// buildDataMap constructs the merged data map. Shell functions are executed
// only when their key is referenced by the body (lazy), each at most once.
func buildDataMap(ctx ResolveContext, referenced map[string]bool) (map[string]any, error) {
	data := make(map[string]any)

	// Level 2: shell functions (lowest data priority) — only those referenced.
	for k, fc := range ctx.Functions {
		if !referenced[k] {
			continue
		}
		if ctx.Preview {
			data[k] = "<func:" + k + ">"
			continue
		}
		out, err := runShellFn(fc.Command)
		if err != nil {
			return nil, fmt.Errorf("function %q failed: %w", k, err)
		}
		data[k] = out
	}

	// Level 3: template defaults
	for k, v := range ctx.Defaults {
		data[k] = v
	}

	// Level 4: merged values
	for k, v := range ctx.MergedVals {
		data[k] = v
	}

	// Level 5: CLI params (highest priority)
	for k, v := range ctx.CLIParams {
		data[k] = v
	}

	return data, nil
}

// referencedFields walks the parsed template tree and returns the set of
// top-level data-variable identifiers ({{ .key }} → "key").
func referencedFields(tmpl *template.Template) map[string]bool {
	set := make(map[string]bool)
	if tmpl.Tree != nil && tmpl.Tree.Root != nil {
		collectFields(tmpl.Tree.Root, set)
	}
	return set
}

// collectFields recursively collects FieldNode identifiers from a parse tree.
func collectFields(node parse.Node, set map[string]bool) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			collectFields(c, set)
		}
	case *parse.ActionNode:
		collectFields(n.Pipe, set)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, c := range n.Cmds {
			collectFields(c, set)
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			collectFields(a, set)
		}
	case *parse.FieldNode:
		if len(n.Ident) > 0 {
			set[n.Ident[0]] = true
		}
	case *parse.IfNode:
		collectFields(n.Pipe, set)
		collectFields(n.List, set)
		collectFields(n.ElseList, set)
	case *parse.RangeNode:
		collectFields(n.Pipe, set)
		collectFields(n.List, set)
		collectFields(n.ElseList, set)
	case *parse.WithNode:
		collectFields(n.Pipe, set)
		collectFields(n.List, set)
		collectFields(n.ElseList, set)
	}
}

// looksLikeJSON reports whether s appears to be a JSON object or array.
func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

// buildFuncMap returns the FuncMap for built-in template functions.
// now/now_ms/now_iso are captured at call time so they stay consistent within
// a single Resolve call; uuid generates a fresh value on every occurrence.
func buildFuncMap() template.FuncMap {
	now := time.Now().UTC()
	return template.FuncMap{
		"uuid":    newUUID,
		"now":     func() string { return strconv.FormatInt(now.Unix(), 10) },
		"now_ms":  func() string { return strconv.FormatInt(now.UnixMilli(), 10) },
		"now_iso": func() string { return now.Format(time.RFC3339) },
		"toJson":  toJsonFn,
		"trim":    strings.TrimSpace,
	}
}

// toJsonFn marshals v to a JSON string for use as a template pipe.
// For a plain string it yields a properly quoted and escaped JSON string,
// making it the recommended way to embed free-text values into a JSON body.
func toJsonFn(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toJson: %w", err)
	}
	return string(b), nil
}

// newUUID generates a random UUID v4 using crypto/rand.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
