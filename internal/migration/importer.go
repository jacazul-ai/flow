// Package migration imports legacy Taskwarrior workflow snapshots into Jaflow.
package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jacazul-ai/jaflow/internal/storage/sqlite"
	"github.com/jacazul-ai/jaflow/internal/task"
)

// LegacyTask is the subset of Taskwarrior export data used by migration.
type LegacyTask struct {
	ID             int64                `json:"id"`
	UUID           string               `json:"uuid"`
	Project        string               `json:"project"`
	Description    string               `json:"description"`
	Status         string               `json:"status"`
	Start          string               `json:"start"`
	End            string               `json:"end"`
	Entry          string               `json:"entry"`
	Modified       string               `json:"modified"`
	Due            string               `json:"due"`
	Wait           string               `json:"wait"`
	Priority       string               `json:"priority"`
	Urgency        float64              `json:"urgency"`
	ExternalTicket string               `json:"externalid"`
	Outcome        string               `json:"outcome"`
	Depends        json.RawMessage      `json:"depends"`
	Annotations    []LegacyAnnotation   `json:"annotations"`
	Tags           []string             `json:"tags"`
	Backlog        int                  `json:"backlog"`
	History        []LegacyHistoryEvent `json:"history"`
}

// LegacyHistoryEvent is a structured source history event for one task.
type LegacyHistoryEvent struct {
	SourceEventID string `json:"source_event_id"`
	Sequence      int64  `json:"sequence"`
	EventType     string `json:"event_type"`
	Property      string `json:"property"`
	OldValue      string `json:"old_value"`
	NewValue      string `json:"new_value"`
	OccurredAt    string `json:"occurred_at"`
	Actor         string `json:"actor"`
	SessionID     string `json:"session_id"`
}

// LegacyAnnotation is one Taskwarrior annotation entry.
type LegacyAnnotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

