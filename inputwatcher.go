package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func watchFile(filePath string) error {
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

		time.Sleep(1 * time.Second)
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

func (iw *InputWatcher) Watch() {
	iw.GetFiles()
	oldFiles := []string{}

	for {
		for _, file := range iw.files {
			if !slices.Contains(oldFiles, file) {
				log.Println("Watching new file:", file)
				go func() {
					for {
						err := watchFile(file)
						if err != nil {
							log.Println(err)
							break
						}

						log.Println("File changed:", file)
						runPlantUML(file, "/output")
					}
				}()
			}
		}

		time.Sleep(1 * time.Second)

		oldFiles = iw.files
		iw.GetFiles()
	}
}
