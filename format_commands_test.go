package flow_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
)

// reportCommands is the closed list from docs/output-formats.md. Each entry
// names the canonical command reported in the envelope; TASK is replaced
// with a seeded task UUID.
var reportCommands = []struct {
	command string
	args    []string
}{
	{command: "status", args: []string{"status"}},
	{command: "ponder", args: []string{"ponder"}},
	{command: "plans", args: []string{"plans"}},
	{command: "plans", args: []string{"inis"}},
	{command: "plans", args: []string{"initiatives"}},
	{command: "tree", args: []string{"tree"}},
	{command: "next", args: []string{"next"}},
	{command: "active", args: []string{"active"}},
	{command: "blocked", args: []string{"blocked"}},
	{command: "overdue", args: []string{"overdue"}},
	{command: "history", args: []string{"history", "initiative", "alpha"}},
	{command: "context", args: []string{"context", "TASK"}},
	{command: "notes", args: []string{"notes", "TASK"}},
	{command: "focus", args: []string{"focus"}},
	{command: "session list", args: []string{"session", "list"}},
	{command: "roadmap show", args: []string{"roadmap", "show"}},
	{command: "cache info", args: []string{"cache", "info"}},
	{command: "onboard", args: []string{"onboard"}},
}

func withTask(args []string, id string) []string {
	resolved := make([]string, len(args))
	for index, arg := range args {
		if arg == "TASK" {
			arg = id
		}
		resolved[index] = arg
	}
	return resolved
}

func TestEveryReportCommandRendersTheEnvelope(t *testing.T) {
	for _, report := range reportCommands {
		t.Run(strings.Join(report.args, " "), func(t *testing.T) {
			env := formatEnv(t)
			ids := seedTasks(t, env)

			args := append(withTask(report.args, ids[0]), "--format", "json")
			code, stdout, stderr := run(t, context.Background(), env, args...)
			if code != 0 {
				t.Fatalf("exit = %d; stderr = %q", code, stderr)
			}
			requireEnvelope(t, decodeEnvelope(t, stdout), report.command, env)
		})
	}
}

func TestEnvFormatAppliesToReportCommands(t *testing.T) {
	for _, report := range reportCommands {
		t.Run(strings.Join(report.args, " "), func(t *testing.T) {
			env := formatEnv(t)
			ids := seedTasks(t, env)
			env.Format = "json"

			code, stdout, stderr := run(t, context.Background(), env, withTask(report.args, ids[0])...)
			if code != 0 {
				t.Fatalf("exit = %d; stderr = %q", code, stderr)
			}
			requireEnvelope(t, decodeEnvelope(t, stdout), report.command, env)
		})
	}
}

// Everything outside the report list is a state change, guidance or an
// artifact. It takes no --format flag, and an injected default format does
// not change what it prints.
var nonReportCommands = [][]string{
	{"help"},
	{"commit"},
	{"plan", "beta", "Gamma task"},
	{"ini", "beta", "Gamma task"},
	{"initiative", "beta", "Gamma task"},
	{"note", "TASK", "decision", "Keep text."},
	{"execute", "TASK"},
	{"focus", "ind", "task", "TASK"},
	{"session", "dump"},
	{"roadmap", "init"},
	{"cache", "clear"},
}

func TestNonReportCommandsRejectTheFormatFlag(t *testing.T) {
	for _, args := range nonReportCommands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := formatEnv(t)
			ids := seedTasks(t, env)

			withFlag := append(withTask(args, ids[0]), "--format", "json")
			code, stdout, stderr := run(t, context.Background(), env, withFlag...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stdout = %q", code, stdout)
			}
			if !strings.Contains(stderr, "format") || !strings.Contains(stderr, "ACTION:") {
				t.Fatalf("stderr = %q, want the rejected flag and ACTION guidance", stderr)
			}
		})
	}
}

func TestEnvFormatDoesNotChangeNonReportCommands(t *testing.T) {
	for _, args := range nonReportCommands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			textEnv := formatEnv(t)
			textIDs := seedTasks(t, textEnv)
			_, want, _ := run(t, context.Background(), textEnv, withTask(args, textIDs[0])...)

			jsonEnv := formatEnv(t)
			jsonIDs := seedTasks(t, jsonEnv)
			jsonEnv.Format = "json"
			_, got, _ := run(t, context.Background(), jsonEnv, withTask(args, jsonIDs[0])...)

			if json.Valid([]byte(got)) && strings.TrimSpace(got) != "" {
				t.Fatalf("stdout = %q, want text: a default format applies only to report commands", got)
			}
			if shape(got, jsonEnv) != shape(want, textEnv) {
				t.Fatalf("stdout with an injected format = %q, want the same text as without it: %q", got, want)
			}
		})
	}
}

// Identifiers differ between two seeded fixtures and between two runs that
// create tasks, so they are masked before two outputs are compared as text.
var (
	fullID  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	shortID = regexp.MustCompile(`\b[0-9a-f]{8}\b`)
)

// shape masks the paths and identifiers that differ between two fixtures.
func shape(output string, env flow.Env) string {
	output = strings.ReplaceAll(output, filepath.Dir(env.DatabasePath), "<db>")
	output = strings.ReplaceAll(output, env.Home, "<home>")
	output = fullID.ReplaceAllString(output, "<id>")
	return shortID.ReplaceAllString(output, "<id>")
}

