package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"nats-runner/internal/domain"
	"nats-runner/internal/vars"
)

// viewForm renders the two-pane editor: settings on the left, live preview on
// the right, with the equivalent CLI command and key hints below.
func (m Model) viewForm() string {
	leftW, rightW := m.paneWidths()

	left := Styles.Border.Width(leftW).Render(m.renderFields())
	right := Styles.Border.Width(rightW).Render(m.renderPreview(rightW))
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	var b strings.Builder
	title := "nats-runner · interactive"
	if m.version != "" {
		title = "nats-runner " + m.version + " · interactive"
	}
	b.WriteString(Styles.Title.Render(title) + "\n")
	b.WriteString(panes + "\n")
	if cli := buildCLI(m); cli != "" {
		b.WriteString(Styles.CLIBox.Width(leftW+rightW+2).Render("$ "+cli) + "\n")
	}
	if m.status != "" {
		b.WriteString(Styles.Err.Render("⚠ "+m.status) + "\n")
	}
	b.WriteString(Styles.Hint.Render(
		"↑/↓ move · ←/→ switch connection/template/entry · Enter open list/toggle · type on variable rows · Ctrl+R run · Ctrl+C quit"))
	return b.String()
}

// paneWidths computes left/right inner widths from the terminal size.
func (m Model) paneWidths() (int, int) {
	w := m.width
	if w <= 0 {
		w = 100
	}
	leftW := w * 42 / 100
	if leftW < 30 {
		leftW = 30
	}
	rightW := w - leftW - 6
	if rightW < 28 {
		rightW = 28
	}
	return leftW, rightW
}

