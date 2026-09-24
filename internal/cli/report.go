package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// Output formats for report commands. docs/output-formats.md is the contract.
const (
	formatText  = "text"
	formatJSON  = "json"
	formatJSONL = "jsonl"
	formatXML   = "xml"

	// formatVersion increments when a field changes meaning or is removed.
	formatVersion = 1

	// structuredCacheSuffix keeps structured cache entries apart from text
	// entries while staying under the same key prefix, so prefix clears reach
	// both.
	structuredCacheSuffix = "#records"
)

// ReportFormat is the --format flag shared by report commands. It is
// embedded only in commands that report workflow data.
type ReportFormat struct {
	Format string `long:"format" value-name:"FORMAT" description:"Report format: text, json, jsonl or xml"`
}

// resolve picks the flag first, then the caller's injected default, then text.
func (f ReportFormat) resolve(opts *config.AppOptions) (string, error) {
	if f.Format != "" {
		return validFormat(f.Format, "--format")
	}
	if opts != nil && opts.Runtime.Format != "" {
		return validFormat(opts.Runtime.Format, "the default format")
	}
	return formatText, nil
}

func validFormat(value string, source string) (string, error) {
	switch value {
	case formatText, formatJSON, formatJSONL, formatXML:
		return value, nil
	}
	return "", fmt.Errorf("unknown output format %q from %s\n"+
		"ACTION: Use one of text, json, jsonl or xml.", value, source)
}

// field is one named value of a record. Values are string, bool, int,
// float64, json.Number, []string, record or []record.
type field struct {
	name  string
	value any
}

// record keeps its fields in order, so every format renders them the same way.
type record []field

// report is one structured command result.
type report struct {
	command        string
	records        []record
	cached         bool
	unchangedSince string
}

func writeReport(opts *config.AppOptions, format string, result report) error {
	if result.records == nil {
		result.records = []record{}
	}
	switch format {
	case formatJSON:
		return writeJSONReport(opts, result)
	case formatJSONL:
		return writeJSONLReport(opts, result)
	case formatXML:
		return writeXMLReport(opts, result)
	}
	return fmt.Errorf("unknown output format %q\nACTION: Use one of text, json, jsonl or xml.", format)
}

func reportMeta(opts *config.AppOptions, result report) record {
	meta := record{
		{"project_id", opts.ProjectID},
		{"session_id", opts.SessionID},
		{"generated_at", time.Now().UTC().Format(time.RFC3339)},
		{"cached", result.cached},
	}
	if result.cached {
		meta = append(meta, field{"unchanged_since", result.unchangedSince})
	}
	return meta
}

func reportHead(opts *config.AppOptions, result report) record {
	return record{
		{"format_version", formatVersion},
		{"command", result.command},
		{"meta", reportMeta(opts, result)},
	}
}

func writeJSONReport(opts *config.AppOptions, result report) error {
	document := append(reportHead(opts, result), field{"records", result.records})
	var buffer bytes.Buffer
	if err := appendJSON(&buffer, document); err != nil {
		return err
	}
	buffer.WriteByte('\n')
	_, err := opts.Out().Write(buffer.Bytes())
	return err
}

func writeJSONLReport(opts *config.AppOptions, result report) error {
	var buffer bytes.Buffer
	if err := appendJSON(&buffer, reportHead(opts, result)); err != nil {
		return err
	}
	buffer.WriteByte('\n')
	for _, current := range result.records {
		if err := appendJSON(&buffer, current); err != nil {
			return err
		}
		buffer.WriteByte('\n')
	}
	_, err := opts.Out().Write(buffer.Bytes())
	return err
}

func appendJSON(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case record:
		buffer.WriteByte('{')
		for index, current := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			name, _ := json.Marshal(current.name)
			buffer.Write(name)
			buffer.WriteByte(':')
			if err := appendJSON(buffer, current.value); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
		return nil
	case []record:
		buffer.WriteByte('[')
		for index, current := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := appendJSON(buffer, current); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
		return nil
	case []string:
		if typed == nil {
			typed = []string{}
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buffer.Write(encoded)
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode report value: %w", err)
	}
	buffer.Write(encoded)
	return nil
}

