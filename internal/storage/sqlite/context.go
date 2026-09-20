package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

// ListAnnotations returns annotations for one task in insertion order.
func (s *Store) ListAnnotations(ctx context.Context, taskID string) ([]task.Annotation, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, kind, body, created_at
		FROM annotations
		WHERE task_id = ?
		ORDER BY id
	`, current.ID)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()

	var annotations []task.Annotation
	for rows.Next() {
		var annotation task.Annotation
		if err := rows.Scan(
			&annotation.ID,
			&annotation.TaskID,
			&annotation.Kind,
			&annotation.Body,
			&annotation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		annotations = append(annotations, annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotations: %w", err)
	}
	return annotations, nil
}

// DeleteAnnotation removes one annotation identified by its creation timestamp.
func (s *Store) DeleteAnnotation(ctx context.Context, taskID string, createdAt string) error {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return errors.New("annotation timestamp is required")
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM annotations
		WHERE task_id = ? AND created_at = ?
	`, current.ID, createdAt)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect deleted annotation: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf(
			"no annotation found with timestamp [%s] on task %s\nACTION: Run 'jczl-flow notes %s' to list valid timestamps.",
			createdAt, shortTaskID(current.ID), shortTaskID(current.ID),
		)
	}
	return nil
}

// InheritedAnnotations returns relevant annotations from dependency ancestors.
// Dependencies are traversed recursively in dependency-first order.
func (s *Store) InheritedAnnotations(ctx context.Context, taskID string) ([]task.ContextEntry, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{current.ID: true}
	var inherited []task.ContextEntry
	for _, dependencyID := range current.Dependencies {
		entries, err := s.collectInheritedAnnotations(ctx, dependencyID, seen)
		if err != nil {
			return nil, err
		}
		inherited = append(inherited, entries...)
	}
	return inherited, nil
}

func (s *Store) collectInheritedAnnotations(ctx context.Context, taskID string, seen map[string]bool) ([]task.ContextEntry, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if seen[current.ID] {
		return nil, nil
	}
	seen[current.ID] = true

	var inherited []task.ContextEntry
	for _, dependencyID := range current.Dependencies {
		entries, err := s.collectInheritedAnnotations(ctx, dependencyID, seen)
		if err != nil {
			return nil, err
		}
		inherited = append(inherited, entries...)
	}

	annotations, err := s.ListAnnotations(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	for _, annotation := range annotations {
		if !isInheritedAnnotation(annotation.Kind) {
			continue
		}
		inherited = append(inherited, task.ContextEntry{
			TaskID:          current.ID,
			TaskDescription: current.Description,
			Annotation:      annotation,
		})
	}
	return inherited, nil
}

func isInheritedAnnotation(kind string) bool {
	switch kind {
	case "DECISION", "RESEARCH", "OUTCOME", "LESSON", "HANDOFF", "QUESTION", "HYPOTHESIS":
		return true
	default:
		return false
	}
}

func shortTaskID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
