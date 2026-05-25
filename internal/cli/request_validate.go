package cli

import requestio "github.com/TencentCloudAgentRuntime/ags-cli/internal/cli/request"

// mergePositionalIntoRequest takes a raw `--request` payload and a
// positional value (e.g. instance-id) and returns a merged JSON body
// where the positional value populates the named field. It also fails
// when the request JSON already supplies a different value for that
// field, surfacing a REQUEST_ARG_CONFLICT usage error.
//
// Per NextPlan §9.0 the CLI does not run client-side request schema
// validation. The only reason this helper exists is to surface
// positional / `--request` conflicts at the CLI layer; the typed SDK
// remains the source of truth for request shape.
func mergePositionalIntoRequest(rawRequest, fieldName, positional string) ([]byte, error) {
	return requestio.MergePositional(rawRequest, fieldName, positional)
}

// validateRequestPayload is intentionally a CLI-layer usage check
// only. Per NextPlan §9.0 the CLI does NOT run client-side schema
// validation against the catalog: required / enum / anyOf / nested
// unknown fields are the server's and the typed SDK's responsibility.
//
// What we do here:
//   - confirm the payload is non-empty.
//   - confirm the top-level value is a JSON object.
//
// Anything beyond that is delegated downstream so the CLI does not
// drift into being a second API validator.
func validateRequestPayload(commandID string, raw []byte) error {
	return requestio.ValidatePayload(commandID, raw)
}

// requestParseError wraps a typed SDK FromJsonString error into a
// stable usage error. Per NextPlan §9.0 the typed SDK is the
// authoritative shape check; we only translate its message into the
// envelope contract.
func requestParseError(commandName string, err error) error {
	return requestio.ParseError(commandName, err)
}
