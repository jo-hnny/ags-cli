package output

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exit codes", func() {
	It("maps detailed kinds to agr.v1 coarse exit codes", func() {
		Expect(ExitCodeForKind(KindSuccess)).To(Equal(0))
		for _, kind := range []string{KindGenericError, KindNotFound, KindConflict, KindRateLimit, KindTimeout, KindNetwork, KindPartialSuccess} {
			Expect(ExitCodeForKind(kind)).To(Equal(1))
		}
		Expect(ExitCodeForKind(KindUsage)).To(Equal(2))
		Expect(ExitCodeForKind(KindAuthOrPermission)).To(Equal(4))
		Expect(ExitCodeForKind(KindRemoteExecFailed)).To(Equal(255))
	})

	It("constructs structured errors", func() {
		Expect(NewUsageError("BAD", "bad", "hint").ExitCode).To(Equal(2))
		Expect(NewAuthError("AUTH", "auth", "hint").ExitCode).To(Equal(4))
		Expect(NewConflictError("CONFLICT", "conflict", "hint").ExitCode).To(Equal(1))
		Expect(NewRemoteExecutionError("REMOTE", "remote", "hint").ExitCode).To(Equal(255))
	})

	It("preserves existing CLI errors", func() {
		orig := NewUsageError("BAD", "bad", "hint")
		got := ClassifyError(orig)
		Expect(got).To(BeIdenticalTo(orig))
		Expect(errors.Is(orig, got)).To(BeTrue())
	})
})
