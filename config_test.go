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
    type: huggingface
    api-key: hf_test_key
  - name: or
    type: openrouter
    base-url: https://openrouter.example/api
    api-key-file: KEYFILE

workingsets:
  - name: glm
    models:
      default: zai-org/GLM-5.1
      opus: zai-org/GLM-5.1
      haiku: Qwen/Qwen3-Coder-30B

contexts:
  - name: glm-hf
    provider: hf
    workingset: glm
  - name: glm-or
    provider: or
    workingset: glm
`

func writeSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	keyfile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyfile, []byte("sk-or-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	content := strings.ReplaceAll(sampleConfig, "KEYFILE", keyfile)
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
	if got := cfg.provider("hf").baseURL(); got != "https://router.huggingface.co" {
		t.Errorf("hf base url = %q", got)
	}
	if got := cfg.provider("or").baseURL(); got != "https://openrouter.example/api" {
		t.Errorf("or base url = %q", got)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]string{
		"unknown slot": `
providers: [{name: p, type: openrouter, api-key: k}]
workingsets: [{name: w, models: {gpt4: x}}]
contexts: [{name: c, provider: p, workingset: w}]`,
		"unknown provider ref": `
providers: [{name: p, type: openrouter, api-key: k}]
workingsets: [{name: w, models: {opus: x}}]
contexts: [{name: c, provider: nope, workingset: w}]`,
		"reserved context name": `
providers: [{name: p, type: openrouter, api-key: k}]
workingsets: [{name: w, models: {opus: x}}]
contexts: [{name: none, provider: p, workingset: w}]`,
		"missing key": `
providers: [{name: p, type: openrouter}]
workingsets: []
contexts: []`,
		"unknown type without base-url": `
providers: [{name: p, type: mystery, api-key: k}]
workingsets: []
contexts: []`,
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

func TestBuildEnv(t *testing.T) {
	cfg, err := loadConfig(writeSample(t))
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=stale",
		"ANTHROPIC_MODEL=stale-model",
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
		"ANTHROPIC_API_KEY=hf_test_key",
		"ANTHROPIC_MODEL=zai-org/GLM-5.1",
		"ANTHROPIC_DEFAULT_OPUS_MODEL=zai-org/GLM-5.1",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=Qwen/Qwen3-Coder-30B",
		"ANTHROPIC_SMALL_FAST_MODEL=Qwen/Qwen3-Coder-30B",
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
