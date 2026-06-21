// Package cli wires together all layers and implements the CLI entry point.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"nats-runner/internal/config"
	"nats-runner/internal/domain"
	"nats-runner/internal/logger"
	natsclient "nats-runner/internal/nats"
	"nats-runner/internal/template"
	"nats-runner/internal/tui"
	"nats-runner/internal/vars"
)

// multiStringFlag implements flag.Value for a repeatable string flag (--values x --values y).
type multiStringFlag []string

func (f *multiStringFlag) String() string     { return strings.Join(*f, ",") }
func (f *multiStringFlag) Set(v string) error { *f = append(*f, v); return nil }

// Execute is the single entry point called from main.
// args should be os.Args (program name at index 0).
func Execute(args []string, version string) {
	if len(args) >= 2 && args[1] == "config" {
		handleConfigCmd(args[2:])
		return
	}
	// Explicit interactive subcommand. The TUI is never auto-launched: bare
	// `nats-runner` prints usage so the tool stays script/automation friendly.
	if len(args) >= 2 && args[1] == "tui" {
		dieIfErr(tui.Run(version))
		return
	}

	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	configPath := fs.String("c", "", "connection name or path to a configs/*.toml file")
	templatePath := fs.String("t", "", "path to template TOML file")
	templateName := fs.String("n", "", "template entry name")
	showVersion := fs.Bool("version", false, "show version and exit")
	loopCount := fs.Int("loop", 1, "number of executions (0 = infinite)")
	loopInterval := fs.String("interval", "0", "interval between loop iterations (e.g. 60s, 500ms)")
	var interactive bool
	fs.BoolVar(&interactive, "i", false, "launch the interactive TUI")
	fs.BoolVar(&interactive, "interactive", false, "launch the interactive TUI")
	var valuesFiles multiStringFlag
	fs.Var(&valuesFiles, "values", "values file path (repeatable, later files take priority)")
	fs.Usage = func() { printUsage(fs) }
	fs.Parse(args[1:]) //nolint:errcheck // ExitOnError handles errors

	if *showVersion {
		fmt.Println("nats-runner version", version)
		return
	}

	if interactive {
		dieIfErr(tui.Run(version))
		return
	}

	// No template/entry and not interactive → show usage and exit (do NOT
	// auto-launch the TUI, which would hang or fail under non-interactive use).
	if *templatePath == "" && *templateName == "" {
		printUsage(fs)
		os.Exit(2)
	}
	if *templatePath == "" {
		die("-t (template path) is required")
	}
	if *templateName == "" {
		die("-n (template name) is required")
	}

	cliParams := parseParams(fs.Args())

	gc, err := config.LoadGlobalConfig()
	dieIfErr(err)

	connCfg, err := config.ResolveConnection(*configPath, gc)
	dieIfErr(err)

	if connCfg.TLS.InsecureSkipVerify {
		fmt.Fprintln(os.Stderr, "Warning: TLS certificate verification is disabled (insecure_skip_verify=true)")
	}

	funcs, err := config.ScanFunctions(gc.FuncsDir)
	dieIfErr(err)

	tmpl, err := template.Load(*templatePath)
	dieIfErr(err)

	entry, err := template.GetEntry(tmpl, *templateName, *templatePath)
	dieIfErr(err)

	mergedVals, err := vars.LoadValuesFiles([]string(valuesFiles))
	dieIfErr(err)

	resolveCtx := vars.ResolveContext{
		CLIParams:  cliParams,
		MergedVals: mergedVals,
		Defaults:   entry.Defaults,
		Functions:  funcs,
	}

	nc, err := natsclient.Connect(connCfg)
	dieIfErr(err)
	defer nc.Close()

	interval, err := parseInterval(*loopInterval)
	dieIfErr(err)

	log, err := logger.New()
	dieIfErr(err)
	defer log.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	execLoop(ctx, nc, entry, resolveCtx, connCfg.TimeoutMs, *loopCount, interval, log, func(r LoopResult) {
		status := "OK"
		if r.Err != nil {
			status = fmt.Sprintf("ERR: %v", r.Err)
		}
		fmt.Printf("[%d] %.2fms %s\n", r.Iteration, r.DurationMs, status)
		if r.Err == nil && r.Reply != "" {
			fmt.Println(r.Reply)
		}
	})

	fmt.Println("Log:", log.Path())
}

// LoopResult carries the outcome of a single loop iteration.
type LoopResult struct {
	Iteration  int
	DurationMs float64
	Reply      string
	Err        error
}

// execLoop executes the NATS call count times (0 = infinite) at the given interval.
// It calls onResult after each iteration and writes to log.
func execLoop(
	ctx context.Context,
	nc *natsgo.Conn,
	entry *domain.TemplateEntry,
	resolveCtx vars.ResolveContext,
	timeoutMs int,
	count int,
	interval time.Duration,
	log *logger.Logger,
	onResult func(LoopResult),
) {
	for i := 1; count == 0 || i <= count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		payload, values, err := vars.ResolveWithValues(entry.Body, resolveCtx)
		start := time.Now()
		var reply string

		if err == nil {
			reply, err = natsclient.Exec(nc, entry, []byte(payload), timeoutMs)
		}
		elapsed := time.Since(start).Seconds() * 1000

		log.WriteEntry(logger.Entry{
			Timestamp:   start,
			Action:      entry.Mode,
			Subject:     entry.Subject,
			Values:      values,
			RequestBody: payload,
			Reply:       reply,
			DurationMs:  elapsed,
			Error:       err,
		})

		r := LoopResult{Iteration: i, DurationMs: elapsed, Reply: reply, Err: err}
		onResult(r)

		if i == count {
			break
		}
		if interval > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}
}

// parseInterval parses a duration string; "0" or "" means no interval.
func parseInterval(s string) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --interval %q: %w", s, err)
	}
	return d, nil
}

// printUsage writes a concise usage summary to the flag set's output (stderr).
func printUsage(fs *flag.FlagSet) {
	w := fs.Output()
	fmt.Fprintln(w, "nats-runner — send NATS messages from TOML templates")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  nats-runner -t <template> -n <entry> [key=value ...]   run a request")
	fmt.Fprintln(w, "  nats-runner tui                                        launch interactive TUI")
	fmt.Fprintln(w, "  nats-runner -i                                         launch interactive TUI")
	fmt.Fprintln(w, "  nats-runner config set|show|list                       manage connections / config")
	fmt.Fprintln(w, "  nats-runner --version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fs.PrintDefaults()
}

// parseParams converts "key=value" CLI arguments into a map.
func parseParams(args []string) map[string]string {
	params := make(map[string]string)
	for _, arg := range args {
		idx := strings.Index(arg, "=")
		if idx < 0 {
			die(fmt.Sprintf("invalid parameter %q: expected key=value format", arg))
		}
		params[arg[:idx]] = arg[idx+1:]
	}
	return params
}

// dieIfErr prints the error to stderr and exits with code 1 if err != nil.
func dieIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// die prints a message to stderr and exits with code 1.
func die(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}
