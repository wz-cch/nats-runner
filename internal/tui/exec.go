package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"nats-runner/internal/domain"
	"nats-runner/internal/logger"
	natsclient "nats-runner/internal/nats"
	"nats-runner/internal/vars"
)

// execState holds the execution-monitor state.
type execState struct {
	running  bool
	stopping bool
	done     bool
	results  []ExecResult
	logPath  string
	cancel   context.CancelFunc
	cmd      string // equivalent CLI snapshot
}

// execEvent is one item streamed from the background execution goroutine.
type execEvent struct {
	result  ExecResult
	done    bool
	logPath string
}

// execMsg delivers an execEvent to the model and carries the channel for chaining.
type execMsg struct {
	result  ExecResult
	done    bool
	logPath string
	ch      <-chan execEvent
}

// startExec validates the form and switches to the execution monitor.
func (m Model) startExec() (tea.Model, tea.Cmd) {
	if m.connCfg == nil || m.connName == "" {
		m.status = "select a connection first"
		return m, nil
	}
	if m.entryName == "" {
		m.status = "select an entry first"
		return m, nil
	}
	entry, ok := m.tmplMap[m.entryName]
	if !ok {
		m.status = "entry not found"
		return m, nil
	}
	if missing := m.missingVars(); len(missing) > 0 {
		m.status = "fill required variables: " + strings.Join(missing, ", ")
		return m, nil
	}

	cli := map[string]string{}
	for k, v := range m.inputs {
		if v != "" {
			cli[k] = v
		}
	}
	rctx := vars.ResolveContext{
		CLIParams:  cli,
		MergedVals: m.mergedVals,
		Defaults:   entry.Defaults,
		Functions:  m.funcs,
	}

	count := 1
	var interval time.Duration
	if m.loopEnabled {
		count = parseCount(m.countStr)
		if d, err := time.ParseDuration(m.intervalStr); err == nil {
			interval = d
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.mode = modeExec
	m.exec = execState{running: true, cancel: cancel, cmd: buildCLI(m)}

	connCfg := *m.connCfg
	return m, startExecCmd(ctx, connCfg, entry, rctx, count, interval, connCfg.TimeoutMs)
}

// startExecCmd launches the background goroutine and returns the first event.
func startExecCmd(ctx context.Context, connCfg domain.ConnectionConfig, entry domain.TemplateEntry,
	rctx vars.ResolveContext, count int, interval time.Duration, timeoutMs int) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan execEvent, 8)
		go runExec(ctx, ch, connCfg, entry, rctx, count, interval, timeoutMs)
		ev := <-ch
		return execMsg{result: ev.result, done: ev.done, logPath: ev.logPath, ch: ch}
	}
}

// listenExecCmd reads the next event from the channel.
func listenExecCmd(ch <-chan execEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return execMsg{done: true}
		}
		return execMsg{result: ev.result, done: ev.done, logPath: ev.logPath, ch: ch}
	}
}

