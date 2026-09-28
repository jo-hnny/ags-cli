# Error diagnostics coverage (Issue #138, steps 1 and 2)

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
