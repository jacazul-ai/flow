package flow_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

var skillFiles = []string{
	"SKILL.md",
	"references/commands.md",
	"references/context.md",
	"references/focus.md",
	"references/initiatives.md",
	"references/reports.md",
	"references/runtime.md",
	"references/sessions.md",
	"references/tasks.md",
}

var skillLink = regexp.MustCompile(`\]\((references/[a-z-]+\.md)\)`)

func TestSkillPrintsHubToStdout(t *testing.T) {
	harness := testharness.NewHarness(t, "skill-project", "skill-session")

	output, err := runFlow(t, harness, "skill")
	if err != nil {
		t.Fatalf("render skill: %v\n%s", err, output)
	}
	if !strings.HasPrefix(output, "---\nname: flow-expert\ndescription: ") {
		t.Fatalf("hub must start with frontmatter naming flow-expert:\n%s", output)
	}
	for _, expected := range []string{
		"jacazul flow onboard",
		"jacazul flow focus",
		"jacazul flow status",
		"jacazul flow ponder",
		"jacazul flow help <command>",
		"(references/initiatives.md)",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("hub missing %q:\n%s", expected, output)
		}
	}
	assertSkillTextIsClean(t, "SKILL.md", output)

	second, err := runFlow(t, harness, "skill")
	if err != nil {
		t.Fatalf("render skill twice: %v\n%s", err, second)
	}
	if output != second {
		t.Fatalf("hub output is not deterministic")
	}
	assertNoStorage(t, harness)
}

func TestSkillOutWritesLinkedTree(t *testing.T) {
	harness := testharness.NewHarness(t, "skill-project", "skill-session")
	dir := filepath.Join(t.TempDir(), "flow-expert")

	output, err := runFlow(t, harness, "skill", "--out", dir)
	if err != nil {
		t.Fatalf("write skill tree: %v\n%s", err, output)
	}
	if output != "Wrote flow-expert skill to "+dir+"\n" {
		t.Fatalf("unexpected skill --out report: %q", output)
	}

	tree := readSkillTree(t, dir)
	if got := sortedSkillFiles(tree); strings.Join(got, ",") != strings.Join(skillFiles, ",") {
		t.Fatalf("skill tree files = %v, want %v", got, skillFiles)
	}

	hub, _ := runFlow(t, harness, "skill")
	if tree["SKILL.md"] != hub {
		t.Fatalf("written SKILL.md differs from the hub printed to stdout")
	}

	linked := map[string]bool{}
	for _, match := range skillLink.FindAllStringSubmatch(tree["SKILL.md"], -1) {
		if _, ok := tree[match[1]]; !ok {
			t.Errorf("hub links %s, which the tree does not contain", match[1])
		}
		linked[match[1]] = true
	}
	for name, content := range tree {
		assertSkillTextIsClean(t, name, content)
		if name != "SKILL.md" && !linked[name] {
			t.Errorf("hub does not route to %s", name)
		}
	}
	assertNoStorage(t, harness)
}

