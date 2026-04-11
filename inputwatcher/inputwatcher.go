package inputwatcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/platforma-dev/platforma/log"
)

type renderer interface {
	ExecuteWithFormat(ctx context.Context, input, output, format string)
}

func WatchFile(ctx context.Context, filePath string) error {
	initialStat, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	for {
		stat, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		if stat.Size() != initialStat.Size() || stat.ModTime() != initialStat.ModTime() {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return nil
}

type InputWatcher struct {
	inputPath  string
	outputPath string
	renderer   renderer

	// Maps public diagram paths to the set of output files (.svg and .png) they generated.
	fileToOutputMap   map[string]map[string]bool
	fileToOutputMutex sync.RWMutex
	changeMutex       sync.Mutex
}

func New(inputPath, outputPath string, renderer renderer) *InputWatcher {
	return &InputWatcher{
		inputPath:         inputPath,
		outputPath:        outputPath,
		renderer:          renderer,
		fileToOutputMap:   make(map[string]map[string]bool),
		fileToOutputMutex: sync.RWMutex{},
	}
}

func (iw *InputWatcher) calculateOutputDir(ctx context.Context, inputFilePath string) string {
	relPath, err := filepath.Rel(iw.inputPath, inputFilePath)
	if err != nil {
		log.ErrorContext(ctx, "error calculating relative path", "path", inputFilePath, "error", err)
		return iw.outputPath
	}

	relDir := filepath.Dir(relPath)
	if relDir == "." {
		return iw.outputPath
	}

	return filepath.Join(iw.outputPath, relDir)
}

func (iw *InputWatcher) getSvgFilesInDir(ctx context.Context, dir string) map[string]bool {
	outputFiles := make(map[string]bool)
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".png")) {
			outputFiles[path] = true
		}
		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "error scanning output files", "dir", dir, "error", err)
	}
	return outputFiles
}

func listPlantUMLFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info == nil || info.IsDir() || !strings.HasSuffix(path, ".puml") {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func isPublicDiagram(path string) bool {
	return !strings.HasPrefix(filepath.Base(path), "_")
}

func filterPublicDiagrams(files []string) []string {
	publicFiles := make([]string, 0, len(files))
	for _, file := range files {
		if isPublicDiagram(file) {
			publicFiles = append(publicFiles, file)
		}
	}
	return publicFiles
}

func parseIncludeTarget(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}

	var remainder string
	for _, directive := range []string{"!include_once", "!include_many", "!include"} {
		if !strings.HasPrefix(trimmed, directive) {
			continue
		}

		if len(trimmed) > len(directive) {
			next := trimmed[len(directive)]
			if next != ' ' && next != '\t' {
				continue
			}
		}

		remainder = strings.TrimSpace(trimmed[len(directive):])
		break
	}
	if remainder == "" {
		return "", false
	}

	if strings.HasPrefix(remainder, "<") {
		return "", false
	}

	if quote := remainder[0]; quote == '"' || quote == '\'' {
		end := strings.IndexRune(remainder[1:], rune(quote))
		if end == -1 {
			return "", false
		}
		return remainder[1 : end+1], true
	}

	return strings.Fields(remainder)[0], true
}

func resolveIncludedPath(inputRoot, includingFile, includeTarget string) (string, bool) {
	if includeTarget == "" {
		return "", false
	}

	if strings.Contains(includeTarget, "://") {
		return "", false
	}

	candidates := []string{includeTarget}
	if filepath.Ext(includeTarget) == "" {
		candidates = append(candidates, includeTarget+".puml")
	}

	for _, candidate := range candidates {
		cleanCandidate := filepath.Clean(candidate)

		var resolved string
		if filepath.IsAbs(cleanCandidate) {
			resolved = cleanCandidate
		} else {
			resolved = filepath.Join(filepath.Dir(includingFile), cleanCandidate)
		}

		absResolved, err := filepath.Abs(resolved)
		if err != nil {
			continue
		}

		absRoot, err := filepath.Abs(inputRoot)
		if err != nil {
			continue
		}

		if absResolved != absRoot && !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) {
			continue
		}

		info, err := os.Stat(absResolved)
		if err != nil || info.IsDir() || filepath.Ext(absResolved) != ".puml" {
			continue
		}

		return absResolved, true
	}

	return "", false
}

