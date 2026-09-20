package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestOnboardPresentsHandoffBeforeFocusAndAcknowledgesIt(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project", "session")

	planOutput, err := runFlow(t, binary, harness, "plan", "parity", "First")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, planOutput)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(planOutput)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", planOutput)
	}
	if output, err := runFlow(t, binary, harness, "focus", "task", match[1]); err != nil {
		t.Fatalf("focus task: %v\n%s", err, output)
	}
	if output, err := runFlow(t, binary, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}

	output, err := runFlow(t, binary, harness, "onboard")
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

	resume, err := runFlow(t, binary, harness, "session", "resume")
	if err != nil {
		t.Fatalf("resume after onboard: %v\n%s", err, resume)
	}
	if !strings.Contains(resume, "already acknowledged") {
		t.Fatalf("resume after onboard = %q, want acknowledgement", resume)
	}
}

func TestOnboardUsesPonderWhenFocusIsEmpty(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runFlow(t, binary, harness, "onboard")
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
