package inputwatcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRenderer struct {
	mu            sync.Mutex
	active        int
	maxActive     int
	perFileActive map[string]int
	perFileMax    map[string]int
	callCount     map[string]int
	started       chan string
	release       <-chan struct{}
	renderFunc    func(input, output string, call int) error
}

func newFakeRenderer() *fakeRenderer {
	return &fakeRenderer{
		perFileActive: make(map[string]int),
		perFileMax:    make(map[string]int),
		callCount:     make(map[string]int),
	}
}

func (r *fakeRenderer) Render(ctx context.Context, input, output string) {
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.perFileActive[input]++
	if r.perFileActive[input] > r.perFileMax[input] {
		r.perFileMax[input] = r.perFileActive[input]
	}
	r.callCount[input]++
	call := r.callCount[input]
	r.mu.Unlock()

	if r.renderFunc != nil {
		if err := r.renderFunc(input, output, call); err != nil {
			panic(err)
		}
	}

	if r.started != nil {
		r.started <- input
	}

	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
		}
	}

	r.mu.Lock()
	r.perFileActive[input]--
	r.active--
	r.mu.Unlock()
}

func createInputFile(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create input directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("@startuml\n@enduml\n"), 0644); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	return path
}

func waitForString(t *testing.T, ch <-chan string, timeout time.Duration) string {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event")
		return ""
	}
}

func waitForExpectedEvent(t *testing.T, ch <-chan string, timeout time.Duration, expected map[string]bool) {
	t.Helper()

	deadline := time.After(timeout)
	for len(expected) > 0 {
		select {
		case value := <-ch:
			if expected[value] {
				delete(expected, value)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events: %v", expected)
		}
	}
}

func assertNoEvent(t *testing.T, ch <-chan string, timeout time.Duration, description string) {
	t.Helper()

	select {
	case value := <-ch:
		t.Fatalf("unexpected %s: %s", description, value)
	case <-time.After(timeout):
	}
}

func TestRenderFilesRespectsParallelism(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	files := []string{
		createInputFile(t, inputDir, "one.puml"),
		createInputFile(t, inputDir, "two.puml"),
		createInputFile(t, inputDir, "nested/three.puml"),
	}

	release := make(chan struct{})
	renderer := newFakeRenderer()
	renderer.started = make(chan string, len(files))
	renderer.release = release
	renderer.renderFunc = func(input, output string, call int) error {
		base := strings.TrimSuffix(filepath.Base(input), ".puml")
		return os.WriteFile(filepath.Join(output, base+".svg"), []byte(base), 0644)
	}

	iw := New(inputDir, outputDir, renderer, 2)

	done := make(chan struct{})
	go func() {
		iw.RenderFiles(context.Background(), files)
		close(done)
	}()

	started := map[string]bool{}
	for len(started) < 2 {
		started[waitForString(t, renderer.started, time.Second)] = true
	}
	assertNoEvent(t, renderer.started, 100*time.Millisecond, "extra render start beyond parallelism limit")

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RenderFiles to finish")
	}

	if renderer.maxActive != 2 {
		t.Fatalf("expected max active renders to be 2, got %d", renderer.maxActive)
	}
}

