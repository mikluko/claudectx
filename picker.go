package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// contextLines renders one picker line per context (plus the reserved
// "none"), name first so the selection can be parsed back out.
func contextLines(cfg *Config) []string {
	width := len(NoneContext)
	for _, x := range cfg.Contexts {
		if len(x.Name) > width {
			width = len(x.Name)
		}
	}
	lines := make([]string, 0, len(cfg.Contexts)+1)
	for _, x := range cfg.Contexts {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, x.Name, contextDetail(x)))
	}
	lines = append(lines, fmt.Sprintf("%-*s  passthrough, environment untouched", width, NoneContext))
	return lines
}

// contextDetail describes a context after its name in the picker. Only
// populated fields appear: a first-party context has no provider or model to
// show, and its config-dir is what distinguishes it from its siblings.
func contextDetail(x Context) string {
	var parts []string
	if x.Provider != "" {
		parts = append(parts, "provider="+x.Provider)
		if m := x.Models["default"]; m != "" {
			parts = append(parts, "model="+m)
		}
	} else {
		parts = append(parts, "subscription")
	}
	if x.ConfigDir != "" {
		parts = append(parts, "config-dir="+x.ConfigDir)
	}
	return strings.Join(parts, " ")
}

// pickContext presents an fzf selector on the controlling terminal and
// returns the chosen context name.
func pickContext(cfg *Config) (string, error) {
	fzf, err := exec.LookPath("fzf")
	if err != nil {
		return "", fmt.Errorf("no context name given and fzf not found in PATH")
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("no context name given and no TTY for interactive selection")
	}
	tty.Close()

	header := "select context"
	if cfg.CurrentContext != "" {
		header = fmt.Sprintf("select context (current default: %s)", cfg.CurrentContext)
	}
	cmd := exec.Command(fzf,
		"--height=40%",
		"--reverse",
		"--prompt=context> ",
		"--header="+header,
	)
	cmd.Stdin = strings.NewReader(strings.Join(contextLines(cfg), "\n") + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("selection cancelled")
	}
	selection := strings.TrimSpace(string(out))
	if selection == "" {
		return "", fmt.Errorf("selection cancelled")
	}
	name, _, _ := strings.Cut(selection, " ")
	return name, nil
}
