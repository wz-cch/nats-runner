// Package cli wires together all layers and implements the CLI entry point.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"nats-runner/internal/config"
	natsclient "nats-runner/internal/nats"
	"nats-runner/internal/template"
	"nats-runner/internal/vars"
)

// Execute is the single entry point called from main.
// args should be os.Args (program name at index 0).
func Execute(args []string, version string) {
	// The "config" subcommand must be intercepted before flag.Parse()
	// because flag rejects non-flag positional arguments like "set".
	if len(args) >= 2 && args[1] == "config" {
		handleConfigCmd(args[2:])
		return
	}

	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	configPath := fs.String("c", "", "path to config.toml (optional; uses global default if not set)")
	templatePath := fs.String("t", "", "path to template TOML file")
	templateName := fs.String("n", "", "template name within the file")
	showVersion := fs.Bool("version", false, "show version and exit")
	fs.Parse(args[1:]) //nolint:errcheck // ExitOnError handles errors

	if *showVersion {
		fmt.Println("nats-runner version", version)
		return
	}
	if *templatePath == "" {
		die("-t (template path) is required")
	}
	if *templateName == "" {
		die("-n (template name) is required")
	}

	cliParams := parseParams(fs.Args())

	cfgPath, err := config.ResolveConfigPath(*configPath)
	dieIfErr(err)

	cfg, err := config.LoadAppConfig(cfgPath)
	dieIfErr(err)

	tmpl, err := template.Load(*templatePath)
	dieIfErr(err)

	entry, err := template.GetEntry(tmpl, *templateName, *templatePath)
	dieIfErr(err)

	payload, err := vars.Resolve(entry.Body, cliParams, entry.Defaults, cfg.Functions)
	dieIfErr(err)

	nc, err := natsclient.Connect(cfg)
	dieIfErr(err)
	defer nc.Close()

	switch entry.Mode {
	case "req":
		dieIfErr(natsclient.ExecReq(nc, entry.Subject, []byte(payload), cfg.Connection.TimeoutMs))
	case "pub":
		dieIfErr(natsclient.ExecPub(nc, entry.Subject, []byte(payload)))
	case "js":
		dieIfErr(natsclient.ExecJS(nc, entry, []byte(payload)))
	default:
		die(fmt.Sprintf("unknown mode %q in template %q", entry.Mode, *templateName))
	}
}

// parseParams converts "key=value" CLI arguments into a map.
// Splits on the first '=' so values may themselves contain '='.
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
