package cli

import (
	"fmt"
	"path/filepath"

	"nats-runner/internal/config"
)

// handleConfigCmd processes the "config set <path>" and "config show" subcommands.
func handleConfigCmd(args []string) {
	if len(args) == 0 {
		die("config requires a subcommand: set <path> | show")
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			die("config set requires a path argument")
		}
		abs, err := filepath.Abs(args[1])
		dieIfErr(err)
		dieIfErr(config.SaveGlobalConfig(abs))
		fmt.Printf("Default config set to: %s\n", abs)

	case "show":
		gc, err := config.LoadGlobalConfig()
		dieIfErr(err)
		fmt.Printf("Default config path: %s\n", gc.DefaultConfigPath)
		cfg, err := config.LoadAppConfig(gc.DefaultConfigPath)
		dieIfErr(err)
		fmt.Printf("URL:      %s\n", cfg.Connection.URL)
		fmt.Printf("AuthMode: %s\n", cfg.Connection.AuthMode)

	default:
		die(fmt.Sprintf("unknown config subcommand %q. Use: set <path> | show", args[0]))
	}
}