// runExec performs the (possibly looping) execution, streaming events on ch.
func runExec(ctx context.Context, ch chan<- execEvent, connCfg domain.ConnectionConfig,
	entry domain.TemplateEntry, rctx vars.ResolveContext, count int, interval time.Duration, timeoutMs int) {
	defer close(ch)

	nc, err := natsclient.Connect(&domain.AppConfig{Connection: connCfg})
	if err != nil {
		ch <- execEvent{result: ExecResult{Status: "ERR: " + err.Error()}, done: true}
		return
	}
	defer nc.Close()

	log, _ := logger.New()
	logPath := ""
	if log != nil {
		logPath = log.Path()
		defer log.Close()
	}

	for i := 1; count == 0 || i <= count; i++ {
		select {
		case <-ctx.Done():
			ch <- execEvent{done: true, logPath: logPath}
			return
		default:
		}

		payload, rerr := vars.Resolve(entry.Body, rctx)
		start := time.Now()
		var reply string
		var execErr error
		if rerr != nil {
			execErr = rerr
		} else {
			reply, execErr = natsclient.Exec(nc, &entry, []byte(payload), timeoutMs)
		}
		elapsed := time.Since(start).Seconds() * 1000

		status := "OK"
		if execErr != nil {
			status = "ERR: " + execErr.Error()
		}
		if log != nil {
			log.WriteEntry(logger.Entry{
				Timestamp:   start,
				Action:      entry.Mode,
				Subject:     entry.Subject,
				RequestBody: payload,
				Reply:       reply,
				DurationMs:  elapsed,
				Error:       execErr,
			})
		}

		short := strings.ReplaceAll(reply, "\n", " ")
		if len(short) > 80 {
			short = short[:80] + "…"
		}
		isLast := count > 0 && i >= count
		ch <- execEvent{
			result:  ExecResult{Iteration: i, DurationMs: elapsed, Status: status, Reply: short},
			done:    isLast,
			logPath: logPath,
		}
		if isLast {
			return
		}
		if interval > 0 {
			select {
			case <-ctx.Done():
				ch <- execEvent{done: true, logPath: logPath}
				return
			case <-time.After(interval):
			}
		}
	}
}

// updateExec handles streamed execution events.
func (m Model) updateExec(msg execMsg) (tea.Model, tea.Cmd) {
	if msg.logPath != "" {
		m.exec.logPath = msg.logPath
	}
	if !m.exec.done && msg.result.Status != "" {
		m.exec.results = append(m.exec.results, msg.result)
	}
	if msg.done {
		m.exec.running = false
		m.exec.stopping = false
		m.exec.done = true
		return m, nil
	}
	return m, listenExecCmd(msg.ch)
}

// handleExecKey handles keys while the execution monitor is active.
func (m Model) handleExecKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.exec.done {
			return m, tea.Quit
		}
		// T4 fix: actually cancel the running goroutine.
		if m.exec.cancel != nil {
			m.exec.cancel()
		}
		m.exec.stopping = true
		return m, nil
	case "esc":
		if m.exec.done {
			m.mode = modeForm
			m.exec = execState{}
			return m, nil
		}
	}
	return m, nil
}

// viewExec renders the execution monitor.
func (m Model) viewExec() string {
	var b strings.Builder
	b.WriteString(Styles.Title.Render("Execution Monitor") + "\n")
	if m.exec.cmd != "" {
		b.WriteString(Styles.CLIBox.Render("$ "+m.exec.cmd) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %-7s  %-10s  %s\n",
		Styles.Header.Render("#"),
		Styles.Header.Render("time(ms)"),
		Styles.Header.Render("status / reply"),
	))
	b.WriteString("  " + strings.Repeat("─", 56) + "\n")

	start := 0
	if len(m.exec.results) > 20 {
		start = len(m.exec.results) - 20
	}
	for _, r := range m.exec.results[start:] {
		statusStyle := Styles.OK
		text := r.Status
		if !strings.HasPrefix(r.Status, "OK") {
			statusStyle = Styles.Err
		} else if r.Reply != "" {
			text = "OK  " + r.Reply
		}
		b.WriteString(fmt.Sprintf("  %-7d  %10.2f  %s\n", r.Iteration, r.DurationMs, statusStyle.Render(text)))
	}

	b.WriteString("\n")
	switch {
	case m.exec.stopping:
		b.WriteString(Styles.Hint.Render("stopping…") + "\n")
	case m.exec.running:
		b.WriteString(Styles.Hint.Render("running…  (Ctrl+C to stop)") + "\n")
	case m.exec.done:
		b.WriteString(Styles.OK.Render("✓ done") + "\n")
		if m.exec.logPath != "" {
			b.WriteString(Styles.Hint.Render("Log: "+m.exec.logPath) + "\n")
		}
		b.WriteString(Styles.Hint.Render("Esc back to form · Ctrl+C quit") + "\n")
	}
	return b.String()
}
