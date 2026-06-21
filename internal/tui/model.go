// Package tui implements the interactive Terminal UI for nats-runner.
//
// The UI is a two-pane form: the left pane is an editable list of fields
// (connection, template, entry, the variables the entry needs, loop options),
// and the right pane shows a live preview of the rendered JSON payload. A
// bottom line shows the equivalent CLI command. Pressing Enter on "▶ Run"
// (or Ctrl+R anywhere) switches to the execution monitor.
package tui

import (
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"nats-runner/internal/config"
	"nats-runner/internal/domain"
	"nats-runner/internal/template"
	"nats-runner/internal/vars"
)

// uiMode selects the top-level view.
type uiMode int

const (
	modeForm uiMode = iota // two-pane editor
	modeExec               // execution monitor
)

// rowKind identifies a form row's behaviour.
type rowKind int

const (
	kindConn rowKind = iota
	kindTemplate
	kindEntry
	kindVar
	kindLoop
	kindCount
	kindInterval
	kindValues
	kindRun
)

// formRow is one focusable line in the left pane.
type formRow struct {
	kind     rowKind
	key      string // variable name (kindVar)
	source   string // resolved source label for kindVar: cli/values/defaults/func or "" (needs input)
	editable bool
}

// ExecResult holds the outcome of one loop iteration displayed in the monitor.
type ExecResult struct {
	Iteration  int
	DurationMs float64
	Status     string // "OK" or "ERR: ..."
	Reply      string // short reply preview
}

// Model is the top-level bubbletea model.
type Model struct {
	version string
	mode    uiMode

	// data sources
	gc        *domain.GlobalConfig
	conns     []config.ConnectionInfo
	tmplFiles []template.TemplateFileInfo
	funcs     map[string]domain.FuncConfig

	// current selection
	connName   string
	connCfg    *domain.ConnectionConfig
	tmplPath   string
	tmplMap    map[string]domain.TemplateEntry
	entryNames []string
	entryName  string

	// user-entered variable values (var name → value)
	inputs map[string]string

	// loop options
	loopEnabled bool
	countStr    string // numeric buffer; 0 = infinite
	intervalStr string // duration string e.g. "1s"
	valuesFiles []string
	mergedVals  map[string]any // cached parse of valuesFiles (for preview)

	// form navigation
	rows   []formRow
	cursor int

	// picker overlay
	pickerActive bool
	pk           picker

	// execution monitor
	exec execState

	width, height int
	status        string // transient banner (errors / hints)
}

// ── messages ────────────────────────────────────────────────────────────────

type loadDataMsg struct {
	conns []config.ConnectionInfo
	files []template.TemplateFileInfo
	err   error
}

// ── lifecycle ───────────────────────────────────────────────────────────────

// Run initialises and starts the TUI, returning when the user quits.
func Run(version string) error {
	gc, _ := config.LoadGlobalConfig()
	if gc == nil {
		gc = &domain.GlobalConfig{}
	}
	// T1 fix: actually load shell functions so they appear and execute in the TUI.
	funcs, _ := config.ScanFunctions(gc.FuncsDir)

	m := Model{
		version:     version,
		mode:        modeForm,
		gc:          gc,
		funcs:       funcs,
		inputs:      map[string]string{},
		mergedVals:  map[string]any{},
		countStr:    "1",
		intervalStr: "1s",
	}
	// Pre-select the saved default connection, if any.
	if gc.DefaultConnection != "" {
		if cfg, err := config.LoadConnectionFile(config.ResolveConnPath(gc.DefaultConnection)); err == nil {
			m.connName = gc.DefaultConnection
			m.connCfg = cfg
		}
	}
	m.rows = buildRows(m)

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadDataCmd(m.gc), tea.WindowSize())
}

func loadDataCmd(gc *domain.GlobalConfig) tea.Cmd {
	return func() tea.Msg {
		conns, _ := config.ListConnections("configs")
		dir := "templates"
		if gc != nil && gc.TemplateDir != "" {
			dir = gc.TemplateDir
		}
		files, err := template.ScanTemplates(dir)
		return loadDataMsg{conns: conns, files: files, err: err}
	}
}