func readLocalIncludes(inputRoot, sourceFile string) ([]string, error) {
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, err
	}

	includes := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		target, ok := parseIncludeTarget(line)
		if !ok {
			continue
		}

		resolved, ok := resolveIncludedPath(inputRoot, sourceFile, target)
		if ok {
			includes = append(includes, resolved)
		}
	}

	return includes, nil
}

func buildAffectedPublicIndex(inputRoot string, sourceFiles []string) (map[string][]string, error) {
	sourceSet := make(map[string]bool, len(sourceFiles))
	for _, file := range sourceFiles {
		sourceSet[file] = true
	}

	adjacency := make(map[string][]string, len(sourceFiles))
	var buildErr error
	for _, file := range sourceFiles {
		includes, err := readLocalIncludes(inputRoot, file)
		if err != nil {
			buildErr = errors.Join(buildErr, err)
			continue
		}

		deduped := make(map[string]bool, len(includes))
		for _, include := range includes {
			if sourceSet[include] {
				deduped[include] = true
			}
		}

		neighbors := make([]string, 0, len(deduped))
		for include := range deduped {
			neighbors = append(neighbors, include)
		}
		sort.Strings(neighbors)
		adjacency[file] = neighbors
	}

	affected := make(map[string][]string, len(sourceFiles))
	for _, publicDiagram := range filterPublicDiagrams(sourceFiles) {
		visited := map[string]bool{}
		stack := []string{publicDiagram}

		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[current] {
				continue
			}

			visited[current] = true
			affected[current] = append(affected[current], publicDiagram)

			for _, include := range adjacency[current] {
				if !visited[include] {
					stack = append(stack, include)
				}
			}
		}
	}

	for _, diagrams := range affected {
		sort.Strings(diagrams)
	}

	return affected, buildErr
}

func (iw *InputWatcher) GetSourceFiles(ctx context.Context) []string {
	files, err := listPlantUMLFiles(iw.inputPath)
	if err != nil {
		log.ErrorContext(ctx, "error getting source files", "error", err)
		return nil
	}

	return files
}

func (iw *InputWatcher) GetPublicFiles(ctx context.Context) []string {
	return filterPublicDiagrams(iw.GetSourceFiles(ctx))
}

func (iw *InputWatcher) ExecuteAndTrack(ctx context.Context, inputFile, outputDir string) {
	svgsBefore := iw.getSvgFilesInDir(ctx, outputDir)

	mtimesBefore := make(map[string]time.Time)
	for svgPath := range svgsBefore {
		if info, err := os.Stat(svgPath); err == nil {
			mtimesBefore[svgPath] = info.ModTime()
		}
	}

	iw.renderer.ExecuteWithFormat(ctx, inputFile, outputDir, "svg")
	iw.renderer.ExecuteWithFormat(ctx, inputFile, outputDir, "png")

	svgsAfter := iw.getSvgFilesInDir(ctx, outputDir)

	generatedSvgs := make(map[string]bool)
	for svgPath := range svgsAfter {
		if !svgsBefore[svgPath] {
			generatedSvgs[svgPath] = true
		} else if info, err := os.Stat(svgPath); err == nil {
			if beforeTime, exists := mtimesBefore[svgPath]; exists && info.ModTime().After(beforeTime) {
				generatedSvgs[svgPath] = true
			}
		}
	}

	if len(generatedSvgs) == 0 {
		baseName := strings.TrimSuffix(filepath.Base(inputFile), ".puml")
		expectedSvg := filepath.Join(outputDir, baseName+".svg")
		expectedPng := filepath.Join(outputDir, baseName+".png")
		if _, err := os.Stat(expectedSvg); err == nil {
			generatedSvgs[expectedSvg] = true
		}
		if _, err := os.Stat(expectedPng); err == nil {
			generatedSvgs[expectedPng] = true
		}
	}

	iw.fileToOutputMutex.RLock()
	oldOutputs := iw.fileToOutputMap[inputFile]
	iw.fileToOutputMutex.RUnlock()

	for oldOutput := range oldOutputs {
		if !generatedSvgs[oldOutput] {
			if err := os.Remove(oldOutput); err != nil {
				if !os.IsNotExist(err) {
					log.ErrorContext(ctx, "failed to delete orphaned output file", "file", oldOutput, "error", err)
				}
			} else {
				log.InfoContext(ctx, "deleted orphaned output file", "file", oldOutput)
			}
		}
	}

	iw.fileToOutputMutex.Lock()
	iw.fileToOutputMap[inputFile] = generatedSvgs
	iw.fileToOutputMutex.Unlock()
}

