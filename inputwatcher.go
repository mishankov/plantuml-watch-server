package main

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func watchFile(ctx context.Context, filePath string) error {
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
	files []string
}

func (iw *InputWatcher) GetFiles() {
	iw.files = []string{}
	err := filepath.Walk("/input", func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".puml") {
			iw.files = append(iw.files, path)
		}

		return nil
	})

	if err != nil {
		log.Fatalln(err)
	}
}

func (iw *InputWatcher) Watch(ctx context.Context) {
	iw.GetFiles()
	oldFiles := []string{}

	for {
		for _, file := range iw.files {
			if !slices.Contains(oldFiles, file) {
				log.Println("Watching new file:", file)
				go func() {
					for {
						err := watchFile(ctx, file)
						if err != nil {
							log.Println("Stopped watchFile:", err)
							break
						}

						log.Println("File changed:", file)
						runPlantUML(file, "/output")
					}
				}()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}

		oldFiles = iw.files
		iw.GetFiles()
	}
}
