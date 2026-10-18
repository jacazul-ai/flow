package cli

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

func TestSkillTreeCoversEveryVisibleCommand(t *testing.T) {
	tree, err := renderSkillTree(defaultSkillSource())
	if err != nil {
		t.Fatalf("render skill tree: %v", err)
	}

	for _, command := range NewCommandRegistry().All(false) {
		reference := skillReferencePath(command.Topic)
		if !strings.Contains(tree[reference], "`"+command.Name+"`") {
			t.Errorf("%s does not explain command %q", reference, command.Name)
		}
		if !strings.Contains(tree[skillCommandIndex], "`"+command.Name+"`") {
			t.Errorf("%s does not index command %q", skillCommandIndex, command.Name)
		}
	}
}

func TestSkillTreeRendersCatalogFactsFromCode(t *testing.T) {
	tree, err := renderSkillTree(defaultSkillSource())
	if err != nil {
		t.Fatalf("render skill tree: %v", err)
	}

	tasks := tree[skillReferencePath(topicTasks)]
	for _, mode := range task.TaskModes() {
		if !strings.Contains(tasks, "`"+mode.String()+"`") {
			t.Errorf("tasks reference misses mode %s", mode)
		}
	}
	for _, limit := range []int{task.TitleWarningLength, task.TitleMaxLength} {
		if !strings.Contains(tasks, strconv.Itoa(limit)) {
			t.Errorf("tasks reference misses title limit %d", limit)
		}
	}
	for _, keyword := range dueDateKeywords {
		if !strings.Contains(tasks, "`"+keyword.name+"`") {
			t.Errorf("tasks reference misses due keyword %q", keyword.name)
		}
	}

	contextReference := tree[skillReferencePath(topicContext)]
	for _, kind := range task.AnnotationKinds() {
		if !strings.Contains(contextReference, "`"+kind.Kind+"`") {
			t.Errorf("context reference misses annotation kind %s", kind.Kind)
		}
		for _, alias := range kind.Aliases {
			if !strings.Contains(contextReference, "`"+alias+"`") {
				t.Errorf("context reference misses alias %q of %s", alias, kind.Kind)
			}
		}
	}

	runtime := tree[skillReferencePath(topicRuntime)]
	for _, fact := range []string{
		config.EnvProject, config.EnvHome, config.EnvSession,
		"--project", "--home", "--session",
		config.DefaultHomeDir, config.DefaultScope,
	} {
		if !strings.Contains(runtime, fact) {
			t.Errorf("runtime reference misses %q", fact)
		}
	}
}

func TestRenderSkillTreeRejectsIncompleteSources(t *testing.T) {
	for name, mutate := range map[string]func(*skillSource){
		"command without topic": func(source *skillSource) {
			source.Commands = append(source.Commands, CommandSpec{Name: "orphan"})
		},
		"topic without reference": func(source *skillSource) {
			source.Commands = append(source.Commands, CommandSpec{Name: "orphan", Topic: "nowhere"})
		},
		"mode without guidance": func(source *skillSource) {
			source.Modes = append(source.Modes, task.TaskMode(999))
		},
		"annotation kind without guidance": func(source *skillSource) {
			source.Kinds = append(source.Kinds, task.AnnotationKind{Kind: "UNEXPLAINED"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := defaultSkillSource()
			mutate(&source)
			if _, err := renderSkillTree(source); err == nil {
				t.Fatalf("render accepted a source with %s", name)
			} else if !strings.Contains(err.Error(), "ACTION:") {
				t.Fatalf("render error lacks an ACTION: %v", err)
			}
		})
	}
}

func TestRenderSkillTreeIgnoresHiddenCommands(t *testing.T) {
	source := defaultSkillSource()
	source.Commands = append(source.Commands, CommandSpec{Name: "secret", Hidden: true})

	tree, err := renderSkillTree(source)
	if err != nil {
		t.Fatalf("render skill tree: %v", err)
	}
	if strings.Contains(tree[skillCommandIndex], "`secret`") {
		t.Fatalf("hidden command leaked into the skill index")
	}
}