func writeXMLReport(opts *config.AppOptions, result report) error {
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	root := xml.StartElement{Name: xml.Name{Local: "report"}}
	if err := encoder.EncodeToken(root); err != nil {
		return err
	}
	if err := encodeXMLFields(encoder, reportHead(opts, result)); err != nil {
		return err
	}
	if err := encodeXMLValue(encoder, "records", result.records); err != nil {
		return err
	}
	if err := encoder.EncodeToken(root.End()); err != nil {
		return err
	}
	if err := encoder.Flush(); err != nil {
		return err
	}
	buffer.WriteByte('\n')
	_, err := opts.Out().Write(buffer.Bytes())
	return err
}

func encodeXMLFields(encoder *xml.Encoder, fields record) error {
	for _, current := range fields {
		if err := encodeXMLValue(encoder, current.name, current.value); err != nil {
			return err
		}
	}
	return nil
}

// encodeXMLValue writes one element per value: scalars as text, records as
// child elements, and lists as repeated <item> or <record> children.
func encodeXMLValue(encoder *xml.Encoder, name string, value any) error {
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	var err error
	switch typed := value.(type) {
	case record:
		err = encodeXMLFields(encoder, typed)
	case []record:
		for _, current := range typed {
			if err = encodeXMLValue(encoder, "record", current); err != nil {
				break
			}
		}
	case []string:
		for _, current := range typed {
			if err = encodeXMLValue(encoder, "item", current); err != nil {
				break
			}
		}
	default:
		err = encoder.EncodeToken(xml.CharData(scalarText(typed)))
	}
	if err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func scalarText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	case nil:
		return ""
	}
	return fmt.Sprint(value)
}

// cachedRecords is the payload stored for a structured report. Records are
// kept as ordered name/value pairs so a cache hit renders the same field
// order as the fresh render.
type cachedRecords struct {
	GeneratedAt string `json:"generated_at"`
	Records     []any  `json:"records"`
}

func structuredCacheKey(textKey string) string {
	return textKey + structuredCacheSuffix
}

// loadCachedReport returns the cached records for a structured report.
func loadCachedReport(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, key string) (report, bool, error) {
	payload, found, err := store.GetCache(ctx, opts.ProjectID, opts.SessionID, structuredCacheKey(key), time.Now().UTC())
	if err != nil || !found {
		return report{}, false, err
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	var cached cachedRecords
	if err := decoder.Decode(&cached); err != nil {
		// A corrupt derived entry is not worth failing the command for.
		return report{}, false, nil
	}
	records := make([]record, 0, len(cached.Records))
	for _, raw := range cached.Records {
		current, ok := decodeCachedValue(raw).(record)
		if !ok {
			return report{}, false, nil
		}
		records = append(records, current)
	}
	return report{records: records, cached: true, unchangedSince: cached.GeneratedAt}, true, nil
}

func storeCachedReport(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, key string, records []record, ttl time.Duration) error {
	encoded := make([]any, 0, len(records))
	for _, current := range records {
		encoded = append(encoded, encodeCachedValue(current))
	}
	payload, err := json.Marshal(cachedRecords{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Records:     encoded,
	})
	if err != nil {
		return fmt.Errorf("encode cached report: %w", err)
	}
	return store.SetCache(ctx, opts.ProjectID, opts.SessionID, structuredCacheKey(key), string(payload), time.Now().UTC().Add(ttl))
}

// A record is encoded as {"fields": [[name, value], ...]} and a list of
// records as a JSON array of those objects, so decoding restores the order.
func encodeCachedValue(value any) any {
	switch typed := value.(type) {
	case record:
		pairs := make([][2]any, 0, len(typed))
		for _, current := range typed {
			pairs = append(pairs, [2]any{current.name, encodeCachedValue(current.value)})
		}
		return map[string]any{"fields": pairs}
	case []record:
		items := make([]any, 0, len(typed))
		for _, current := range typed {
			items = append(items, encodeCachedValue(current))
		}
		return items
	}
	return value
}

func decodeCachedValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		pairs, _ := typed["fields"].([]any)
		decoded := make(record, 0, len(pairs))
		for _, raw := range pairs {
			pair, ok := raw.([]any)
			if !ok || len(pair) != 2 {
				continue
			}
			name, _ := pair[0].(string)
			decoded = append(decoded, field{name, decodeCachedValue(pair[1])})
		}
		return decoded
	case []any:
		return decodeCachedList(typed)
	}
	return value
}

