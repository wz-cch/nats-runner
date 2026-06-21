package cli

import (
	"flag"
	"fmt"
	"strings"

	"nats-runner/internal/config"
	"nats-runner/internal/domain"
)

// handleConfigCmd dispatches config subcommands:
//
//	config set <name>
//	config set --template-dir <path>
//	config set --funcs-dir <path>
//	config set --values-dir <path>
//	config show
//	config list
func handleConfigCmd(args []string) {
	if len(args) == 0 {
		die("config requires a subcommand: set | show | list")
	}
	switch args[0] {
	case "set":
		handleConfigSet(args[1:])
	case "show":
		handleConfigShow()
	case "list":
		handleConfigList()
	default:
		die(fmt.Sprintf("unknown config subcommand %q; use: set | show | list", args[0]))
	}
}

func handleConfigSet(args []string) {
	fs := flag.NewFlagSet("config set", flag.ExitOnError)
	templateDir := fs.String("template-dir", "", "set template directory")
	funcsDir := fs.String("funcs-dir", "", "set funcs directory")
	valuesDir := fs.String("values-dir", "", "set values directory")
	fs.Parse(args) //nolint:errcheck // ExitOnError handles errors

	// Positional arg = connection name
	if name := fs.Arg(0); name != "" {
		path := config.ResolveConnPath(name)
		if _, err := config.LoadConnectionFile(path); err != nil {
			die(fmt.Sprintf("connection %q: %v", name, err))
		}
		dieIfErr(config.SaveGlobalConfig(&domain.GlobalConfig{DefaultConnection: name}))
		fmt.Printf("Default connection set to: %s\n", name)
		return
	}

	// Directory flags
	if *templateDir == "" && *funcsDir == "" && *valuesDir == "" {
		die("config set requires a connection name or at least one --*-dir flag")
	}
	gc := &domain.GlobalConfig{
		TemplateDir: *templateDir,
		FuncsDir:    *funcsDir,
		ValuesDir:   *valuesDir,
	}
	dieIfErr(config.SaveGlobalConfig(gc))
	var parts []string
	if *templateDir != "" {
		parts = append(parts, "template_dir="+*templateDir)
	}
	if *funcsDir != "" {
		parts = append(parts, "funcs_dir="+*funcsDir)
	}
	if *valuesDir != "" {
		parts = append(parts, "values_dir="+*valuesDir)
	}
	fmt.Printf("Updated: %s\n", strings.Join(parts, "  "))
}

func handleConfigShow() {
	gc, err := config.LoadGlobalConfig()
	dieIfErr(err)

	fmt.Printf("Connection:\t%s\n", orUnset(gc.DefaultConnection))
	if gc.DefaultConnection != "" {
		path := config.ResolveConnPath(gc.DefaultConnection)
		if cfg, err := config.LoadConnectionFile(path); err == nil {
			fmt.Printf("NATS URL:\t%s\n", cfg.URL)
			fmt.Printf("Auth mode:\t%s\n", cfg.AuthMode)
		}
	}
	fmt.Printf("Template dir:\t%s\n", orUnset(gc.TemplateDir))
	fmt.Printf("Funcs dir:\t%s\n", orUnset(gc.FuncsDir))
	fmt.Printf("Values dir:\t%s\n", orUnset(gc.ValuesDir))

	if gc.FuncsDir != "" {
		funcs, err := config.ScanFunctions(gc.FuncsDir)
		if err == nil && len(funcs) > 0 {
			var names []string
			for k, v := range funcs {
				names = append(names, fmt.Sprintf("%s（%s）", k, v.Command))
			}
			fmt.Printf("Functions:\t%s\n", strings.Join(names, ", "))
		}
	}
}

func handleConfigList() {
	conns, err := config.ListConnections("configs")
	dieIfErr(err)
	if len(conns) == 0 {
		fmt.Println("No connections found (configs/ is empty)")
		return
	}
	fmt.Println("Connections:")
	for _, c := range conns {
		fmt.Printf("  %-14s → %-28s（%s）\n", c.Name, c.Path, c.URL)
	}
}

func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}
