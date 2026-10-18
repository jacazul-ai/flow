package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v6"
	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

//go:embed all:templates/flow-expert
var skillTemplates embed.FS

const (
	skillTemplateRoot  = "templates/flow-expert"
	skillHub           = "SKILL.md"
	skillReferencesDir = "references"
	skillCommandIndex  = "references/commands.md"
)

// skillTopicOrder is the order the hub routes to its references; the command
// index follows it. Every topic in the registry must appear here.
var skillTopicOrder = []string{
	topicInitiatives,
	topicTasks,
	topicFocus,
	topicSessions,
	topicContext,
	topicReports,
	topicRuntime,
}

// modeGuidance explains what each task mode authorizes an agent to do.
// Rendering fails when a mode in the catalog has no guidance.
var modeGuidance = map[task.TaskMode]string{
	task.ModeDesign:      "architecture, trade-offs, contracts, and task breakdown; no direct edits",
	task.ModeSpike:       "time-boxed research or a disposable proof of concept",
	task.ModeInvestigate: "read-only code exploration and de-risking",
	task.ModeGuide:       "the operator codes; the agent gives steps and suggested diffs",
	task.ModeExecute:     "the agent edits code, tests, and required documentation",
	task.ModeRefine:      "polish and incremental improvement with direct edits",
	task.ModeTest:        "verification; add or update tests in scope",
	task.ModeDebug:       "root-cause analysis; propose the fix before applying it",
	task.ModeReview:      "audit and critique; edits only on explicit escalation",
}

// kindGuidance explains when to record each annotation kind. Rendering fails
// when a kind in the catalog has no guidance.
var kindGuidance = map[string]string{
	"AC":         "an acceptance criterion was agreed",
	"BLOCKED":    "progress depends on something outside the task",
	"DECISION":   "a choice was made, including its trade-offs",
	"HANDOFF":    "another agent or session continues the task",
	"HYPOTHESIS": "an unproven explanation must survive the session",
	"LESSON":     "something failed and the fix is known",
	"LINK":       "a reference a cold-start agent needs",
	"NOTE":       "a fact a cold-start agent needs that fits no other kind",
	"OUTCOME":    "the final result; required before completion",
	"QUESTION":   "an open point must survive the session",
	"RESEARCH":   "a finding opens or closes an option",
}

