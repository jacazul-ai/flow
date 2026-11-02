package flow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

const migrateExport = `[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "[EXECUTE] Import task",
    "status": "pending",
    "tags": ["imported"],
    "annotations": [
      {"entry": "20260830T120000Z", "description": "DECISION: Keep UUID"}
    ]
  }
]`

const migrateExportWithRecordError = `[
  {
    "uuid": "99999999-9999-4999-8999-999999999999",
    "project": "foo",
    "description": "Valid task",
    "status": "pending"
  },
  {
    "uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    "project": "foo",
    "description": "Invalid date task",
    "status": "pending",
    "due": "not-a-date"
  }
]`

func writeExport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write export: %v", err)
	}
	return path
}

func migrateOptions(t *testing.T, content string, apply bool) flow.MigrateOptions {
	t.Helper()
	return flow.MigrateOptions{
		Source: writeExport(t, content),
		Origin: filepath.Join(t.TempDir(), "silo"),
		Apply:  apply,
	}
}

func assertNoDatabaseFiles(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read database directory: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("database directory holds %v, want nothing", names)
	}
}

func TestMigrateAPIDryRunReportsWithoutCreatingDatabase(t *testing.T) {
	env := initEnv(t, "migrate-project")

	result, err := flow.MigrateTaskwarrior(context.Background(), env, migrateOptions(t, migrateExport, false))
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if result.Applied {
		t.Fatalf("dry-run result = %+v, want not applied", result)
	}
	if result.Records.Created != 1 || result.Records.Annotations != 1 || result.Records.Tags != 1 {
		t.Fatalf("dry-run records = %+v, want one task, annotation, and tag", result.Records)
	}
	assertNoDatabaseFiles(t, env.DatabasePath)
}

func TestMigrateAPIApplyCreatesDatabaseWithOriginMark(t *testing.T) {
	env := initEnv(t, "migrate-project")
	options := migrateOptions(t, migrateExport, true)
	before := time.Now().Add(-time.Second)

	result, err := flow.MigrateTaskwarrior(context.Background(), env, options)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.Applied || result.Records.Created == 0 {
		t.Fatalf("apply result = %+v, want applied records", result)
	}

	code, stdout, stderr := run(t, context.Background(), env, "status", "parity", "--force")
	if code != 0 || !strings.Contains(stdout, "Import task") {
		t.Fatalf("status after apply = %d, stdout = %q, stderr = %q; want imported task", code, stdout, stderr)
	}

	initResult, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("Init after apply: %v", err)
	}
	if initResult.Created || initResult.Origin == nil {
		t.Fatalf("Init after apply = %+v, want an existing database with an origin", initResult)
	}
	if initResult.Origin.Path != options.Origin {
		t.Fatalf("origin path = %q, want %q", initResult.Origin.Path, options.Origin)
	}
	if initResult.Origin.MigratedAt.Before(before) || initResult.Origin.MigratedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("origin time = %v, want the apply time", initResult.Origin.MigratedAt)
	}
	assertOnlyDatabaseFiles(t, env.DatabasePath)
}

func assertOnlyDatabaseFiles(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read database directory: %v", err)
	}
	base := filepath.Base(path)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), base) || strings.Contains(entry.Name(), "migrating") {
			t.Fatalf("database directory holds %q next to %q", entry.Name(), base)
		}
	}
}

func TestInitAPIReportsNoOriginForUnmigratedDatabase(t *testing.T) {
	env := initEnv(t, "migrate-project")

	result, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if result.Origin != nil {
		t.Fatalf("origin = %+v, want nil for a database created empty", result.Origin)
	}

	cliSource := writeExport(t, migrateExport)
	if code, _, stderr := run(t, context.Background(), env, "migrate", "taskwarrior", "--source", cliSource, "--apply"); code != 0 {
		t.Fatalf("CLI apply exit = %d, stderr = %q", code, stderr)
	}
	result, err = flow.Init(context.Background(), env)
	if err != nil || result.Origin != nil {
		t.Fatalf("Init after CLI apply = %+v, %v; want no origin mark from the CLI", result, err)
	}
}

