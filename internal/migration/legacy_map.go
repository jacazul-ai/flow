package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

// LegacyMap contains explicit overrides for legacy migration prefixes.
type LegacyMap struct {
	annotationKinds map[string]string
	taskModes       map[string]task.TaskMode
}

type legacyMapFile struct {
	Annotations map[string]*string `json:"annotations"`
	Modes       map[string]*string `json:"modes"`
}

// LoadLegacyMap reads an optional explicit migration map. An empty path or a
// JSON null document returns the default no-overrides behavior.
func LoadLegacyMap(path string) (LegacyMap, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return LegacyMap{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return LegacyMap{}, fmt.Errorf("read legacy map: %w\nACTION: Check --legacy-map and rerun the migration", err)
	}

	var document *legacyMapFile
	if err := json.Unmarshal(data, &document); err != nil {
		return LegacyMap{}, fmt.Errorf("decode legacy map: %w\nACTION: Fix --legacy-map JSON and rerun the migration", err)
	}
	if document == nil {
		return LegacyMap{}, nil
	}

	mapping := LegacyMap{
		annotationKinds: make(map[string]string),
		taskModes:       make(map[string]task.TaskMode),
	}
	if err := loadAnnotationMappings(&mapping, document.Annotations); err != nil {
		return LegacyMap{}, err
	}
	if err := loadModeMappings(&mapping, document.Modes); err != nil {
		return LegacyMap{}, err
	}
	return mapping, nil
}

func loadAnnotationMappings(mapping *LegacyMap, entries map[string]*string) error {
	seen := make(map[string]struct{}, len(entries))
	for rawPrefix, rawTarget := range entries {
		prefix, err := normalizeMapPrefix(rawPrefix, "annotation")
		if err != nil {
			return err
		}
		if _, exists := seen[prefix]; exists {
			return fmt.Errorf("legacy map has duplicate annotation prefix %q\nACTION: Keep one case-insensitive entry per prefix", rawPrefix)
		}
		seen[prefix] = struct{}{}
		if rawTarget == nil {
			continue
		}
		target, known := task.NormalizeAnnotationKind(*rawTarget)
		if !known {
			return fmt.Errorf("legacy map annotation %q targets unsupported kind %q\nACTION: Use a native annotation kind or null", rawPrefix, *rawTarget)
		}
		mapping.annotationKinds[prefix] = target
	}
	return nil
}

func loadModeMappings(mapping *LegacyMap, entries map[string]*string) error {
	seen := make(map[string]struct{}, len(entries))
	for rawPrefix, rawTarget := range entries {
		prefix, err := normalizeMapPrefix(rawPrefix, "mode")
		if err != nil {
			return err
		}
		if _, exists := seen[prefix]; exists {
			return fmt.Errorf("legacy map has duplicate mode prefix %q\nACTION: Keep one case-insensitive entry per prefix", rawPrefix)
		}
		seen[prefix] = struct{}{}
		if rawTarget == nil {
			continue
		}
		mode, err := task.ParseTaskMode(*rawTarget)
		if err != nil || mode == task.ModeUnspecified {
			return fmt.Errorf("legacy map mode %q targets unsupported mode %q\nACTION: Use a native task mode or null", rawPrefix, *rawTarget)
		}
		mapping.taskModes[prefix] = mode
	}
	return nil
}

func normalizeMapPrefix(value string, category string) (string, error) {
	prefix := strings.ToUpper(strings.TrimSpace(value))
	if prefix == "" {
		return "", fmt.Errorf("legacy map contains an empty %s prefix\nACTION: Remove the empty key or name the legacy prefix", category)
	}
	return prefix, nil
}

func (mapping LegacyMap) annotation(prefix string) (string, bool) {
	value, ok := mapping.annotationKinds[strings.ToUpper(strings.TrimSpace(prefix))]
	return value, ok
}

func (mapping LegacyMap) mode(prefix string) (task.TaskMode, bool) {
	value, ok := mapping.taskModes[strings.ToUpper(strings.TrimSpace(prefix))]
	return value, ok
}
