// Package sqlite provides the provisional local project workflow store.
package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacazul-ai/jaflow/internal/task"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Store persists one project's workflow state in SQLite.
type Store struct {
	db *sql.DB
}

// Open opens or creates a database and applies the current schema.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// GetOrCreateInitiative returns an initiative by project and name.
func (s *Store) GetOrCreateInitiative(ctx context.Context, input task.CreateInitiativeInput) (task.Initiative, error) {
	if err := validateInitiative(input); err != nil {
		return task.Initiative{}, err
	}

	initiative, err := s.findInitiative(ctx, input.ProjectID, input.Name)
	if err == nil {
		return initiative, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return task.Initiative{}, fmt.Errorf("find initiative: %w", err)
	}

	created := task.Initiative{
		ID:             newUUID(),
		ProjectID:      input.ProjectID,
		Name:           input.Name,
		Status:         task.InitiativeActive,
		ExternalTicket: input.ExternalTicket,
	}
	now := timestamp()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO initiatives
			(id, project_id, name, status, external_ticket, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, created.ID, created.ProjectID, created.Name, created.Status,
		created.ExternalTicket, now, now)
	if err != nil {
		return task.Initiative{}, fmt.Errorf("create initiative: %w", err)
	}
	if err := s.AppendHistoryEvent(ctx, task.HistoryEvent{
		InitiativeID:  created.ID,
		EventType:     "create",
		Property:      "name",
		NewValue:      created.Name,
		OccurredAt:    now,
	}); err != nil {
		return task.Initiative{}, err
	}
	return created, nil
}

// CreateTask persists a pending task and its dependency edges.
func (s *Store) CreateTask(ctx context.Context, input task.CreateTaskInput) (task.Task, error) {
	if err := validateTask(input); err != nil {
		return task.Task{}, err
	}
	if err := s.requireInitiative(ctx, input.InitiativeID); err != nil {
		return task.Task{}, err
	}
	if err := s.requireDependencies(ctx, input.Dependencies); err != nil {
		return task.Task{}, err
	}
	if !input.Mode.Valid() {
		return task.Task{}, fmt.Errorf("invalid task mode %d", input.Mode)
	}

	priority := strings.ToUpper(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = "M"
	}
	if priority != "L" && priority != "M" && priority != "H" {
		return task.Task{}, fmt.Errorf("invalid task priority %q", input.Priority)
	}

	created := task.Task{
		ID:           newUUID(),
		InitiativeID: input.InitiativeID,
		Description:  input.Description,
		Mode:         input.Mode,
		DueAt:        input.DueAt,
		Priority:     priority,
		Urgency:      input.Urgency,
		WaitUntil:    input.WaitUntil,
		Status:       task.Pending,
		Dependencies: append([]string(nil), input.Dependencies...),
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.Task{}, fmt.Errorf("begin task transaction: %w", err)
	}
	defer tx.Rollback()

	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks
			(id, initiative_id, description, mode, status, outcome,
			 external_ticket, priority, urgency, wait_until, due_at,
			 task_mode_code, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?, ?, ?)
	`, created.ID, created.InitiativeID, created.Description, legacyModeName(created.Mode),
		created.Status, created.Priority, created.Urgency, created.WaitUntil,
		created.DueAt, created.Mode, now, now); err != nil {
		return task.Task{}, fmt.Errorf("create task: %w", err)
	}
	for _, dependencyID := range created.Dependencies {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_dependencies (task_id, depends_on_id)
			VALUES (?, ?)
		`, created.ID, dependencyID); err != nil {
			return task.Task{}, fmt.Errorf("create dependency: %w", err)
		}
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       created.ID,
		InitiativeID: created.InitiativeID,
		EventType:    "create",
		Property:     "description",
		NewValue:     created.Description,
		OccurredAt:   now,
	}); err != nil {
		return task.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit task: %w", err)
	}
	return created, nil
}

