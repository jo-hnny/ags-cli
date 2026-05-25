package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
)

// RenderEnvelope writes an Envelope to w as JSON. If jqExpr is non-empty,
// the expression is applied and only the filtered result is written.
func RenderEnvelope(w io.Writer, env *Envelope, jqExpr string) error {
	encoder := json.NewEncoder(w)

	if jqExpr == "" {
		return encoder.Encode(env)
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	return applyJQ(w, data, jqExpr)
}

func applyJQ(w io.Writer, data []byte, expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid --jq expression: %w", err)
	}

	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("failed to parse JSON for --jq: %w", err)
	}

	iter := query.Run(input)
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)

	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, isErr := v.(error); isErr {
			return fmt.Errorf("--jq evaluation error: %w", e)
		}
		if s, isStr := v.(string); isStr {
			fmt.Fprintln(&buf, s)
		} else {
			if err := encoder.Encode(v); err != nil {
				return fmt.Errorf("--jq output encoding error: %w", err)
			}
		}
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("--jq output write error: %w", err)
	}
	return nil
}

// RenderEnvelopeToStdout is a convenience for error paths (e.g. root.go)
// that need to write a failed envelope without going through Wrap.
func RenderEnvelopeToStdout(env *Envelope) error {
	encoder := json.NewEncoder(os.Stdout)
	return encoder.Encode(env)
}
