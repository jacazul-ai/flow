package task_test

import (
	"testing"

	"github.com/jacazul-ai/flow/internal/task"
)

func TestTaskModeCatalogRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		mode task.TaskMode
		text string
	}{
		{name: "unspecified", mode: task.ModeUnspecified, text: "UNSPECIFIED"},
		{name: "design", mode: task.ModeDesign, text: "DESIGN"},
		{name: "spike", mode: task.ModeSpike, text: "SPIKE"},
		{name: "investigate", mode: task.ModeInvestigate, text: "INVESTIGATE"},
		{name: "guide", mode: task.ModeGuide, text: "GUIDE"},
		{name: "execute", mode: task.ModeExecute, text: "EXECUTE"},
		{name: "refine", mode: task.ModeRefine, text: "REFINE"},
		{name: "test", mode: task.ModeTest, text: "TEST"},
		{name: "debug", mode: task.ModeDebug, text: "DEBUG"},
		{name: "review", mode: task.ModeReview, text: "REVIEW"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.mode.String(); got != test.text {
				t.Fatalf("mode string = %q, want %q", got, test.text)
			}
			parsed, err := task.ParseTaskMode(test.text)
			if err != nil {
				t.Fatalf("parse mode: %v", err)
			}
			if parsed != test.mode {
				t.Fatalf("parsed mode = %d, want %d", parsed, test.mode)
			}
			if !parsed.Valid() {
				t.Fatal("catalog mode reported invalid")
			}
		})
	}
}

func TestParseTaskModeRejectsUnknownValue(t *testing.T) {
	if _, err := task.ParseTaskMode("UNKNOWN"); err == nil {
		t.Fatal("expected unknown task mode to fail")
	}
}
