package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"nats-runner/internal/config"
	"nats-runner/internal/domain"
	"nats-runner/internal/template"
	"nats-runner/internal/vars"
)

// picker is a modal list selector overlaid on the form.
type picker struct {
	kind   rowKind // kindConn / kindTemplate / kindEntry / kindValues
	title  string
	items  []string // display labels
	paths  []string // parallel underlying values (conn name / file path / entry name)
	cursor int
	multi  bool
	sel    map[int]bool
}

// ── openers ───────────────────────────────────────────────────────────────────

func (m *Model) openConnPicker() {
	items := make([]string, len(m.conns))
	paths := make([]string, len(m.conns))
	for i, c := range m.conns {
		items[i] = fmt.Sprintf("%s  (%s)", c.Name, c.URL)
		paths[i] = c.Name
	}
	m.startPicker(picker{kind: kindConn, title: "Select connection", items: items, paths: paths})
}

func (m *Model) openTemplatePicker() {
	items := make([]string, len(m.tmplFiles))
	paths := make([]string, len(m.tmplFiles))
	for i, f := range m.tmplFiles {
		items[i] = fmt.Sprintf("%s  (%d entries)", f.FileName, f.EntryCount)
		paths[i] = f.Path
	}
	m.startPicker(picker{kind: kindTemplate, title: "Select template", items: items, paths: paths})
}

func (m *Model) openEntryPicker() {
	items := make([]string, len(m.entryNames))
	paths := make([]string, len(m.entryNames))
	for i, name := range m.entryNames {
		e := m.tmplMap[name]
		items[i] = fmt.Sprintf("%-20s [%s] %s", name, e.Mode, e.Subject)
		paths[i] = name
	}
	m.startPicker(picker{kind: kindEntry, title: "Select entry", items: items, paths: paths})
}

func (m *Model) openValuesPicker() {
	paths, names := scanValuesCandidates(m.gc)
	pk := picker{kind: kindValues, title: "Select values files (space to toggle)", items: names, paths: paths, multi: true, sel: map[int]bool{}}
	// Pre-check already-selected files.
	for i, p := range paths {
		for _, sel := range m.valuesFiles {
			if sel == p {
				pk.sel[i] = true
			}
		}
	}
	m.startPicker(pk)
}

func (m *Model) startPicker(pk picker) {
	m.pk = pk
	m.pickerActive = true
}

// ── key handling ────────────────────────────────────────────────────────────

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.pickerActive = false
	case "up", "k":
		m.pk.cursor = clamp(m.pk.cursor-1, 0, len(m.pk.items)-1)
	case "down", "j":
		m.pk.cursor = clamp(m.pk.cursor+1, 0, len(m.pk.items)-1)
	case " ":
		if m.pk.multi && m.pk.cursor < len(m.pk.items) {
			if m.pk.sel == nil {
				m.pk.sel = map[int]bool{}
			}
			m.pk.sel[m.pk.cursor] = !m.pk.sel[m.pk.cursor]
		}
	case "enter":
		m.commitPicker()
	}
	return m, nil
}

// commitPicker applies the picker selection and closes the overlay.
func (m *Model) commitPicker() {
	defer func() {
		m.pickerActive = false
		m.rows = buildRows(*m)
		m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
	}()

	if m.pk.multi {
		var files []string
		for i := range m.pk.paths {
			if m.pk.sel[i] {
				files = append(files, m.pk.paths[i])
			}
		}
		m.valuesFiles = files
		merged, err := vars.LoadValuesFiles(files)
		if err != nil {
			m.status = err.Error()
			m.mergedVals = map[string]any{}
			return
		}
		m.mergedVals = merged
		return
	}

	if m.pk.cursor >= len(m.pk.paths) {
		return
	}
	val := m.pk.paths[m.pk.cursor]
	switch m.pk.kind {
	case kindConn:
		m.selectConn(val)
	case kindTemplate:
		m.selectTemplate(val)
	case kindEntry:
		m.entryName = val
	}
}

// selectConn loads and selects a connection by name; sets status on error.
func (m *Model) selectConn(name string) {
	cfg, err := config.LoadConnectionFile(config.ResolveConnPath(name))
	if err != nil {
		m.status = err.Error()
		return
	}
	m.connName = name
	m.connCfg = cfg
}

// selectTemplate loads a template file and auto-selects its first entry, so the
// variable rows appear immediately without a separate entry-pick step.
func (m *Model) selectTemplate(path string) {
	tmplMap, err := template.Load(path)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.tmplPath = path
	m.tmplMap = tmplMap
	m.entryNames = sortedTemplateNames(tmplMap)
	if len(m.entryNames) > 0 {
		m.entryName = m.entryNames[0]
	} else {
		m.entryName = ""
	}
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m Model) viewPicker() string {
	var b strings.Builder
	b.WriteString(Styles.Title.Render(m.pk.title) + "\n\n")
	if len(m.pk.items) == 0 {
		b.WriteString(Styles.Hint.Render("  (no items)") + "\n\n")
	}
	for i, it := range m.pk.items {
		mark := "  "
		if m.pk.multi {
			if m.pk.sel[i] {
				mark = Styles.OK.Render("[x] ")
			} else {
				mark = "[ ] "
			}
		}
		line := mark + it
		if i == m.pk.cursor {
			b.WriteString(Styles.Selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n")
	hint := "↑/↓ move · Enter select · Esc cancel"
	if m.pk.multi {
		hint = "↑/↓ move · Space toggle · Enter confirm · Esc cancel"
	}
	b.WriteString(Styles.Hint.Render(hint))
	return b.String()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sortedTemplateNames returns the entry names of a template map, sorted.
func sortedTemplateNames(tmplMap map[string]domain.TemplateEntry) []string {
	names := make([]string, 0, len(tmplMap))
	for k := range tmplMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// scanValuesCandidates lists *.toml / *.json files in the configured values
// directory (default "values"), returning parallel slices of full paths and
// display base names.
func scanValuesCandidates(gc *domain.GlobalConfig) (paths, names []string) {
	dir := "values"
	if gc != nil && gc.ValuesDir != "" {
		dir = gc.ValuesDir
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".toml") && !strings.HasSuffix(n, ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, n))
		names = append(names, n)
	}
	return paths, names
}
