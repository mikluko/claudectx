# claudectx

Launch [Claude Code](https://code.claude.com) against any Anthropic-compatible
endpoint (Hugging Face router, OpenRouter, llama.cpp, local proxies, …) using
named contexts, kubeconfig-style.

A **provider** holds the endpoint, credentials, and provider-specific
environment. A **context** wires a provider to a map of Claude model slots →
provider model IDs, and/or a **config dir** — a self-contained Claude Code
profile with its own credentials, settings, and session store. `claudectx`
resolves the selected context into environment variables and `exec`s `claude`.

A context with a `config-dir` and no provider is *first-party*: it switches
Claude Code accounts rather than endpoints.

## Install

```sh
go install github.com/mikluko/claudectx@latest
```

## Usage

```sh
claudectx --set-default NAME        # select the default context
claudectx [claude args...]          # run the default context
claudectx --context NAME [args...]  # run a named context
claudectx --context none            # passthrough: run claude untouched
```

Everything after the first unrecognized argument is passed to `claude`
verbatim. The reserved context name `none` runs `claude` with the environment
exactly as-is — no variables set or stripped — and may also be set as the
default.

## Manifest

`~/.config/claudectx/config.yaml` (override with `CLAUDECTX_CONFIG`):

```yaml
current-context: glm@hf

providers:
  - name: hf
    base-url: https://router.huggingface.co
    api-key-file: ~/.config/claudectx/hf.token
  - name: moonshot
    base-url: https://api.moonshot.ai/anthropic
    api-key-op: work/Private/moonshot     # 1Password item
    env:
      ENABLE_TOOL_SEARCH: "1"             # extra provider-specific vars
  - name: local
    base-url: http://localhost:8080
    api-key: dummy                        # inline works too

contexts:
  # First-party: no provider, just a Claude Code profile to switch to.
  - name: work
    config-dir: ~/.claude-work
  - name: personal
    config-dir: ~/.claude-personal
  - name: glm@hf
    provider: hf
    config-dir: ~/.claude-hf              # optional: own session store
    models:
      default: zai-org/GLM-5.1          # -> ANTHROPIC_MODEL and --model
      opus: zai-org/GLM-5.1             # -> ANTHROPIC_DEFAULT_OPUS_MODEL
      sonnet: zai-org/GLM-5.1           # -> ANTHROPIC_DEFAULT_SONNET_MODEL
      haiku: Qwen/Qwen3-Coder-30B-A3B-Instruct  # -> ANTHROPIC_DEFAULT_HAIKU_MODEL, ANTHROPIC_SMALL_FAST_MODEL
  - name: kimi@moonshot
    provider: moonshot
    models:
      default: kimi-k3
      opus: kimi-k2.7-code
      sonnet: kimi-k2.6
      haiku: kimi-k2.6
      subagent: kimi-k2.6               # -> CLAUDE_CODE_SUBAGENT_MODEL
```

Model slots are optional; only present slots emit variables. Valid slots:
`default`, `fable`, `opus`, `sonnet`, `haiku`, `subagent`.

Every context needs a `provider`, a `config-dir`, or both — one with neither
would be indistinguishable from `none`.

### Config dirs

`config-dir` sets `CLAUDE_CONFIG_DIR`, which is the root of everything Claude
Code keeps per profile: credentials, `settings.json`, MCP servers, prompt
history, and the session store.

Credentials are **not** stored in the config dir. On macOS they live in the
keychain under a service name derived from the config dir path
(`Claude Code-credentials-<hash>`), so each config dir is separately
authenticated and needs its own login:

```sh
CLAUDE_CONFIG_DIR=~/.claude-work claude auth login
```

Two accounts can therefore run side by side, in parallel, without logging each
other out. Pin each profile to its intended account with `forceLoginOrgUUID` in
that profile's `settings.json` so a mis-login fails loudly.

claudectx **fails before exec** when a `config-dir` does not exist or is not a
directory. Claude Code would otherwise bootstrap a fresh, unauthenticated
profile there and present it as an empty history — indistinguishable from
having lost one.

### Credentials

Exactly one of `api-key`, `api-key-file`, `api-key-op` per provider.
First-party contexts have no provider and so need none.

`api-key-op` resolves a 1Password item through the `op` CLI at launch time.
The reference is `account/vault/item`, `vault/item`, or bare `item` (names or
IDs, as accepted by `op`). The item's `credential` field is used (API
Credential category), falling back to its password field. Resolution fails
fast when `op` is not installed or the item cannot be found.

### Environment contract

For a non-`none` context, claudectx strips any pre-existing values of the
variables it manages, then sets:

| Variable | Source |
|---|---|
| `ANTHROPIC_BASE_URL` | provider `base-url` (Claude Code appends `/v1/messages`) |
| `ANTHROPIC_AUTH_TOKEN` | provider key |
| `CLAUDE_CONFIG_DIR` | context `config-dir` (`~` expanded) |
| `ANTHROPIC_MODEL` | models `default` |
| `ANTHROPIC_DEFAULT_FABLE_MODEL` | models `fable` |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | models `opus` |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | models `sonnet` |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `ANTHROPIC_SMALL_FAST_MODEL` | models `haiku` |
| `CLAUDE_CODE_SUBAGENT_MODEL` | models `subagent` |
| provider `env` entries | verbatim, overriding inherited values |

The first two are set only for a context that has a provider; `CLAUDE_CONFIG_DIR`
only for one that has a `config-dir`.

Only `ANTHROPIC_AUTH_TOKEN` carries the key: Claude Code warns when both it
and `ANTHROPIC_API_KEY` are set. A stale `ANTHROPIC_API_KEY` from the parent
shell is stripped along with the other managed variables. All other
environment variables pass through unchanged.

`CLAUDE_CONFIG_DIR` is managed, so an exported value is stripped for *every*
non-`none` context — including contexts that declare no `config-dir`, which
therefore always land in the default `~/.claude`. This keeps the launched
profile a function of the manifest alone. Use `--context none` to hand an
exported `CLAUDE_CONFIG_DIR` through untouched.

The `default` model is additionally passed to claude as `--model`: a model
pinned in Claude Code settings outranks the `ANTHROPIC_MODEL` environment
variable, but the command line outranks settings. A `--model` you pass
yourself takes precedence.

`--set-default` rewrites the manifest in place; comments and ordering are
preserved.

### Sessions are provider-bound

Conversation history carries provider-specific artifacts (thinking-block
signatures, server-tool blocks, model references). Resuming a session that
was started against a different provider replays that history and typically
fails with provider errors (HTTP 500 retry loops). Start a fresh session
after switching contexts; use `--resume`/`--continue` only within the same
provider.

Giving each provider context its own `config-dir` largely removes the hazard:
sessions are then stored per context, so `--resume` and `--continue` cannot
reach a session recorded against a different provider in the first place.
