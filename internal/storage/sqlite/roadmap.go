package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

var roadmapPhases = map[string]struct{}{
	"in-progress": {},
	"next":        {},
	"blocked":     {},
	"future":      {},
	"shipped":     {},
	"cancelled":   {},
}

// ListRoadmap returns roadmap entries for one project.
func (s *Store) ListRoadmap(ctx context.Context, projectID string) ([]task.RoadmapEntry, error) {
	if projectID == "" {
		return nil, errors.New("project ID is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, COALESCE(initiative_id, ''), phase,
		       description, status
		FROM roadmap_entries
		WHERE project_id = ?
		ORDER BY CASE phase
			WHEN 'in-progress' THEN 1
			WHEN 'next' THEN 2
			WHEN 'blocked' THEN 3
			WHEN 'future' THEN 4
			WHEN 'shipped' THEN 5
			ELSE 6 END, description
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list roadmap: %w", err)
	}
	defer rows.Close()

	var entries []task.RoadmapEntry
	for rows.Next() {
		var entry task.RoadmapEntry
		var status string
		if err := rows.Scan(
			&entry.ID,
			&entry.ProjectID,
			&entry.InitiativeID,
			&entry.Phase,
			&entry.Description,
			&status,
		); err != nil {
			return nil, fmt.Errorf("scan roadmap entry: %w", err)
		}
		entry.Status = task.Status(status)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roadmap: %w", err)
	}
	return entries, nil
}

// InitializeRoadmap creates one ledger entry per initiative.
func (s *Store) InitializeRoadmap(ctx context.Context, projectID string) error {
	entries, err := s.ListRoadmap(ctx, projectID)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return errors.New("roadmap already initialized\nACTION: Use 'jaflow roadmap show' or 'jaflow roadmap add'; do not initialize again.")
	}
	summaries, err := s.ListInitiatives(ctx, projectID, true, true)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		phase := roadmapPhase(summary)
		if err := s.AddRoadmapEntry(ctx, task.RoadmapEntry{
			ID:           newUUID(),
			ProjectID:    projectID,
			InitiativeID: summary.Initiative.ID,
			Phase:        phase,
			Description:  summary.Initiative.Name,
			Status:       task.Pending,
		}); err != nil {
			return err
		}
	}
	return nil
}

// AddRoadmapEntry adds one phase to the project ledger.
func (s *Store) AddRoadmapEntry(ctx context.Context, entry task.RoadmapEntry) error {
	if entry.ProjectID == "" || entry.Description == "" {
		return errors.New("project ID and roadmap description are required")
	}
	if _, ok := roadmapPhases[entry.Phase]; !ok {
		return fmt.Errorf("invalid roadmap phase %q", entry.Phase)
	}
	if entry.Status == "" {
		entry.Status = task.Pending
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO roadmap_entries
			(id, project_id, initiative_id, phase, description, status, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
	`, entry.ID, entry.ProjectID, entry.InitiativeID, entry.Phase,
		entry.Description, entry.Status, timestamp(), timestamp())
	if err != nil {
		return fmt.Errorf("add roadmap entry: %w", err)
	}
	return nil
}

// ShipRoadmapEntry marks a roadmap entry as shipped by ID or description.
func (s *Store) ShipRoadmapEntry(ctx context.Context, projectID string, identifier string) (task.RoadmapEntry, error) {
	if projectID == "" || strings.TrimSpace(identifier) == "" {
		return task.RoadmapEntry{}, errors.New("project ID and roadmap entry are required")
	}
	var entry task.RoadmapEntry
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, COALESCE(initiative_id, ''), phase,
		       description, status
		FROM roadmap_entries
		WHERE project_id = ? AND (id = ? OR description = ?)
	`, projectID, identifier, identifier).Scan(
		&entry.ID,
		&entry.ProjectID,
		&entry.InitiativeID,
		&entry.Phase,
		&entry.Description,
		&status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task.RoadmapEntry{}, fmt.Errorf(
			"roadmap entry %q not found\nACTION: Run 'jaflow roadmap show' to list valid entries.", identifier,
		)
	}
	if err != nil {
		return task.RoadmapEntry{}, fmt.Errorf("find roadmap entry: %w", err)
	}
	entry.Status = task.Status(status)
	if entry.Phase == "shipped" || entry.Status == task.Completed {
		return task.RoadmapEntry{}, fmt.Errorf("roadmap entry %q is already shipped", entry.Description)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE roadmap_entries
		SET phase = 'shipped', status = ?, updated_at = ?
		WHERE project_id = ? AND id = ?
	`, task.Completed, timestamp(), projectID, entry.ID)
	if err != nil {
		return task.RoadmapEntry{}, fmt.Errorf("ship roadmap entry: %w", err)
	}
	entry.Phase = "shipped"
	entry.Status = task.Completed
	return entry, nil
}

func roadmapPhase(summary task.InitiativeSummary) string {
	if summary.Initiative.Status == task.InitiativeCompleted {
		return "shipped"
	}
	if summary.Active > 0 {
		return "in-progress"
	}
	if summary.Pending > 0 {
		return "next"
	}
	return "future"
}

func validRoadmapPhase(phase string) bool {
	_, ok := roadmapPhases[strings.ToLower(strings.TrimSpace(phase))]
	return ok
}
