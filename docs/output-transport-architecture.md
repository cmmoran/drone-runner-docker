# Output Transport Architecture

## Purpose

This document describes the target-state transport design for step outputs in
`drone-runner-docker`.

The current implementation works, but still depends on filesystem-based
transport in one of several forms:

- workspace-backed files
- host-backed files
- Docker `CopyFromContainer` fallback

The target design replaces these with IPC while preserving the current
`from_output` semantics.

## Goals

- preserve current user-facing `drone-output` CLI semantics
- preserve current `from_output` behavior
- allow any normal step to publish outputs
- only steps with `from_output` consume from runner state
- keep outputs isolated per pipeline
- remove filesystem transport assumptions for output propagation
- support environments with and without shared runner/step mounts

## Non-Goals

- cross-pipeline output sharing
- cross-stage output sharing
- server-side persistence of outputs
- UI persistence of outputs
- detached step output consumption

## Current Behavior

Today, the runner:

- injects `/drone/bin/drone-output` into step containers
- stores flattened outputs as `map[string]string`
- resolves `from_output` for downstream steps from runner-held state

Transport is currently file-based and may use:

- a host-backed `/drone/outputs/<uuid>/<step>`
- a workspace-backed `${DRONE_WORKSPACE}/${DRONE_OUTPUT_DIR}/${step}`
- Docker `CopyFromContainer` fallback

## Target Behavior

The target design changes only the output transport, not the semantics.

The target model is:

- step invokes `drone-output`
- `drone-output` sends normalized output operations to the runner over IPC
- runner stores outputs in a concurrent-safe per-pipeline in-memory store
- downstream steps resolve `from_output` from that store

No output files need to be collected after step completion in the final model.

## Store Model

The runner maintains a process-lifetime output service with per-pipeline
isolation.

```go
type OutputStore struct {
    pipelines TTLMap[string, *PipelineOutputs]
    tokens    TTLMap[string, StepIdentity]
}

type StepIdentity struct {
    PipelineID string
    Step       string
}

type PipelineOutputs struct {
    mu     sync.RWMutex
    byStep map[string]map[string]string
}
```

### Notes

- the top-level store is keyed by `pipelineID`
- each pipeline entry stores flattened output values by step
- step identity is derived from a token, not from untrusted client fields
- explicit teardown deletes pipeline state
- TTL is fallback cleanup only

## Pipeline ID

The pipeline store key should uniquely identify one executing pipeline unit.

Recommended format:

```text
<repo-id>/<build-id>/<stage-id>
```

This is preferred over using only the stage ID.

## TTL

The store uses a sync TTL map so leaked state expires automatically.

Recommended default TTL:

- `72h`

This matches Drone's maximum timeout and acts as a fallback safety net.

TTL does not replace explicit cleanup on pipeline teardown.

## Tokens

Every normal step receives an opaque step token.

The runner records:

```go
token -> { pipelineID, stepName }
```

All IPC requests are authenticated through this token.

The runner must not trust caller-provided `pipelineID` or `step` names.

## Transport Modes

Supported target transports:

- `unix`
- `http`

Legacy compatibility mode during migration:

- `file`

Exactly one transport is selected per pipeline.

## Transport Selection

Recommended selection order:

1. `unix` if a shared IPC mount is available to both runner and step containers
2. `http` otherwise

The transport choice should be made once per pipeline and injected into all
normal steps in that pipeline.

## Unix Transport

Unix mode uses a per-pipeline Unix domain socket.

Example socket path:

```text
/drone/ipc/<pipeline-id>/outputs.sock
```

### Properties

- local IPC only
- request/response capable
- no network exposure
- natural pipeline scoping

### Lifecycle

- created at pipeline start
- removed at pipeline teardown
- one listener goroutine per pipeline is acceptable

Unix mode is preferred when shared mount visibility exists.

## HTTP Transport

HTTP mode uses a runner-local HTTP endpoint reachable from step containers.

Example:

```text
POST http://<runner-address>:<port>/outputs
```

### Properties