// ListTasks returns project tasks, optionally filtered by initiative name.
func (s *Store) ListTasks(ctx context.Context, projectID string, initiativeName string) ([]task.Task, error) {
	if projectID == "" {
		return nil, errors.New("project ID is required")
	}

	query := `
		SELECT t.id, t.initiative_id, i.name, t.description,
		       t.status, t.outcome, t.external_ticket,
		       t.started_at, t.completed_at, t.disposition, t.due_at,
		       t.priority, t.urgency, t.wait_until, t.task_mode_code
		FROM tasks t
		JOIN initiatives i ON i.id = t.initiative_id
		WHERE i.project_id = ?
	`
	args := []any{projectID}
	if initiativeName != "" {
		query += " AND i.name = ?"
		args = append(args, initiativeName)
	}
	query += " ORDER BY t.created_at, t.id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []task.Task
	for rows.Next() {
		var current task.Task
		var status string
		var modeCode int64
		if err := rows.Scan(
			&current.ID,
			&current.InitiativeID,
			&current.InitiativeName,
			&current.Description,
			&status,
			&current.Outcome,
			&current.ExternalTicket,
			&current.StartedAt,
			&current.CompletedAt,
			&current.Disposition,
			&current.DueAt,
			&current.Priority,
			&current.Urgency,
			&current.WaitUntil,
			&modeCode,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		current.Status = task.Status(status)
		current.Mode = task.TaskMode(modeCode)
		tasks = append(tasks, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task rows: %w", err)
	}
	for index := range tasks {
		dependencies, err := s.dependencies(ctx, tasks[index].ID)
		if err != nil {
			return nil, err
		}
		tasks[index].Dependencies = dependencies
	}
	return tasks, nil
}

func (s *Store) configure(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure SQLite database: %w", err)
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	migrationFS, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		s.db,
		migrationFS,
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}

func findInitiativeQuery() string {
	return `
		SELECT id, project_id, name, status, external_ticket
		FROM initiatives
		WHERE project_id = ? AND name = ?
	`
}

func (s *Store) findInitiative(ctx context.Context, projectID string, name string) (task.Initiative, error) {
	var initiative task.Initiative
	var status string
	err := s.db.QueryRowContext(ctx, findInitiativeQuery(), projectID, name).Scan(
		&initiative.ID,
		&initiative.ProjectID,
		&initiative.Name,
		&status,
		&initiative.ExternalTicket,
	)
	initiative.Status = task.InitiativeStatus(status)
	return initiative, err
}

func (s *Store) requireInitiative(ctx context.Context, initiativeID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM initiatives WHERE id = ?", initiativeID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("initiative %q not found", initiativeID)
	}
	if err != nil {
		return fmt.Errorf("read initiative: %w", err)
	}
	return nil
}

func (s *Store) requireDependencies(ctx context.Context, dependencies []string) error {
	for _, dependencyID := range dependencies {
		if dependencyID == "" {
			return errors.New("dependency ID cannot be empty")
		}
		var exists int
		err := s.db.QueryRowContext(ctx,
			"SELECT 1 FROM tasks WHERE id = ?", dependencyID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("dependency %q not found", dependencyID)
		}
		if err != nil {
			return fmt.Errorf("read dependency: %w", err)
		}
	}
	return nil
}

func (s *Store) dependencies(ctx context.Context, taskID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT depends_on_id
		FROM task_dependencies
		WHERE task_id = ?
		ORDER BY depends_on_id
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list dependencies: %w", err)
	}
	defer rows.Close()

	var dependencies []string
	for rows.Next() {
		var dependencyID string
		if err := rows.Scan(&dependencyID); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		dependencies = append(dependencies, dependencyID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dependencies: %w", err)
	}
	return dependencies, nil
}

func validateInitiative(input task.CreateInitiativeInput) error {
	if input.ProjectID == "" {
		return errors.New("project ID is required")
	}
	if input.Name == "" {
		return errors.New("initiative name is required")
	}
	return nil
}

func validateTask(input task.CreateTaskInput) error {
	if input.InitiativeID == "" {
		return errors.New("initiative ID is required")
	}
	if input.Description == "" {
		return errors.New("task description is required")
	}
	return nil
}

func legacyModeName(mode task.TaskMode) string {
	if mode == task.ModeUnspecified {
		return ""
	}
	return mode.String()
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate workflow UUID: %v", err))
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16],
	)
}
