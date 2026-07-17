package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const usage = `claudectx — launch Claude Code with provider contexts

Usage:
  claudectx [claude args...]                 run the default context
  claudectx --context NAME [claude args...]  run the named context
  claudectx --set-default NAME               select the default context

The reserved context name "none" runs claude with the environment
untouched, as if claudectx were not involved.

Manifest: ~/.config/claudectx/config.yaml (override with CLAUDECTX_CONFIG)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claudectx:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var setDefaultName, contextName string
	var passthrough []string

	i := 0
loop:
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Print(usage)
			return nil
		case a == "--set-default":
			if i+1 >= len(args) {
				return fmt.Errorf("--set-default requires a context name")
			}
			setDefaultName = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--set-default="):
			setDefaultName = strings.TrimPrefix(a, "--set-default=")
			i++
		case a == "--context":
			if i+1 >= len(args) {
				return fmt.Errorf("--context requires a context name")
			}
			contextName = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--context="):
			contextName = strings.TrimPrefix(a, "--context=")
			i++
		default:
			// Everything from the first unrecognized argument on is
			// passed to claude verbatim.
			passthrough = args[i:]
			break loop
		}
	}

	if setDefaultName != "" && contextName != "" {
		return fmt.Errorf("--set-default and --context are mutually exclusive")
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	if setDefaultName != "" {
		if len(passthrough) > 0 {
			return fmt.Errorf("unexpected arguments after --set-default: %s", strings.Join(passthrough, " "))
		}
		if err := setDefault(path, setDefaultName); err != nil {
			return err
		}
		fmt.Printf("default context set to %q\n", setDefaultName)
		return nil
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	if contextName == "" {
		contextName = cfg.CurrentContext
		if contextName == "" {
			return fmt.Errorf("no default context; select one with --set-default or run with --context")
		}
	}

	env, err := cfg.buildEnv(contextName, os.Environ())
	if err != nil {
		return err
	}

	claude, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	argv := append([]string{claude}, passthrough...)
	return syscall.Exec(claude, argv, env)
}