// renderFields renders the left-pane list of focusable rows.
func (m Model) renderFields() string {
	entry, hasEntry := m.tmplMap[m.entryName]
	var b strings.Builder
	for i, r := range m.rows {
		focused := i == m.cursor
		b.WriteString(Cursor(focused))
		b.WriteString(m.renderRow(r, entry, hasEntry, focused))
		if i < len(m.rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderRow renders a single form row.
func (m Model) renderRow(r formRow, entry domain.TemplateEntry, hasEntry, focused bool) string {
	label := func(s string) string { return Styles.Label.Render(fmt.Sprintf("%-9s", s)) }
	val := func(s string) string { return Styles.Value.Render(s) }
	unset := Styles.Hint.Render("(unset)")

	switch r.kind {
	case kindConn:
		v := unset
		if m.connName != "" {
			suffix := ""
			if m.connCfg != nil {
				suffix = Styles.Source.Render("  " + m.connCfg.URL)
			}
			v = val(m.connName) + suffix
		}
		return label("Conn") + v

	case kindTemplate:
		v := unset
		if m.tmplPath != "" {
			v = val(filepath.Base(m.tmplPath))
		}
		return label("Template") + v

	case kindEntry:
		v := unset
		if m.entryName != "" {
			meta := ""
			if hasEntry {
				meta = Styles.Source.Render(fmt.Sprintf("  [%s] %s", entry.Mode, entry.Subject))
			}
			v = val(m.entryName) + meta
		}
		return label("Entry") + v

	case kindVar:
		return m.renderVarRow(r, entry, focused)

	case kindLoop:
		state := "off"
		if m.loopEnabled {
			state = "on"
		}
		return label("Loop") + val(state) + Styles.Hint.Render("  (Enter toggles)")

	case kindCount:
		return label("Count") + m.renderEditable(m.countStr, focused) +
			Styles.Hint.Render("  (0 = infinite)")

	case kindInterval:
		return label("Interval") + m.renderEditable(m.intervalStr, focused)

	case kindValues:
		v := Styles.Hint.Render("(none)")
		if len(m.valuesFiles) > 0 {
			names := make([]string, len(m.valuesFiles))
			for i, p := range m.valuesFiles {
				names[i] = filepath.Base(p)
			}
			v = val(strings.Join(names, ", "))
		}
		return label("Values") + v

	case kindRun:
		return Styles.OK.Render("▶ Run") + Styles.Hint.Render("  (Enter / Ctrl+R)")
	}
	return ""
}

// renderVarRow renders one variable input row with its source tag.
func (m Model) renderVarRow(r formRow, entry domain.TemplateEntry, focused bool) string {
	src := r.source
	tag := Styles.Source.Render(fmt.Sprintf("%-9s", "·"+src))
	if src == "" {
		tag = Styles.Err.Render(fmt.Sprintf("%-9s", "★ input"))
	}
	name := Styles.Value.Render(fmt.Sprintf("%-16s", r.key))

	// Show the user input if present, otherwise the effective baseline value.
	shown := m.inputs[r.key]
	if shown == "" {
		shown = baselineValue(m, entry, r.key)
	}
	return name + " " + tag + " " + m.renderEditable(shown, focused)
}

// renderEditable renders an editable value, adding a caret when focused.
func (m Model) renderEditable(s string, focused bool) string {
	if focused {
		return Styles.Value.Render(s) + Styles.Selected.Render("▏")
	}
	if s == "" {
		return Styles.Hint.Render("…")
	}
	return Styles.Value.Render(s)
}

// baselineValue returns the value a variable would take if the user types nothing.
func baselineValue(m Model, entry domain.TemplateEntry, k string) string {
	if v, ok := m.mergedVals[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := entry.Defaults[k]; ok {
		return v
	}
	if _, ok := m.funcs[k]; ok {
		return "<func>"
	}
	return ""
}

// renderPreview renders the right pane: subject/mode and the live JSON payload.
func (m Model) renderPreview(width int) string {
	if m.entryName == "" {
		return Styles.Header.Render("Preview") + "\n\n" +
			Styles.Hint.Render("Select connection → template → entry\nto preview the rendered payload")
	}
	entry := m.tmplMap[m.entryName]

	var b strings.Builder
	b.WriteString(Styles.Header.Render("Preview") + "\n")
	b.WriteString(Styles.Label.Render("subject ") + Styles.Value.Render(entry.Subject) + "\n")
	b.WriteString(Styles.Label.Render("mode    ") + Styles.Value.Render(entry.Mode) + "\n")

	payload, err := m.previewPayload()
	if err != nil {
		b.WriteString(Styles.Err.Render("render failed:") + "\n")
		b.WriteString(Styles.Err.Render(wrap(err.Error(), width)))
		return b.String()
	}
	b.WriteString(Styles.Source.Render("payload (JSON ✓)") + "\n")
	b.WriteString(Styles.Normal.Render(clampLines(payload, m.previewHeight())))
	return b.String()
}

// previewPayload renders the payload with shell functions stubbed (no exec).
// Unsatisfied required variables are rendered as empty strings so the preview
// always shows structure instead of a missing-key error.
func (m Model) previewPayload() (string, error) {
	entry, ok := m.tmplMap[m.entryName]
	if !ok {
		return "", nil
	}
	names, _ := vars.ReferencedVars(entry.Body)
	cli := map[string]string{}
	for k, v := range m.inputs {
		if v != "" {
			cli[k] = v
		}
	}
	for _, k := range names {
		if _, ok := cli[k]; ok {
			continue
		}
		if _, ok := m.mergedVals[k]; ok {
			continue
		}
		if _, ok := entry.Defaults[k]; ok {
			continue
		}
		if _, ok := m.funcs[k]; ok {
			continue
		}
		cli[k] = "" // unfilled → blank in preview
	}
	return vars.Resolve(entry.Body, vars.ResolveContext{
		CLIParams:  cli,
		MergedVals: m.mergedVals,
		Defaults:   entry.Defaults,
		Functions:  m.funcs,
		Preview:    true,
	})
}

func (m Model) previewHeight() int {
	h := m.height - 12
	if h < 6 {
		h = 6
	}
	return h
}

// ── text helpers ──────────────────────────────────────────────────────────────

func clampLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	kept := lines[:max]
	return strings.Join(kept, "\n") + "\n" + fmt.Sprintf("… (+%d lines)", len(lines)-max)
}

func wrap(s string, width int) string {
	if width < 8 {
		width = 8
	}
	var b strings.Builder
	r := []rune(s)
	for len(r) > width {
		b.WriteString(string(r[:width]) + "\n")
		r = r[width:]
	}
	b.WriteString(string(r))
	return b.String()
}
