package output

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Envelope", func() {

	Describe("NewSuccessEnvelope", func() {
		It("creates a succeeded envelope with agr.v1 schema", func() {
			env := NewSuccessEnvelope("test.cmd", map[string]any{"Key": "val"}, "cloud", 42)
			Expect(env.SchemaVersion).To(Equal("agr.v1"))
			Expect(env.Command).To(Equal("test.cmd"))
			Expect(env.Status).To(Equal("succeeded"))
			Expect(env.Failure).To(BeNil())
			Expect(env.Warnings).To(Equal([]string{}))
			Expect(env.Meta.Backend).To(Equal("cloud"))
			Expect(env.Meta.DurationMs).To(Equal(int64(42)))
		})
	})

	Describe("NewFailedEnvelope", func() {
		It("creates a failed envelope with Failure", func() {
			f := &Failure{Code: "TEST_ERR", Kind: KindGenericError, Message: "test"}
			env := NewFailedEnvelope("test.cmd", f, "e2b", 10)
			Expect(env.Status).To(Equal("failed"))
			Expect(env.Data).To(BeNil())
			Expect(env.Failure).To(Equal(f))
		})
	})

	Describe("NewPartialEnvelope", func() {
		It("creates a partial envelope with warnings", func() {
			env := NewPartialEnvelope("test.cmd", "data", []string{"w1"}, "cloud", 5)
			Expect(env.Status).To(Equal("partial"))
			Expect(env.Warnings).To(Equal([]string{"w1"}))
		})
	})

	Describe("RenderEnvelope", func() {
		It("renders JSON without jq", func() {
			env := NewSuccessEnvelope("v", map[string]any{"X": 1}, "cloud", 0)
			var buf bytes.Buffer
			Expect(RenderEnvelope(&buf, env, "")).To(Succeed())
			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["SchemaVersion"]).To(Equal("agr.v1"))
		})

		It("applies jq expression", func() {
			env := NewSuccessEnvelope("v", map[string]any{"X": 1}, "cloud", 0)
			var buf bytes.Buffer
			Expect(RenderEnvelope(&buf, env, ".Data.X")).To(Succeed())
			Expect(buf.String()).To(ContainSubstring("1"))
		})

		It("returns error for invalid jq expression", func() {
			env := NewSuccessEnvelope("v", nil, "cloud", 0)
			var buf bytes.Buffer
			err := RenderEnvelope(&buf, env, ".[invalid")
			Expect(err).To(HaveOccurred())
		})

		It("does not write partial jq output when evaluation fails", func() {
			env := NewSuccessEnvelope("v", map[string]any{
				"ExitCodes": map[string]string{"0": "success"},
			}, "cloud", 0)
			var buf bytes.Buffer
			err := RenderEnvelope(&buf, env, ".Data.ExitCodes[0].Code")
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(BeEmpty())
		})

		It("outputs string jq results as raw text", func() {
			env := NewSuccessEnvelope("v", map[string]any{"Name": "hello"}, "cloud", 0)
			var buf bytes.Buffer
			Expect(RenderEnvelope(&buf, env, ".Data.Name")).To(Succeed())
			Expect(buf.String()).To(Equal("hello\n"))
		})
	})
})
