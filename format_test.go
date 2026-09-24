package flow_test

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

// These tests hold the contract in docs/output-formats.md.

var validFormats = []string{"text", "json", "jsonl", "xml"}

func formatEnv(t *testing.T) flow.Env {
	t.Helper()
	return flow.Env{
		ProjectID:    "project-alpha",
		SessionID:    "session-alpha",
		DatabasePath: filepath.Join(t.TempDir(), "flow.sqlite3"),
		Home:         t.TempDir(),
	}
}

// seedTasks creates one initiative with two chained tasks and returns their
// full UUIDs, read from the store rather than parsed from presentation.
func seedTasks(t *testing.T, env flow.Env) []string {
	t.Helper()

	code, _, stderr := run(t, context.Background(), env, "plan", "alpha", "Alpha task", "Beta task")
	if code != 0 {
		t.Fatalf("seed plan exit = %d; stderr = %q", code, stderr)
	}
	store, err := sqlite.Open(context.Background(), env.DatabasePath)
	if err != nil {
		t.Fatalf("open seeded store: %v", err)
	}
	defer store.Close()

	tasks, err := store.ListTasks(context.Background(), env.ProjectID, "alpha")
	if err != nil {
		t.Fatalf("list seeded tasks: %v", err)
	}
	ids := make([]string, 0, len(tasks))
	for _, current := range tasks {
		ids = append(ids, current.ID)
	}
	return ids
}

// decodeEnvelope requires stdout to be exactly one JSON document: no banner,
// no cache prose and no trailing text around it.
func decodeEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\nstdout = %q", err, stdout)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout carries content after the JSON document: %q", stdout)
	}
	return envelope
}

func requireEnvelope(t *testing.T, envelope map[string]any, command string, env flow.Env) {
	t.Helper()

	if version, _ := envelope["format_version"].(float64); version != 1 {
		t.Fatalf("format_version = %v, want 1", envelope["format_version"])
	}
	if envelope["command"] != command {
		t.Fatalf("command = %v, want %q", envelope["command"], command)
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %v, want an object", envelope["meta"])
	}
	if meta["project_id"] != env.ProjectID {
		t.Fatalf("meta.project_id = %v, want %q", meta["project_id"], env.ProjectID)
	}
	if meta["session_id"] != env.SessionID {
		t.Fatalf("meta.session_id = %v, want %q", meta["session_id"], env.SessionID)
	}
	if generated, _ := meta["generated_at"].(string); generated == "" {
		t.Fatalf("meta.generated_at = %v, want a timestamp", meta["generated_at"])
	}
	if _, ok := meta["cached"].(bool); !ok {
		t.Fatalf("meta.cached = %v, want a boolean", meta["cached"])
	}
	if _, ok := envelope["records"].([]any); !ok {
		t.Fatalf("records = %v, want an array", envelope["records"])
	}
}

func records(t *testing.T, envelope map[string]any) []map[string]any {
	t.Helper()

	raw, _ := envelope["records"].([]any)
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("record = %v, want an object", item)
		}
		result = append(result, record)
	}
	return result
}

func recordsMention(t *testing.T, recs []map[string]any, id string) bool {
	t.Helper()

	for _, record := range recs {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("encode record: %v", err)
		}
		if strings.Contains(string(encoded), id) {
			return true
		}
	}
	return false
}

func sortedKeys(record map[string]any) []string {
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestFormatDefaultsToText(t *testing.T) {
	env := formatEnv(t)
	seedTasks(t, env)

	code, stdout, stderr := run(t, context.Background(), env, "status")
	if code != 0 {
		t.Fatalf("exit = %d; stderr = %q", code, stderr)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("stdout = %q, want text when no format is selected", stdout)
	}
	if !strings.Contains(stdout, "Alpha task") {
		t.Fatalf("stdout = %q, want the text status", stdout)
	}
}

func TestFormatSelectionPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		envFormat string
		args      []string
		wantJSON  bool
	}{
		{name: "flag selects json", args: []string{"status", "--format", "json"}, wantJSON: true},
		{name: "env selects json", envFormat: "json", args: []string{"status"}, wantJSON: true},
		{name: "flag text beats env json", envFormat: "json", args: []string{"status", "--format", "text"}},
		{name: "flag json beats env text", envFormat: "text", args: []string{"status", "--format", "json"}, wantJSON: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := formatEnv(t)
			seedTasks(t, env)
			env.Format = test.envFormat

			code, stdout, stderr := run(t, context.Background(), env, test.args...)
			if code != 0 {
				t.Fatalf("exit = %d; stderr = %q", code, stderr)
			}
			if test.wantJSON {
				requireEnvelope(t, decodeEnvelope(t, stdout), "status", env)
				return
			}
			if json.Valid([]byte(stdout)) || !strings.Contains(stdout, "Alpha task") {
				t.Fatalf("stdout = %q, want text", stdout)
			}
		})
	}
}

