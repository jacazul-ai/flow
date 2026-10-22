package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ExportTaskwarrior runs the Taskwarrior binary and decodes its JSON export.
func ExportTaskwarrior(ctx context.Context, dataDir string) ([]LegacyTask, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, errors.New("Taskwarrior data directory is required\nACTION: Set --taskdata or provide a runtime home with Taskwarrior data.")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	taskPath, err := exec.LookPath("task")
	if err != nil {
		return nil, fmt.Errorf("Taskwarrior executable 'task' is unavailable: %w\nACTION: Install Taskwarrior or add 'task' to PATH, then rerun the migration", err)
	}

	command := exec.CommandContext(ctx, taskPath, "rc.data.location="+dataDir, "export")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("Taskwarrior export failed: %w: %s\nACTION: Verify the Taskwarrior data directory and rerun the migration", err, message)
		}
		return nil, fmt.Errorf("Taskwarrior export failed: %w\nACTION: Verify the Taskwarrior data directory and rerun the migration", err)
	}

	return DecodeTaskwarriorExport(stdout.Bytes())
}

// DecodeTaskwarriorExport decodes one Taskwarrior JSON export.
func DecodeTaskwarriorExport(data []byte) ([]LegacyTask, error) {
	var tasks []LegacyTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("decode Taskwarrior export: %w", err)
	}
	return tasks, nil
}
