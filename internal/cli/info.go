package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/jacazul-ai/flow/internal/config"
)

// InfoCommand reports the resolved runtime context without opening storage.
type InfoCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies resolved runtime options to the command.
func (cmd *InfoCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders runtime paths, sources, and existence checks.
func (cmd *InfoCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("info accepts no arguments")
	}
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	info := runtimeInfoRecord(cmd.appOpts, workingDirectory)
	if format != formatText {
		return writeReport(cmd.appOpts, format, report{
			command: "info",
			records: []record{info},
		})
	}
	printRuntimeInfo(cmd.appOpts.Out(), info)
	return nil
}

func runtimeInfoRecord(opts *config.AppOptions, workingDirectory string) record {
	runtimeFormat := opts.Runtime.Format
	if runtimeFormat == "" {
		runtimeFormat = formatText
	}
	return record{
		{"project_id", opts.ProjectID},
		{"project_id_source", sourceOrDefault(opts.Runtime.ProjectSource)},
		{"session_id", opts.SessionID},
		{"session_id_source", sourceOrDefault(opts.Runtime.SessionSource)},
		{"home", opts.Home},
		{"home_source", sourceOrDefault(opts.Runtime.HomeSource)},
		{"home_exists", pathExists(opts.Home)},
		{"taskdata", opts.TaskData},
		{"taskdata_source", sourceOrDefault(opts.Runtime.TaskDataSource)},
		{"taskdata_exists", pathExists(opts.TaskData)},
		{"database_path", opts.DatabasePath},
		{"database_source", sourceOrDefault(opts.Runtime.DatabaseSource)},
		{"database_exists", pathExists(opts.DatabasePath)},
		{"legacy_database_path", opts.LegacyDatabasePath},
		{"legacy_database_source", legacyDatabaseSource(opts)},
		{"legacy_database_exists", pathExists(opts.LegacyDatabasePath)},
		{"runtime_format", runtimeFormat},
		{"runtime_format_source", sourceOrDefault(opts.Runtime.FormatSource)},
		{"working_directory", workingDirectory},
		{"version", versionOrDev(opts.Runtime.Version)},
	}
}

func sourceOrDefault(source string) string {
	if source == "" {
		return "default"
	}
	return source
}

func legacyDatabaseSource(opts *config.AppOptions) string {
	if opts.LegacyDatabasePath == "" {
		return "not-derived"
	}
	return "derived"
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func versionOrDev(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

func printRuntimeInfo(out io.Writer, info record) {
	fmt.Fprintln(out, "RUNTIME INFO")
	for _, current := range info {
		fmt.Fprintf(out, "%s: %v\n", current.name, current.value)
	}
}