// BuildBundle validates and maps a Taskwarrior export to native records.
func BuildBundle(projectID string, source []LegacyTask) (task.ImportBundle, []string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return task.ImportBundle{}, nil, errors.New("project ID is required")
	}

	byReference := make(map[string]string, len(source)*2)
	byUUID := make(map[string]struct{}, len(source))
	for _, current := range source {
		uuid := strings.TrimSpace(current.UUID)
		if uuid == "" {
			return task.ImportBundle{}, nil, errors.New("source task UUID cannot be empty")
		}
		if _, exists := byUUID[uuid]; exists {
			return task.ImportBundle{}, nil, fmt.Errorf("duplicate source task UUID %q", uuid)
		}
		byUUID[uuid] = struct{}{}
		byReference[uuid] = uuid
		if current.ID > 0 {
			key := strconv.FormatInt(current.ID, 10)
			if previous, exists := byReference[key]; exists && previous != uuid {
				return task.ImportBundle{}, nil, fmt.Errorf("duplicate source task numeric ID %s", key)
			}
			byReference[key] = uuid
		}
	}

	warnings := make([]string, 0)
	initiativeIDs := make(map[string]string)
	initiativeNames := make([]string, 0)
	importedTasks := make([]task.ImportedTask, 0, len(source))
	annotations := make([]task.ImportedAnnotation, 0)
	historyEvents := make([]task.HistoryEvent, 0)
	for _, current := range source {
		initiativeName, discarded := sourceInitiative(current.Project)
		if _, exists := initiativeIDs[initiativeName]; !exists {
			initiativeIDs[initiativeName] = deterministicID(projectID, initiativeName)
			initiativeNames = append(initiativeNames, initiativeName)
		}

		createdAt, err := normalizeTimestamp(current.Entry)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s entry: %w", current.UUID, err)
		}
		if createdAt == "" {
			createdAt = migrationTimestamp()
		}
		updatedAt, err := normalizeTimestamp(current.Modified)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s modified: %w", current.UUID, err)
		}
		if updatedAt == "" {
			updatedAt = createdAt
		}

		description, mode, modeWarning := migrateDescription(current.Description)
		if modeWarning != "" {
			warnings = append(warnings, fmt.Sprintf("task %s: %s", current.UUID, modeWarning))
		}
		dueAt, err := normalizeDate(current.Due)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s due: %w", current.UUID, err)
		}
		waitUntil, err := normalizeDate(current.Wait)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s wait: %w", current.UUID, err)
		}
		startedAt, err := normalizeTimestamp(current.Start)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s start: %w", current.UUID, err)
		}
		completedAt, err := normalizeTimestamp(current.End)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s end: %w", current.UUID, err)
		}

		status := task.Pending
		switch strings.ToLower(strings.TrimSpace(current.Status)) {
		case "completed", "deleted":
			status = task.Completed
		case "pending", "waiting", "recurring":
			if startedAt != "" {
				status = task.Active
			}
		default:
			warnings = append(warnings, fmt.Sprintf("task %s: unknown status %q mapped to pending", current.UUID, current.Status))
		}
		disposition := ""
		if discarded || containsTag(current.Tags, "DISCARDED") || strings.EqualFold(current.Status, "deleted") {
			status = task.Completed
			disposition = "discarded"
		}

		priority := strings.ToUpper(strings.TrimSpace(current.Priority))
		if priority == "" {
			priority = "M"
		}
		if priority != "L" && priority != "M" && priority != "H" {
			warnings = append(warnings, fmt.Sprintf("task %s: unknown priority %q defaulted to M", current.UUID, current.Priority))
			priority = "M"
		}
		outcome := strings.TrimSpace(current.Outcome)
		for _, annotation := range current.Annotations {
			kind, body, warning := migrateAnnotation(annotation.Description)
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("task %s: %s", current.UUID, warning))
			}
			created, err := normalizeTimestamp(annotation.Entry)
			if err != nil {
				return task.ImportBundle{}, warnings, fmt.Errorf("task %s annotation: %w", current.UUID, err)
			}
			if created == "" {
				created = createdAt
			}
			annotations = append(annotations, task.ImportedAnnotation{
				TaskID:    current.UUID,
				Kind:      kind,
				Body:      body,
				CreatedAt: created,
			})
			if kind == "OUTCOME" && outcome == "" {
				outcome = body
			}
		}
		for _, sourceEvent := range current.History {
			if strings.TrimSpace(sourceEvent.SourceEventID) == "" || strings.TrimSpace(sourceEvent.EventType) == "" {
				return task.ImportBundle{}, warnings, fmt.Errorf("task %s history event is missing source ID or type", current.UUID)
			}
			occurredAt, err := normalizeTimestamp(sourceEvent.OccurredAt)
			if err != nil {
				return task.ImportBundle{}, warnings, fmt.Errorf("task %s history: %w", current.UUID, err)
			}
			if occurredAt == "" {
				occurredAt = createdAt
			}
			historyEvents = append(historyEvents, task.HistoryEvent{
				TaskID:        current.UUID,
				InitiativeID:  initiativeIDs[initiativeName],
				Source:        "taskchampion",
				SourceEventID: strings.TrimSpace(sourceEvent.SourceEventID),
				Sequence:      sourceEvent.Sequence,
				EventType:     strings.ToLower(strings.TrimSpace(sourceEvent.EventType)),
				Property:      strings.TrimSpace(sourceEvent.Property),
				OldValue:      sourceEvent.OldValue,
				NewValue:      sourceEvent.NewValue,
				OccurredAt:    occurredAt,
				Actor:         strings.TrimSpace(sourceEvent.Actor),
				SessionID:     strings.TrimSpace(sourceEvent.SessionID),
			})
		}
		if current.Tags != nil && len(current.Tags) > 0 {
			warnings = append(warnings, fmt.Sprintf("task %s: source tags require a native retention decision", current.UUID))
		}

		importedTasks = append(importedTasks, task.ImportedTask{
			ID:             current.UUID,
			InitiativeID:   initiativeIDs[initiativeName],
			Description:    description,
			Mode:           mode,
			Status:         status,
			Outcome:        outcome,
			ExternalTicket: strings.TrimSpace(current.ExternalTicket),
			StartedAt:      startedAt,
			CompletedAt:    completedAt,
			Disposition:    disposition,
			DueAt:          dueAt,
			Priority:       priority,
			Urgency:        current.Urgency,
			WaitUntil:      waitUntil,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		})
	}

	dependencies := make([]task.ImportedDependency, 0)
	seenDependencies := make(map[string]struct{})
	for _, current := range source {
		refs, err := dependencyReferences(current.Depends)
		if err != nil {
			return task.ImportBundle{}, warnings, fmt.Errorf("task %s dependencies: %w", current.UUID, err)
		}
		for _, reference := range refs {
			dependencyID, err := resolveReference(reference, byReference, byUUID)
			if err != nil {
				return task.ImportBundle{}, warnings, fmt.Errorf("task %s dependency %q: %w", current.UUID, reference, err)
			}
			if dependencyID == current.UUID {
				return task.ImportBundle{}, warnings, fmt.Errorf("task %s depends on itself", current.UUID)
			}
			key := current.UUID + "\x00" + dependencyID
			if _, exists := seenDependencies[key]; exists {
				continue
			}
			seenDependencies[key] = struct{}{}
			dependencies = append(dependencies, task.ImportedDependency{
				TaskID:      current.UUID,
				DependsOnID: dependencyID,
			})
		}
	}

	initiatives := make([]task.ImportedInitiative, 0, len(initiativeNames))
	for _, name := range initiativeNames {
		status := task.InitiativeActive
		allCompleted := true
		backlog := false
		for index, sourceTask := range source {
			initiativeName, _ := sourceInitiative(sourceTask.Project)
			if initiativeName != name {
				continue
			}
			if sourceTask.Status != "completed" && sourceTask.Status != "deleted" {
				allCompleted = false
			}
			if sourceTask.Backlog == 1 {
				backlog = true
			}
			_ = index
		}
		if allCompleted {
			status = task.InitiativeCompleted
		} else if backlog {
			status = task.InitiativeBacklog
		}
		initiatives = append(initiatives, task.ImportedInitiative{
			ID:        initiativeIDs[name],
			ProjectID: projectID,
			Name:      name,
			Status:    status,
			CreatedAt: migrationTimestamp(),
			UpdatedAt: migrationTimestamp(),
		})
	}

	return task.ImportBundle{
		ProjectID:    projectID,
		Initiatives:  initiatives,
		Tasks:        importedTasks,
		Dependencies: dependencies,
		Annotations:  annotations,
		History:      historyEvents,
	}, warnings, nil
}

