package task

// ImportBundle is a validated, project-scoped migration payload.
type ImportBundle struct {
	ProjectID    string
	Initiatives  []ImportedInitiative
	Tasks        []ImportedTask
	Dependencies []ImportedDependency
	Annotations  []ImportedAnnotation
	History      []HistoryEvent
	Sessions     []ImportedSession
	SessionNotes []ImportedSessionNote
}

// ImportedInitiative is an initiative record with source timestamps.
type ImportedInitiative struct {
	ID             string
	ProjectID      string
	Name           string
	Status         InitiativeStatus
	ExternalTicket string
	CreatedAt      string
	UpdatedAt      string
}

// ImportedTask is a task record with source timestamps.
type ImportedTask struct {
	ID             string
	InitiativeID   string
	Description    string
	Mode           TaskMode
	Status         Status
	Outcome        string
	ExternalTicket string
	StartedAt      string
	CompletedAt    string
	Disposition    string
	DueAt          string
	Priority       string
	Urgency        float64
	WaitUntil      string
	CreatedAt      string
	UpdatedAt      string
}

// ImportedDependency is one source dependency edge.
type ImportedDependency struct {
	TaskID      string
	DependsOnID string
}

// ImportedAnnotation is a source annotation mapped to a native semantic kind.
type ImportedAnnotation struct {
	TaskID    string
	Kind      string
	Body      string
	CreatedAt string
}

// HistoryEvent is one immutable task state transition.
type HistoryEvent struct {
	ID            string
	TaskID        string
	InitiativeID  string
	Source        string
	SourceEventID string
	Sequence      int64
	EventType     string
	Property      string
	OldValue      string
	NewValue      string
	OccurredAt    string
	Actor         string
	SessionID     string
	RecordedAt    string
}

// ImportedSession is one source focus state for a project session.
type ImportedSession struct {
	State     FocusState
	UpdatedAt string
}

// ImportedSessionNote is one source handoff note for a project session.
type ImportedSessionNote struct {
	ProjectID      string
	SessionID      string
	Content        string
	AcknowledgedAt string
	UpdatedAt      string
}

// ImportResult summarizes one native import operation.
type ImportResult struct {
	Created      int
	Updated      int
	Unchanged    int
	Dependencies int
	Annotations  int
	History      int
}