- no shared filesystem required
- request/response semantics
- single global listener is simpler than per-pipeline ports

### Requirements

- runner address reachable from step containers
- injected per-step auth token
- stable runner-side bind and advertise config

HTTP is the fallback mode when shared-mount Unix sockets are not practical.

## Injected Environment Variables

Common:

- `DRONE_OUTPUT_TRANSPORT`
- `DRONE_OUTPUT_TOKEN`

Unix mode:

- `DRONE_OUTPUT_TRANSPORT=unix`
- `DRONE_OUTPUT_SOCKET=/drone/ipc/<pipeline-id>/outputs.sock`

HTTP mode:

- `DRONE_OUTPUT_TRANSPORT=http`
- `DRONE_OUTPUT_URL=http://<runner-address>:<port>/outputs`

In target IPC mode, `DRONE_OUTPUT_DIR` is no longer required.

## Protocol

The transport payload should represent normalized output operations.

Recommended request schema:

```json
{
  "version": 1,
  "token": "opaque-step-token",
  "ops": [
    {"op":"set","key":"version","value":"1.2.3"},
    {"op":"set","key":"image.name","value":"sentinel/synergy-drone"},
    {"op":"unset","key":"image.digest"}
  ]
}
```

Recommended response schema:

```json
{"ok": true}
```

Error response:

```json
{"ok": false, "error": "invalid output key"}
```

NDJSON framing is acceptable for Unix mode. HTTP mode should use normal JSON
request and response bodies.

## Operation Model

`drone-output` normalizes all commands into operations.

```go
type OutputOp struct {
    Op    string  `json:"op"` // set | unset
    Key   string  `json:"key"`
    Value *string `json:"value,omitempty"`
}
```

### Mapping

- `set` -> one or more `set` operations after flattening
- `put` -> one or more `set` and `unset` operations after flattening
- `unset` -> one `unset` operation

Flattening rules remain the same as the current implementation.

## Runner Handler

Both transports should call the same runner-side apply path.

```go
ApplyOutputOps(token string, ops []OutputOp) error
```

The handler should:

1. validate the token
2. derive `pipelineID` and `stepName`
3. load the pipeline store entry
4. acquire the pipeline write lock
5. apply operations to `byStep[stepName]`
6. refresh TTL
7. return an acknowledgement

## `drone-output` Client Behavior

The `drone-output` CLI remains the same:

```bash
drone-output set [--format env|json|yaml|toml] <key> <value>
drone-output put [--format env|json|yaml|toml] <path>
drone-output unset <key>
```

Internally it should:

1. parse and flatten the input
2. convert the result to normalized output operations
3. inspect `DRONE_OUTPUT_TRANSPORT`
4. send one request to the configured runner transport
5. fail on transport or acknowledgement error

## `from_output`

`from_output` semantics remain unchanged.

Consumers still resolve values from runner-held state using:

- `build.version`
- `build/version`
- `steps.build.outputs.version`
- `step=build,key=version`

Negative index resolution remains read-time only.

## Failure Semantics

Producer-side failures:

- invalid token
- malformed request payload
- invalid output key
- unavailable transport

These should make `drone-output` fail.

Consumer-side failures remain unchanged:

- unknown producer
- missing dependency on producer
- missing referenced output key

Not an error:

- a step publishes no outputs
- a step never invokes `drone-output`

## Migration Plan

### Phase 1

- add protocol types
- add output service and TTL-backed store
- add Unix transport
- add HTTP transport
- keep file mode as compatibility fallback

### Phase 2

- teach `drone-output` to use transport mode when injected
- inject transport env and tokens into steps
- resolve `from_output` from output service

### Phase 3

- remove file-based output transport once parity is proven
- remove `/drone/outputs` file transport
- remove workspace file transport
- remove Docker output collection fallback

## Relationship To Current Working Implementation

The current working implementation is still useful as a gold master for
behavior:

- helper binary injection works
- output flattening works
- `from_output` resolution works
- per-pipeline isolation behavior is understood

The target transport design should preserve those semantics while replacing the
filesystem transport with IPC.
