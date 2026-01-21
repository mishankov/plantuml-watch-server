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
}

func New(inputPath, outputPath string, pulm *plantuml.PlantUML) *InputWatcher {
	return &InputWatcher{inputPath: inputPath, outputPath: outputPath, pulm: pulm}
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

func (iw *InputWatcher) GetFiles() []string {
	files := []string{}
	err := filepath.Walk(iw.inputPath, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".puml") {
			files = append(files, path)
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
				iw.pulm.Execute(file, outputDir)

				// Fix goroutine closure bug by passing file and outputDir as parameters
				go func(watchedFile string, watchedOutputDir string) {
					for {
						err := WatchFile(ctx, watchedFile)
						if err != nil {
							log.Println("Stopped watchFile:", err)
							break
						}

						log.Println("File changed:", watchedFile)
						iw.pulm.Execute(watchedFile, watchedOutputDir)
					}
				}(file, outputDir)
			}
		}

		// Detect deleted files and remove corresponding SVGs
		for _, oldFile := range oldFiles {
			if !slices.Contains(files, oldFile) {
				log.Println("File removed:", oldFile)

				// Calculate the corresponding SVG file path
				outputDir := iw.calculateOutputDir(oldFile)
				svgFileName := strings.TrimSuffix(filepath.Base(oldFile), ".puml") + ".svg"
				svgPath := filepath.Join(outputDir, svgFileName)

				// Remove the orphaned SVG file
				if err := os.Remove(svgPath); err != nil {
					if !os.IsNotExist(err) {
						log.Printf("Failed to delete SVG %s: %v", svgPath, err)
					}
				} else {
					log.Println("Deleted orphaned SVG:", svgPath)
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
