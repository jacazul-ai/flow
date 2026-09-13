package cli

import "testing"

func TestWithoutTaskIDsIgnoresUnknownRemovedTasks(t *testing.T) {
	remaining := withoutTaskIDs(nil, map[string]bool{"missing": true})
	if len(remaining) != 0 {
		t.Fatalf("remaining tasks = %v, want none", remaining)
	}
}
