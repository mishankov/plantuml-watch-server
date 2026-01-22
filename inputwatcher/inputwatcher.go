package inputwatcher

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mishankov/plantuml-watch-server/plantuml"
)

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
	fileToSvgMap map[string]map[string]bool
}

func New(inputPath, outputPath string, pulm *plantuml.PlantUML) *InputWatcher {
	return &InputWatcher{
		inputPath:    inputPath,
		outputPath:   outputPath,
		pulm:         pulm,
		fileToSvgMap: make(map[string]map[string]bool),
	}
}

func (iw *InputWatcher) calculateOutputDir(inputFilePath string) string {
	relPath, err := filepath.Rel(iw.inputPath, inputFilePath)
	if err != nil {
		log.Printf("Error calculating relative path for %s: %v", inputFilePath, err)
		return iw.outputPath
	}

	relDir := filepath.Dir(relPath)
	if relDir == "." {
		return iw.outputPath
	}

	return filepath.Join(iw.outputPath, relDir)
}

// getSvgFilesInDir returns a map of all output files (.svg and .png) in the given directory and its subdirectories
func (iw *InputWatcher) getSvgFilesInDir(dir string) map[string]bool {
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
		log.Printf("Error scanning output files in %s: %v", dir, err)
	}
	return outputFiles
}

// ExecuteAndTrack executes PlantUML for a file and tracks which SVGs were generated
func (iw *InputWatcher) ExecuteAndTrack(inputFile, outputDir string) {
	// Get SVG files before execution
	svgsBefore := iw.getSvgFilesInDir(outputDir)

	// Get modification times of existing SVGs
	mtimesBefore := make(map[string]time.Time)
	for svgPath := range svgsBefore {
		if info, err := os.Stat(svgPath); err == nil {
			mtimesBefore[svgPath] = info.ModTime()
		}
	}

	// Execute PlantUML for both SVG and PNG formats
	iw.pulm.ExecuteWithFormat(inputFile, outputDir, "svg")
	iw.pulm.ExecuteWithFormat(inputFile, outputDir, "png")

	// Get SVG files after execution
	svgsAfter := iw.getSvgFilesInDir(outputDir)

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
	oldSvgs := iw.fileToSvgMap[inputFile]

	// Delete output files that are no longer generated
	for oldSvg := range oldSvgs {
		if !generatedSvgs[oldSvg] {
			if err := os.Remove(oldSvg); err != nil {
				if !os.IsNotExist(err) {
					log.Printf("Failed to delete orphaned output file %s: %v", oldSvg, err)
				}
			} else {
				log.Println("Deleted orphaned output file:", oldSvg)
			}
		}
	}

	// Update the mapping
	iw.fileToSvgMap[inputFile] = generatedSvgs
}

func (iw *InputWatcher) GetFiles() []string {
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
		log.Fatalln(err)
	}

	return files
}

func (iw *InputWatcher) Watch(ctx context.Context) {
	files := iw.GetFiles()
	oldFiles := []string{}

	for {
		for _, file := range files {
			if !slices.Contains(oldFiles, file) {
				log.Println("Watching new file:", file)
				outputDir := iw.calculateOutputDir(file)
				iw.ExecuteAndTrack(file, outputDir)

				// Fix goroutine closure bug by passing file and outputDir as parameters
				go func(watchedFile string, watchedOutputDir string) {
					for {
						err := WatchFile(ctx, watchedFile)
						if err != nil {
							log.Println("Stopped watchFile:", err)
							break
						}

						log.Println("File changed:", watchedFile)
						iw.ExecuteAndTrack(watchedFile, watchedOutputDir)
					}
				}(file, outputDir)
			}
		}

		// Detect deleted files and remove corresponding output files
		for _, oldFile := range oldFiles {
			if !slices.Contains(files, oldFile) {
				log.Println("File removed:", oldFile)

				// Delete all output files that were generated by this file
				if svgs, exists := iw.fileToSvgMap[oldFile]; exists {
					for svgPath := range svgs {
						if err := os.Remove(svgPath); err != nil {
							if !os.IsNotExist(err) {
								log.Printf("Failed to delete output file %s: %v", svgPath, err)
							}
						} else {
							log.Println("Deleted orphaned output file:", svgPath)
						}
					}
					// Remove the mapping
					delete(iw.fileToSvgMap, oldFile)
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}

		oldFiles = files
		files = iw.GetFiles()
	}
}