func TestEnvFromOSReadsFormat(t *testing.T) {
	t.Setenv("JACAZUL_FLOW_FORMAT", "jsonl")

	if got := flow.EnvFromOS().Format; got != "jsonl" {
		t.Fatalf("EnvFromOS().Format = %q, want %q", got, "jsonl")
	}
}

// Run resolves the format from Env and the flag only. The process
// environment belongs to EnvFromOS, and a format preference gets no
// exception to that boundary.
func TestRunIgnoresFormatInProcessEnvironment(t *testing.T) {
	env := formatEnv(t)
	seedTasks(t, env)
	t.Setenv("JACAZUL_FLOW_FORMAT", "json")

	code, stdout, stderr := run(t, context.Background(), env, "status")
	if code != 0 {
		t.Fatalf("exit = %d; stderr = %q", code, stderr)
	}
	if json.Valid([]byte(stdout)) {
		t.Fatalf("stdout = %q, want text: Run must not read the process environment", stdout)
	}
}

func TestFormatRejectsUnknownValues(t *testing.T) {
	tests := []struct {
		name      string
		envFormat string
		args      []string
	}{
		{name: "flag", args: []string{"status", "--format", "yaml"}},
		{name: "env", envFormat: "yaml", args: []string{"status"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := formatEnv(t)
			seedTasks(t, env)
			env.Format = test.envFormat

			code, stdout, stderr := run(t, context.Background(), env, test.args...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stdout = %q", code, stdout)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty on failure", stdout)
			}
			if !strings.Contains(stderr, "yaml") || !strings.Contains(stderr, "ACTION:") {
				t.Fatalf("stderr = %q, want the rejected value and ACTION guidance", stderr)
			}
			for _, format := range validFormats {
				if !strings.Contains(stderr, format) {
					t.Fatalf("stderr = %q, want the valid format %q named", stderr, format)
				}
			}
		})
	}
}

func TestJSONEnvelopeCarriesTaskRecords(t *testing.T) {
	env := formatEnv(t)
	ids := seedTasks(t, env)

	code, stdout, stderr := run(t, context.Background(), env, "status", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d; stderr = %q", code, stderr)
	}
	envelope := decodeEnvelope(t, stdout)
	requireEnvelope(t, envelope, "status", env)

	recs := records(t, envelope)
	if len(recs) != len(ids) {
		t.Fatalf("records = %d, want %d", len(recs), len(ids))
	}
	for _, id := range ids {
		if !recordsMention(t, recs, id) {
			t.Fatalf("records do not carry the full task UUID %s: %v", id, recs)
		}
	}
	if cached := envelope["meta"].(map[string]any)["cached"]; cached != false {
		t.Fatalf("meta.cached = %v, want false on a fresh render", cached)
	}
}

func TestJSONLCarriesTheSameContentAsJSON(t *testing.T) {
	env := formatEnv(t)
	seedTasks(t, env)

	code, jsonOut, stderr := run(t, context.Background(), env, "status", "--format", "json", "--force")
	if code != 0 {
		t.Fatalf("json exit = %d; stderr = %q", code, stderr)
	}
	want := records(t, decodeEnvelope(t, jsonOut))

	code, stdout, stderr := run(t, context.Background(), env, "status", "--format", "jsonl", "--force")
	if code != 0 {
		t.Fatalf("jsonl exit = %d; stderr = %q", code, stderr)
	}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != len(want)+1 {
		t.Fatalf("jsonl lines = %d, want one meta line and %d records; stdout = %q", len(lines), len(want), stdout)
	}

	var head map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &head); err != nil {
		t.Fatalf("meta line is not valid JSON: %v; line = %q", err, lines[0])
	}
	if _, ok := head["records"]; ok {
		t.Fatalf("meta line = %q, want no records array: records follow one per line", lines[0])
	}
	head["records"] = []any{}
	requireEnvelope(t, head, "status", env)

	for index, line := range lines[1:] {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("record line %d is not valid JSON on its own: %v; line = %q", index+1, err, line)
		}
		if !slices.Equal(sortedKeys(record), sortedKeys(want[index])) {
			t.Fatalf("jsonl record fields = %v, want the json field set %v", sortedKeys(record), sortedKeys(want[index]))
		}
	}
}

