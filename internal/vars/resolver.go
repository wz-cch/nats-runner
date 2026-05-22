// Package vars implements the four-level variable resolution engine.
package vars

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// varPattern matches {{word}} placeholders in a template body.
var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

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

// Resolve replaces all {{var}} placeholders in body using the four-level priority:
//
//  1. cliParams   — key=value pairs supplied on the command line (highest priority)
//  2. defaults    — static defaults declared in the template
//  3. functions   — shell command results from [functions] in config.toml
//  4. builtins    — now / now_ms / now_iso (lowest priority)
//
// Function results are cached by variable name for the duration of this call:
//   - Two occurrences of {{uuid}} produce the same value (command runs once).
//   - {{uuid}} and {{uuid2}} each run their own command independently.
//
// Returns an error for any unresolved placeholder or failed function command.
func Resolve(
	body string,
	cliParams map[string]string,
	defaults map[string]string,
	functions map[string]string,
) (string, error) {
	cache := make(map[string]string)
	var firstErr error

	result := varPattern.ReplaceAllStringFunc(body, func(match string) string {
		if firstErr != nil {
			return match
		}
		name := match[2 : len(match)-2] // strip {{ and }}

		// Priority 1: CLI params
		if v, ok := cliParams[name]; ok {
			return v
		}
		// Priority 2: Template defaults
		if defaults != nil {
			if v, ok := defaults[name]; ok {
				return v
			}
		}
		// Priority 3: Config functions (cached per variable name)
		if functions != nil {
			if cmd, ok := functions[name]; ok {
				if cached, hit := cache[name]; hit {
					return cached
				}
				out, err := runShellFn(cmd)
				if err != nil {
					firstErr = fmt.Errorf("function %q failed: %w", name, err)
					return match
				}
				cache[name] = out
				return out
			}
		}
		// Priority 4: Built-in variables
		if v, ok := resolveBuiltin(name); ok {
			return v
		}

		firstErr = fmt.Errorf(
			"variable %q is not defined. Provide it via CLI params, template defaults, or config functions",
			name,
		)
		return match
	})

	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

// resolveBuiltin returns values for the built-in variable names.
func resolveBuiltin(name string) (string, bool) {
	now := time.Now().UTC()
	switch name {
	case "now":
		return fmt.Sprintf("%d", now.Unix()), true
	case "now_ms":
		return fmt.Sprintf("%d", now.UnixMilli()), true
	case "now_iso":
		return now.Format(time.RFC3339), true
	case "uuid":
		return newUUID(), true
	}
	return "", false
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