// ── bubbletea interface ───────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadDataMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		m.conns = msg.conns
		m.tmplFiles = msg.files
		return m, nil

	case execMsg:
		return m.updateExec(msg)

	case tea.KeyMsg:
		if m.mode == modeExec {
			return m.handleExecKey(msg)
		}
		if m.pickerActive {
			return m.handlePickerKey(msg)
		}
		return m.handleFormKey(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.mode == modeExec {
		return m.viewExec()
	}
	if m.pickerActive {
		return m.viewPicker()
	}
	return m.viewForm()
}

// ── form key handling ─────────────────────────────────────────────────────────

func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+r":
		return m.startExec()
	case "up":
		m.cursor = clamp(m.cursor-1, 0, len(m.rows)-1)
		return m, nil
	case "down", "tab":
		m.cursor = clamp(m.cursor+1, 0, len(m.rows)-1)
		return m, nil
	case "shift+tab":
		m.cursor = clamp(m.cursor-1, 0, len(m.rows)-1)
		return m, nil
	case "left":
		m.cycleSelector(-1)
		return m, nil
	case "right":
		m.cycleSelector(1)
		return m, nil
	case "enter":
		return m.activateRow()
	case "backspace":
		m.editCurrent("", true)
		return m, nil
	}
	if s, ok := typedText(msg); ok {
		m.editCurrent(s, false)
	}
	return m, nil
}

// activateRow handles Enter on the focused row.
func (m Model) activateRow() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) {
		return m, nil
	}
	switch m.rows[m.cursor].kind {
	case kindConn:
		m.openConnPicker()
	case kindTemplate:
		m.openTemplatePicker()
	case kindEntry:
		if m.tmplPath == "" {
			m.status = "select a template first"
			break
		}
		m.openEntryPicker()
	case kindValues:
		m.openValuesPicker()
	case kindLoop:
		m.loopEnabled = !m.loopEnabled
		m.rows = buildRows(m)
		m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
	case kindRun:
		return m.startExec()
	default:
		// editable rows: Enter just advances
		m.cursor = clamp(m.cursor+1, 0, len(m.rows)-1)
	}
	return m, nil
}

// editCurrent edits the focused editable row. When del is true it removes the
// last rune; otherwise it appends s (digits only for the count field).
func (m *Model) editCurrent(s string, del bool) {
	if m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor]
	switch row.kind {
	case kindVar:
		cur := m.inputs[row.key]
		m.inputs[row.key] = applyEdit(cur, s, del)
		m.rows = buildRows(*m) // refresh source labels
	case kindCount:
		if del {
			m.countStr = trimLastRune(m.countStr)
		} else if isDigits(s) {
			m.countStr += s
		}
	case kindInterval:
		if del {
			m.intervalStr = trimLastRune(m.intervalStr)
		} else {
			m.intervalStr += s
		}
	}
}

// cycleSelector changes the connection/template/entry on the focused row
// in-place (left/right arrows), so the common case needs no picker step.
func (m *Model) cycleSelector(delta int) {
	if m.cursor >= len(m.rows) {
		return
	}
	switch m.rows[m.cursor].kind {
	case kindConn:
		if len(m.conns) == 0 {
			return
		}
		i := wrapIndex(indexOfConn(m.conns, m.connName)+delta, len(m.conns))
		m.selectConn(m.conns[i].Name)
	case kindTemplate:
		if len(m.tmplFiles) == 0 {
			return
		}
		i := wrapIndex(indexOfTmpl(m.tmplFiles, m.tmplPath)+delta, len(m.tmplFiles))
		m.selectTemplate(m.tmplFiles[i].Path)
	case kindEntry:
		if len(m.entryNames) == 0 {
			return
		}
		i := wrapIndex(indexOfStr(m.entryNames, m.entryName)+delta, len(m.entryNames))
		m.entryName = m.entryNames[i]
	default:
		return
	}
	m.rows = buildRows(*m)
	m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
}