// LoadTaskwarriorExport reads one explicit JSON export snapshot.
func LoadTaskwarriorExport(path string) ([]LegacyTask, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("source export path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source export: %w", err)
	}
	var tasks []LegacyTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("decode source export: %w", err)
	}
	return tasks, nil
}

// Importer applies validated migration bundles to a native project store.
type Importer struct {
	store *sqlite.Store
}

// NewImporter creates an importer for one native project store.
func NewImporter(store *sqlite.Store) *Importer {
	return &Importer{store: store}
}

// DryRun validates a bundle and reports the changes without writing.
func (i *Importer) DryRun(ctx context.Context, bundle task.ImportBundle) (task.ImportResult, error) {
	if err := validateBundle(bundle); err != nil {
		return task.ImportResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return task.ImportResult{}, err
	}
	return task.ImportResult{
		Created:      len(bundle.Tasks),
		Dependencies: len(bundle.Dependencies),
		Annotations:  len(bundle.Annotations),
	}, nil
}

// Apply writes a validated bundle atomically to the native store.
func (i *Importer) Apply(ctx context.Context, bundle task.ImportBundle) (task.ImportResult, error) {
	if i == nil || i.store == nil {
		return task.ImportResult{}, errors.New("migration store is required")
	}
	if err := validateBundle(bundle); err != nil {
		return task.ImportResult{}, err
	}
	return i.store.ApplyImport(ctx, bundle)
}

var modePrefix = regexp.MustCompile(`^\[([A-Z-]+)\]\s*(.*)$`)

