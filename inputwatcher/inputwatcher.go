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
	inputPath string
	pulm      *plantuml.PlantUML
}

func New(inputPath string, pulm *plantuml.PlantUML) *InputWatcher {
	return &InputWatcher{inputPath: inputPath, pulm: pulm}
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
				go func() {
					for {
						err := WatchFile(ctx, file)
						if err != nil {
							log.Println("Stopped watchFile:", err)
							break
						}

						log.Println("File changed:", file)
						iw.pulm.Execute(file, "/output")
					}
				}()
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