// ── row construction ──────────────────────────────────────────────────────────

// buildRows assembles the ordered list of form rows from the current model.
func buildRows(m Model) []formRow {
	rows := []formRow{
		{kind: kindConn},
		{kind: kindTemplate},
		{kind: kindEntry},
	}
	if entry, ok := m.tmplMap[m.entryName]; ok && m.entryName != "" {
		names, _ := vars.ReferencedVars(entry.Body)
		for _, k := range names {
			rows = append(rows, formRow{
				kind:     kindVar,
				key:      k,
				source:   classifySource(m, entry, k),
				editable: true,
			})
		}
	}
	rows = append(rows, formRow{kind: kindLoop})
	if m.loopEnabled {
		rows = append(rows,
			formRow{kind: kindCount, editable: true},
			formRow{kind: kindInterval, editable: true},
		)
	}
	rows = append(rows, formRow{kind: kindValues})
	rows = append(rows, formRow{kind: kindRun})
	return rows
}

// classifySource reports where variable k's value currently comes from.
// Empty string means the variable is unsatisfied and needs user input.
func classifySource(m Model, entry domain.TemplateEntry, k string) string {
	if v, ok := m.inputs[k]; ok && v != "" {
		return "cli"
	}
	if _, ok := m.mergedVals[k]; ok {
		return "values"
	}
	if _, ok := entry.Defaults[k]; ok {
		return "defaults"
	}
	if _, ok := m.funcs[k]; ok {
		return "func"
	}
	return ""
}

// missingVars returns referenced variables that are unsatisfied (need input).
func (m Model) missingVars() []string {
	entry, ok := m.tmplMap[m.entryName]
	if !ok {
		return nil
	}
	var missing []string
	for _, r := range m.rows {
		if r.kind == kindVar && classifySource(m, entry, r.key) == "" {
			missing = append(missing, r.key)
		}
	}
	return missing
}

// ── equivalent CLI ─────────────────────────────────────────────────────────────

// buildCLI renders the equivalent command line for the current form state.
func buildCLI(m Model) string {
	if m.connName == "" || m.tmplPath == "" || m.entryName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("nats-runner -c " + m.connName + " -t " + m.tmplPath + " -n " + m.entryName)
	for _, k := range sortedKeys(m.inputs) {
		if v := m.inputs[k]; v != "" {
			b.WriteString(" " + k + "=" + shellQuote(v))
		}
	}
	for _, vf := range m.valuesFiles {
		b.WriteString(" --values " + vf)
	}
	if m.loopEnabled {
		b.WriteString(" --loop " + strconv.Itoa(parseCount(m.countStr)) + " --interval " + m.intervalStr)
	}
	return b.String()
}

// ── small helpers ───────────────────────────────────────────────────────────

// wrapIndex wraps i into [0, n) with proper handling of negatives.
func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

func indexOfStr(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

func indexOfConn(list []config.ConnectionInfo, name string) int {
	for i, c := range list {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func indexOfTmpl(list []template.TemplateFileInfo, path string) int {
	for i, f := range list {
		if f.Path == path {
			return i
		}
	}
	return -1
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// typedText extracts literal text from a key message (runes or space).
// Using msg.Runes avoids the "runes:" prefix that msg.String() adds for pastes.
func typedText(msg tea.KeyMsg) (string, bool) {
	switch msg.Type {
	case tea.KeyRunes:
		return string(msg.Runes), true
	case tea.KeySpace:
		return " ", true
	default:
		return "", false
	}
}

func applyEdit(cur, s string, del bool) string {
	if del {
		return trimLastRune(cur)
	}
	return cur + s
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseCount parses the count buffer; invalid/empty falls back to 1.
func parseCount(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 1
	}
	return n
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func shellQuote(s string) string {
	if strings.ContainsAny(s, " \t\"'") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
