package cli

import "github.com/jacazul-ai/flow/internal/task"

// taskTitle renders a task the way the reference tw-flow showed it, with the
// interaction mode beside the description: "[DESIGN] Design schema". A task
// without a stored mode keeps its description as written, legacy prefix
// included. Structured reports keep mode and description apart.
func taskTitle(current task.Task) string {
	if current.Mode == task.ModeUnspecified || !current.Mode.Valid() {
		return current.Description
	}
	return "[" + current.Mode.String() + "] " + current.Description
}