// xmlNode is a generic element tree, enough to compare the XML framing with
// the JSON content without fixing a record schema in this test.
type xmlNode struct {
	XMLName  xml.Name
	Children []xmlNode `xml:",any"`
	Text     string    `xml:",chardata"`
}

func (node xmlNode) child(name string) (xmlNode, bool) {
	for _, child := range node.Children {
		if child.XMLName.Local == name {
			return child, true
		}
	}
	return xmlNode{}, false
}

func TestXMLCarriesTheSameContentAsJSON(t *testing.T) {
	env := formatEnv(t)
	seedTasks(t, env)

	code, jsonOut, stderr := run(t, context.Background(), env, "status", "--format", "json", "--force")
	if code != 0 {
		t.Fatalf("json exit = %d; stderr = %q", code, stderr)
	}
	want := records(t, decodeEnvelope(t, jsonOut))

	code, stdout, stderr := run(t, context.Background(), env, "status", "--format", "xml", "--force")
	if code != 0 {
		t.Fatalf("xml exit = %d; stderr = %q", code, stderr)
	}
	var root xmlNode
	decoder := xml.NewDecoder(strings.NewReader(stdout))
	if err := decoder.Decode(&root); err != nil {
		t.Fatalf("stdout is not one XML document: %v\nstdout = %q", err, stdout)
	}
	if rest := stdout[decoder.InputOffset():]; strings.TrimSpace(rest) != "" {
		t.Fatalf("stdout carries content after the XML document: %q", rest)
	}

	for _, name := range []string{"format_version", "command", "meta", "records"} {
		if _, ok := root.child(name); !ok {
			t.Fatalf("xml root has no <%s> element; stdout = %q", name, stdout)
		}
	}
	if version, _ := root.child("format_version"); strings.TrimSpace(version.Text) != "1" {
		t.Fatalf("<format_version> = %q, want 1", version.Text)
	}
	meta, _ := root.child("meta")
	for _, name := range []string{"project_id", "session_id", "generated_at", "cached"} {
		if _, ok := meta.child(name); !ok {
			t.Fatalf("xml <meta> has no <%s> element; stdout = %q", name, stdout)
		}
	}

	recs, _ := root.child("records")
	if len(recs.Children) != len(want) {
		t.Fatalf("xml records = %d, want %d", len(recs.Children), len(want))
	}
	for index, record := range recs.Children {
		names := make([]string, 0, len(record.Children))
		for _, field := range record.Children {
			names = append(names, field.XMLName.Local)
		}
		sort.Strings(names)
		if !slices.Equal(slices.Compact(names), sortedKeys(want[index])) {
			t.Fatalf("xml record fields = %v, want the json field set %v", names, sortedKeys(want[index]))
		}
		for _, field := range record.Children {
			requireSameValue(t, field, want[index][field.XMLName.Local])
		}
	}
}

// requireSameValue compares one XML element with the JSON value of the same
// field: a list by its item count, a scalar by its text.
func requireSameValue(t *testing.T, element xmlNode, want any) {
	t.Helper()

	name := element.XMLName.Local
	if list, ok := want.([]any); ok {
		if len(element.Children) != len(list) {
			t.Fatalf("xml <%s> has %d items, want %d as in json", name, len(element.Children), len(list))
		}
		return
	}
	text := strings.TrimSpace(element.Text)
	var expected string
	switch typed := want.(type) {
	case string:
		expected = typed
	case bool:
		expected = strconv.FormatBool(typed)
	case float64:
		expected = strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return
	}
	if text != expected {
		t.Fatalf("xml <%s> = %q, want %q as in json", name, text, expected)
	}
}
