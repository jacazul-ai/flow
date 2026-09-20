package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/task"
)

// LegacyFocusState is the JSON shape persisted by legacy FocusManager.
type LegacyFocusState struct {
	FocusedPlan     string             `json:"focused_plan"`
	FocusedIni      string             `json:"focused_ini"`
	FocusedTaskUUID string             `json:"focused_task_uuid"`
	TaskTrack       []LegacyFocusEntry `json:"task_track"`
	PlansOfInterest []string           `json:"plans_of_interest"`
	InisOfInterest  []string           `json:"inis_of_interest"`
}

// LegacyFocusEntry is one task-stack entry from a legacy focus file.
type LegacyFocusEntry struct {
	UUID string `json:"uuid"`
	Plan string `json:"plan"`
	Ini  string `json:"ini"`
}

// LegacySession contains one legacy focus file and optional handoff note.
type LegacySession struct {
	SessionID        string
	Focus            LegacyFocusState
	NoteContent      string
	NoteUpdatedAt    string
	NoteAcknowledged string
}

// LoadLegacyState reads focus files and handoff notes from an explicit directory.
func LoadLegacyState(dataDir string) ([]LegacySession, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read legacy state directory: %w", err)
	}

	focusFiles := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case name == "focus.json":
			focusFiles["global"] = filepath.Join(dataDir, name)
		case strings.HasPrefix(name, "focus-") && strings.HasSuffix(name, ".json"):
			sessionID := strings.TrimSuffix(strings.TrimPrefix(name, "focus-"), ".json")
			if sessionID != "" {
				focusFiles[sessionID] = filepath.Join(dataDir, name)
			}
		}
	}

	sessions := make([]LegacySession, 0, len(focusFiles))
	for sessionID, path := range focusFiles {
		focus, err := loadFocusFile(path)
		if err != nil {
			return nil, err
		}
		session := LegacySession{SessionID: sessionID, Focus: focus}
		if info, statErr := os.Stat(path); statErr == nil {
			session.NoteUpdatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		}
		notePath := filepath.Join(dataDir, "session-note-"+sessionID+".md")
		note, err := os.ReadFile(notePath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read legacy session note %s: %w", sessionID, err)
		}
		if err == nil {
			session.NoteContent = string(note)
			if info, statErr := os.Stat(notePath); statErr == nil {
				session.NoteUpdatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
			}
			session.NoteAcknowledged = acknowledgedAt(session.NoteContent)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// BuildBundleWithState adds legacy focus and session state to a task bundle.
func BuildBundleWithState(projectID string, source []LegacyTask, sessions []LegacySession) (task.ImportBundle, []string, error) {
	bundle, warnings, err := BuildBundle(projectID, source)
	if err != nil {
		return task.ImportBundle{}, warnings, err
	}
	initiativeIDs := make(map[string]string, len(bundle.Initiatives))
	for _, initiative := range bundle.Initiatives {
		initiativeIDs[initiative.Name] = initiative.ID
	}
	taskIDs := make(map[string]string, len(bundle.Tasks))
	for _, current := range bundle.Tasks {
		taskIDs[current.ID] = current.ID
	}

	for _, sourceSession := range sessions {
		sessionID := strings.TrimSpace(sourceSession.SessionID)
		if sessionID == "" {
			return task.ImportBundle{}, warnings, fmt.Errorf("legacy session ID cannot be empty")
		}
		focusedPlan := strings.TrimSpace(sourceSession.Focus.FocusedPlan)
		if focusedPlan == "" {
			focusedPlan = strings.TrimSpace(sourceSession.Focus.FocusedIni)
		}
		initiativeID := initiativeIDs[focusedPlan]
		if focusedPlan != "" && initiativeID == "" {
			warnings = append(warnings, fmt.Sprintf("session %s: focused initiative %q not found", sessionID, focusedPlan))
		}
		focusedTask, warning := resolveTaskReference(sourceSession.Focus.FocusedTaskUUID, taskIDs)
		if warning != "" {
			warnings = append(warnings, fmt.Sprintf("session %s: %s", sessionID, warning))
		}
		state := task.FocusState{
			ProjectID:       projectID,
			SessionID:       sessionID,
			InitiativeID:    initiativeID,
			FocusedTaskID:   focusedTask,
			TaskStack:       []task.FocusEntry{},
			PlansOfInterest: append([]string(nil), sourceSession.Focus.PlansOfInterest...),
		}
		if len(state.PlansOfInterest) == 0 {
			state.PlansOfInterest = append(state.PlansOfInterest, sourceSession.Focus.InisOfInterest...)
		}
		for _, entry := range sourceSession.Focus.TaskTrack {
			taskID, warning := resolveTaskReference(entry.UUID, taskIDs)
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("session %s: %s", sessionID, warning))
				continue
			}
			plan := strings.TrimSpace(entry.Plan)
			if plan == "" {
				plan = strings.TrimSpace(entry.Ini)
			}
			state.TaskStack = append(state.TaskStack, task.FocusEntry{
				TaskID:       taskID,
				InitiativeID: initiativeIDs[plan],
			})
		}
		bundle.Sessions = append(bundle.Sessions, task.ImportedSession{
			State:     state,
			UpdatedAt: sourceSession.NoteUpdatedAt,
		})
		if sourceSession.NoteContent != "" {
			updatedAt := sourceSession.NoteUpdatedAt
			if updatedAt == "" {
				updatedAt = migrationTimestamp()
			}
			bundle.SessionNotes = append(bundle.SessionNotes, task.ImportedSessionNote{
				ProjectID:      projectID,
				SessionID:      sessionID,
				Content:        sourceSession.NoteContent,
				AcknowledgedAt: sourceSession.NoteAcknowledged,
				UpdatedAt:      updatedAt,
			})
		}
	}
	return bundle, warnings, nil
}

func loadFocusFile(path string) (LegacyFocusState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LegacyFocusState{}, fmt.Errorf("read legacy focus file: %w", err)
	}
	var focus LegacyFocusState
	if err := json.Unmarshal(data, &focus); err != nil {
		return LegacyFocusState{}, fmt.Errorf("decode legacy focus file %s: %w", path, err)
	}
	return focus, nil
}

func acknowledgedAt(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "acknowledged:"); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveTaskReference(reference string, tasks map[string]string) (string, string) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", ""
	}
	if taskID, ok := tasks[reference]; ok {
		return taskID, ""
	}
	matches := make([]string, 0, 1)
	for taskID := range tasks {
		if strings.HasPrefix(taskID, reference) {
			matches = append(matches, taskID)
		}
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	if len(matches) > 1 {
		return "", fmt.Sprintf("ambiguous task reference %q", reference)
	}
	return "", fmt.Sprintf("task reference %q not found", reference)
}
