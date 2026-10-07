package sqlite

import (
	"encoding/json"

	"github.com/jacazul-ai/flow/internal/task"
)

func encodeInitiativeMetadata(metadata task.InitiativeMetadata) (string, error) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeInitiativeMetadata(raw string) (task.InitiativeMetadata, error) {
	if raw == "" || raw == "{}" {
		return task.InitiativeMetadata{}, nil
	}
	var metadata task.InitiativeMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return task.InitiativeMetadata{}, err
	}
	return metadata, nil
}

func encodeTaskMetadata(metadata task.TaskMetadata) (string, error) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeTaskMetadata(raw string) (task.TaskMetadata, error) {
	if raw == "" || raw == "{}" {
		return task.TaskMetadata{}, nil
	}
	var metadata task.TaskMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return task.TaskMetadata{}, err
	}
	return metadata, nil
}
