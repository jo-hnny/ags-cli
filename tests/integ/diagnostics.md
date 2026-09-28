# Error diagnostics coverage (Issue #138, steps 1–3)

The root `renderExecuteError` is the only error-chain printer. It prints before
checking the already-written-output marker. The marker carries a cause but never
requests a second envelope or stream event. Debug output is redacted before it
reaches stderr and limited to 8 KiB plus a truncation marker. Long chains share
the terminal budget across nodes so outer context cannot crowd out a short cause.

## Debug log files

`--debug` (or `AGR_DEBUG=1`) creates a private log under
`$HOME/.agr/logs/agr-<UTC timestamp>-<unique suffix>.log` for each invocation.
`--debug-log <path>` enables debug and appends to the specified file, creating
parent directories as needed. For example:

```sh
agr --debug instance get ssi-example
agr --debug-log ./logs/agr.log instance get ssi-example -o json
```

The file contains full, redacted debug messages and a copy of stderr. The 8 KiB
limit applies only to terminal diagnostics; the file has no length limit.
Stdout and remote-program data keep their existing contracts. Both successful and failed exits print `Debug log:`
with the file path to stderr. Error exits close the file explicitly before
`os.Exit`. Log creation/write failures emit a warning without replacing the
original command result or claiming a complete log. Existing custom files are
appended, not truncated. New log files use mode 0600 and directories use 0700.
`agr schema -o json` exposes these options in `Data.GlobalFlags`.

Redaction replaces credential values only and never removes surrounding text:
diagnostics exist to keep the failure reason. Text formats are not parsed.
Credentials the CLI holds (active SecretId/SecretKey/Token and data-plane or
deployment access tokens registered when obtained) are replaced by exact value,
raw and URL-encoded. Diagnostics and failures also replace the credential after
`Bearer`/`Basic`, URL passwords and signed query values, and credential-named
`Details` fields.

The copied stderr is written unchanged except for known values, and the file
does not depend on how writes split the stream. Every occurrence of every value
is found, and overlapping occurrences (of one value or of different values) are
merged into one `[REDACTED]`. The writer holds back only a tail that could still
become a value, together with any complete value that crosses into that tail,
so written bytes are never part of a later match; there is no line buffering or
line limit. Held-back bytes are not strictly bounded: a continuous chain of
overlapping occurrences (only possible when values overlap themselves or each
other, and they are repeated back to back) is held, and rescanned on each write,
until the chain ends. Normally delimited output does not form such chains. If
output ends with a fragment that may start a value, it is written as
`[REDACTED]`.
Business stderr is still forwarded immediately and unchanged.

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
- A silent loopback peer drives real handshake timeouts; whether the socket
  deadline or the context timer fires first, the probe reports a timeout.
- Wrapped classification, cloud RequestId, timeout/cancellation, credential/
  signed-URL redaction and non-mutating failure sanitization have local tests.
- Redaction tests assert exact output: printed and marshaled `http.Header`
  values (nil, empty, single, multi) keep every neighbouring field in failures,
  stderr and the log; known values are replaced at every write split.
- Chunking equivalence: any split of the stream (every two-way split, bytewise,
  and seeded random chunking) produces the same file as one write, including
  self-overlapping values, values overlapping each other, and output ending
  inside a value.
- Real parent/child subprocesses run the production connect and tunnel handlers
  against a loopback WebSocket handshake rejection. They verify HTTP 403, custom
  child classification/exit code, text/JSON framing, debug forwarding (an
  explicit `--debug=false` overrides an inherited `AGR_DEBUG=1`), credential
  environment forwarding and redaction in the entire persisted log as well as
  the displayed tail. Redaction covers every credential the child holds: the
  forwarded session credentials and the data-plane token from the tunnel's
  token provider, logged in header, JSON and `%v` forms.
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
context before invocation and preserve classification. AST tests recursively scan
repository Go production sources, including anonymous functions and files excluded
by platform/channel build tags. They reject SDK Action calls/method references
outside the helper and require the Action to match its SDK method. Action names
are derived from the installed SDK, including context-free variants, rather than
from a maintained list or a general `WithContext` suffix. This is a conservative
name-based guard, not full Go type analysis; unrelated methods with the exact same
Action name can still fail loudly. Locally declared receiver function fields are
recognized as injection hooks. Test files, the `tests/` harness, testdata, hidden
directories and vendor dependencies are excluded. In-memory mutations
remove a real helper invocation in each file; negative fixtures cover new direct
calls, context-free calls, method aliases and mismatched Actions. A new third file
in a new command directory is injected into a filesystem fixture to prove that
file discovery catches direct calls and aliases even without an SDK import or an
active build tag.

HTTP proxy business responses (4xx) use `[HTTP]` only in verbose mode; 5xx always
use `[ERROR]`. Both retain
observed status and allowlisted request IDs. Tests cover 401/404/503 with verbose
on/off. WebSocket handshake rejection remains a connection failure with `[ERROR]`.

Context collection supports joined/multiple-wrapped errors. Inner context wins
within one chain, existing classified details remain authoritative, and the first
metadata-bearing branch wins at a join. Sibling fields are not combined; all causes
remain available to `errors.Is/As`. Tests exercise both `errors.Join` and multiple
`%w`, including filesystem fallback and conflicting sibling metadata.
