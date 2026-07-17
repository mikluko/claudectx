package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// NoneContext is a reserved context name: it runs claude with the
// environment untouched, exactly as if claudectx were not involved.
const NoneContext = "none"

type Config struct {
	CurrentContext string       `yaml:"current-context,omitempty"`
	Providers      []Provider   `yaml:"providers"`
	WorkingSets    []WorkingSet `yaml:"workingsets"`
	Contexts       []Context    `yaml:"contexts"`
}

type Provider struct {
	Name string `yaml:"name"`
	// BaseURL is any Anthropic-compatible endpoint; Claude Code appends
	// /v1/messages to it. E.g. https://router.huggingface.co,
	// https://openrouter.ai/api, or a local proxy.
	BaseURL    string `yaml:"base-url"`
	APIKey     string `yaml:"api-key,omitempty"`
	APIKeyFile string `yaml:"api-key-file,omitempty"`
	// APIKeyOP is a 1Password item reference resolved via the op CLI:
	// "account/vault/item", "vault/item", or "item".
	APIKeyOP string `yaml:"api-key-op,omitempty"`
}

type WorkingSet struct {
	Name   string            `yaml:"name"`
	Models map[string]string `yaml:"models"`
}

type Context struct {
	Name       string `yaml:"name"`
	Provider   string `yaml:"provider"`
	WorkingSet string `yaml:"workingset"`
}

// modelEnvVars maps workingset model slots to the Claude Code environment
// variables that carry them.
var modelEnvVars = map[string][]string{
	"default": {"ANTHROPIC_MODEL"},
	"fable":   {"ANTHROPIC_DEFAULT_FABLE_MODEL"},
	"opus":    {"ANTHROPIC_DEFAULT_OPUS_MODEL"},
	"sonnet":  {"ANTHROPIC_DEFAULT_SONNET_MODEL"},
	"haiku":   {"ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_SMALL_FAST_MODEL"},
}

// modelSlots is the deterministic emission order for modelEnvVars.
var modelSlots = []string{"default", "fable", "opus", "sonnet", "haiku"}

// managedEnvVars is every variable claudectx owns: pre-existing values are
// stripped from the environment before a context is applied so stale
// credentials or model overrides cannot leak through.
var managedEnvVars = []string{
	"ANTHROPIC_BASE_URL",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
	"ANTHROPIC_DEFAULT_FABLE_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
}

func configPath() (string, error) {
	if p := os.Getenv("CLAUDECTX_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claudectx", "config.yaml"), nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	providers := map[string]bool{}
	for _, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("provider with empty name")
		}
		if providers[p.Name] {
			return fmt.Errorf("duplicate provider %q", p.Name)
		}
		providers[p.Name] = true
		if p.BaseURL == "" {
			return fmt.Errorf("provider %q: base-url required", p.Name)
		}
		sources := 0
		for _, s := range []string{p.APIKey, p.APIKeyFile, p.APIKeyOP} {
			if s != "" {
				sources++
			}
		}
		if sources != 1 {
			return fmt.Errorf("provider %q: exactly one of api-key, api-key-file, api-key-op required", p.Name)
		}
	}
	workingsets := map[string]bool{}
	for _, w := range c.WorkingSets {
		if w.Name == "" {
			return fmt.Errorf("workingset with empty name")
		}
		if workingsets[w.Name] {
			return fmt.Errorf("duplicate workingset %q", w.Name)
		}
		workingsets[w.Name] = true
		for slot := range w.Models {
			if _, ok := modelEnvVars[slot]; !ok {
				return fmt.Errorf("workingset %q: unknown model slot %q (valid: %s)", w.Name, slot, strings.Join(modelSlots, ", "))
			}
		}
	}
	contexts := map[string]bool{}
	for _, x := range c.Contexts {
		if x.Name == NoneContext {
			return fmt.Errorf("context name %q is reserved", NoneContext)
		}
		if contexts[x.Name] {
			return fmt.Errorf("duplicate context %q", x.Name)
		}
		contexts[x.Name] = true
		if !providers[x.Provider] {
			return fmt.Errorf("context %q: unknown provider %q", x.Name, x.Provider)
		}
		if !workingsets[x.WorkingSet] {
			return fmt.Errorf("context %q: unknown workingset %q", x.Name, x.WorkingSet)
		}
	}
	// A dangling current-context (e.g. after a context rename) must not
	// fail the load: that would break --set-default too and leave the
	// manifest unfixable from the CLI. Treat it as no default instead.
	if c.CurrentContext != "" && c.CurrentContext != NoneContext && !contexts[c.CurrentContext] {
		fmt.Fprintf(os.Stderr, "claudectx: ignoring current-context %q: no such context\n", c.CurrentContext)
		c.CurrentContext = ""
	}
	return nil
}

