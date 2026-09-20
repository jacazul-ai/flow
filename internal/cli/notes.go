package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// NoteCommand adds or deletes structured task context.
type NoteCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *NoteCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute adds an annotation or deletes one by creation timestamp.
func (cmd *NoteCommand) Execute(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("note requires a task UUID, type, and message\nACTION: Run 'jczl-flow help note'.")
	}

	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	kind := strings.TrimSpace(args[1])
	if isAnnotationDelete(kind) {
		if len(args) != 3 {
			return fmt.Errorf("note delete requires one annotation timestamp\nACTION: Run 'jczl-flow notes %s' to list valid timestamps.", shortID(current.ID))
		}
		if err := store.DeleteAnnotation(context.Background(), current.ID, args[2]); err != nil {
			return err
		}
		if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
			return err
		}
		fmt.Printf("Deleted annotation [%s] from task %s\n", args[2], shortID(current.ID))
		return nil
	}

	canonical, ok := task.NormalizeAnnotationKind(kind)
	if !ok {
		return fmt.Errorf(
			"invalid note type: %q\nACTION: Use one of the allowed semantic types: %s",
			kind, allowedAnnotationKinds,
		)
	}
	body := strings.TrimSpace(strings.Join(args[2:], " "))
	if body == "" {
		return errorsForEmptyNote()
	}
	if err := store.AddAnnotation(context.Background(), current.ID, canonical, body); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Added %s note to task %s\n", canonical, shortID(current.ID))
	return nil
}

// NotesCommand lists all annotations attached to one task.
type NotesCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *NotesCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders annotations with their creation timestamps.
func (cmd *NotesCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("notes requires exactly one task UUID\nACTION: Run 'jczl-flow help notes'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	annotations, err := store.ListAnnotations(context.Background(), current.ID)
	if err != nil {
		return err
	}
	if len(annotations) == 0 {
		fmt.Printf("No annotations on task %s.\n", shortID(current.ID))
		return nil
	}
	fmt.Printf("══ Notes for task %s ══\n", shortID(current.ID))
	for _, annotation := range annotations {
		fmt.Printf("  [%s] %s: %s\n", annotation.CreatedAt, annotation.Kind, annotation.Body)
	}
	return nil
}

// ContextCommand renders direct and inherited context for one task.
type ContextCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *ContextCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders the task context and dependency-inherited annotations.
func (cmd *ContextCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("context requires exactly one task UUID\nACTION: Run 'jczl-flow help context'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	direct, err := store.ListAnnotations(context.Background(), current.ID)
	if err != nil {
		return err
	}
	inherited, err := store.InheritedAnnotations(context.Background(), current.ID)
	if err != nil {
		return err
	}

	var output strings.Builder
	fmt.Fprintf(&output, "CONTEXT FOR TASK %s: %s\n", shortID(current.ID), current.Description)
	writeDirectContext(&output, direct)
	writeInheritedContext(&output, inherited)
	if len(direct) == 0 && len(inherited) == 0 {
		output.WriteString("No context recorded.\n")
	}
	fmt.Print(output.String())
	return nil
}

const allowedAnnotationKinds = "AC, BLOCKED, DECISION, HANDOFF, HYPOTHESIS, LESSON, LINK, NOTE, OUTCOME, QUESTION, RESEARCH"

func isAnnotationDelete(kind string) bool {
	return strings.EqualFold(kind, "delete") || strings.EqualFold(kind, "del")
}

func errorsForEmptyNote() error {
	return fmt.Errorf("note message cannot be empty\nACTION: Provide a message after the semantic type.")
}

func clearTaskCaches(store *sqlite.Store, opts *config.AppOptions, current task.Task) error {
	ctx := context.Background()
	if err := store.ClearCacheKey(ctx, opts.ProjectID, opts.SessionID, "status"); err != nil {
		return err
	}
	if current.InitiativeName != "" {
		if err := store.ClearCacheKey(ctx, opts.ProjectID, opts.SessionID, "status_"+current.InitiativeName); err != nil {
			return err
		}
	}
	if err := store.ClearCache(ctx, opts.ProjectID, opts.SessionID, "ponder"); err != nil {
		return err
	}
	return store.ClearCache(ctx, opts.ProjectID, opts.SessionID, "plans")
}

func writeDirectContext(writer io.Writer, annotations []task.Annotation) {
	if len(annotations) == 0 {
		return
	}
	fmt.Fprintln(writer, "DIRECT CONTEXT:")
	for _, annotation := range annotations {
		fmt.Fprintf(writer, "  - %s: %s\n", annotation.Kind, annotation.Body)
	}
}

func writeInheritedContext(writer io.Writer, entries []task.ContextEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintln(writer, "══ INHERITED CONTEXT ══")
	for _, entry := range entries {
		fmt.Fprintf(writer, "Task (%s) [%s]:\n", shortID(entry.TaskID), entry.TaskDescription)
		fmt.Fprintf(writer, "  - %s: %s\n", entry.Annotation.Kind, entry.Annotation.Body)
	}
}