func TestExecuteAndTrackRemovesStaleOutputs(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	inputFile := createInputFile(t, inputDir, "diagram.puml")

	renderer := newFakeRenderer()
	renderer.renderFunc = func(input, output string, call int) error {
		base := strings.TrimSuffix(filepath.Base(input), ".puml")
		if err := os.WriteFile(filepath.Join(output, base+".svg"), []byte("svg"), 0644); err != nil {
			return err
		}
		if call == 1 {
			if err := os.WriteFile(filepath.Join(output, base+".png"), []byte("png"), 0644); err != nil {
				return err
			}
		}
		return nil
	}

	iw := New(inputDir, outputDir, renderer, 1)
	renderOutputDir := filepath.Join(outputDir)

	iw.ExecuteAndTrack(context.Background(), inputFile, renderOutputDir)

	if _, err := os.Stat(filepath.Join(outputDir, "diagram.svg")); err != nil {
		t.Fatalf("expected svg output after first render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "diagram.png")); err != nil {
		t.Fatalf("expected png output after first render: %v", err)
	}

	iw.ExecuteAndTrack(context.Background(), inputFile, renderOutputDir)

	if _, err := os.Stat(filepath.Join(outputDir, "diagram.svg")); err != nil {
		t.Fatalf("expected svg output after second render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "diagram.png")); !os.IsNotExist(err) {
		t.Fatalf("expected stale png output to be removed, got err=%v", err)
	}
}

func TestRunWatchesExistingFilesWithoutInitialRender(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	existingFile := createInputFile(t, inputDir, "existing.puml")
	newFile := filepath.Join(inputDir, "new.puml")

	renderer := newFakeRenderer()
	renderer.started = make(chan string, 10)
	renderer.renderFunc = func(input, output string, call int) error {
		base := strings.TrimSuffix(filepath.Base(input), ".puml")
		return os.WriteFile(filepath.Join(output, base+".svg"), []byte(base), 0644)
	}

	iw := New(inputDir, outputDir, renderer, 2)
	iw.pollInterval = 10 * time.Millisecond

	watchStarted := make(chan string, 10)
	iw.watchFileFunc = func(ctx context.Context, filePath string) error {
		watchStarted <- filePath
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- iw.Run(ctx)
	}()

	if watched := waitForString(t, watchStarted, time.Second); watched != existingFile {
		t.Fatalf("expected existing file watcher to start first, got %s", watched)
	}

	assertNoEvent(t, renderer.started, 100*time.Millisecond, "initial render for existing file")

	if err := os.WriteFile(newFile, []byte("@startuml\n@enduml\n"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	if rendered := waitForString(t, renderer.started, time.Second); rendered != newFile {
		t.Fatalf("expected new file to render once, got %s", rendered)
	}
	if watched := waitForString(t, watchStarted, time.Second); watched != newFile {
		t.Fatalf("expected new file watcher to start, got %s", watched)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != context.Canceled {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher to stop")
	}
}

func TestRunParallelizesAcrossFilesAndSerializesSameFile(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	fileA := createInputFile(t, inputDir, "a.puml")
	fileB := createInputFile(t, inputDir, "b.puml")

	releaseRenders := make(chan struct{})
	renderer := newFakeRenderer()
	renderer.started = make(chan string, 10)
	renderer.release = releaseRenders
	renderer.renderFunc = func(input, output string, call int) error {
		base := strings.TrimSuffix(filepath.Base(input), ".puml")
		return os.WriteFile(filepath.Join(output, base+".svg"), []byte(fmt.Sprintf("%s-%d", base, call)), 0644)
	}

	iw := New(inputDir, outputDir, renderer, 2)
	iw.pollInterval = 10 * time.Millisecond

	var mu sync.Mutex
	watchCalls := make(map[string]int)
	watchEvents := make(chan string, 10)
	aChange1 := make(chan struct{})
	aChange2 := make(chan struct{})
	bChange1 := make(chan struct{})

	iw.watchFileFunc = func(ctx context.Context, filePath string) error {
		mu.Lock()
		watchCalls[filePath]++
		call := watchCalls[filePath]
		mu.Unlock()

		watchEvents <- fmt.Sprintf("%s#%d", filepath.Base(filePath), call)

		var release <-chan struct{}
		switch {
		case filePath == fileA && call == 1:
			release = aChange1
		case filePath == fileA && call == 2:
			release = aChange2
		case filePath == fileB && call == 1:
			release = bChange1
		default:
			<-ctx.Done()
			return ctx.Err()
		}

		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- iw.Run(ctx)
	}()

	waitForExpectedEvent(t, watchEvents, time.Second, map[string]bool{
		"a.puml#1": true,
		"b.puml#1": true,
	})

	close(aChange1)
	close(bChange1)

	waitForExpectedEvent(t, renderer.started, time.Second, map[string]bool{
		fileA: true,
		fileB: true,
	})
	assertNoEvent(t, watchEvents, 100*time.Millisecond, "second watch cycle for file A while its render is still running")

	close(releaseRenders)

	waitForExpectedEvent(t, watchEvents, time.Second, map[string]bool{
		"a.puml#2": true,
	})

	close(aChange2)

	if rendered := waitForString(t, renderer.started, time.Second); rendered != fileA {
		t.Fatalf("expected second render for file A, got %s", rendered)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != context.Canceled {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher to stop")
	}

	if renderer.maxActive != 2 {
		t.Fatalf("expected max active renders to be 2, got %d", renderer.maxActive)
	}
	if renderer.perFileMax[fileA] != 1 {
		t.Fatalf("expected file A renders to be serialized, got max overlap %d", renderer.perFileMax[fileA])
	}
	if renderer.perFileMax[fileB] != 1 {
		t.Fatalf("expected file B renders to be serialized, got max overlap %d", renderer.perFileMax[fileB])
	}
}