func TestMigrateAPIRefusesExistingDatabase(t *testing.T) {
	env := initEnv(t, "migrate-project")
	if _, err := flow.Init(context.Background(), env); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, apply := range []bool{false, true} {
		_, err := flow.MigrateTaskwarrior(context.Background(), env, migrateOptions(t, migrateExport, apply))
		if !errors.Is(err, flow.ErrDatabaseExists) {
			t.Fatalf("apply=%v err = %v, want ErrDatabaseExists", apply, err)
		}
		if !strings.Contains(err.Error(), env.DatabasePath) || !strings.Contains(err.Error(), "ACTION:") {
			t.Fatalf("apply=%v err = %q, want the path and an ACTION", apply, err)
		}
	}
	code, stdout, _ := run(t, context.Background(), env, "status", "parity", "--force")
	if code != 0 || strings.Contains(stdout, "Import task") {
		t.Fatalf("status = %d, %q; want the existing database untouched", code, stdout)
	}
}

func TestMigrateAPIRefusesLegacyFlowDatabase(t *testing.T) {
	home := t.TempDir()
	env := flow.Env{ProjectID: "migrate-project", SessionID: "migrate-session", Home: home}
	legacy := filepath.Join(home, "jaflow", "migrate-project", "jaflow.sqlite3")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("create legacy directory: %v", err)
	}
	store, err := sqlite.Open(context.Background(), legacy)
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	_, err = flow.MigrateTaskwarrior(context.Background(), env, migrateOptions(t, migrateExport, true))
	if !errors.Is(err, flow.ErrDatabaseExists) {
		t.Fatalf("err = %v, want ErrDatabaseExists for the legacy flow database", err)
	}
	if _, err := os.Stat(filepath.Join(home, "flow", "migrate-project", "flow.sqlite3")); !os.IsNotExist(err) {
		t.Fatalf("migration created the current database: %v", err)
	}
}

func TestMigrateAPIReturnsResultWithRecordErrors(t *testing.T) {
	for _, apply := range []bool{false, true} {
		env := initEnv(t, "migrate-project")

		result, err := flow.MigrateTaskwarrior(context.Background(), env, migrateOptions(t, migrateExportWithRecordError, apply))
		if !errors.Is(err, flow.ErrMigrationRecordErrors) || !strings.Contains(err.Error(), "ACTION:") {
			t.Fatalf("apply=%v err = %v, want ErrMigrationRecordErrors with ACTION", apply, err)
		}
		if result.Applied != apply || result.Records.Created == 0 {
			t.Fatalf("apply=%v result = %+v, want filled records", apply, result)
		}
		var hasError bool
		for _, issue := range result.Issues {
			if issue.Severity == "ERROR" && issue.Message != "" {
				hasError = true
			}
		}
		if !hasError {
			t.Fatalf("apply=%v issues = %+v, want an ERROR issue", apply, result.Issues)
		}
	}
}

func TestMigrateAPIRequiresSourceAndOrigin(t *testing.T) {
	env := initEnv(t, "migrate-project")
	source := writeExport(t, migrateExport)

	for name, options := range map[string]flow.MigrateOptions{
		"missing source": {Origin: "/silo", Apply: true},
		"missing origin": {Source: source, Apply: true},
	} {
		_, err := flow.MigrateTaskwarrior(context.Background(), env, options)
		if err == nil || !strings.Contains(err.Error(), "ACTION:") {
			t.Fatalf("%s: err = %v, want an actionable error", name, err)
		}
	}
	assertNoDatabaseFiles(t, env.DatabasePath)
}

