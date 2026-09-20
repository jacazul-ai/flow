package flow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// consumerModule is the go.mod of the stand-in consumer. The replace keeps the
// build local, so the test never reaches the network for the engine itself.
const consumerModule = `module example.com/consumer

go 1.25.0

require github.com/jacazul-ai/flow v0.0.0

replace github.com/jacazul-ai/flow => {{.Engine}}
`

// buildConsumer assembles the consumer module in a temporary directory and
// builds it against this checkout. Dependencies resolve from the module cache
// with the proxy off, because building the engine already populated it.
func buildConsumer(t *testing.T) string {
	t.Helper()

	engine, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate engine: %v", err)
	}
	root := t.TempDir()

	var module strings.Builder
	if err := template.Must(template.New("go.mod").Parse(consumerModule)).Execute(&module, struct{ Engine string }{engine}); err != nil {
		t.Fatalf("render go.mod: %v", err)
	}
	copyInto(t, root, "go.mod", []byte(module.String()))
	copyInto(t, root, "go.sum", readFile(t, filepath.Join(engine, "go.sum")))
	copyInto(t, root, "main.go", readFile(t, filepath.Join(engine, "testdata", "consumer", "main.go")))

	binary := filepath.Join(root, "consumer")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build consumer against the public API: %v\n%s"+
			"ACTION: If the failure is a missing dependency, warm the module cache "+
			"with 'go mod download' first; this build keeps GOPROXY=off so the test "+
			"never reaches the network.", err, output)
	}
	return binary
}

func copyInto(t *testing.T, dir string, name string, content []byte) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

// The engine promises jacazul-ai-cli that package flow is enough to embed it.
// A test inside this module cannot hold that promise: it may import internal/
// packages a real consumer is denied. This one builds a separate module, so
// the toolchain enforces the same boundary jacazul will meet.
func TestExternalModuleDrivesWorkflowThroughPublicAPI(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain unavailable: %v", err)
	}

	binary := buildConsumer(t)
	root := t.TempDir()
	transcript := filepath.Join(root, "transcript.txt")

	consumer := exec.Command(binary, filepath.Join(root, "home"), transcript)
	consumer.Env = []string{}
	output, err := consumer.CombinedOutput()
	if err != nil {
		t.Fatalf("consumer failed: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Fatalf("consumer process streams received %q; the engine must write only to the injected Streams", output)
	}

	recorded := string(readFile(t, transcript))
	for _, want := range []string{
		"Created task ",
		"First task",
		"Focused task ",
		"Started task ",
		"Recorded outcome for task ",
		"Completed task ",
		"CONSUMER OK",
	} {
		if !strings.Contains(recorded, want) {
			t.Fatalf("transcript = %q, want it to contain %q", recorded, want)
		}
	}
}