func (iw *InputWatcher) renderPublicDiagram(ctx context.Context, inputFile string) {
	iw.ExecuteAndTrack(ctx, inputFile, iw.calculateOutputDir(ctx, inputFile))
}

func (iw *InputWatcher) RenderPublicDiagram(ctx context.Context, inputFile string) {
	iw.changeMutex.Lock()
	defer iw.changeMutex.Unlock()

	iw.renderPublicDiagram(ctx, inputFile)
}

func (iw *InputWatcher) affectedPublicDiagrams(ctx context.Context, changedFile string) []string {
	sourceFiles := iw.GetSourceFiles(ctx)
	publicFiles := filterPublicDiagrams(sourceFiles)

	affectedIndex, err := buildAffectedPublicIndex(iw.inputPath, sourceFiles)
	if err != nil {
		log.ErrorContext(ctx, "failed building include dependency index; falling back to all public diagrams", "error", err)
		return publicFiles
	}

	affected := affectedIndex[changedFile]
	if len(affected) == 0 && isPublicDiagram(changedFile) {
		return []string{changedFile}
	}

	return affected
}

func (iw *InputWatcher) HandleSourceChange(ctx context.Context, changedFile string) {
	iw.changeMutex.Lock()
	defer iw.changeMutex.Unlock()

	for _, publicDiagram := range iw.affectedPublicDiagrams(ctx, changedFile) {
		log.InfoContext(ctx, "regenerating affected diagram", "source", changedFile, "diagram", publicDiagram)
		iw.renderPublicDiagram(ctx, publicDiagram)
	}
}

func (iw *InputWatcher) DeleteTrackedOutputs(ctx context.Context, inputFile string) {
	iw.fileToOutputMutex.RLock()
	outputs, exists := iw.fileToOutputMap[inputFile]
	iw.fileToOutputMutex.RUnlock()

	if !exists {
		return
	}

	for outputPath := range outputs {
		if err := os.Remove(outputPath); err != nil {
			if !os.IsNotExist(err) {
				log.ErrorContext(ctx, "failed to delete output file", "file", outputPath, "error", err)
			}
		} else {
			log.InfoContext(ctx, "deleted orphaned output file", "file", outputPath)
		}
	}

	iw.fileToOutputMutex.Lock()
	delete(iw.fileToOutputMap, inputFile)
	iw.fileToOutputMutex.Unlock()
}

func (iw *InputWatcher) Run(ctx context.Context) error {
	sourceFiles := iw.GetSourceFiles(ctx)
	oldFiles := []string{}

	for {
		for _, file := range sourceFiles {
			if !slices.Contains(oldFiles, file) {
				log.InfoContext(ctx, "watching new source file", "file", file)
				if isPublicDiagram(file) {
					iw.RenderPublicDiagram(ctx, file)
				}

				go func(watchedFile string) {
					for {
						err := WatchFile(ctx, watchedFile)
						if err != nil {
							log.ErrorContext(ctx, "stopped watching file", "file", watchedFile, "error", err)
							break
						}

						log.InfoContext(ctx, "source file changed", "file", watchedFile)
						iw.HandleSourceChange(ctx, watchedFile)
					}
				}(file)
			}
		}

		for _, oldFile := range oldFiles {
			if !slices.Contains(sourceFiles, oldFile) {
				log.InfoContext(ctx, "file removed", "file", oldFile)
				if isPublicDiagram(oldFile) {
					iw.DeleteTrackedOutputs(ctx, oldFile)
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}

		oldFiles = sourceFiles
		sourceFiles = iw.GetSourceFiles(ctx)
	}
}
