package main

import (
	"slices"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want cliArgs
	}{
		{"bare", nil, cliArgs{}},
		{"passthrough only", []string{"-p", "hi"}, cliArgs{passthrough: []string{"-p", "hi"}}},
		{"set-default with name", []string{"--set-default", "foo"},
			cliArgs{setDefault: true, setDefaultName: "foo"}},
		{"set-default without name", []string{"--set-default"},
			cliArgs{setDefault: true}},
		{"set-default equals", []string{"--set-default=foo"},
			cliArgs{setDefault: true, setDefaultName: "foo"}},
		{"context with name and passthrough", []string{"--context", "foo", "--version"},
			cliArgs{context: true, contextName: "foo", passthrough: []string{"--version"}}},
		{"context without name", []string{"--context"},
			cliArgs{context: true}},
		{"context without name plus passthrough", []string{"--context", "--version"},
			cliArgs{context: true, passthrough: []string{"--version"}}},
		{"context equals", []string{"--context=foo", "-p", "hi"},
			cliArgs{context: true, contextName: "foo", passthrough: []string{"-p", "hi"}}},
		{"help", []string{"--help"}, cliArgs{help: true}},
		{"version", []string{"--version"}, cliArgs{version: true}},
		{"version short", []string{"-v"}, cliArgs{version: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.help != tc.want.help ||
				got.version != tc.want.version ||
				got.setDefault != tc.want.setDefault ||
				got.setDefaultName != tc.want.setDefaultName ||
				got.context != tc.want.context ||
				got.contextName != tc.want.contextName ||
				!slices.Equal(got.passthrough, tc.want.passthrough) {
				t.Errorf("parseArgs(%q):\n got %+v\nwant %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestClaudeArgv(t *testing.T) {
	cases := []struct {
		name        string
		model       string
		passthrough []string
		want        []string
	}{
		{"inject for bare run", "m1", nil,
			[]string{"claude", "--model", "m1"}},
		{"inject before passthrough", "m1", []string{"-p", "hi"},
			[]string{"claude", "--model", "m1", "-p", "hi"}},
		{"user --model wins", "m1", []string{"--model", "m2"},
			[]string{"claude", "--model", "m2"}},
		{"user --model= wins", "m1", []string{"--model=m2"},
			[]string{"claude", "--model=m2"}},
		{"no default slot", "", []string{"-p", "hi"},
			[]string{"claude", "-p", "hi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := claudeArgv("claude", tc.model, tc.passthrough)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestContextModel(t *testing.T) {
	cfg := &Config{
		Contexts: []Context{{Name: "c", Provider: "p", Models: map[string]string{"default": "m1"}}},
	}
	if got := cfg.contextModel("c"); got != "m1" {
		t.Errorf("contextModel(c) = %q", got)
	}
	if got := cfg.contextModel(NoneContext); got != "" {
		t.Errorf("contextModel(none) = %q", got)
	}
	if got := cfg.contextModel("nope"); got != "" {
		t.Errorf("contextModel(nope) = %q", got)
	}
}

func TestContextLines(t *testing.T) {
	cfg := &Config{
		Contexts: []Context{
			{Name: "glm-hf", Provider: "hf", Models: map[string]string{"default": "glm"}},
			{Name: "x", Provider: "or", Models: map[string]string{"default": "kimi"}},
		},
	}
	lines := contextLines(cfg)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if !strings.HasPrefix(lines[0], "glm-hf  ") || !strings.Contains(lines[0], "provider=hf") {
		t.Errorf("line 0: %q", lines[0])
	}
	// Short names are padded so columns align.
	if !strings.HasPrefix(lines[1], "x       ") {
		t.Errorf("line 1 not padded: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "none    ") {
		t.Errorf("last line must be the reserved none context: %q", lines[2])
	}
	// Selection parses back to the bare name.
	name, _, _ := strings.Cut(lines[0], " ")
	if name != "glm-hf" {
		t.Errorf("parsed name %q", name)
	}
}