func decodeCachedList(items []any) any {
	if len(items) == 0 {
		return []string{}
	}
	if _, ok := items[0].(map[string]any); ok {
		records := make([]record, 0, len(items))
		for _, item := range items {
			if current, ok := decodeCachedValue(item).(record); ok {
				records = append(records, current)
			}
		}
		return records
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, scalarText(item))
	}
	return values
}

// clearStructuredCacheKey removes the structured twin of an exact text key.
func clearStructuredCacheKey(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, key string) error {
	return store.ClearCacheKey(ctx, opts.ProjectID, opts.SessionID, structuredCacheKey(key))
}

// taskRecord describes one task with its full UUID first; the short ID is
// presentation only.
func taskRecord(ctx context.Context, store *sqlite.Store, current task.Task) (record, error) {
	ticket, inherited, err := store.FindExternalTicket(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	return record{
		{"id", current.ID},
		{"short_id", shortID(current.ID)},
		{"initiative", current.InitiativeName},
		{"description", current.Description},
		{"status", string(current.Status)},
		{"mode", current.Mode.String()},
		{"priority", current.Priority},
		{"urgency", current.Urgency},
		{"due_at", current.DueAt},
		{"ticket", ticket},
		{"ticket_inherited", inherited},
		{"dependencies", nonNilStrings(current.Dependencies)},
	}, nil
}

func taskRecords(ctx context.Context, store *sqlite.Store, tasks []task.Task) ([]record, error) {
	records := make([]record, 0, len(tasks))
	for _, current := range tasks {
		built, err := taskRecord(ctx, store, current)
		if err != nil {
			return nil, err
		}
		records = append(records, built)
	}
	return records, nil
}

func initiativeRecord(summary task.InitiativeSummary) record {
	return record{
		{"id", summary.Initiative.ID},
		{"name", summary.Initiative.Name},
		{"status", string(summary.Initiative.Status)},
		{"ticket", summary.Initiative.ExternalTicket},
		{"pending", summary.Pending},
		{"active", summary.Active},
		{"completed", summary.Completed},
		{"blocked", summary.Blocked},
	}
}

func initiativeRecords(summaries []task.InitiativeSummary) []record {
	records := make([]record, 0, len(summaries))
	for _, summary := range summaries {
		records = append(records, initiativeRecord(summary))
	}
	return records
}

func annotationRecord(taskID string, taskDescription string, annotation task.Annotation, inherited bool) record {
	return record{
		{"task_id", taskID},
		{"task_description", taskDescription},
		{"kind", annotation.Kind},
		{"body", annotation.Body},
		{"created_at", annotation.CreatedAt},
		{"inherited", inherited},
	}
}

func contextRecords(current task.Task, direct []task.Annotation, inherited []task.ContextEntry) []record {
	records := make([]record, 0, len(direct)+len(inherited))
	for _, annotation := range direct {
		records = append(records, annotationRecord(current.ID, current.Description, annotation, false))
	}
	for _, entry := range inherited {
		records = append(records, annotationRecord(entry.TaskID, entry.TaskDescription, entry.Annotation, true))
	}
	return records
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
