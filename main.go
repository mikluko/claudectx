package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"syscall"
)

// version is set via -ldflags "-X main.version=..." by release builds;
// go-install builds fall back to module build info.
var version string

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

const usage = `claudectx — launch Claude Code with provider contexts

Usage:
  claudectx [claude args...]                 run the default context
  claudectx --context [NAME] [claude args..] run the named context
  claudectx --set-default [NAME]             select the default context

When NAME is omitted (and when running without a default context set),
an interactive fzf selector is presented on TTYs.

The reserved context name "none" runs claude with the environment
untouched, as if claudectx were not involved.

Manifest: ~/.config/claudectx/config.yaml (override with CLAUDECTX_CONFIG)
`

type cliArgs struct {
	help           bool
	version        bool
	setDefault     bool
	setDefaultName string
	context        bool
	contextName    string
	passthrough    []string
}

func parseArgs(args []string) (cliArgs, error) {
	var c cliArgs
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			c.help = true
			return c, nil
		// Only leading --version is claudectx's own; after --context or
		// other flags it is passthrough for claude.
		case i == 0 && (a == "-v" || a == "--version"):
			c.version = true
			return c, nil
		case a == "--set-default":
			c.setDefault = true
			if v, ok := flagValue(args, i); ok {
				c.setDefaultName = v
				i++
			}
			i++
		case strings.HasPrefix(a, "--set-default="):
			c.setDefault = true
			c.setDefaultName = strings.TrimPrefix(a, "--set-default=")
			i++
		case a == "--context":
			c.context = true
			if v, ok := flagValue(args, i); ok {
				c.contextName = v
				i++
			}
			i++
		case strings.HasPrefix(a, "--context="):
			c.context = true
			c.contextName = strings.TrimPrefix(a, "--context=")
			i++
		default:
			// Everything from the first unrecognized argument on is
			// passed to claude verbatim.
			c.passthrough = args[i:]
			return c, nil
		}
	}
	return c, nil
}

// flagValue returns the argument following args[i] when it looks like a
// value rather than another flag. Context names never start with "-".
func flagValue(args []string, i int) (string, bool) {
	if i+1 >= len(args) {
		return "", false
	}
	v := args[i+1]
	if v == "" || strings.HasPrefix(v, "-") {
		return "", false
	}
	return v, true
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claudectx:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	c, err := parseArgs(args)
	if err != nil {
		return err
	}
	if c.help {
		fmt.Print(usage)
		return nil
	}
	if c.version {
		fmt.Printf("%s (claudectx)\n", versionString())
		if claude, err := exec.LookPath("claude"); err == nil {
			out, err := exec.Command(claude, "--version").Output()
			if err == nil {
				fmt.Print(string(out))
			}
		}
		return nil
	}
	if c.setDefault && c.context {
		return fmt.Errorf("--set-default and --context are mutually exclusive")
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	if c.setDefault {
		if len(c.passthrough) > 0 {
			return fmt.Errorf("unexpected arguments after --set-default: %s", strings.Join(c.passthrough, " "))
		}
		name := c.setDefaultName
		if name == "" {
			if name, err = pickContext(cfg); err != nil {
				return err
			}
		}
		if err := setDefault(path, name); err != nil {
			return err
		}
		fmt.Printf("default context set to %q\n", name)
		return nil
	}

	name := c.contextName
	if name == "" && !c.context {
		name = cfg.CurrentContext
	}
	if name == "" {
		if name, err = pickContext(cfg); err != nil {
			if c.context {
				return err
			}
			return fmt.Errorf("no default context: %w (or select one with --set-default)", err)
		}
	}

	env, err := cfg.buildEnv(name, os.Environ())
	if err != nil {
		return err
	}

	claude, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	argv := claudeArgv(claude, cfg.contextModel(name), c.passthrough)
	return syscall.Exec(claude, argv, env)
}

// claudeArgv builds the claude argument vector. The workingset default model
// is also passed as --model: a model pinned in Claude Code settings outranks
// the ANTHROPIC_MODEL environment variable, but the command line outranks
// settings. Skipped when the user supplies their own --model.
func claudeArgv(claude, model string, passthrough []string) []string {
	argv := []string{claude}
	if model != "" && !hasModelFlag(passthrough) {
		argv = append(argv, "--model", model)
	}
	return append(argv, passthrough...)
}

func hasModelFlag(args []string) bool {
	for _, a := range args {
		if a == "--model" || strings.HasPrefix(a, "--model=") {
			return true
		}
	}
	return false
}
