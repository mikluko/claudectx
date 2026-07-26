package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const sampleConfig = `# personal manifest
current-context: glm-hf

providers:
  - name: hf
    base-url: https://router.huggingface.co
    api-key: hf_test_key
    env:
      ENABLE_TOOL_SEARCH: "1"
  - name: or
    base-url: https://openrouter.example/api
    api-key-file: KEYFILE

contexts:
  - name: glm-hf
    provider: hf
    models:
      default: zai-org/GLM-5.1
      fable: zai-org/GLM-5.1
      opus: zai-org/GLM-5.1
      haiku: Qwen/Qwen3-Coder-30B
      subagent: Qwen/Qwen3-Coder-30B
  - name: glm-or
    provider: or
    models:
      default: zai-org/GLM-5.1
  - name: glm-hf-dir
    provider: hf
    config-dir: CONFIGDIR
    models:
      default: zai-org/GLM-5.1
  - name: firstparty
    config-dir: CONFIGDIR
`

// sampleConfigDir is where writeSample materialises the CONFIGDIR
// placeholder, relative to the manifest: the config-dir existence check needs
// a live path, and tests need to predict it without threading it through.
func sampleConfigDir(manifest string) string {
	return filepath.Join(filepath.Dir(manifest), "profile")
}

func writeSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	keyfile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyfile, []byte("sk-or-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.Mkdir(sampleConfigDir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(sampleConfig, "KEYFILE", keyfile)
	content = strings.ReplaceAll(content, "CONFIGDIR", sampleConfigDir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "glm-hf" {
		t.Errorf("current-context = %q", cfg.CurrentContext)
	}
	if got := cfg.provider("hf").BaseURL; got != "https://router.huggingface.co" {
		t.Errorf("hf base url = %q", got)
	}
	if got := cfg.provider("or").BaseURL; got != "https://openrouter.example/api" {
		t.Errorf("or base url = %q", got)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]string{
		"unknown slot": `
providers: [{name: p, base-url: "https://x", api-key: k}]
contexts: [{name: c, provider: p, models: {gpt4: x}}]`,
		"unknown provider ref": `
providers: [{name: p, base-url: "https://x", api-key: k}]
contexts: [{name: c, provider: nope, models: {opus: x}}]`,
		"reserved context name": `
providers: [{name: p, base-url: "https://x", api-key: k}]
contexts: [{name: none, provider: p, models: {opus: x}}]`,
		"missing key": `
providers: [{name: p, base-url: "https://x"}]
contexts: []`,
		"missing base-url": `
providers: [{name: p, api-key: k}]
contexts: []`,
		"neither provider nor config-dir": `
providers: [{name: p, base-url: "https://x", api-key: k}]
contexts: [{name: c, models: {opus: x}}]`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadConfig(path); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestDanglingCurrentContext(t *testing.T) {
	// A current-context that references a renamed/removed context must
	// load as empty default, not fail — otherwise --set-default cannot
	// repair the manifest.
	content := strings.Replace(sampleConfig, "current-context: glm-hf", "current-context: renamed-away", 1)
	dir := t.TempDir()
	keyfile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyfile, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	content = strings.ReplaceAll(content, "KEYFILE", keyfile)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("dangling current-context must not fail load: %v", err)
	}
	if cfg.CurrentContext != "" {
		t.Errorf("current-context = %q, want empty", cfg.CurrentContext)
	}
	// And --set-default can repair it.
	if err := setDefault(path, "glm-hf"); err != nil {
		t.Fatalf("setDefault on dangling manifest: %v", err)
	}
	cfg, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "glm-hf" {
		t.Errorf("current-context = %q after repair", cfg.CurrentContext)
	}
}

func TestBuildEnv(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=stale",
		"ANTHROPIC_MODEL=stale-model",
		"ENABLE_TOOL_SEARCH=stale",
		"ANTHROPIC_LOG=debug",
	}
	env, err := cfg.buildEnv("glm-hf", base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_LOG=debug",
		"ANTHROPIC_BASE_URL=https://router.huggingface.co",
		"ANTHROPIC_AUTH_TOKEN=hf_test_key",
		"ANTHROPIC_MODEL=zai-org/GLM-5.1",
		"ANTHROPIC_DEFAULT_FABLE_MODEL=zai-org/GLM-5.1",
		"ANTHROPIC_DEFAULT_OPUS_MODEL=zai-org/GLM-5.1",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=Qwen/Qwen3-Coder-30B",
		"ANTHROPIC_SMALL_FAST_MODEL=Qwen/Qwen3-Coder-30B",
		"CLAUDE_CODE_SUBAGENT_MODEL=Qwen/Qwen3-Coder-30B",
		"ENABLE_TOOL_SEARCH=1",
	}
	if !slices.Equal(env, want) {
		t.Errorf("env mismatch:\n got %q\nwant %q", env, want)
	}
	if slices.Contains(env, "ANTHROPIC_MODEL=stale-model") {
		t.Error("stale managed var leaked through")
	}
}

func TestBuildEnvKeyFile(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	env, err := cfg.buildEnv("glm-or", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env, "ANTHROPIC_AUTH_TOKEN=sk-or-test") {
		t.Errorf("key file not read/trimmed: %q", env)
	}
}

func TestBuildEnvNone(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=/usr/bin", "ANTHROPIC_API_KEY=keep-me"}
	env, err := cfg.buildEnv(NoneContext, base)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(env, base) {
		t.Errorf("none context must not touch env:\n got %q\nwant %q", env, base)
	}
}

func TestBuildEnvConfigDir(t *testing.T) {
	path := writeSample(t)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=/usr/bin", "CLAUDE_CONFIG_DIR=/stale/profile"}
	env, err := cfg.buildEnv("glm-hf-dir", base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_BASE_URL=https://router.huggingface.co",
		"ANTHROPIC_AUTH_TOKEN=hf_test_key",
		"CLAUDE_CONFIG_DIR=" + sampleConfigDir(path),
		"ANTHROPIC_MODEL=zai-org/GLM-5.1",
		"ENABLE_TOOL_SEARCH=1",
	}
	if !slices.Equal(env, want) {
		t.Errorf("env mismatch:\n got %q\nwant %q", env, want)
	}
	if slices.Contains(env, "CLAUDE_CONFIG_DIR=/stale/profile") {
		t.Error("inherited CLAUDE_CONFIG_DIR leaked through")
	}
}

func TestBuildEnvFirstParty(t *testing.T) {
	path := writeSample(t)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	env, err := cfg.buildEnv("firstparty", []string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatal(err)
	}
	// No provider means no endpoint and no key: claude authenticates from
	// the profile in config-dir instead.
	want := []string{"PATH=/usr/bin", "CLAUDE_CONFIG_DIR=" + sampleConfigDir(path)}
	if !slices.Equal(env, want) {
		t.Errorf("env mismatch:\n got %q\nwant %q", env, want)
	}
}

func TestBuildEnvConfigDirExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cfg := &Config{Contexts: []Context{{Name: "c", ConfigDir: "~"}}}
	env, err := cfg.buildEnv("c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env, "CLAUDE_CONFIG_DIR="+home) {
		t.Errorf("~ not expanded: %q", env)
	}
}

