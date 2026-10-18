package cli

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/jacazul-ai/flow/internal/task"
)

var taskTagPattern = regexp.MustCompile(`^[^\s+\-]+$`)

// TagArguments carries Taskwarrior-style tag predicates after CLI parsing.
type TagArguments struct {
	WithTags    []string `long:"with-tag" description:"Include tasks with this tag"`
	WithoutTags []string `long:"without-tag" description:"Exclude tasks with this tag"`
}

// TagFilter returns the native tag predicate represented by the arguments.
func (args TagArguments) TagFilter() task.TagFilter {
	return task.TagFilter{
		Include: task.NormalizeTags(args.WithTags),
		Exclude: task.NormalizeTags(args.WithoutTags),
	}
}

func (args TagArguments) hasTagMutation() bool {
	return len(args.WithTags) > 0 || len(args.WithoutTags) > 0
}

func formatTaskTags(tags []string) string {
	normalized := task.NormalizeTags(tags)
	if len(normalized) == 0 {
		return ""
	}
	values := make([]string, 0, len(normalized))
	for _, tag := range normalized {
		values = append(values, "+"+tag)
	}
	return strings.Join(values, " ")
}

func filterTasksByTags(tasks []task.Task, filter task.TagFilter) []task.Task {
	if len(filter.Include) == 0 && len(filter.Exclude) == 0 {
		return tasks
	}
	filtered := make([]task.Task, 0, len(tasks))
	for _, current := range tasks {
		if filter.Matches(current.Tags) {
			filtered = append(filtered, current)
		}
	}
	return filtered
}

func tagFilterCacheSuffix(filter task.TagFilter) string {
	if len(filter.Include) == 0 && len(filter.Exclude) == 0 {
		return ""
	}
	include := append([]string(nil), filter.Include...)
	exclude := append([]string(nil), filter.Exclude...)
	sort.Strings(include)
	sort.Strings(exclude)
	return "|with=" + encodeTagKey(include) + "|without=" + encodeTagKey(exclude)
}

func encodeTagKey(tags []string) string {
	encoded := make([]string, 0, len(tags))
	for _, tag := range tags {
		encoded = append(encoded, url.QueryEscape(tag))
	}
	return strings.Join(encoded, ",")
}

func validateTagMutation(args TagArguments) error {
	include := make(map[string]struct{}, len(args.WithTags))
	for _, tag := range task.NormalizeTags(args.WithTags) {
		include[tag] = struct{}{}
	}
	for _, tag := range task.NormalizeTags(args.WithoutTags) {
		if _, exists := include[tag]; exists {
			return fmt.Errorf("tag %q cannot be added and removed in one operation\nACTION: Choose either +%s or -%s.", tag, tag, tag)
		}
	}
	return nil
}

func isTaskTagPredicate(value string) (string, bool, bool) {
	if len(value) < 2 || (value[0] != '+' && value[0] != '-') {
		return "", false, false
	}
	name := value[1:]
	if !taskTagPattern.MatchString(name) {
		return "", false, false
	}
	for _, char := range name {
		if unicode.IsSpace(char) || char == '=' {
			return "", false, false
		}
	}
	return name, value[0] == '+', true
}

// NormalizeTagPredicates protects Taskwarrior-style predicates from go-flags.
// It rewrites them into command-local long options before the root parser sees
// a leading minus as an option.
func NormalizeTagPredicates(args []string) []string {
	command := firstCommand(args)
	if !supportsTagPredicates(command) {
		return args
	}
	result := make([]string, 0, len(args))
	for _, value := range args {
		name, include, ok := isTaskTagPredicate(value)
		if !ok || isKnownShortOption(value) {
			result = append(result, value)
			continue
		}
		if include {
			result = append(result, "--with-tag="+name)
		} else {
			result = append(result, "--without-tag="+name)
		}
	}
	return result
}

func firstCommand(args []string) string {
	for index := 0; index < len(args); index++ {
		value := args[index]
		if strings.HasPrefix(value, "--") {
			if !strings.Contains(value, "=") {
				index++
			}
			continue
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		return value
	}
	return ""
}

func supportsTagPredicates(command string) bool {
	switch command {
	case "status", "active", "blocked", "overdue", "next", "tree", "onboard", "amend":
		return true
	default:
		return false
	}
}

func isKnownShortOption(value string) bool {
	switch value {
	case "-v", "-V", "-h":
		return true
	default:
		return false
	}
}
