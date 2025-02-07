package main

import (
	"context"
	"embed"
	"os"

	"github.com/mishankov/plantuml-watch-server/config"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/mishankov/plantuml-watch-server/plantuml"
	"github.com/mishankov/plantuml-watch-server/server"
)

//go:embed static
var staticFiles embed.FS

//go:embed templates
var templateFiles embed.FS

func main() {
	ctx := context.Background()

	config := config.NewFromCLIArgs()
	puml := plantuml.New(config.PlantUMLPath)
	iw := inputwatcher.New(config.InputFolder, puml)
	server := server.New(staticFiles, templateFiles, config.OutputFolder, config.Port)

	// Remove all stale outputs
	os.RemoveAll(config.OutputFolder + "/")

	// Generate initial SVGs
	puml.Execute(config.InputFolder+"/*.puml", config.OutputFolder)

	// Watch input changes
	go iw.Watch(ctx)

	// Run server
	server.Serve()
}
