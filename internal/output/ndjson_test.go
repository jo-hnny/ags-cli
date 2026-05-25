package output

import (
	"bytes"
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NDJSONWriter", func() {

	var (
		buf *bytes.Buffer
		nw  *NDJSONWriter
	)

	BeforeEach(func() {
		buf = &bytes.Buffer{}
		nw = NewNDJSONWriter(buf, "instance.exec")
	})

	parseEvents := func() []NDJSONEvent {
		var events []NDJSONEvent
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var ev NDJSONEvent
			ExpectWithOffset(1, json.Unmarshal([]byte(line), &ev)).To(Succeed())
			events = append(events, ev)
		}
		return events
	}

	It("uses agr.events.v1 schema version (AC8)", func() {
		Expect(nw.WriteStarted(nil)).To(Succeed())
		events := parseEvents()
		Expect(events).To(HaveLen(1))
		Expect(events[0].SchemaVersion).To(Equal("agr.events.v1"))
	})

	It("emits started event with command", func() {
		Expect(nw.WriteStarted(map[string]string{"InstanceId": "sb-1"})).To(Succeed())
		events := parseEvents()
		Expect(events[0].Type).To(Equal("started"))
		Expect(events[0].Command).To(Equal("instance.exec"))
	})

	It("emits stdout/stderr events with Chunk", func() {
		Expect(nw.WriteStdout("hello\n")).To(Succeed())
		Expect(nw.WriteStderr("warn\n")).To(Succeed())
		events := parseEvents()
		Expect(events).To(HaveLen(2))
		Expect(events[0].Type).To(Equal("stdout"))
		Expect(events[1].Type).To(Equal("stderr"))
	})

	It("emits completed event", func() {
		Expect(nw.WriteCompleted(map[string]any{"ExitCode": 0})).To(Succeed())
		events := parseEvents()
		Expect(events[0].Type).To(Equal("completed"))
	})

	It("emits failed event with Failure", func() {
		f := &Failure{Code: "ERR", Kind: KindRemoteExecFailed, Message: "bad"}
		Expect(nw.WriteFailed(map[string]any{"ExitCode": 1}, f)).To(Succeed())
		events := parseEvents()
		Expect(events[0].Type).To(Equal("failed"))
		Expect(events[0].Failure).NotTo(BeNil())
		Expect(events[0].Failure.Code).To(Equal("ERR"))
	})

	It("writes one JSON line per event", func() {
		Expect(nw.WriteStarted(nil)).To(Succeed())
		Expect(nw.WriteStdout("a")).To(Succeed())
		Expect(nw.WriteCompleted(nil)).To(Succeed())
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		Expect(lines).To(HaveLen(3))
		for _, line := range lines {
			var m map[string]any
			Expect(json.Unmarshal([]byte(line), &m)).To(Succeed())
		}
	})

	It("includes Timestamp in RFC3339 format", func() {
		Expect(nw.WriteStarted(nil)).To(Succeed())
		events := parseEvents()
		Expect(events[0].Timestamp).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`))
	})

	Describe("Full stream lifecycle (AC8)", func() {

		It("emits started -> stdout -> stderr -> completed sequence", func() {
			Expect(nw.WriteStarted(map[string]string{"InstanceId": "sb-1"})).To(Succeed())
			Expect(nw.WriteStdout("line1\n")).To(Succeed())
			Expect(nw.WriteStderr("warn1\n")).To(Succeed())
			Expect(nw.WriteCompleted(map[string]any{"ExitCode": 0})).To(Succeed())

			events := parseEvents()
			Expect(events).To(HaveLen(4))
			Expect(events[0].Type).To(Equal("started"))
			Expect(events[1].Type).To(Equal("stdout"))
			Expect(events[2].Type).To(Equal("stderr"))
			Expect(events[3].Type).To(Equal("completed"))

			for _, ev := range events {
				Expect(ev.SchemaVersion).To(Equal("agr.events.v1"))
				Expect(ev.Command).To(Equal("instance.exec"))
			}
		})

		It("emits started -> failed sequence for errors", func() {
			Expect(nw.WriteStarted(map[string]string{"InstanceId": "sb-1"})).To(Succeed())
			f := &Failure{Code: "CONFIG_ERROR", Kind: KindUsage, Message: "bad config"}
			Expect(nw.WriteFailed(nil, f)).To(Succeed())

			events := parseEvents()
			Expect(events).To(HaveLen(2))
			Expect(events[0].Type).To(Equal("started"))
			Expect(events[1].Type).To(Equal("failed"))
			Expect(events[1].Failure).NotTo(BeNil())
			Expect(events[1].Failure.Code).To(Equal("CONFIG_ERROR"))
		})

		It("remote execution failure carries details in Data with nil Failure (AC12)", func() {
			Expect(nw.WriteStarted(nil)).To(Succeed())
			Expect(nw.WriteFailed(map[string]any{"ExitCode": 42}, nil)).To(Succeed())

			events := parseEvents()
			Expect(events[1].Type).To(Equal("failed"))
			Expect(events[1].Failure).To(BeNil())
			dataMap := events[1].Data.(map[string]any)
			Expect(dataMap["ExitCode"]).To(BeNumerically("==", 42))
		})
	})

	Describe("code run stream lifecycle", func() {
		It("emits events with instance.code.run command", func() {
			codeNW := NewNDJSONWriter(buf, "instance.code.run")
			Expect(codeNW.WriteStarted(map[string]string{"InstanceId": "sb-2"})).To(Succeed())
			Expect(codeNW.WriteStdout("hello\n")).To(Succeed())
			Expect(codeNW.WriteCompleted(map[string]any{"ExecutionCount": 1})).To(Succeed())

			events := parseEvents()
			Expect(events).To(HaveLen(3))
			for _, ev := range events {
				Expect(ev.Command).To(Equal("instance.code.run"))
			}
		})
	})
})
