package cli

import (
	"fmt"

	"github.com/jacazul-ai/flow/internal/config"
)

// CacheCommand inspects and clears derived output cache entries.
type CacheCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *CacheCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute supports cache info and scoped cache clear operations.
func (cmd *CacheCommand) Execute(args []string) error {
	if len(args) == 0 {
		args = []string{"info"}
	}
	if args[0] != "info" && cmd.Format != "" {
		return fmt.Errorf("cache %s takes no --format: it changes state\nACTION: Use --format only with 'jczl-flow cache info'.", args[0])
	}
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil && args[0] == "info" {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := cmd.appOpts.Context()
	switch args[0] {
	case "info":
		if len(args) != 1 {
			return fmt.Errorf("cache info accepts no arguments")
		}
		count, err := store.CacheEntryCount(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
		if err != nil {
			return err
		}
		if format != formatText {
			return writeReport(cmd.appOpts, format, report{command: "cache info", records: []record{{
				{"entries", count},
				{"location", cmd.appOpts.DatabasePath},
			}}})
		}
		fmt.Fprintf(cmd.appOpts.Out(), "🐊 Cache: %d file(s) in %s\n", count, cmd.appOpts.DatabasePath)
		return nil
	case "clear":
		if len(args) > 2 {
			return fmt.Errorf("cache clear accepts an optional status or ponder scope\nACTION: Run 'jczl-flow cache clear [status|ponder]'.")
		}
		prefix := ""
		if len(args) == 2 {
			switch args[1] {
			case "status":
				prefix = "status"
			case "ponder":
				prefix = "ponder"
			default:
				return fmt.Errorf("unknown cache scope %q\nACTION: Use status, ponder, or omit the scope.", args[1])
			}
		}
		if err := store.ClearCache(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID, prefix); err != nil {
			return err
		}
		label := "all"
		if prefix != "" {
			label = prefix
		}
		fmt.Fprintf(cmd.appOpts.Out(), "Cache cleared: %s\n", label)
		return nil
	default:
		return fmt.Errorf("unknown cache action %q\nACTION: Use info or clear.", args[0])
	}
}
