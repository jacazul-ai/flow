package cli

import (
	"testing"
	"time"
)

func TestSessionAgeClassifiesHeartbeatWindows(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		age      time.Duration
		wantAge  string
		wantStat string
	}{
		{name: "current", age: 0, wantAge: "0s", wantStat: "active"},
		{name: "active boundary", age: 7199 * time.Second, wantAge: "1h", wantStat: "active"},
		{name: "idle boundary", age: 2 * time.Hour, wantAge: "2h", wantStat: "idle"},
		{name: "idle", age: 4 * time.Hour, wantAge: "4h", wantStat: "idle"},
		{name: "orphan boundary", age: 8 * time.Hour, wantAge: "8h", wantStat: "orphan"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updatedAt := now.Add(-test.age).Format(time.RFC3339Nano)
			age, status := sessionAge(updatedAt, now)
			if age != test.wantAge || status != test.wantStat {
				t.Fatalf("sessionAge(%q) = (%q, %q), want (%q, %q)",
					updatedAt, age, status, test.wantAge, test.wantStat)
			}
		})
	}
}

func TestSessionAgeRejectsInvalidTimestamp(t *testing.T) {
	age, status := sessionAge("not-a-timestamp", time.Time{})
	if age != "?" || status != "unknown" {
		t.Fatalf("invalid session timestamp = (%q, %q), want (?, unknown)", age, status)
	}
}