// SkillCommand renders the canonical flow-expert agent skill.
type SkillCommand struct {
	Out     string `long:"out" value-name:"DIR" description:"Write the complete skill tree to DIR"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies the output stream to the command.
func (cmd *SkillCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute prints the skill hub, or writes the whole tree with --out, without
// opening workflow storage.
func (cmd *SkillCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("skill accepts no arguments\nACTION: Run 'jczl-flow skill' or 'jczl-flow skill --out <dir>'.")
	}

	tree, err := renderSkillTree(defaultSkillSource())
	if err != nil {
		return fmt.Errorf("render flow-expert skill: %w", err)
	}
	if cmd.Out == "" {
		_, err = fmt.Fprint(cmd.appOpts.Out(), tree[skillHub])
		return err
	}
	if err := writeSkillTree(cmd.Out, tree); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.appOpts.Out(), "Wrote flow-expert skill to %s\n", cmd.Out)
	return err
}

// skillSource holds the catalogs the skill is generated from.
type skillSource struct {
	Commands []CommandSpec
	Modes    []task.TaskMode
	Kinds    []task.AnnotationKind
}

func defaultSkillSource() skillSource {
	return skillSource{
		Commands: NewCommandRegistry().All(true),
		Modes:    task.TaskModes(),
		Kinds:    task.AnnotationKinds(),
	}
}

type skillCommand struct {
	Name    string
	Summary string
	Aliases []string
}

type skillTopic struct {
	Name     string
	Commands []skillCommand
}

type skillMode struct {
	Name     string
	Guidance string
}

type skillKind struct {
	Kind     string
	Aliases  []string
	Guidance string
}

type skillRuntime struct {
	Concern string
	Flag    string
	Env     string
	Default string
}

func skillReferencePath(topic string) string {
	return path.Join(skillReferencesDir, topic+".md")
}

// renderSkillTree renders every template into a map keyed by the file's path
// inside the skill. It fails when a catalog entry has nowhere to be explained.
func renderSkillTree(source skillSource) (map[string]string, error) {
	templates, err := readSkillTemplates()
	if err != nil {
		return nil, err
	}
	data, err := skillData(source, templates)
	if err != nil {
		return nil, err
	}

	tree := make(map[string]string, len(templates))
	for name, content := range templates {
		template, err := pongo2.FromBytes(content)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		rendered, err := template.Execute(data)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}
		tree[name] = rendered
	}
	return tree, nil
}

func readSkillTemplates() (map[string][]byte, error) {
	root, err := fs.Sub(skillTemplates, skillTemplateRoot)
	if err != nil {
		return nil, err
	}
	templates := map[string][]byte{}
	err = fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(root, name)
		templates[name] = content
		return err
	})
	return templates, err
}

func skillData(source skillSource, templates map[string][]byte) (pongo2.Context, error) {
	topics, err := skillTopics(source.Commands, templates)
	if err != nil {
		return nil, err
	}
	modes, err := skillModes(source.Modes)
	if err != nil {
		return nil, err
	}
	kinds, err := skillKinds(source.Kinds)
	if err != nil {
		return nil, err
	}

	dueKeywords := make([]string, 0, len(dueDateKeywords))
	for _, keyword := range dueDateKeywords {
		dueKeywords = append(dueKeywords, keyword.name)
	}
	topicList := make([]skillTopic, 0, len(skillTopicOrder))
	for _, name := range skillTopicOrder {
		topicList = append(topicList, topics[name])
	}
	return pongo2.Context{
		"topics":       topics,
		"topic_list":   topicList,
		"modes":        modes,
		"kinds":        kinds,
		"title_ideal":  task.TitleWarningLength,
		"title_max":    task.TitleMaxLength,
		"due_layout":   "YYYY-MM-DD",
		"due_keywords": dueKeywords,
		"runtime":      skillRuntimeRows(),
	}, nil
}

// skillTopics groups visible commands by topic, in registry order, and keys
// the result by topic name for the templates.
func skillTopics(commands []CommandSpec, templates map[string][]byte) (map[string]skillTopic, error) {
	topics := map[string]skillTopic{}
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		if command.Topic == "" {
			return nil, fmt.Errorf("command %q has no skill topic\nACTION: Set topic on its help entry in internal/cli/command.go.", command.Name)
		}
		reference := skillReferencePath(command.Topic)
		_, hasTemplate := templates[reference]
		if !hasTemplate || !slices.Contains(skillTopicOrder, command.Topic) {
			return nil, fmt.Errorf("command %q has topic %q without %s\nACTION: Add the reference template or use an existing topic.", command.Name, command.Topic, reference)
		}
		topic := topics[command.Topic]
		topic.Name = command.Topic
		topic.Commands = append(topic.Commands, skillCommand{
			Name:    command.Name,
			Summary: embeddedText(command.Summary),
			Aliases: command.Aliases,
		})
		topics[command.Topic] = topic
	}
	return topics, nil
}

func skillModes(modes []task.TaskMode) ([]skillMode, error) {
	rendered := make([]skillMode, 0, len(modes))
	for _, mode := range modes {
		guidance, ok := modeGuidance[mode]
		if !ok {
			return nil, fmt.Errorf("task mode %s has no skill guidance\nACTION: Add it to modeGuidance in internal/cli/skill.go.", mode)
		}
		rendered = append(rendered, skillMode{Name: mode.String(), Guidance: guidance})
	}
	return rendered, nil
}

func skillKinds(kinds []task.AnnotationKind) ([]skillKind, error) {
	rendered := make([]skillKind, 0, len(kinds))
	for _, kind := range kinds {
		guidance, ok := kindGuidance[kind.Kind]
		if !ok {
			return nil, fmt.Errorf("annotation kind %s has no skill guidance\nACTION: Add it to kindGuidance in internal/cli/skill.go.", kind.Kind)
		}
		rendered = append(rendered, skillKind{Kind: kind.Kind, Aliases: kind.Aliases, Guidance: guidance})
	}
	return rendered, nil
}

func skillRuntimeRows() []skillRuntime {
	options := reflect.TypeOf(config.AppOptions{})
	flag := func(field string) string {
		option, _ := options.FieldByName(field)
		return "--" + option.Tag.Get("long")
	}
	return []skillRuntime{
		{"Project", flag("ProjectID"), config.EnvProject, "resolved from the working directory, else " + config.DefaultScope},
		{"Home", flag("Home"), config.EnvHome, "$HOME/" + config.DefaultHomeDir},
		{"Session", flag("SessionID"), config.EnvSession, config.DefaultScope},
	}
}

// writeSkillTree writes the rendered tree below dir and removes reference
// files the tree no longer produces. Files outside the generated paths, such
// as hand-written evals, are left alone.
func writeSkillTree(dir string, tree map[string]string) error {
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create skill directory: %w\nACTION: Choose a writable --out directory.", err)
		}
		if err := os.WriteFile(target, []byte(tree[name]), 0o644); err != nil {
			return fmt.Errorf("write %s: %w\nACTION: Choose a writable --out directory.", target, err)
		}
	}
	return removeStaleReferences(dir, tree)
}

func removeStaleReferences(dir string, tree map[string]string) error {
	entries, err := os.ReadDir(filepath.Join(dir, skillReferencesDir))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := path.Join(skillReferencesDir, entry.Name())
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		if _, ok := tree[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("remove stale reference %s: %w", name, err)
		}
	}
	return nil
}

func embeddedText(value string) string {
	return strings.ReplaceAll(value, "jczl-flow", "jacazul flow")
}