func TestSkillOutRewritesTreeAndDropsStaleReferences(t *testing.T) {
	harness := testharness.NewHarness(t, "skill-project", "skill-session")
	dir := filepath.Join(t.TempDir(), "flow-expert")

	if output, err := runFlow(t, harness, "skill", "--out", dir); err != nil {
		t.Fatalf("write skill tree: %v\n%s", err, output)
	}
	first := readSkillTree(t, dir)

	stale := filepath.Join(dir, "references", "retired.md")
	if err := os.WriteFile(stale, []byte("retired\n"), 0o644); err != nil {
		t.Fatalf("plant stale reference: %v", err)
	}
	evals := filepath.Join(dir, "evals", "case.md")
	if err := os.MkdirAll(filepath.Dir(evals), 0o755); err != nil {
		t.Fatalf("create evals directory: %v", err)
	}
	if err := os.WriteFile(evals, []byte("hand-written\n"), 0o644); err != nil {
		t.Fatalf("plant hand-written eval: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatalf("edit generated hub: %v", err)
	}

	if output, err := runFlow(t, harness, "skill", "--out", dir); err != nil {
		t.Fatalf("rewrite skill tree: %v\n%s", err, output)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale reference survived the rewrite: %v", err)
	}
	if content, err := os.ReadFile(evals); err != nil || string(content) != "hand-written\n" {
		t.Fatalf("rewrite touched files outside the generated tree: %q %v", content, err)
	}
	second := readSkillTree(t, dir)
	delete(second, "evals/case.md")
	for name, content := range first {
		if second[name] != content {
			t.Fatalf("rewrite of %s is not deterministic", name)
		}
	}
}

// TestSkillScenariosRun executes every bash block of the skill as one
// scenario on a fresh project, so no example can teach a command or an
// argument the engine rejects.
func TestSkillScenariosRun(t *testing.T) {
	harness := testharness.NewHarness(t, "skill-project", "skill-session")
	dir := filepath.Join(t.TempDir(), "flow-expert")
	if output, err := runFlow(t, harness, "skill", "--out", dir); err != nil {
		t.Fatalf("write skill tree: %v\n%s", err, output)
	}

	scenarios := 0
	for _, name := range sortedSkillFiles(readSkillTree(t, dir)) {
		content, _ := os.ReadFile(filepath.Join(dir, name))
		for index, block := range bashBlocks(string(content)) {
			scenarios++
			t.Run(fmt.Sprintf("%s#%d", name, index+1), func(t *testing.T) {
				runSkillScenario(t, block)
			})
		}
	}
	if scenarios == 0 {
		t.Fatalf("skill has no executable scenarios")
	}
}

func runSkillScenario(t *testing.T, block string) {
	scenario := testharness.NewHarness(t, "scenario-project", "scenario-session")
	bindings := map[string]string{}
	created := []string{"<first-uuid>", "<second-uuid>"}

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		command, ok := strings.CutPrefix(line, "jacazul flow ")
		if !ok {
			t.Fatalf("scenario line is not a jacazul flow command: %q", line)
		}
		for placeholder, value := range bindings {
			command = strings.ReplaceAll(command, placeholder, value)
		}
		if strings.Contains(command, "<") {
			t.Fatalf("scenario line uses an unbound placeholder: %q", line)
		}
		args, err := splitShellWords(command)
		if err != nil {
			t.Fatalf("scenario line %q: %v", line, err)
		}
		output, err := runFlow(t, scenario, args...)
		if err != nil {
			t.Fatalf("scenario line %q failed: %v\n%s", line, err, output)
		}
		for _, outputLine := range strings.Split(output, "\n") {
			id, ok := strings.CutPrefix(outputLine, "Created task ")
			if !ok || len(bindings) >= len(created) {
				continue
			}
			bindings[created[len(bindings)]] = strings.SplitN(id, ":", 2)[0]
		}
	}
}

func bashBlocks(content string) []string {
	var blocks []string
	var current []string
	inside := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case !inside && line == "```bash":
			inside, current = true, nil
		case inside && line == "```":
			inside = false
			blocks = append(blocks, strings.Join(current, "\n"))
		case inside:
			current = append(current, line)
		}
	}
	return blocks
}

// splitShellWords splits one command line on spaces, honoring single and
// double quotes the way a POSIX shell would for these plain examples.
func splitShellWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	inWord := false
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}

func readSkillTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("read skill tree: %v", err)
	}
	return tree
}

func sortedSkillFiles(tree map[string]string) []string {
	keys := make([]string, 0, len(tree))
	for key := range tree {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertSkillTextIsClean(t *testing.T, name string, content string) {
	t.Helper()
	for _, forbidden := range []string{"jczl-flow", "TODO", "{#", "{%", "{{"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("%s exposes %q", name, forbidden)
		}
	}
}

func assertNoStorage(t *testing.T, harness *testharness.Harness) {
	t.Helper()
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("skill opened workflow storage: %v", err)
	}
}