func (c *Config) context(name string) (*Context, error) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], nil
		}
	}
	names := make([]string, 0, len(c.Contexts)+1)
	for _, x := range c.Contexts {
		names = append(names, x.Name)
	}
	names = append(names, NoneContext)
	return nil, fmt.Errorf("no context %q (available: %s)", name, strings.Join(names, ", "))
}

func (c *Config) provider(name string) *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i]
		}
	}
	return nil
}

func (c *Config) workingset(name string) *WorkingSet {
	for i := range c.WorkingSets {
		if c.WorkingSets[i].Name == name {
			return &c.WorkingSets[i]
		}
	}
	return nil
}

func (p *Provider) apiKey() (string, error) {
	switch {
	case p.APIKey != "":
		return p.APIKey, nil
	case p.APIKeyOP != "":
		key, err := opAPIKey(p.APIKeyOP)
		if err != nil {
			return "", fmt.Errorf("provider %q: %w", p.Name, err)
		}
		return key, nil
	}
	data, err := os.ReadFile(expandHome(p.APIKeyFile))
	if err != nil {
		return "", fmt.Errorf("provider %q: %w", p.Name, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("provider %q: api-key-file %s is empty", p.Name, p.APIKeyFile)
	}
	return key, nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// buildEnv returns the full environment for launching claude under the named
// context. The reserved context "none" returns base unchanged.
func (c *Config) buildEnv(name string, base []string) ([]string, error) {
	if name == NoneContext {
		return base, nil
	}
	ctx, err := c.context(name)
	if err != nil {
		return nil, err
	}
	prov := c.provider(ctx.Provider)
	ws := c.workingset(ctx.WorkingSet)
	key, err := prov.apiKey()
	if err != nil {
		return nil, err
	}

	env := make([]string, 0, len(base)+len(managedEnvVars))
	for _, kv := range base {
		if isManagedEnv(kv) {
			continue
		}
		env = append(env, kv)
	}
	// Only ANTHROPIC_AUTH_TOKEN carries the key: Claude Code warns when
	// both it and ANTHROPIC_API_KEY are set. Stale ANTHROPIC_API_KEY
	// values are stripped above via managedEnvVars.
	env = append(env,
		"ANTHROPIC_BASE_URL="+prov.BaseURL,
		"ANTHROPIC_AUTH_TOKEN="+key,
	)
	// Deterministic slot order regardless of map iteration.
	for _, slot := range modelSlots {
		model := ws.Models[slot]
		if model == "" {
			continue
		}
		for _, v := range modelEnvVars[slot] {
			env = append(env, v+"="+model)
		}
	}
	return env, nil
}

// contextModel returns the workingset "default" model for a context, or ""
// when the context is "none", unknown, or has no default slot.
func (c *Config) contextModel(name string) string {
	if name == NoneContext {
		return ""
	}
	for _, x := range c.Contexts {
		if x.Name == name {
			if ws := c.workingset(x.WorkingSet); ws != nil {
				return ws.Models["default"]
			}
		}
	}
	return ""
}

func isManagedEnv(kv string) bool {
	name, _, ok := strings.Cut(kv, "=")
	return ok && slices.Contains(managedEnvVars, name)
}

// setDefault updates current-context in the manifest file in place,
// preserving comments and ordering via the yaml.Node round-trip.
func setDefault(path, name string) error {
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}
	if name != NoneContext {
		if _, err := cfg.context(name); err != nil {
			return err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: root is not a mapping", path)
	}
	root := doc.Content[0]
	value := &yaml.Node{Kind: yaml.ScalarNode, Value: name}
	found := false
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "current-context" {
			root.Content[i+1] = value
			found = true
			break
		}
	}
	if !found {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: "current-context"}
		root.Content = append([]*yaml.Node{key, value}, root.Content...)
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return os.WriteFile(path, []byte(buf.String()), mode)
}
