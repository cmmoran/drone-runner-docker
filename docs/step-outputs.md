# Step Outputs

## Summary

This document specifies a runner-only step output feature for this fork of
`drone-runner-docker`.

A step can publish outputs using a helper command named `drone-output`, and
downstream dependent steps can consume those outputs via `from_output` in step
`environment`.

This feature does not require Drone server changes.

## Scope

- Linux-only
- runner-only
- same pipeline execution only
- same stage only
- outputs usable only in `environment`
- no UI or server persistence
- no cross-stage or cross-runner sharing
- no Windows support

## Runner Configuration

Current transport modes:

- `DRONE_OUTPUT_TRANSPORT=file`
- `DRONE_OUTPUT_TRANSPORT=auto`
- `DRONE_OUTPUT_TRANSPORT=unix`
- `DRONE_OUTPUT_TRANSPORT=http`

Recommended rollout remains `file` until IPC is explicitly enabled and tested.

### Unix Mode

Unix IPC requires all of the following:

- the runner must inject the `drone-output` helper
- `DRONE_OUTPUT_SOCKET_ROOT` must exist inside the runner container
- `DRONE_RUNNER_VOLUMES` must map the same host path into step containers at that same target path

Example:

```text
DRONE_OUTPUT_TRANSPORT=auto
DRONE_OUTPUT_SOCKET_ROOT=/drone/outputs
DRONE_RUNNER_VOLUMES=/srv/data/platform/tools/drone-runner/drone-outputs:/drone/outputs
```

If the shared socket root is not present inside the runner, the compiler falls
back to `file` mode.

### HTTP Mode

HTTP IPC requires:

- `DRONE_OUTPUT_TRANSPORT=http` or `auto`
- `DRONE_OUTPUT_HTTP_BIND`
- `DRONE_OUTPUT_HTTP_ADVERTISE`

The runner only injects HTTP output transport when both bind and advertise are
configured.

## User Experience

Producer:

```yaml
steps:
  - name: build
    commands:
      - drone-output set version 1.2.3
      - drone-output set --format json image '{"name":"repo/app","digest":"sha256:abc"}'
      - drone-output put metadata.yaml
      - drone-output unset image.digest
```

Consumer:

```yaml
steps:
  - name: publish
    depends_on: [build]
    environment:
      VERSION:
        from_output: build.version
      IMAGE_NAME:
        from_output: build.image.name
      LAST_TAG:
        from_output: build.tags.-1
```

## `from_output`

Supported input forms:

- `build.version`
- `build/version`
- `steps.build.outputs.version`
- `step=build,key=version`

Canonical documented form:

- `build.version`

Internal normalized form:

```go
type OutputRef struct {
    Step string
    Key  string
}
```

Parsing rule:

- first identifier is the producing step
- the remainder is the output key path

Examples:

- `build.version` => step=`build`, key=`version`
- `build.image.digest` => step=`build`, key=`image.digest`

## Negative List Indexing

Negative indices are supported in `from_output` only.

Rules:

- negative index is allowed only as the final path segment
- `-1` means the last list item
- `-2` means the second-to-last list item
- stored outputs always use canonical positive indices

Examples:

- `build.tags.-1`
- `build.tags.-2`

Invalid in the MVP:

- `build.matrix.-1.name`

## Silent Helper Mount

Every Linux step container silently receives:

- a static Go binary mounted at:
  - `/drone/bin/drone-output`
- `DRONE_OUTPUT_DIR=.drone-outputs` by default
- the effective per-step output path is `$DRONE_WORKSPACE/$DRONE_OUTPUT_DIR/<step-name>`

User images do not need modification.

## CLI

Commands:

- `drone-output set [--format env|json|yaml|toml] <key> <value>`
- `drone-output put [--format env|json|yaml|toml] <path>`
- `drone-output unset <key>`

## Unified Output Model

All commands operate on a flattened output map:

```go
map[string]string
```

Structured inputs may contain:

- scalars
- lists
- maps

All leaves must ultimately be scalar values or `null`.

Flattening uses dot paths:

- maps append `.field`
- lists append `.0`, `.1`, etc.

Example:

```json
{
  "image": {
    "name": "repo/app",
    "digest": "sha256:abc"
  },
  "tags": ["latest", "1.2.3"]
}
```

Flattens to:

- `image.name=repo/app`
- `image.digest=sha256:abc`
- `tags.0=latest`
- `tags.1=1.2.3`

## `set`

Usage:

```bash
drone-output set [--format env|json|yaml|toml] <key> <value>
```

Behavior:

- `<key>` is a prefix
- `<value>` may be scalar, list, or map
- flattened leaves merge into current step outputs under the prefix

Scalar example:

```bash
drone-output set version 1.2.3
```

Result:

- `version=1.2.3`

Structured example:

```bash
drone-output set --format json image '{"name":"repo/app","digest":"sha256:abc"}'
```

Result:

- `image.name=repo/app`
- `image.digest=sha256:abc`

List example:

```bash
drone-output set --format json tags '["latest","1.2.3"]'
```

Result:

- `tags.0=latest`
- `tags.1=1.2.3`

Trivial-value rule:

- if the value is trivial, treat it as a scalar string
- if `--format` is provided for a trivial value, the flag may be ignored

Examples:

```bash
drone-output set --format json version 1.2.3
drone-output set --format yaml enabled true
drone-output set value null
```