func TestBuildEnvConfigDirUnusable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ configDir, wantErr string }{
		"missing":   {filepath.Join(dir, "nope"), "does not exist"},
		"not a dir": {file, "is not a directory"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{Contexts: []Context{{Name: "c", ConfigDir: tc.configDir}}}
			_, err := cfg.buildEnv("c", nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildEnvUnknownContext(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.buildEnv("nope", nil); err == nil {
		t.Error("expected error for unknown context")
	}
}

func TestSetDefault(t *testing.T) {
	path := writeSample(t)

	if err := setDefault(path, "glm-or"); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "glm-or" {
		t.Errorf("current-context = %q, want glm-or", cfg.CurrentContext)
	}

	// Comments survive the yaml.Node round-trip.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# personal manifest") {
		t.Error("comment lost on rewrite")
	}

	// Reserved name is accepted as default.
	if err := setDefault(path, NoneContext); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != NoneContext {
		t.Errorf("current-context = %q, want none", cfg.CurrentContext)
	}

	// Unknown name is rejected.
	if err := setDefault(path, "nope"); err == nil {
		t.Error("expected error for unknown context")
	}
}

func TestSetDefaultInsertsKey(t *testing.T) {
	content := strings.Replace(sampleConfig, "current-context: glm-hf\n", "", 1)
	dir := t.TempDir()
	keyfile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyfile, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	content = strings.ReplaceAll(content, "KEYFILE", keyfile)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := setDefault(path, "glm-hf"); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "glm-hf" {
		t.Errorf("current-context = %q, want glm-hf", cfg.CurrentContext)
	}
}
