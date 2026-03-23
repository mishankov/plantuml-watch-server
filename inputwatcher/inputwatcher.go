package inputwatcher

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/platforma-dev/platforma/log"
)

type Renderer interface {
	Render(ctx context.Context, input, output string)
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
	inputPath     string
	outputPath    string
	renderer      Renderer
	renderSlots   chan struct{}
	pollInterval  time.Duration
	watchFileFunc func(ctx context.Context, filePath string) error
	// Maps .puml file path to the set of output files (.svg and .png) it generated
	fileToSvgMap   map[string]map[string]bool
	fileToSvgMutex sync.RWMutex
}

func New(inputPath, outputPath string, renderer Renderer, parallelism int) *InputWatcher {
	if parallelism < 1 {
		parallelism = 1
	}

	return &InputWatcher{
		inputPath:     inputPath,
		outputPath:    outputPath,
		renderer:      renderer,
		renderSlots:   make(chan struct{}, parallelism),
		pollInterval:  100 * time.Millisecond,
		watchFileFunc: WatchFile,
		fileToSvgMap:  make(map[string]map[string]bool),
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

func (iw *InputWatcher) getOutputFilesInDir(ctx context.Context, dir string) map[string]bool {
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

func (iw *InputWatcher) acquireRenderSlot(ctx context.Context) bool {
	select {
	case iw.renderSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (iw *InputWatcher) releaseRenderSlot() {
	<-iw.renderSlots
}

func (iw *InputWatcher) RenderFiles(ctx context.Context, files []string) {
	var wg sync.WaitGroup

	for _, file := range files {
		outputDir := iw.calculateOutputDir(ctx, file)

		wg.Add(1)
		go func(filePath string, renderOutputDir string) {
			defer wg.Done()
			iw.ExecuteAndTrack(ctx, filePath, renderOutputDir)
		}(file, outputDir)
	}

	wg.Wait()
}

func moveFile(src, dst string) error {
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}

	if err := target.Close(); err != nil {
		return err
	}

	return os.Remove(src)
}

// ExecuteAndTrack executes PlantUML for a file and tracks which outputs were generated.
func (iw *InputWatcher) ExecuteAndTrack(ctx context.Context, inputFile, outputDir string) {
	if !iw.acquireRenderSlot(ctx) {
		return
	}
	defer iw.releaseRenderSlot()

	parentDir := filepath.Dir(outputDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		log.ErrorContext(ctx, "failed to prepare output parent directory", "dir", parentDir, "error", err)
		return
	}

	tempDir, err := os.MkdirTemp(parentDir, ".render-*")
	if err != nil {
		log.ErrorContext(ctx, "failed to create temporary render directory", "dir", parentDir, "error", err)
		return
	}
	defer os.RemoveAll(tempDir)

	iw.renderer.Render(ctx, inputFile, tempDir)

	tempOutputs := iw.getOutputFilesInDir(ctx, tempDir)
	generatedOutputs := make(map[string]bool)

	for tempPath := range tempOutputs {
		relPath, err := filepath.Rel(tempDir, tempPath)
		if err != nil {
			log.ErrorContext(ctx, "failed to calculate rendered file path", "file", tempPath, "error", err)
			continue
		}

		finalPath := filepath.Join(outputDir, relPath)
		if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
			log.ErrorContext(ctx, "failed to prepare output directory", "dir", filepath.Dir(finalPath), "error", err)
			continue
		}

		if err := moveFile(tempPath, finalPath); err != nil {
			log.ErrorContext(ctx, "failed to move rendered output", "src", tempPath, "dst", finalPath, "error", err)
			continue
		}

		generatedOutputs[finalPath] = true
	}

	// If no output files were detected as generated, fall back to expected naming
	if len(generatedOutputs) == 0 {
		baseName := strings.TrimSuffix(filepath.Base(inputFile), ".puml")
		expectedSvg := filepath.Join(tempDir, baseName+".svg")
		expectedPng := filepath.Join(tempDir, baseName+".png")
		if _, err := os.Stat(expectedSvg); err == nil {
			finalPath := filepath.Join(outputDir, baseName+".svg")
			if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err == nil {
				if err := moveFile(expectedSvg, finalPath); err == nil {
					generatedOutputs[finalPath] = true
				}
			}
		}
		if _, err := os.Stat(expectedPng); err == nil {
			finalPath := filepath.Join(outputDir, baseName+".png")
			if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err == nil {
				if err := moveFile(expectedPng, finalPath); err == nil {
					generatedOutputs[finalPath] = true
				}
			}
		}
	}

	// Get old output files for this input file
	iw.fileToSvgMutex.RLock()
	oldSvgs := iw.fileToSvgMap[inputFile]
	iw.fileToSvgMutex.RUnlock()

	// Delete output files that are no longer generated
	for oldSvg := range oldSvgs {
		if !generatedOutputs[oldSvg] {
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
	iw.fileToSvgMap[inputFile] = generatedOutputs
	iw.fileToSvgMutex.Unlock()
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

func (iw *InputWatcher) startWatchingFile(ctx context.Context, watchedFile string, watchedOutputDir string) {
	go func() {
		for {
			err := iw.watchFileFunc(ctx, watchedFile)
			if err != nil {
				log.ErrorContext(ctx, "stopped watching file", "error", err)
				break
			}

			log.InfoContext(ctx, "file changed", "file", watchedFile)
			iw.ExecuteAndTrack(ctx, watchedFile, watchedOutputDir)
		}
	}()
}

func (iw *InputWatcher) Run(ctx context.Context) error {
	oldFiles := iw.GetFiles(ctx)
	for _, file := range oldFiles {
		log.InfoContext(ctx, "watching existing file", "file", file)
		iw.startWatchingFile(ctx, file, iw.calculateOutputDir(ctx, file))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(iw.pollInterval):
		}

		files := iw.GetFiles(ctx)

		for _, file := range files {
			if !slices.Contains(oldFiles, file) {
				log.InfoContext(ctx, "watching new file", "file", file)
				outputDir := iw.calculateOutputDir(ctx, file)
				iw.ExecuteAndTrack(ctx, file, outputDir)
				iw.startWatchingFile(ctx, file, outputDir)
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

		oldFiles = files
	}
}
