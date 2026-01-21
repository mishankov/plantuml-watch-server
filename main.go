package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/mishankov/plantuml-watch-server/config"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/mishankov/plantuml-watch-server/plantuml"
	"github.com/mishankov/plantuml-watch-server/server"
)

//go:embed static
var staticFiles embed.FS

//go:embed templates
var templateFiles embed.FS

func calculateOutputDirForFile(inputFilePath, inputRoot, outputRoot string) string {
	relPath, err := filepath.Rel(inputRoot, inputFilePath)
	if err != nil {
		log.Printf("Error calculating relative path for %s: %v", inputFilePath, err)
		return outputRoot
	}

	relDir := filepath.Dir(relPath)
	if relDir == "." {
		return outputRoot
	}

	return filepath.Join(outputRoot, relDir)
}

func main() {
	ctx := context.Background()

	config, err := config.NewFromCLIArgs()
	if err != nil {
		log.Fatalln(err)
	}

	puml := plantuml.New(config.PlantUMLPath)
	iw := inputwatcher.New(config.InputFolder, config.OutputFolder, puml)
	server := server.New(staticFiles, templateFiles, config.OutputFolder, config.Port)

	// Remove all stale outputs
	os.RemoveAll(config.OutputFolder + "/")

	// Generate initial SVGs - iterate through each file to preserve structure
	inputPattern := config.InputFolder + "/**.puml"
	files, err := filepath.Glob(inputPattern)
	if err != nil {
		log.Fatalln("Error finding .puml files:", err)
	}

	for _, file := range files {
		outputDir := calculateOutputDirForFile(file, config.InputFolder, config.OutputFolder)
		puml.Execute(file, outputDir)
	}

	// Watch input changes
	go iw.Watch(ctx)

	// Run server
	server.Serve()
}
