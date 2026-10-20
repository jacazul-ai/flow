package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// SessionCommand groups native session lifecycle commands.
type SessionCommand struct {
	List     SessionListCommand     `command:"list" description:"List project sessions"`
	Show     SessionShowCommand     `command:"show" description:"Show the current session"`
	Resume   SessionResumeCommand   `command:"resume" description:"Resume a session handoff"`
	Ack      SessionAckCommand      `command:"ack" description:"Acknowledge a session handoff"`
	Dump     SessionDumpCommand     `command:"dump" description:"Create a session handoff draft"`
	Finalize SessionFinalizeCommand `command:"finalize" description:"Finalize a session dump"`
	Dumps    SessionDumpsCommand    `command:"dumps" description:"List session dumps"`
	Purge    SessionPurgeCommand    `command:"purge" description:"Purge orphan sessions"`
}

// Execute requires a session subcommand.
func (cmd *SessionCommand) Execute(args []string) error {
	return fmt.Errorf("session requires a subcommand\nACTION: Run 'jczl-flow help session'.")
}

// SessionListCommand lists persisted sessions for the current project.
type SessionListCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionListCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute lists sessions, anchors, age, and activity status.
func (cmd *SessionListCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session list accepts no arguments")
	}
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListSessions(cmd.appOpts.Context(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if format != formatText {
		records := make([]record, 0, len(sessions))
		for _, session := range sessions {
			age, status := sessionAge(session.UpdatedAt, now)
			records = append(records, record{
				{"session_id", session.SessionID},
				{"current", session.SessionID == cmd.appOpts.SessionID},
				{"task_id", session.FocusedTaskID},
				{"initiative_id", session.InitiativeID},
				{"plan", session.InitiativeName},
				{"task", displaySessionTaskID(session.FocusedTaskID)},
				{"updated_at", session.UpdatedAt},
				{"age", age},
				{"status", status},
				{"handoff", session.Handoff},
			})
		}
		return writeReport(cmd.appOpts, format, report{command: "session list", records: records})
	}
	if len(sessions) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No sessions found.")
		return nil
	}
	writer := tabwriter.NewWriter(cmd.appOpts.Out(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SESSION\tPLAN\tTASK\tAGE\tSTATUS\tHANDOFF")
	for _, session := range sessions {
		marker := "  "
		if session.SessionID == cmd.appOpts.SessionID {
			marker = "* "
		}
		age, status := sessionAge(session.UpdatedAt, now)
		fmt.Fprintf(writer, "%s%s\t%s\t%s\t%s\t%s\t%s\n",
			marker,
			session.SessionID,
			displaySessionValue(session.InitiativeName),
			displaySessionTaskID(session.FocusedTaskID),
			age,
			status,
			displaySessionValue(session.Handoff),
		)
	}
	return writer.Flush()
}

// SessionShowCommand shows the current session focus.
type SessionShowCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionShowCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute displays the current session focus.
func (cmd *SessionShowCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session show accepts no arguments")
	}
	return (&FocusShowCommand{appOpts: cmd.appOpts}).Execute(nil)
}

// SessionResumeCommand displays a pending session handoff.
type SessionResumeCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionResumeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute prints a pending note or reports that it was acknowledged.
func (cmd *SessionResumeCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session resume accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	dump, found, err := store.DeliverLatestSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(cmd.appOpts.Out(), "📋 SESSION HANDOFF — session %s\n\n%s", cmd.appOpts.SessionID, dump.Content)
		return nil
	}
	latest, found, err := store.GetLatestSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if found && latest.Status == task.SessionDumpRead {
		fmt.Fprintf(cmd.appOpts.Out(), "Session dump already acknowledged: session %s. Context will not be replayed.\n", cmd.appOpts.SessionID)
		return nil
	}
	if found && latest.Status == task.SessionDumpDraft {
		return fmt.Errorf("session dump %s is still a draft\nACTION: Finalize it with session finalize before resuming.", shortSessionDumpID(latest.ID))
	}
	note, found, err := store.GetSessionNote(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil || !found {
		return err
	}
	if note.AcknowledgedAt != "" {
		fmt.Fprintf(cmd.appOpts.Out(), "Session note already acknowledged: session %s. Context will not be replayed.\n", cmd.appOpts.SessionID)
		return nil
	}
	fmt.Fprintf(cmd.appOpts.Out(), "📋 SESSION HANDOFF — session %s\n\n%s", cmd.appOpts.SessionID, note.Content)
	return nil
}

// SessionAckCommand acknowledges a session handoff note.
type SessionAckCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionAckCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute records an acknowledgement marker on the current session note.
func (cmd *SessionAckCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session ack accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	if _, found, err := store.AcknowledgeLatestSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
		return err
	} else if found {
		fmt.Fprintln(cmd.appOpts.Out(), "Session dump acknowledged. Context loaded.")
		return nil
	}
	latest, found, err := store.GetLatestSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if found && latest.Status == task.SessionDumpDraft {
		return fmt.Errorf("session dump %s is still a draft\nACTION: Finalize it with session finalize before acknowledging.", shortSessionDumpID(latest.ID))
	}
	note, found, err := store.GetSessionNote(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(cmd.appOpts.Out(), "No session note found.")
		return nil
	}
	if note.AcknowledgedAt != "" {
		fmt.Fprintln(cmd.appOpts.Out(), "Session note already acknowledged.")
		return nil
	}
	if _, _, err := store.AcknowledgeSessionNote(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
		return err
	}
	fmt.Fprintln(cmd.appOpts.Out(), "Session note acknowledged. Context loaded.")
	return nil
}

