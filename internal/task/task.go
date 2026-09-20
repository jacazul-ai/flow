package task

import (
	"fmt"
	"strings"
)

// Status describes the current state of a workflow task.
type Status string

const (
	Pending   Status = "pending"
	Active    Status = "active"
	Completed Status = "completed"
)

// TaskMode identifies the workflow mode of a task.
type TaskMode uint16

const (
	ModeUnspecified TaskMode = 0
	ModeDesign      TaskMode = 1
	ModeSpike       TaskMode = 2
	ModeInvestigate TaskMode = 3
	ModeGuide       TaskMode = 4
	ModeExecute     TaskMode = 5
	ModeRefine      TaskMode = 6
	ModeTest        TaskMode = 7
	ModeDebug       TaskMode = 8
	ModeReview      TaskMode = 9
)

var taskModeNames = map[TaskMode]string{
	ModeUnspecified: "UNSPECIFIED",
	ModeDesign:      "DESIGN",
	ModeSpike:       "SPIKE",
	ModeInvestigate: "INVESTIGATE",
	ModeGuide:       "GUIDE",
	ModeExecute:     "EXECUTE",
	ModeRefine:      "REFINE",
	ModeTest:        "TEST",
	ModeDebug:       "DEBUG",
	ModeReview:      "REVIEW",
}

// String returns the stable catalog name for a task mode.
func (mode TaskMode) String() string {
	if name, ok := taskModeNames[mode]; ok {
		return name
	}
	return "UNKNOWN"
}

// Valid reports whether mode is part of the built-in task mode catalog.
func (mode TaskMode) Valid() bool {
	_, ok := taskModeNames[mode]
	return ok
}

// ParseTaskMode converts a catalog name to its stable task mode code.
func ParseTaskMode(value string) (TaskMode, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ModeUnspecified, nil
	}
	for mode, name := range taskModeNames {
		if name == value {
			return mode, nil
		}
	}
	return ModeUnspecified, fmt.Errorf("invalid task mode %q", value)
}

// Annotation is a structured piece of durable workflow context.
type Annotation struct {
	ID        int64  `json:"id"`
	TaskID    string `json:"task_id"`
	Kind      string `json:"kind"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// ContextEntry associates inherited context with its source task.
type ContextEntry struct {
	TaskID          string
	TaskDescription string
	Annotation      Annotation
}

var annotationKinds = map[string]string{
	"research":   "RESEARCH",
	"r":          "RESEARCH",
	"decision":   "DECISION",
	"d":          "DECISION",
	"outcome":    "OUTCOME",
	"o":          "OUTCOME",
	"handoff":    "HANDOFF",
	"h":          "HANDOFF",
	"blocked":    "BLOCKED",
	"b":          "BLOCKED",
	"lesson":     "LESSON",
	"l":          "LESSON",
	"question":   "QUESTION",
	"q":          "QUESTION",
	"hypothesis": "HYPOTHESIS",
	"y":          "HYPOTHESIS",
	"ac":         "AC",
	"a":          "AC",
	"note":       "NOTE",
	"n":          "NOTE",
	"link":       "LINK",
}

// NormalizeAnnotationKind converts a semantic note alias to its canonical kind.
func NormalizeAnnotationKind(kind string) (string, bool) {
	canonical, ok := annotationKinds[strings.ToLower(strings.TrimSpace(kind))]
	return canonical, ok
}

// InitiativeStatus describes the lifecycle of an initiative.
type InitiativeStatus string

const (
	InitiativeActive    InitiativeStatus = "active"
	InitiativeBacklog   InitiativeStatus = "backlog"
	InitiativeCompleted InitiativeStatus = "completed"
	InitiativeArchived  InitiativeStatus = "archived"
)

// Initiative is a first-class workflow aggregate for a project.
type Initiative struct {
	ID             string
	ProjectID      string
	Name           string
	Status         InitiativeStatus
	ExternalTicket string
}

// CreateInitiativeInput contains the fields required to create or find an initiative.
type CreateInitiativeInput struct {
	ProjectID      string
	Name           string
	ExternalTicket string
}

// Task is the local workflow representation shared by task backends.
type Task struct {
	ID             string       `json:"id"`
	Position       int64        `json:"position"`
	InitiativeID   string       `json:"initiative_id"`
	InitiativeName string       `json:"initiative_name"`
	Description    string       `json:"description"`
	Mode           TaskMode     `json:"mode,omitempty"`
	Status         Status       `json:"status"`
	Outcome        string       `json:"outcome,omitempty"`
	ExternalTicket string       `json:"external_ticket,omitempty"`
	StartedAt      string       `json:"started_at,omitempty"`
	CompletedAt    string       `json:"completed_at,omitempty"`
	Disposition    string       `json:"disposition,omitempty"`
	DueAt          string       `json:"due_at,omitempty"`
	Priority       string       `json:"priority,omitempty"`
	Urgency        float64      `json:"urgency,omitempty"`
	WaitUntil      string       `json:"wait_until,omitempty"`
	Dependencies   []string     `json:"dependencies,omitempty"`
	Annotations    []Annotation `json:"annotations,omitempty"`

	// ProjectID and Plan are compatibility fields for the legacy adapter.
	ProjectID string `json:"project_id,omitempty"`
	Plan      string `json:"plan,omitempty"`
}

// FocusState stores the current navigation anchor for one project session.
type FocusState struct {
	ProjectID       string
	SessionID       string
	InitiativeID    string
	FocusedTaskID   string
	TaskStack       []FocusEntry
	PlansOfInterest []string
}

// RoadmapEntry is one strategic ledger phase for a project initiative.
type RoadmapEntry struct {
	ID           string
	ProjectID    string
	InitiativeID string
	Phase        string
	Description  string
	Status       Status
}

// InitiativeSummary contains dashboard counts for one initiative.
type InitiativeSummary struct {
	Initiative Initiative
	Pending    int
	Active     int
	Completed  int
	Blocked    int
}

// SessionInfo describes one persisted project session.
type SessionInfo struct {
	ProjectID     string
	SessionID     string
	InitiativeID  string
	FocusedTaskID string
	UpdatedAt     string
}

// SessionNote stores a resumable handoff note for one project session.
type SessionNote struct {
	ProjectID      string
	SessionID      string
	Content        string
	AcknowledgedAt string
	UpdatedAt      string
}

// FocusEntry records one task in the focus stack.
type FocusEntry struct {
	TaskID       string `json:"task_id"`
	InitiativeID string `json:"initiative_id"`
}

// CreateTaskInput contains the fields required to create a workflow task.
type CreateTaskInput struct {
	InitiativeID string
	Description  string
	Mode         TaskMode
	DueAt        string
	Priority     string
	Urgency      float64
	WaitUntil    string
	Dependencies []string

	// ProjectID and Plan are compatibility fields for the legacy adapter.
	ProjectID string
	Plan      string
}

// TaskMetadataUpdate contains optional task fields that may be amended.
type TaskMetadataUpdate struct {
	Description    *string
	ExternalTicket *string
}
