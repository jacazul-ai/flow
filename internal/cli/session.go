package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// SessionCommand groups native session lifecycle commands.
type SessionCommand struct {
	List   SessionListCommand   `command:"list" description:"List project sessions"`
	Show   SessionShowCommand   `command:"show" description:"Show the current session"`
	Resume SessionResumeCommand `command:"resume" description:"Resume a session handoff"`
	Ack    SessionAckCommand    `command:"ack" description:"Acknowledge a session handoff"`
	Dump   SessionDumpCommand   `command:"dump" description:"Create a session handoff"`
	Purge  SessionPurgeCommand  `command:"purge" description:"Purge orphan sessions"`
}

// Execute requires a session subcommand.
func (cmd *SessionCommand) Execute(args []string) error {
	return fmt.Errorf("session requires a subcommand\nACTION: Run 'jczl-flow help session'.")
}

// SessionListCommand lists persisted sessions for the current project.
type SessionListCommand struct {
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
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListSessions(context.Background(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No sessions found.")
		return nil
	}
	fmt.Fprintln(cmd.appOpts.Out(), "SESSIONS:")
	now := time.Now().UTC()
	for _, session := range sessions {
		marker := " "
		if session.SessionID == cmd.appOpts.SessionID {
			marker = "*"
		}
		age, status := sessionAge(session.UpdatedAt, now)
		fmt.Fprintf(cmd.appOpts.Out(), "%s %s task:%s initiative:%s updated:%s age:%s status:%s\n",
			marker,
			session.SessionID,
			displayTaskID(session.FocusedTaskID),
			displayTaskID(session.InitiativeID),
			session.UpdatedAt,
			age,
			status,
		)
	}
	return nil
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

	note, found, err := store.GetSessionNote(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
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

	note, found, err := store.GetSessionNote(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
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
	if _, _, err := store.AcknowledgeSessionNote(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
		return err
	}
	fmt.Fprintln(cmd.appOpts.Out(), "Session note acknowledged. Context loaded.")
	return nil
}

// SessionDumpCommand creates a resumable note from the current focus.
type SessionDumpCommand struct {
	Force   bool `long:"force" description:"Overwrite an existing session note"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionDumpCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute persists the current focus as a session handoff note.
func (cmd *SessionDumpCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session dump accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	existing, found, err := store.GetSessionNote(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if found && !cmd.Force {
		return existingSessionNoteError(existing)
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
	note := task.SessionNote{
		ProjectID: cmd.appOpts.ProjectID,
		SessionID: cmd.appOpts.SessionID,
		Content:   content,
	}
	if err := store.SaveSessionNote(ctx, note); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Session dump written for session %s.\n\n%s", cmd.appOpts.SessionID, content)
	return nil
}

// SessionPurgeCommand removes old non-current sessions after confirmation.
type SessionPurgeCommand struct {
	Confirm bool `long:"confirm" description:"Delete orphan sessions"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *SessionPurgeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute lists orphan sessions or deletes them with --confirm.
func (cmd *SessionPurgeCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("session purge accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListSessions(context.Background(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var orphans []task.SessionInfo
	for _, session := range sessions {
		if session.SessionID == cmd.appOpts.SessionID || session.SessionID == "global" {
			continue
		}
		_, status := sessionAge(session.UpdatedAt, now)
		if status == "orphan" {
			orphans = append(orphans, session)
		}
	}
	if len(orphans) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No orphan sessions to purge.")
		return nil
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Orphan sessions (%d):\n", len(orphans))
	for _, session := range orphans {
		fmt.Fprintf(cmd.appOpts.Out(), "  %s\n", session.SessionID)
	}
	if !cmd.Confirm {
		fmt.Fprintln(cmd.appOpts.Out(), "\nDry run. Use --confirm to delete.")
		return nil
	}
	for _, session := range orphans {
		if err := store.DeleteSession(context.Background(), cmd.appOpts.ProjectID, session.SessionID); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Purged %d orphan session(s).\n", len(orphans))
	return nil
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

func existingSessionNoteError(note task.SessionNote) error {
	if strings.Contains(note.Content, "<!-- FILL IN -->") {
		return fmt.Errorf("session note already exists for session %s\nIt has an unfilled section. Fill it in instead of regenerating.\nACTION: Use --force to overwrite.", note.SessionID)
	}
	if note.AcknowledgedAt != "" || strings.Contains(note.Content, "acknowledged:") {
		return fmt.Errorf("session note already acknowledged for session %s\nACTION: Read it before replacing it, or use --force to overwrite.", note.SessionID)
	}
	return fmt.Errorf("session note already exists for session %s\nACTION: Read it before replacing it, or use --force to overwrite.", note.SessionID)
}

func renderSessionDump(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, focus task.FocusState) (string, error) {
	var output strings.Builder
	output.WriteString("# Session Handoff Note\n")
	output.WriteString("\n")
	output.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().UTC().Format("2006-01-02")))
	output.WriteString(fmt.Sprintf("**Session:** %s\n", opts.SessionID))
	output.WriteString("\n---\n\n")
	output.WriteString("## Current Focus\n\n")

	if focus.InitiativeID == "" && focus.FocusedTaskID == "" {
		output.WriteString("No focused initiative or task.\n\n")
	} else {
		if focus.InitiativeID != "" {
			output.WriteString(fmt.Sprintf("**Initiative:** `%s`\n", focus.InitiativeID))
		}
		if focus.FocusedTaskID != "" {
			current, err := store.GetTask(ctx, focus.FocusedTaskID)
			if err != nil {
				return "", err
			}
			output.WriteString(fmt.Sprintf("**Task:** `%s` %s\n", shortID(current.ID), current.Description))
		}
	}
	output.WriteString("\nRestore focus:\n\n")
	if focus.FocusedTaskID != "" {
		output.WriteString(fmt.Sprintf("jczl-flow focus task %s\n", shortID(focus.FocusedTaskID)))
	} else if focus.InitiativeID != "" {
		output.WriteString(fmt.Sprintf("jczl-flow focus plan %s\n", focus.InitiativeID))
	}
	output.WriteString("\n---\n\n## Session Notes\n\n<!-- FILL IN -->\n")
	return output.String(), nil
}
