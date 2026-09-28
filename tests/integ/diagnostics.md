# Error diagnostics coverage (Issue #138, steps 1–3)

The root `renderExecuteError` is the only error-chain printer. It prints before
checking the already-written-output marker. The marker carries a cause but never
requests a second envelope or stream event. Debug output is redacted before it
reaches stderr and limited to 8 KiB plus a truncation marker.

## Exit-path audit

| Path | Diagnostic propagation / reason for no cause |
| --- | --- |
| Legacy `Wrap` and JSON-capable registry modules | Handler errors retain causes through classification; JSON markers retain the original error. |
| Text-only registry modules (`WrapNoJSON`) | Returned errors reach the same root exit; classified session failures retain their causes. |
| `instance exec` and `instance code run` NDJSON connection/execution failures | The command writes one failed event, and `Result.Cause` passes through `FromCommandResult` and `Handled` to the root exit. |
| JSON schema/help errors | Already-written markers carry the original cause. |
| Result-based failures | Explicit causes or the structured failure reach the root; no cause is invented for a business result. |
| Login, mobile adb, exec/code remote nonzero exit | Remote program result, not a local Go error; preserve exit code/output without an invented debug error. |
| Successful file download, proxy, mobile tunnel and stream completion | No local failure to diagnose; no error-chain output. Startup/transfer errors still return through the common exit. |
| Standalone `cmdtree` builder | Propagates `Result.Cause` in its exit error. Production registry installation replaces its RunE with the wrappers above. |
| Cobra help/argument rejection before a handler | Error returns reach the root. The two direct help-output incompatibility exits have only static usage text, not an underlying error chain. |
| Background mobile tunnel readiness | Shared readiness messages preserve child Failure and exit code. Parent startup, protocol, exit and wait failures have separate codes/stages. Debug forwards a separately labeled, redacted, bounded log tail; only created logs contribute LogPath. |

## `errors.Is` / `errors.As` audit

Classifiers now give an existing CLIError priority over its SDK cause, including
the direct cloud classifier and dynamic control-plane route. Session classification
also preserves an existing CLIError before looking for a Connect error. Matching
timeout/cancellation causes is newly possible without changing their public
classification. Resource not-found checks continue to inspect the first CLIError's
Code/Kind. Mobile-list recovery errors and WebSocket close errors do not receive
new CLI wrappers on their existing paths.

## Regression evidence

- Before implementation, `TestClassificationPreservesCause` failed for unknown,
  timeout and cancellation errors; `TestDebugErrorProcess` failed for both text
  and JSON output.
- The process test uses a synthetic command with no production-command allowlist,
  exercises legacy/registry/text-only routes, debug on/off and text/JSON/NDJSON,
  and checks one diagnostic and unchanged machine framing.
- Command tests drive the real NDJSON connection failure paths using a local
  token-cache filesystem failure, before any cloud request.
- A loopback HTTP server rejects the real WebSocket handshake; the test verifies
  status preservation and omission of its response body.
- Wrapped classification, cloud RequestId, timeout/cancellation, credential/header/
  signed-URL redaction and non-mutating failure sanitization have local tests.
- Real parent/child subprocesses run the production connect and tunnel handlers
  against a loopback WebSocket handshake rejection. They verify HTTP 403, custom
  child classification/exit code, text/JSON framing, debug forwarding, credential
  environment forwarding and redaction in the entire persisted log as well as
  the displayed tail.
- Process fixtures exercise start failure, early exit, no readiness message,
  malformed/invalid messages, readiness timeout, cancellation, context deadline,
  unavailable log files and success. Failure paths assert the child was reaped.
- Log-tail tests cover a partial first line, bounded UTF-8 output and retention
  of the latest cause; writer tests check redaction before persistence.
- Replacing only the child tunnel handler with the step-1 implementation via a
  Go overlay makes the handshake subprocess regression fail with debug both on
  and off: the old string-only message loses the expected auth failure/exit 4.

No credentialed live cloud or real mobile-device verification is claimed here.