// SessionDumpCommand creates a draft from the current focus.
type SessionDumpCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionDumpCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute persists the current focus as a session handoff draft.
func (cmd *SessionDumpCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session dump accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := cmd.appOpts.Context()
	latest, found, err := store.GetLatestSessionDump(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if found && latest.Status != task.SessionDumpRead {
		return fmt.Errorf("session dump already exists for session %s and is not acknowledged\nACTION: Read, finalize, and acknowledge the pending dump before creating another.", cmd.appOpts.SessionID)
	}
	focus, err := store.LoadFocus(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if err := store.SaveFocus(ctx, focus); err != nil {
		return err
	}
	content, err := renderSessionDump(ctx, store, cmd.appOpts, focus)
	if err != nil {
		return err
	}
	focusSummary, err := sessionFocusSummary(ctx, store, cmd.appOpts.ProjectID, focus)
	if err != nil {
		return err
	}
	dump, err := store.CreateSessionDumpDraft(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID, focusSummary, content)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Session dump draft %s created for session %s.\n\n%s", shortSessionDumpID(dump.ID), cmd.appOpts.SessionID, content)
	return nil
}

// SessionFinalizeCommand seals a draft after the agent supplies its sections.
type SessionFinalizeCommand struct {
	References string `long:"references" description:"Relevant tickets, commits, files, or links"`
	Actions    string `long:"actions" description:"Ordered next actions for the dump"`
	Notes      string `long:"notes" description:"Additional dump notes"`
	appOpts    *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options.
func (cmd *SessionFinalizeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute finalizes one draft dump.
func (cmd *SessionFinalizeCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("session finalize requires a dump UUID")
	}
	if strings.TrimSpace(cmd.Actions) == "" {
		return fmt.Errorf("session dump Actions cannot be empty\nACTION: Provide --actions with the ordered next commands.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	dump, found, err := store.GetSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, args[0])
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("session dump %q not found", args[0])
	}
	content := dumpContent(string(dump.Content), cmd.References, cmd.Actions, cmd.Notes)
	finalized, err := store.FinalizeSessionDump(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, dump.ID, content)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Session dump %s finalized and pending acknowledgement.\n", shortSessionDumpID(finalized.ID))
	return nil
}

// SessionDumpsCommand lists the current session's dump metadata.
type SessionDumpsCommand struct {
	All     bool `long:"all" description:"Include history outside the current session state"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options.
func (cmd *SessionDumpsCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute lists dumps without loading their payloads.
func (cmd *SessionDumpsCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session dumps accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	var dumps []task.SessionDump
	if cmd.All {
		dumps, err = store.ListAllSessionDumps(cmd.appOpts.Context(), cmd.appOpts.ProjectID)
	} else {
		dumps, err = store.ListSessionDumps(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	}
	if err != nil {
		return err
	}
	if len(dumps) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No session dumps found.")
		return nil
	}
	for _, dump := range dumps {
		if cmd.All {
			fmt.Fprintf(cmd.appOpts.Out(), "%s %s %s %s %s\n", dump.CreatedAt, dump.SessionID, displaySessionDumpStatus(dump.Status), shortSessionDumpID(dump.ID), dump.FocusSummary)
			continue
		}
		fmt.Fprintf(cmd.appOpts.Out(), "%s %s %s %s\n", dump.CreatedAt, displaySessionDumpStatus(dump.Status), shortSessionDumpID(dump.ID), dump.FocusSummary)
	}
	return nil
}

// SessionPurgeCommand marks orphan sessions or hard-deletes purged history.
type SessionPurgeCommand struct {
	Hard    bool `long:"hard" description:"Physically delete logically purged sessions"`
	Confirm bool `long:"confirm" description:"Confirm the selected purge operation"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionPurgeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute applies logical purge by default and reserves physical deletion for --hard --confirm.
func (cmd *SessionPurgeCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session purge accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListAllSessions(cmd.appOpts.Context(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var targets []task.SessionInfo
	for _, session := range sessions {
		if session.SessionID == cmd.appOpts.SessionID || session.SessionID == "global" {
			continue
		}
		if cmd.Hard {
			if session.State == "purged" {
				targets = append(targets, session)
			}
			continue
		}
		if session.State == "active" {
			_, status := sessionAge(session.UpdatedAt, now)
			if status == "orphan" {
				targets = append(targets, session)
			}
		}
	}
	if len(targets) == 0 {
		if cmd.Hard {
			fmt.Fprintln(cmd.appOpts.Out(), "No logically purged orphan sessions to hard-purge.")
		} else {
			fmt.Fprintln(cmd.appOpts.Out(), "No orphan sessions to purge.")
		}
		return nil
	}

	if cmd.Hard {
		fmt.Fprintf(cmd.appOpts.Out(), "Logically purged sessions (%d):\n", len(targets))
	} else {
		fmt.Fprintf(cmd.appOpts.Out(), "Orphan sessions (%d):\n", len(targets))
	}
	for _, session := range targets {
		fmt.Fprintf(cmd.appOpts.Out(), "  %s\n", session.SessionID)
	}
	if cmd.Hard {
		if !cmd.Confirm {
			fmt.Fprintln(cmd.appOpts.Out(), "\nDry run. Use --hard --confirm to delete permanently.")
			return nil
		}
		for _, session := range targets {
			if err := store.HardDeletePurgedSession(cmd.appOpts.Context(), cmd.appOpts.ProjectID, session.SessionID, cmd.appOpts.SessionID); err != nil {
				return err
			}
		}
		fmt.Fprintf(cmd.appOpts.Out(), "Hard-purged %d session(s).\n", len(targets))
		return nil
	}
	for _, session := range targets {
		if err := store.PurgeSession(cmd.appOpts.Context(), cmd.appOpts.ProjectID, session.SessionID, cmd.appOpts.SessionID); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Marked %d orphan session(s) purged.\n", len(targets))
	return nil
}

func displaySessionValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func displaySessionTaskID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "-"
	}
	return shortID(id)
}

func sessionAge(updatedAt string, now time.Time) (string, string) {
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return "?", "unknown"
	}
	seconds := now.Sub(updated).Seconds()
	if seconds < 0 {
		seconds = 0
	}
	switch {
	case seconds < 7200:
		return formatAge(seconds), "active"
	case seconds < 28800:
		return formatAge(seconds), "idle"
	default:
		return formatAge(seconds), "orphan"
	}
}

func formatAge(seconds float64) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", int(seconds))
	case seconds < 3600:
		return fmt.Sprintf("%dm", int(seconds/60))
	case seconds < 86400:
		return fmt.Sprintf("%dh", int(seconds/3600))
	default:
		return fmt.Sprintf("%dd", int(seconds/86400))
	}
}

func sessionFocusSummary(ctx context.Context, store *sqlite.Store, projectID string, focus task.FocusState) (string, error) {
	if focus.FocusedTaskID != "" {
		current, err := store.GetTask(ctx, focus.FocusedTaskID)
		if err != nil {
			return "", err
		}
		if current.InitiativeName != "" {
			return current.InitiativeName, nil
		}
		return shortSessionDumpID(focus.FocusedTaskID), nil
	}
	if focus.InitiativeID != "" {
		initiative, err := store.FindInitiativeByID(ctx, projectID, focus.InitiativeID)
		if err != nil {
			return "", err
		}
		return initiative.Name, nil
	}
	return "no focus", nil
}

func shortSessionDumpID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func displaySessionDumpStatus(status task.SessionDumpStatus) string {
	if status == task.SessionDumpRead {
		return "acknowledged"
	}
	return string(status)
}

func dumpContent(content string, references string, actions string, notes string) string {
	references = sectionValue(references, "No additional references recorded.")
	notes = sectionValue(notes, "No additional notes recorded.")
	content = strings.Replace(content, "<!-- FILL IN REFERENCES -->", references, 1)
	content = strings.Replace(content, "<!-- FILL IN ACTIONS -->", strings.TrimSpace(actions), 1)
	content = strings.Replace(content, "<!-- FILL IN NOTES -->", notes, 1)
	if strings.Contains(content, "<!-- FILL IN -->") {
		body := strings.TrimSpace(actions)
		if notes != "No additional notes recorded." {
			body += "\n\n" + notes
		}
		content = strings.Replace(content, "<!-- FILL IN -->", body, 1)
	}
	return strings.TrimRight(content, "\n") + "\n"
}

func sectionValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func renderSessionDump(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, focus task.FocusState) (string, error) {
	var output strings.Builder
	output.WriteString("# Session Handoff Note\n\n")
	output.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().UTC().Format("2006-01-02")))
	output.WriteString(fmt.Sprintf("**Session:** %s\n\n---\n\n", opts.SessionID))
	output.WriteString("## Focus\n\n")

	var focusedTask task.Task
	if focus.FocusedTaskID != "" {
		current, err := store.GetTask(ctx, focus.FocusedTaskID)
		if err != nil {
			return "", err
		}
		focusedTask = current
		output.WriteString(fmt.Sprintf("**Plan:** `%s`\n", current.InitiativeName))
		output.WriteString(fmt.Sprintf("**Task:** `%s` %s\n", shortID(current.ID), current.Description))
	} else if focus.InitiativeID != "" {
		initiative, err := store.FindInitiativeByID(ctx, opts.ProjectID, focus.InitiativeID)
		if err != nil {
			return "", err
		}
		output.WriteString(fmt.Sprintf("**Plan:** `%s`\n", initiative.Name))
	} else {
		output.WriteString("No focused plan or task.\n")
	}
	output.WriteString("\n**Restore focus:**\n\n")
	if focus.FocusedTaskID != "" {
		output.WriteString(fmt.Sprintf("jczl-flow focus task %s\n", shortID(focus.FocusedTaskID)))
	} else if focus.InitiativeID != "" {
		output.WriteString(fmt.Sprintf("jczl-flow focus plan %s\n", focus.InitiativeID))
	} else {
		output.WriteString("No focus command.\n")
	}

	output.WriteString("\n---\n\n## References\n\n")
	if focusedTask.ExternalTicket != "" {
		output.WriteString(fmt.Sprintf("**Ticket:** `%s`\n\n", focusedTask.ExternalTicket))
	}
	output.WriteString("<!-- FILL IN REFERENCES -->\n\n")
	output.WriteString("## Actions\n\n<!-- FILL IN ACTIONS -->\n\n")
	output.WriteString("## Notes\n\n<!-- FILL IN NOTES -->\n")
	return output.String(), nil
}