func TestMigrateAPIFailedApplyLeavesNoDatabase(t *testing.T) {
	env := initEnv(t, "migrate-project")
	options := migrateOptions(t, `not json`, true)

	if _, err := flow.MigrateTaskwarrior(context.Background(), env, options); err == nil {
		t.Fatal("apply of an invalid export succeeded")
	}
	assertNoDatabaseFiles(t, env.DatabasePath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := flow.MigrateTaskwarrior(ctx, env, migrateOptions(t, migrateExport, true)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled apply err = %v, want context.Canceled", err)
	}
	assertNoDatabaseFiles(t, env.DatabasePath)
}

func TestMigrateAPIImportsLegacyFocusAndSessionNotes(t *testing.T) {
	env := initEnv(t, "migrate-project")
	env.SessionID = "global"
	legacyDir := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create legacy state directory: %v", err)
	}
	focus := []byte(`{
  "focused_plan": "parity",
  "focused_task_uuid": "11111111-1111-4111-8111-111111111111",
  "task_track": [{"uuid": "11111111-1111-4111-8111-111111111111", "plan": "parity"}],
  "plans_of_interest": ["parity"]
}`)
	note := []byte("Resume imported task.\n")
	if err := os.WriteFile(filepath.Join(legacyDir, "focus.json"), focus, 0o600); err != nil {
		t.Fatalf("write legacy focus: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session-note-global.md"), note, 0o600); err != nil {
		t.Fatalf("write legacy session note: %v", err)
	}
	options := migrateOptions(t, migrateExport, true)
	options.LegacyDataDir = legacyDir

	if _, err := flow.MigrateTaskwarrior(context.Background(), env, options); err != nil {
		t.Fatalf("apply with legacy state: %v", err)
	}

	code, stdout, stderr := run(t, context.Background(), env, "focus")
	if code != 0 || !strings.Contains(stdout, "Initiative: parity") || !strings.Contains(stdout, "Task: 11111111") {
		t.Fatalf("migrated focus = %d, %q, %q; want the parity anchor", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, context.Background(), env, "session", "resume")
	if code != 0 || !strings.Contains(stdout, "Resume imported task.") {
		t.Fatalf("migrated note = %d, %q, %q; want the handoff note", code, stdout, stderr)
	}
	for name, want := range map[string][]byte{"focus.json": focus, "session-note-global.md": note} {
		got, err := os.ReadFile(filepath.Join(legacyDir, name))
		if err != nil || string(got) != string(want) {
			t.Fatalf("legacy %s after migration = %q, %v; want it untouched", name, got, err)
		}
	}
}

func TestMigrateAPIWithoutLegacyDataDirImportsNoSessions(t *testing.T) {
	env := initEnv(t, "migrate-project")
	env.SessionID = "global"

	if _, err := flow.MigrateTaskwarrior(context.Background(), env, migrateOptions(t, migrateExport, true)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	code, stdout, _ := run(t, context.Background(), env, "focus")
	if code != 0 || strings.Contains(stdout, "Initiative: parity") {
		t.Fatalf("focus without legacy state = %d, %q; want no imported anchor", code, stdout)
	}
}

func TestMigrateAPIKeepsProjectsIsolated(t *testing.T) {
	home := t.TempDir()
	alpha := flow.Env{ProjectID: "project-alpha", SessionID: "migrate-session", Home: home}
	beta := flow.Env{ProjectID: "project-beta", SessionID: "migrate-session", Home: home}
	options := migrateOptions(t, migrateExport, true)

	if _, err := flow.MigrateTaskwarrior(context.Background(), alpha, options); err != nil {
		t.Fatalf("alpha apply: %v", err)
	}
	betaResult, err := flow.Init(context.Background(), beta)
	if err != nil {
		t.Fatalf("beta Init: %v", err)
	}
	if !betaResult.Created || betaResult.Origin != nil {
		t.Fatalf("beta Init = %+v, want a new database without alpha's origin", betaResult)
	}
	code, stdout, _ := run(t, context.Background(), beta, "status", "parity", "--force")
	if code != 0 || strings.Contains(stdout, "Import task") {
		t.Fatalf("beta status = %d, %q; want no alpha tasks", code, stdout)
	}
	alphaResult, err := flow.Init(context.Background(), alpha)
	if err != nil || alphaResult.Origin == nil || alphaResult.Origin.Path != options.Origin {
		t.Fatalf("alpha Init = %+v, %v; want alpha's origin intact", alphaResult, err)
	}
}
