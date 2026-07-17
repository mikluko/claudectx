# claudectx

Launch [Claude Code](https://code.claude.com) against any Anthropic-compatible
endpoint (Hugging Face router, OpenRouter, llama.cpp, local proxies, …) using
named contexts, kubeconfig-style.

A **provider** holds the router endpoint and credentials. A **workingset**
maps Claude model slots to provider model IDs. A **context** wires a provider
to a workingset. `claudectx` resolves the selected context into environment
variables and `exec`s `claude`.

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
current-context: glm-hf

providers:
  - name: hf
    base-url: https://router.huggingface.co
    api-key-file: ~/.config/claudectx/hf.token
  - name: or
    base-url: https://openrouter.ai/api
    api-key-op: work/Private/openrouter   # 1Password item
  - name: local
    base-url: http://localhost:8080
    api-key: dummy                        # inline works too

workingsets:
  - name: glm
    models:
      default: zai-org/GLM-5.1          # -> ANTHROPIC_MODEL
      opus: zai-org/GLM-5.1             # -> ANTHROPIC_DEFAULT_OPUS_MODEL
      sonnet: zai-org/GLM-5.1           # -> ANTHROPIC_DEFAULT_SONNET_MODEL
      haiku: Qwen/Qwen3-Coder-30B-A3B-Instruct  # -> ANTHROPIC_DEFAULT_HAIKU_MODEL, ANTHROPIC_SMALL_FAST_MODEL

contexts:
  - name: glm-hf
    provider: hf
    workingset: glm
```

Model slots are optional; only present slots emit variables. Valid slots:
`default`, `fable`, `opus`, `sonnet`, `haiku`.

### Credentials

Exactly one of `api-key`, `api-key-file`, `api-key-op` per provider.

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
| `ANTHROPIC_MODEL` | workingset `default` |
| `ANTHROPIC_DEFAULT_FABLE_MODEL` | workingset `fable` |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | workingset `opus` |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | workingset `sonnet` |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `ANTHROPIC_SMALL_FAST_MODEL` | workingset `haiku` |

Only `ANTHROPIC_AUTH_TOKEN` carries the key: Claude Code warns when both it
and `ANTHROPIC_API_KEY` are set. A stale `ANTHROPIC_API_KEY` from the parent
shell is stripped along with the other managed variables. All other
environment variables pass through unchanged.

The workingset `default` model is additionally passed to claude as
`--model`: a model pinned in Claude Code settings outranks the
`ANTHROPIC_MODEL` environment variable, but the command line outranks
settings. A `--model` you pass yourself takes precedence.

`--set-default` rewrites the manifest in place; comments and ordering are
preserved.