func migrateDescription(description string) (string, task.TaskMode, string) {
	matches := modePrefix.FindStringSubmatch(strings.TrimSpace(description))
	if len(matches) == 0 {
		return strings.TrimSpace(description), task.ModeUnspecified, ""
	}
	mode, err := task.ParseTaskMode(matches[1])
	if err != nil || mode == task.ModeUnspecified {
		return strings.TrimSpace(description), task.ModeUnspecified, fmt.Sprintf("unknown mode prefix [%s] retained in description", matches[1])
	}
	return strings.TrimSpace(matches[2]), mode, ""
}

func migrateAnnotation(description string) (string, string, string) {
	kind := "NOTE"
	body := strings.TrimSpace(description)
	if prefix, remainder, ok := strings.Cut(body, ":"); ok {
		if canonical, known := task.NormalizeAnnotationKind(prefix); known {
			kind = canonical
			body = strings.TrimSpace(remainder)
		} else if strings.TrimSpace(prefix) != "" {
			body = fmt.Sprintf("[LEGACY:%s] %s", strings.TrimSpace(prefix), strings.TrimSpace(remainder))
			return kind, body, fmt.Sprintf("unknown annotation kind %q retained as NOTE", strings.TrimSpace(prefix))
		}
	}
	if body == "" {
		body = "(empty legacy annotation)"
	}
	return kind, body, ""
}

func sourceInitiative(project string) (string, bool) {
	project = strings.TrimSpace(project)
	if project == "" {
		return "unscoped", false
	}
	for _, suffix := range []string{":_archive", ":_trash", ":_deleted"} {
		if strings.HasSuffix(project, suffix) {
			name := strings.TrimSuffix(project, suffix)
			if name == "" {
				name = "unscoped"
			}
			return name, true
		}
	}
	return project, false
}

func deterministicID(projectID string, initiativeName string) string {
	digest := sha256.Sum256([]byte(projectID + "\x00" + initiativeName))
	bytes := digest[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(bytes[0:4]),
		hex.EncodeToString(bytes[4:6]),
		hex.EncodeToString(bytes[6:8]),
		hex.EncodeToString(bytes[8:10]),
		hex.EncodeToString(bytes[10:16]),
	)
}

func dependencyReferences(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("must be an array: %w", err)
	}
	refs := make([]string, 0, len(values))
	for _, value := range values {
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			if strings.TrimSpace(text) != "" {
				refs = append(refs, strings.TrimSpace(text))
			}
			continue
		}
		var number json.Number
		if err := json.Unmarshal(value, &number); err != nil {
			return nil, fmt.Errorf("invalid dependency reference %s", string(value))
		}
		refs = append(refs, number.String())
	}
	return refs, nil
}

func resolveReference(reference string, byReference map[string]string, byUUID map[string]struct{}) (string, error) {
	if uuid, ok := byReference[reference]; ok {
		return uuid, nil
	}
	matches := make([]string, 0, 1)
	for uuid := range byUUID {
		if strings.HasPrefix(uuid, reference) {
			matches = append(matches, uuid)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous source reference")
	}
	return "", fmt.Errorf("unresolved source reference")
}

func normalizeTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, layout := range []string{
		"20060102T150405Z",
		"20060102T150405",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), nil
		}
	}
	return "", fmt.Errorf("invalid timestamp %q", value)
}

func normalizeDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if timestamp, err := normalizeTimestamp(value); err == nil {
		return timestamp[:10], nil
	}
	for _, layout := range []string{"20060102", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format("2006-01-02"), nil
		}
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	switch strings.ToLower(value) {
	case "today":
		return today.Format("2006-01-02"), nil
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format("2006-01-02"), nil
	case "yesterday":
		return today.AddDate(0, 0, -1).Format("2006-01-02"), nil
	default:
		return "", fmt.Errorf("invalid date %q", value)
	}
}

func containsTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), wanted) {
			return true
		}
	}
	return false
}

func migrationTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func validateBundle(bundle task.ImportBundle) error {
	if strings.TrimSpace(bundle.ProjectID) == "" {
		return errors.New("import project ID is required")
	}
	initiativeIDs := make(map[string]struct{}, len(bundle.Initiatives))
	for _, initiative := range bundle.Initiatives {
		if initiative.ID == "" || initiative.ProjectID != bundle.ProjectID || initiative.Name == "" {
			return fmt.Errorf("invalid imported initiative %q", initiative.Name)
		}
		if _, exists := initiativeIDs[initiative.ID]; exists {
			return fmt.Errorf("duplicate imported initiative %q", initiative.ID)
		}
		initiativeIDs[initiative.ID] = struct{}{}
	}
	taskIDs := make(map[string]struct{}, len(bundle.Tasks))
	for _, current := range bundle.Tasks {
		if current.ID == "" || current.InitiativeID == "" || current.Description == "" {
			return errors.New("imported task requires ID, initiative, and description")
		}
		if _, exists := initiativeIDs[current.InitiativeID]; !exists {
			return fmt.Errorf("task %q references unknown initiative %q", current.ID, current.InitiativeID)
		}
		if _, exists := taskIDs[current.ID]; exists {
			return fmt.Errorf("duplicate imported task %q", current.ID)
		}
		if !current.Mode.Valid() {
			return fmt.Errorf("task %q has invalid mode %d", current.ID, current.Mode)
		}
		if current.Priority != "L" && current.Priority != "M" && current.Priority != "H" {
			return fmt.Errorf("task %q has invalid priority %q", current.ID, current.Priority)
		}
		taskIDs[current.ID] = struct{}{}
	}
	for _, dependency := range bundle.Dependencies {
		if dependency.TaskID == dependency.DependsOnID {
			return fmt.Errorf("task %q depends on itself", dependency.TaskID)
		}
		if _, exists := taskIDs[dependency.TaskID]; !exists {
			return fmt.Errorf("dependency task %q is not in the import", dependency.TaskID)
		}
		if _, exists := taskIDs[dependency.DependsOnID]; !exists {
			return fmt.Errorf("dependency target %q is not in the import", dependency.DependsOnID)
		}
	}
	for _, annotation := range bundle.Annotations {
		if _, exists := taskIDs[annotation.TaskID]; !exists {
			return fmt.Errorf("annotation task %q is not in the import", annotation.TaskID)
		}
		if annotation.Kind == "" || annotation.Body == "" || annotation.CreatedAt == "" {
			return fmt.Errorf("annotation for task %q is incomplete", annotation.TaskID)
		}
	}
	for _, event := range bundle.History {
		if event.TaskID == "" && event.InitiativeID == "" {
			return errors.New("history event requires task or initiative ID")
		}
		if event.TaskID != "" {
			if _, exists := taskIDs[event.TaskID]; !exists {
				return fmt.Errorf("history event task %q is not in the import", event.TaskID)
			}
		}
		if event.InitiativeID != "" {
			if _, exists := initiativeIDs[event.InitiativeID]; !exists {
				return fmt.Errorf("history event initiative %q is not in the import", event.InitiativeID)
			}
		}
		if event.Source == "" || event.SourceEventID == "" || event.EventType == "" || event.OccurredAt == "" {
			return fmt.Errorf("history event %q is incomplete", event.ID)
		}
	}
	for _, session := range bundle.Sessions {
		if session.State.ProjectID != bundle.ProjectID || session.State.SessionID == "" {
			return fmt.Errorf("session %q crosses import project boundary", session.State.SessionID)
		}
		if session.State.InitiativeID != "" {
			if _, exists := initiativeIDs[session.State.InitiativeID]; !exists {
				return fmt.Errorf("session %q references unknown initiative %q", session.State.SessionID, session.State.InitiativeID)
			}
		}
		if session.State.FocusedTaskID != "" {
			if _, exists := taskIDs[session.State.FocusedTaskID]; !exists {
				return fmt.Errorf("session %q references unknown task %q", session.State.SessionID, session.State.FocusedTaskID)
			}
		}
		for _, entry := range session.State.TaskStack {
			if _, exists := taskIDs[entry.TaskID]; !exists {
				return fmt.Errorf("session %q stack references unknown task %q", session.State.SessionID, entry.TaskID)
			}
		}
	}
	for _, note := range bundle.SessionNotes {
		if note.ProjectID != bundle.ProjectID || note.SessionID == "" || note.Content == "" || note.UpdatedAt == "" {
			return fmt.Errorf("session note %q is incomplete or crosses import project boundary", note.SessionID)
		}
	}
	return nil
}
