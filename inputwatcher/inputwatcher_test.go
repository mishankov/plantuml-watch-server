package inputwatcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

type renderCall struct {
	input  string
	output string
	format string
}

type stubRenderer struct {
	mu    sync.Mutex
	calls []renderCall
}

func (r *stubRenderer) ExecuteWithFormat(_ context.Context, input, output, format string) {
	r.mu.Lock()
	r.calls = append(r.calls, renderCall{input: input, output: output, format: format})
	r.mu.Unlock()

	_ = os.MkdirAll(output, 0o755)
	baseName := filepath.Base(input[:len(input)-len(filepath.Ext(input))])
	_ = os.WriteFile(filepath.Join(output, baseName+"."+format), []byte(format), 0o644)
}

func (r *stubRenderer) callCountsByInput() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()

	counts := make(map[string]int, len(r.calls))
	for _, call := range r.calls {
		counts[call.input]++
	}
	return counts
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func TestListPlantUMLFilesAndFilterPublicDiagrams(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "_shared.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "main.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "nested", "_leaf.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "nested", "diagram.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "ignore.txt"), "ignore")

	files, err := listPlantUMLFiles(root)
	if err != nil {
		t.Fatalf("listPlantUMLFiles failed: %v", err)
	}

	wantFiles := []string{
		filepath.Join(root, "_shared.puml"),
		filepath.Join(root, "main.puml"),
		filepath.Join(root, "nested", "_leaf.puml"),
		filepath.Join(root, "nested", "diagram.puml"),
	}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("unexpected files: got %v want %v", files, wantFiles)
	}

	wantPublic := []string{
		filepath.Join(root, "main.puml"),
		filepath.Join(root, "nested", "diagram.puml"),
	}
	if public := filterPublicDiagrams(files); !reflect.DeepEqual(public, wantPublic) {
		t.Fatalf("unexpected public files: got %v want %v", public, wantPublic)
	}
}

func TestBuildAffectedPublicIndexTracksTransitiveIncludes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "_shared.puml"), "!include nested/_leaf.puml\n@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "main.puml"), "!include _shared.puml\n@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "nested", "_leaf.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "nested", "diagram.puml"), "!include ../_shared.puml\n@startuml\n@enduml\n")

	files, err := listPlantUMLFiles(root)
	if err != nil {
		t.Fatalf("listPlantUMLFiles failed: %v", err)
	}

	affected, err := buildAffectedPublicIndex(root, files)
	if err != nil {
		t.Fatalf("buildAffectedPublicIndex failed: %v", err)
	}

	leaf := filepath.Join(root, "nested", "_leaf.puml")
	shared := filepath.Join(root, "_shared.puml")
	main := filepath.Join(root, "main.puml")
	diagram := filepath.Join(root, "nested", "diagram.puml")

	if got, want := affected[leaf], []string{main, diagram}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected leaf dependents: got %v want %v", got, want)
	}
	if got, want := affected[shared], []string{main, diagram}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected shared dependents: got %v want %v", got, want)
	}
	if got, want := affected[main], []string{main}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected main dependents: got %v want %v", got, want)
	}
	if got, want := affected[diagram], []string{diagram}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected diagram dependents: got %v want %v", got, want)
	}
}

func TestHandleSourceChangeRegeneratesAffectedPublicDiagramsOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	output := filepath.Join(root, "output")
	writeTestFile(t, filepath.Join(root, "_shared.puml"), "@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "main.puml"), "!include _shared.puml\n@startuml\n@enduml\n")
	writeTestFile(t, filepath.Join(root, "nested", "diagram.puml"), "!include ../_shared.puml\n!include_once ../_shared.puml\n@startuml\n@enduml\n")

	renderer := &stubRenderer{}
	watcher := New(root, output, renderer)

	watcher.HandleSourceChange(context.Background(), filepath.Join(root, "_shared.puml"))

	counts := renderer.callCountsByInput()
	if got, want := counts[filepath.Join(root, "main.puml")], 2; got != want {
		t.Fatalf("unexpected render count for main: got %d want %d", got, want)
	}
	if got, want := counts[filepath.Join(root, "nested", "diagram.puml")], 2; got != want {
		t.Fatalf("unexpected render count for diagram: got %d want %d", got, want)
	}
	if len(counts) != 2 {
		t.Fatalf("expected exactly two public diagrams to render, got %v", counts)
	}
}

func TestDeleteTrackedOutputsRemovesGeneratedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	output := filepath.Join(root, "output")
	inputFile := filepath.Join(root, "main.puml")
	writeTestFile(t, inputFile, "@startuml\n@enduml\n")

	renderer := &stubRenderer{}
	watcher := New(root, output, renderer)
	watcher.ExecuteAndTrack(context.Background(), inputFile, output)

	svgPath := filepath.Join(output, "main.svg")
	pngPath := filepath.Join(output, "main.png")
	if _, err := os.Stat(svgPath); err != nil {
		t.Fatalf("expected svg output to exist: %v", err)
	}
	if _, err := os.Stat(pngPath); err != nil {
		t.Fatalf("expected png output to exist: %v", err)
	}

	watcher.DeleteTrackedOutputs(context.Background(), inputFile)

	if _, err := os.Stat(svgPath); !os.IsNotExist(err) {
		t.Fatalf("expected svg output to be deleted, got %v", err)
	}
	if _, err := os.Stat(pngPath); !os.IsNotExist(err) {
		t.Fatalf("expected png output to be deleted, got %v", err)
	}
}