Results:

- `version="1.2.3"`
- `enabled="true"`
- `value="null"`

Important rule:

- plain `set ... null` means the literal string `"null"`
- if the intent is deletion, use `unset`

Structured null rule:

- inside explicitly structured `set` payloads, `null` means unset for that leaf

Example:

```bash
drone-output set --format json image '{"name":"repo/app","digest":null}'
```

Results:

- set `image.name=repo/app`
- unset `image.digest`

Merge semantics:

- merge into existing outputs
- overwrite only affected keys
- preserve unrelated keys

## `put`

Usage:

```bash
drone-output put [--format env|json|yaml|toml] <path>
```

Behavior:

- parse the file
- reduce recursively to flattened key/value leaves
- merge into current step outputs
- overwrite affected keys only
- preserve unrelated keys

Format detection:

- if `--format` is given, use it
- otherwise detect by extension

Extension mapping:

- `.env` => `env`
- `.json` => `json`
- `.yaml` and `.yml` => `yaml`
- `.toml` => `toml`

If no recognized extension and no `--format`:

- return an error

Key-value file format:

- one `key=value` per line
- blank lines ignored
- `#` comments allowed

Null rule:

- `null` leaf means unset
- empty string remains set-but-empty

Example:

```yaml
image:
  name: repo/app
  digest: null
```

Results:

- set `image.name=repo/app`
- unset `image.digest`

If parsing or reduction fails:

- print a clear error to stderr
- exit non-zero
- the caller decides whether to propagate failure

## `unset`

Usage:

```bash
drone-output unset <key>
```

Behavior:

- remove the exact key if present
- also remove all descendants under that prefix

Examples:

- `unset version` removes `version`
- `unset image` removes:
  - `image.name`
  - `image.digest`
  - `image.tags.0`
- `unset image.digest` removes only `image.digest`

No globbing:

- `unset image.*` is invalid
- there are no wildcard semantics

Missing key:

- no-op success

## Key and Path Rules

Flattened output keys may contain path segments.

Rules:

- allowed characters: letters, numbers, `_`, `-`, `.`
- no empty segments
- no leading `.`
- no trailing `.`
- no `..`

Examples valid:

- `version`
- `image.digest`
- `tags.0`

Examples invalid:

- `.digest`
- `image..digest`
- `bad key`

## Consumption Rules

`from_output` is allowed only in `environment`.

A consumer step may only read outputs from steps it depends on:

- direct dependency, or
- transitive dependency

Valid:

```yaml
steps:
  - name: publish
    depends_on: [build]
    environment:
      VERSION:
        from_output: build.version
```

Invalid:

- reading from an unrelated parallel step
- reading from a future step
- reading from a detached or service step in the MVP

If a referenced output is missing at runtime:

- fail the consumer step before launch with a clear error

If a producer step fails:

- its outputs are not published

## Manifest and Compiler Model

Add `from_output` support in this fork's step environment parsing.

Compiled step should carry:

- resolved environment values
- unresolved output references

Conceptually:

```go
type Step struct {
    ...
    Envs       map[string]string
    OutputEnvs map[string]OutputRef
}
```

Compiler responsibilities:

- parse and normalize `from_output`
- validate that the producer step exists
- validate the dependency chain
- inject the helper mount
- inject the outputs volume
- inject `DRONE_OUTPUT_DIR`

## Runtime Model

The runner maintains live outputs for the current pipeline execution:

```go
map[stepName]map[key]value
```

Execution flow:

1. step starts
2. step uses `drone-output`
3. helper writes output files in `DRONE_OUTPUT_DIR`
4. step exits successfully
5. runner reads output files and updates the in-memory output map
6. before a downstream step starts, runner resolves `OutputEnvs`
7. resolved values are injected into that step environment

Stored files always use canonical positive indices.

Negative index resolution happens only when consuming via `from_output`.

## Storage Backend

The storage mechanism is an implementation detail:

- per-step subdirectories under `$DRONE_WORKSPACE/$DRONE_OUTPUT_DIR/<step-name>`
- helper writes files there
- runner reads them after successful step completion

Users interact with `drone-output`, not with files directly.

## Failure Semantics

Compile or lint time failures:

- malformed `from_output`
- unknown producer step
- producer not in the dependency graph
- invalid output key or path reference

Runtime failures:

- helper cannot write
- invalid imported file in `put`
- invalid stored output structure during collection
- missing referenced output for consumer
- negative index out of range

Command exit behavior:

- `set`: non-zero on invalid key, invalid input, or write failure
- `put`: non-zero on invalid format, invalid reduction, or write failure
- `unset`: non-zero on invalid key, success on missing key

## Implementation Areas

- `engine/resource/pipeline.go`
- `engine/spec.go`
- `engine/compiler/compiler.go`
- `cmd/drone-output/main.go`
- a new local runtime execer package in this repo
- `command/daemon/daemon.go`
- `command/daemon/process.go`

## Non-goals

- Windows
- server persistence or UI integration
- using outputs in `when`, `image`, `settings`, or `commands`
- cross-stage outputs
- secret outputs
- service or detached-step producers

## Main Cost

The main implementation cost is the local runtime execer needed to collect
outputs after one step and inject them into downstream dependent steps.

The helper binary and syntax changes are comparatively small.