## Stage 3: mobile WebSocket boundary

Startup/recovery probes and runtime connections share handshake observation
capture. Local HTTP rejection and delayed-response tests verify endpoint, effective
budget, HTTP status and allowlisted request IDs. Injected DNS/refused-connection
errors preserve causes without inventing response metadata. Parent/child process
tests assert those details survive readiness forwarding. URL user info, queries,
fragments, response bodies and non-allowlisted headers are not retained in the
new metadata; unsafe or oversized request IDs are omitted.
Running the new parent/child handshake assertions against the stage-2 handler
and transport via a Go overlay fails with debug on and off because Endpoint,
TimeoutMs and RequestId are missing.

## Stage 3: remaining boundaries

- Local subprocess failures retain Program and observed subprocess_start/exit;
  tunnel startup additionally retains its existing ready/wait/exit stages and
  bounded redacted log diagnostics. A real test subprocess exits 17 in buffered
  and streaming ADB modes without turning that business result into an error.
- File PathErrors retain Operation/Path and the OS cause. Request input and file
  upload/download usage errors no longer discard causes. A failed download to
  stdout returns the read/write failure instead of reporting success.
- Configuration read errors retain OS causes. Parse/decode errors retain safe
  path/location metadata; raw parser input is deliberately not retained because
  unloaded credential values are not yet registered with the redactor.
- Exec/code, file transfer and webshell SDK errors carry remote_execute and the
  operation/instance. Connection preparation carries remote_connect. PTY uses
  local_terminal, remote_start and remote_stream based on observed progress.
  Existing remote program stdout/stderr and nonzero-exit semantics are unchanged.
- Typed and raw Cloud API routes retain target/action and known caller deadline
  budgets. The SDK does not expose HTTP status/headers at this error boundary:
  those fields are omitted, while original SDK Code/Message/RequestId survive.
- Real loopback TLS HTTP and WebSocket rejections exercise the shared instance/
  deployment proxy. Logs contain observed status and allowlisted request ID,
  never response bodies, arbitrary headers, credentials or URL queries. HTTP
  rejection bodies are still forwarded as business responses.
- Text failures display available metadata; schema and explain describe the
  optional fields. stable and preview share these implementations.

Validation is local fault injection, not a credentialed cloud/mobile/Windows
runtime verification. No DNS/TCP/TLS tracing, automatic retries, or private SDK
transport replacement is introduced.

Regression check: a Go overlay that disables boundary propagation and restores
stage-2 proxy/download implementations makes the new local-file, process-start,
cloud-context, HTTP/WS rejection, NDJSON connection and failed-download tests
fail on their missing metadata or swallowed failure assertions.

Additional inventory: the unused OpenBrowser utility has no command caller;
process-identity `ps` fallback returns a boolean and intentionally keeps its
existing cleanup policy; registryHTTP belongs to patch-test fixture setup, not
the shipped command request path. These helpers do not emit CLI failures and
are not converted into new failure envelopes by this change.

## Reviewer regression guards

Typed Cloud API wrappers use the generic `client.CallCloud` helper to capture
context before invocation and preserve classification. AST tests scan both wrapper
files, including anonymous functions, reject SDK calls/method references outside
the helper and require the Action to match its SDK method. In-memory mutations
remove a real helper invocation in each file; negative fixtures cover new direct
calls, context-free calls, method aliases and mismatched Actions.

HTTP proxy business responses (4xx) use `[HTTP]`; 5xx use `[ERROR]`. Both retain
observed status and allowlisted request IDs. Tests cover 401/404/503 with verbose
on/off. WebSocket handshake rejection remains a connection failure with `[ERROR]`.

Context collection supports joined/multiple-wrapped errors. Inner context wins
within one chain, existing classified details remain authoritative, and the first
metadata-bearing branch wins at a join. Sibling fields are not combined; all causes
remain available to `errors.Is/As`. Tests exercise both `errors.Join` and multiple
`%w`, including filesystem fallback and conflicting sibling metadata.
