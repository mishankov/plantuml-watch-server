package inputwatcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mishankov/plantuml-watch-server/plantuml"
	"github.com/platforma-dev/platforma/log"
)

var ErrOutputNotTracked = errors.New("output file is not tracked")

type CompileResult struct {
	OK      bool
	Message string
}

type trackedGeneration struct {
	ModTime time.Time
	Result  CompileResult
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
	pulm       *plantuml.PlantUML
	// Maps .puml file path to the set of output files (.svg and .png) it generated
	fileToSvgMap   map[string]map[string]bool
	fileToSvgMutex sync.RWMutex
	compileCache   map[string]trackedGeneration
	compileMutex   sync.RWMutex
	fileLocks      map[string]*sync.Mutex
	fileLocksMutex sync.Mutex
}

func New(inputPath, outputPath string, pulm *plantuml.PlantUML) *InputWatcher {
	return &InputWatcher{
		inputPath:    inputPath,
		outputPath:   outputPath,
		pulm:         pulm,
		fileToSvgMap: make(map[string]map[string]bool),
		compileCache: make(map[string]trackedGeneration),
		fileLocks:    make(map[string]*sync.Mutex),
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

// getSvgFilesInDir returns a map of all output files (.svg and .png) in the given directory and its subdirectories
func (iw *InputWatcher) getSvgFilesInDir(ctx context.Context, dir string) map[string]bool {
	outputFiles := make(map[string]bool)
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
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

func (iw *InputWatcher) getFileLock(inputFile string) *sync.Mutex {
	iw.fileLocksMutex.Lock()
	defer iw.fileLocksMutex.Unlock()

	lock, ok := iw.fileLocks[inputFile]
	if !ok {
		lock = &sync.Mutex{}
		iw.fileLocks[inputFile] = lock
	}

	return lock
}

func (iw *InputWatcher) outputPathForDiagram(outputRel string) (string, error) {
	cleanRel := filepath.Clean(outputRel)
	fullPath := filepath.Join(iw.outputPath, cleanRel+".svg")

	absOutputPath, err := filepath.Abs(iw.outputPath)
	if err != nil {
		return "", err
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	if absFullPath != absOutputPath && !strings.HasPrefix(absFullPath, absOutputPath+string(filepath.Separator)) {
		return "", ErrOutputNotTracked
	}

	return absFullPath, nil
}

func (iw *InputWatcher) relativeInputPath(inputFile string) string {
	relPath, err := filepath.Rel(iw.inputPath, inputFile)
	if err != nil {
		return filepath.Base(inputFile)
	}

	return filepath.ToSlash(relPath)
}

func (iw *InputWatcher) ResolveInputForOutput(outputFile string) (string, bool) {
	iw.fileToSvgMutex.RLock()
	defer iw.fileToSvgMutex.RUnlock()

	for inputFile, outputs := range iw.fileToSvgMap {
		if outputs[outputFile] {
			return inputFile, true
		}
	}

	return "", false
}

func (iw *InputWatcher) setCompileResult(inputFile string, modTime time.Time, result CompileResult) {
	iw.compileMutex.Lock()
	defer iw.compileMutex.Unlock()

	iw.compileCache[inputFile] = trackedGeneration{
		ModTime: modTime,
		Result:  result,
	}
}

func (iw *InputWatcher) cachedCompileResult(inputFile string, modTime time.Time) (CompileResult, bool) {
	iw.compileMutex.RLock()
	defer iw.compileMutex.RUnlock()

	tracked, ok := iw.compileCache[inputFile]
	if !ok {
		return CompileResult{}, false
	}

	if tracked.ModTime.Equal(modTime) {
		return tracked.Result, true
	}

	return CompileResult{}, false
}

// ExecuteAndTrack executes PlantUML for a file and tracks which SVGs were generated.
func (iw *InputWatcher) ExecuteAndTrack(ctx context.Context, inputFile, outputDir string) CompileResult {
	// Get SVG files before execution
	svgsBefore := iw.getSvgFilesInDir(ctx, outputDir)

	// Get modification times of existing SVGs
	mtimesBefore := make(map[string]time.Time)
	for svgPath := range svgsBefore {
		if info, err := os.Stat(svgPath); err == nil {
			mtimesBefore[svgPath] = info.ModTime()
		}
	}

	outputText, err := iw.pulm.ExecuteWithFormat(ctx, inputFile, outputDir, "svg")
	if err != nil {
		return CompileResult{
			OK:      false,
			Message: outputText,
		}
	}

	if _, err := iw.pulm.ExecuteWithFormat(ctx, inputFile, outputDir, "png"); err != nil {
		log.WarnContext(ctx, "png generation failed after successful svg generation", "input", inputFile, "error", err)
	}

	// Get SVG files after execution
	svgsAfter := iw.getSvgFilesInDir(ctx, outputDir)

	// Determine which SVGs were created or modified by this execution
	generatedSvgs := make(map[string]bool)
	for svgPath := range svgsAfter {
		// New file or modified file
		if !svgsBefore[svgPath] {
			generatedSvgs[svgPath] = true
		} else if info, err := os.Stat(svgPath); err == nil {
			if beforeTime, exists := mtimesBefore[svgPath]; exists {
				if info.ModTime().After(beforeTime) {
					generatedSvgs[svgPath] = true
				}
			}
		}
	}

	// If no output files were detected as generated, fall back to expected naming
	if len(generatedSvgs) == 0 {
		// Assume the output files have the same base name as the .puml file
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

	// Get old output files for this input file
	iw.fileToSvgMutex.RLock()
	oldSvgs := iw.fileToSvgMap[inputFile]
	iw.fileToSvgMutex.RUnlock()

	// Delete output files that are no longer generated
	for oldSvg := range oldSvgs {
		if !generatedSvgs[oldSvg] {
			if err := os.Remove(oldSvg); err != nil {
				if !os.IsNotExist(err) {
					log.ErrorContext(ctx, "failed to delete orphaned output file", "file", oldSvg, "error", err)
				}
			} else {
				log.InfoContext(ctx, "deleted orphaned output file", "file", oldSvg)
			}
		}
	}

	// Update the mapping
	iw.fileToSvgMutex.Lock()
	iw.fileToSvgMap[inputFile] = generatedSvgs
	iw.fileToSvgMutex.Unlock()

	return CompileResult{OK: true}
}

func (iw *InputWatcher) RegenerateIfNeeded(ctx context.Context, inputFile string) CompileResult {
	info, err := os.Stat(inputFile)
	if err != nil {
		log.ErrorContext(ctx, "failed to stat input file before regeneration", "input", inputFile, "error", err)
		return CompileResult{
			OK:      false,
			Message: err.Error(),
		}
	}

	if cached, ok := iw.cachedCompileResult(inputFile, info.ModTime()); ok {
		log.InfoContext(ctx, "skipping duplicate compile for unchanged file", "input", inputFile)
		return cached
	}

	lock := iw.getFileLock(inputFile)
	lock.Lock()
	defer lock.Unlock()

	info, err = os.Stat(inputFile)
	if err != nil {
		log.ErrorContext(ctx, "failed to stat input file during regeneration", "input", inputFile, "error", err)
		return CompileResult{
			OK:      false,
			Message: err.Error(),
		}
	}

	if cached, ok := iw.cachedCompileResult(inputFile, info.ModTime()); ok {
		log.InfoContext(ctx, "skipping duplicate compile after waiting for file lock", "input", inputFile)
		return cached
	}

	outputDir := iw.calculateOutputDir(ctx, inputFile)
	result := iw.ExecuteAndTrack(ctx, inputFile, outputDir)
	iw.setCompileResult(inputFile, info.ModTime(), result)
	return result
}

func (iw *InputWatcher) ReadSourceForOutput(outputRel string) (string, string, error) {
	outputFile, err := iw.outputPathForDiagram(outputRel)
	if err != nil {
		return "", "", err
	}

	inputFile, ok := iw.ResolveInputForOutput(outputFile)
	if !ok {
		return "", "", ErrOutputNotTracked
	}

	content, err := os.ReadFile(inputFile)
	if err != nil {
		return "", "", err
	}

	return iw.relativeInputPath(inputFile), string(content), nil
}

func (iw *InputWatcher) WriteSourceForOutput(ctx context.Context, outputRel string, content string) (string, CompileResult, error) {
	outputFile, err := iw.outputPathForDiagram(outputRel)
	if err != nil {
		return "", CompileResult{}, err
	}

	inputFile, ok := iw.ResolveInputForOutput(outputFile)
	if !ok {
		return "", CompileResult{}, ErrOutputNotTracked
	}

	fileInfo, err := os.Stat(inputFile)
	if err != nil {
		return "", CompileResult{}, err
	}

	lock := iw.getFileLock(inputFile)
	lock.Lock()
	defer lock.Unlock()

	if err := os.WriteFile(inputFile, []byte(content), fileInfo.Mode().Perm()); err != nil {
		return "", CompileResult{}, err
	}

	info, err := os.Stat(inputFile)
	if err != nil {
		return "", CompileResult{}, err
	}

	result := iw.ExecuteAndTrack(ctx, inputFile, iw.calculateOutputDir(ctx, inputFile))
	iw.setCompileResult(inputFile, info.ModTime(), result)

	return iw.relativeInputPath(inputFile), result, nil
}

func (iw *InputWatcher) GetFiles(ctx context.Context) []string {
	files := []string{}
	err := filepath.Walk(iw.inputPath, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".puml") {
			// Skip files prefixed with underscore
			if !strings.HasPrefix(filepath.Base(path), "_") {
				files = append(files, path)
			}
		}

		return nil
	})

	if err != nil {
		log.ErrorContext(ctx, "error getting files", "error", err)
	}

	return files
}

func (iw *InputWatcher) Run(ctx context.Context) error {
	files := iw.GetFiles(ctx)
	oldFiles := []string{}

	for {
		for _, file := range files {
			if !slices.Contains(oldFiles, file) {
				log.InfoContext(ctx, "watching new file", "file", file)
				iw.RegenerateIfNeeded(ctx, file)

				go func(watchedFile string) {
					for {
						err := WatchFile(ctx, watchedFile)
						if err != nil {
							log.ErrorContext(ctx, "stopped watching file", "error", err)
							break
						}

						log.InfoContext(ctx, "file changed", "file", watchedFile)

						iw.RegenerateIfNeeded(ctx, watchedFile)
					}
				}(file)
			}
		}

		// Detect deleted files and remove corresponding output files
		for _, oldFile := range oldFiles {
			if !slices.Contains(files, oldFile) {
				log.InfoContext(ctx, "file removed", "file", oldFile)

				// Delete all output files that were generated by this file
				iw.fileToSvgMutex.RLock()
				svgs, exists := iw.fileToSvgMap[oldFile]
				iw.fileToSvgMutex.RUnlock()

				if exists {
					for svgPath := range svgs {
						if err := os.Remove(svgPath); err != nil {
							if !os.IsNotExist(err) {
								log.ErrorContext(ctx, "failed to delete output file", "file", svgPath, "error", err)

							}
						} else {
							log.InfoContext(ctx, "deleted orphaned output file", "file", svgPath)
						}
					}
					// Remove the mapping
					iw.fileToSvgMutex.Lock()
					delete(iw.fileToSvgMap, oldFile)
					iw.fileToSvgMutex.Unlock()
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}

		oldFiles = files
		files = iw.GetFiles(ctx)
	}
}
