package flow_test

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestOnboardPresentsHandoffBeforeFocusAndAcknowledgesIt(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")

	planOutput, err := runFlow(t, harness, "plan", "parity", "First")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, planOutput)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(planOutput)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", planOutput)
	}
	if output, err := runFlow(t, harness, "focus", "task", match[1]); err != nil {
		t.Fatalf("focus task: %v\n%s", err, output)
	}
	if output, err := runFlow(t, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "onboard")
	if err != nil {
		t.Fatalf("onboard: %v\n%s", err, output)
	}
	for _, section := range []string{"ONBOARD BRIEFING", "SESSION HANDOFF", "SESSION CONTEXT", "FOCUS CONTEXT", "PENDING:"} {
		if !strings.Contains(output, section) {
			t.Fatalf("onboard = %q, want %q", output, section)
		}
	}
	if strings.Index(output, "SESSION HANDOFF") > strings.Index(output, "SESSION CONTEXT") {
		t.Fatalf("onboard rendered focus before handoff: %q", output)
	}

	resume, err := runFlow(t, harness, "session", "resume")
	if err != nil {
		t.Fatalf("resume after onboard: %v\n%s", err, resume)
	}
	if !strings.Contains(resume, "already acknowledged") {
		t.Fatalf("resume after onboard = %q, want acknowledgement", resume)
	}
}

func TestOnboardUsesPonderWhenFocusIsEmpty(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runFlow(t, harness, "onboard")
	if err != nil {
		t.Fatalf("onboard without focus: %v\n%s", err, output)
	}
	for _, section := range []string{"ONBOARD BRIEFING", "SESSION CONTEXT", "PULSE SUMMARY", "TASK LANDSCAPE"} {
		if !strings.Contains(output, section) {
			t.Fatalf("onboard without focus = %q, want %q", output, section)
		}
	}
	if strings.Contains(output, "SESSION HANDOFF") {
		t.Fatalf("onboard without handoff rendered one: %q", output)
	}
}

func TestOnboardPendingHandoffUsesPonderWithoutFocus(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	planOutput, err := runFlow(t, harness, "plan", "parity", "First")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, planOutput)
	}
	if output, err := runFlow(t, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "onboard")
	if err != nil {
		t.Fatalf("onboard without focus: %v\n%s", err, output)
	}
	for _, section := range []string{"SESSION HANDOFF", "PONDER:", "PULSE SUMMARY", "TASK LANDSCAPE"} {
		if !strings.Contains(output, section) {
			t.Fatalf("onboard without focus = %q, want %q", output, section)
		}
	}
	if strings.Contains(output, "STATUS:") {
		t.Fatalf("onboard without focus rendered focused status: %q", output)
	}
	if strings.Index(output, "SESSION HANDOFF") > strings.Index(output, "PONDER:") {
		t.Fatalf("onboard rendered ponder before handoff: %q", output)
	}
}

func TestOnboardPreservesInheritedContextAfterHandoff(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	planOutput, err := runFlow(t, harness, "plan", "context", "Root", "Middle", "Leaf")
	if err != nil {
		t.Fatalf("create context plan: %v\n%s", err, planOutput)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(planOutput, -1)
	if len(matches) != 3 {
		t.Fatalf("context plan output = %q, want three task UUIDs", planOutput)
	}
	root, leaf := matches[0][1], matches[2][1]
	if output, err := runFlow(t, harness, "note", root, "decision", "Root decision"); err != nil {
		t.Fatalf("add root decision: %v\n%s", err, output)
	}
	if output, err := runFlow(t, harness, "focus", "task", leaf); err != nil {
		t.Fatalf("focus leaf task: %v\n%s", err, output)
	}
	if output, err := runFlow(t, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "onboard")
	if err != nil {
		t.Fatalf("onboard: %v\n%s", err, output)
	}
	for _, section := range []string{"SESSION HANDOFF", "FOCUS CONTEXT", "INHERITED CONTEXT", "DECISION: Root decision", "STATUS:"} {
		if !strings.Contains(output, section) {
			t.Fatalf("onboard = %q, want %q", output, section)
		}
	}
	handoffIndex := strings.Index(output, "SESSION HANDOFF")
	focusIndex := strings.Index(output, "FOCUS CONTEXT")
	decisionIndex := strings.Index(output, "DECISION: Root decision")
	if handoffIndex > focusIndex || decisionIndex < focusIndex {
		t.Fatalf("onboard lost handoff/focus ordering or inherited context: %q", output)
	}
	if !strings.Contains(output, "Task: "+leaf+" Leaf") {
		t.Fatalf("onboard = %q, want focused leaf task", output)
	}
}

func TestOnboardDoesNotExecuteFocusedTask(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	planOutput, err := runFlow(t, harness, "plan", "parity", "First")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, planOutput)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(planOutput)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", planOutput)
	}
	if output, err := runFlow(t, harness, "focus", "task", match[1]); err != nil {
		t.Fatalf("focus task: %v\n%s", err, output)
	}
	if output, err := runFlow(t, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "onboard", "--format", "json")
	if err != nil {
		t.Fatalf("onboard: %v\n%s", err, output)
	}
	if !strings.Contains(output, `"description":"First"`) || !strings.Contains(output, `"status":"pending"`) {
		t.Fatalf("onboard changed focused task state: %q", output)
	}
}

func TestOnboardFailureKeepsPendingHandoff(t *testing.T) {
	for _, formatArgs := range [][]string{nil, {"--format", "json"}} {
		name := "text"
		if len(formatArgs) > 0 {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			harness := testharness.NewHarness(t, "project", "session")
			planOutput, err := runFlow(t, harness, "plan", "parity", "First")
			if err != nil {
				t.Fatalf("create plan: %v\n%s", err, planOutput)
			}
			match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(planOutput)
			if len(match) != 2 {
				t.Fatalf("plan output = %q, want task UUID", planOutput)
			}
			if output, err := runFlow(t, harness, "focus", "task", match[1]); err != nil {
				t.Fatalf("focus task: %v\n%s", err, output)
			}
			if output, err := runFlow(t, harness, "session", "dump"); err != nil {
				t.Fatalf("session dump: %v\n%s", err, output)
			}

			args := append([]string{"onboard"}, formatArgs...)
			var stderr bytes.Buffer
			code := flow.Run(context.Background(), args, flowEnv(harness), flow.Streams{
				Stdout: failingWriter{},
				Stderr: &stderr,
			})
			if code == 0 {
				t.Fatalf("onboard exit = 0, stderr = %q; want output failure", stderr.String())
			}
			resume, err := runFlow(t, harness, "session", "resume")
			if err != nil {
				t.Fatalf("session resume: %v\n%s", err, resume)
			}
			if strings.Contains(resume, "already acknowledged") {
				t.Fatalf("failed onboard acknowledged handoff: %q", resume)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