// The cache fact survives in structured output as a field that carries the
// cached records, instead of being suppressed with the terminal prose.
func TestCachedReportsSetTheCacheFact(t *testing.T) {
	for _, command := range []string{"status", "plans", "ponder"} {
		t.Run(command, func(t *testing.T) {
			env := formatEnv(t)
			seedTasks(t, env)

			code, first, stderr := run(t, context.Background(), env, command, "--format", "json")
			if code != 0 {
				t.Fatalf("first exit = %d; stderr = %q", code, stderr)
			}
			code, second, stderr := run(t, context.Background(), env, command, "--format", "json")
			if code != 0 {
				t.Fatalf("second exit = %d; stderr = %q", code, stderr)
			}

			fresh := decodeEnvelope(t, first)
			cached := decodeEnvelope(t, second)
			requireEnvelope(t, cached, command, env)

			freshMeta := fresh["meta"].(map[string]any)
			cachedMeta := cached["meta"].(map[string]any)
			if freshMeta["cached"] != false {
				t.Fatalf("first meta.cached = %v, want false", freshMeta["cached"])
			}
			if cachedMeta["cached"] != true {
				t.Fatalf("second meta.cached = %v, want true", cachedMeta["cached"])
			}
			if since, ok := cachedMeta["unchanged_since"]; !ok || since == nil || since == "" {
				t.Fatalf("meta.unchanged_since = %v, want it set on a cached response", since)
			}
			if !reflect.DeepEqual(cached["records"], fresh["records"]) {
				t.Fatalf("cached records = %v, want the records of the fresh render %v", cached["records"], fresh["records"])
			}
		})
	}
}

// A text cache entry must never leak into a structured response, and a
// structured entry must never leak into text.
func TestCacheDoesNotCrossFormats(t *testing.T) {
	t.Run("text then json", func(t *testing.T) {
		env := formatEnv(t)
		ids := seedTasks(t, env)

		if code, _, stderr := run(t, context.Background(), env, "status"); code != 0 {
			t.Fatalf("text exit = %d; stderr = %q", code, stderr)
		}
		code, stdout, stderr := run(t, context.Background(), env, "status", "--format", "json")
		if code != 0 {
			t.Fatalf("json exit = %d; stderr = %q", code, stderr)
		}
		envelope := decodeEnvelope(t, stdout)
		requireEnvelope(t, envelope, "status", env)
		if !recordsMention(t, records(t, envelope), ids[0]) {
			t.Fatalf("records = %v, want the task records after a text render", envelope["records"])
		}
	})

	t.Run("json then text", func(t *testing.T) {
		env := formatEnv(t)
		seedTasks(t, env)

		if code, _, stderr := run(t, context.Background(), env, "status", "--format", "json"); code != 0 {
			t.Fatalf("json exit = %d; stderr = %q", code, stderr)
		}
		code, stdout, stderr := run(t, context.Background(), env, "status")
		if code != 0 {
			t.Fatalf("text exit = %d; stderr = %q", code, stderr)
		}
		if json.Valid([]byte(stdout)) || !strings.Contains(stdout, "Alpha task") {
			t.Fatalf("stdout = %q, want the full text status: a json render must not warm the text cache", stdout)
		}
	})
}

// Errors stay on stderr in text with their ACTION guidance whatever format
// is selected; stdout stays empty so a parser never reads a half document.
func TestStructuredFormatsKeepErrorsInText(t *testing.T) {
	for _, format := range []string{"json", "jsonl", "xml"} {
		t.Run(format, func(t *testing.T) {
			env := formatEnv(t)
			seedTasks(t, env)

			code, stdout, stderr := run(t, context.Background(), env,
				"context", "00000000-0000-0000-0000-000000000000", "--format", format)
			if code != 1 {
				t.Fatalf("exit = %d, want 1", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty on failure", stdout)
			}
			if !strings.HasPrefix(stderr, "ERROR:") {
				t.Fatalf("stderr = %q, want a text ERROR line", stderr)
			}
			if strings.Contains(stderr, "unknown flag") {
				t.Fatalf("stderr = %q, want the command's own failure, not a rejected --format", stderr)
			}
		})
	}
}

// A state change invalidates structured cache entries exactly like text
// ones; a stale cached=true after a write would hide the change.
func TestStateChangesInvalidateStructuredCache(t *testing.T) {
	for _, command := range []string{"status", "plans", "ponder"} {
		t.Run(command, func(t *testing.T) {
			env := formatEnv(t)
			ids := seedTasks(t, env)

			if code, _, stderr := run(t, context.Background(), env, command, "--format", "json"); code != 0 {
				t.Fatalf("first exit = %d; stderr = %q", code, stderr)
			}
			if code, _, stderr := run(t, context.Background(), env, "note", ids[0], "decision", "Invalidate the cache."); code != 0 {
				t.Fatalf("note exit = %d; stderr = %q", code, stderr)
			}
			code, stdout, stderr := run(t, context.Background(), env, command, "--format", "json")
			if code != 0 {
				t.Fatalf("second exit = %d; stderr = %q", code, stderr)
			}
			meta := decodeEnvelope(t, stdout)["meta"].(map[string]any)
			if meta["cached"] != false {
				t.Fatalf("meta.cached = %v after a state change, want false", meta["cached"])
			}
		})
	}
}
