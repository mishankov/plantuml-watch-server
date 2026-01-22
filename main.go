package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mishankov/plantuml-watch-server/config"
	"github.com/mishankov/plantuml-watch-server/handlers"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/mishankov/plantuml-watch-server/plantuml"
	"github.com/platforma-dev/platforma/application"
	"github.com/platforma-dev/platforma/httpserver"
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
	app := application.New()

	config, err := config.NewFromCLIArgs()
	if err != nil {
		log.Fatalln(err)
	}

	puml := plantuml.New(config.PlantUMLPath)
	iw := inputwatcher.New(config.InputFolder, config.OutputFolder, puml)

	// Preparing termplates
	tmpls, err := template.New("").ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatalln(err)
	}

	app.OnStartFunc(func(ctx context.Context) error {
		// Remove all stale outputs
		os.RemoveAll(config.OutputFolder + "/")

		// Generate initial SVGs - iterate through each file to preserve structure
		inputPattern := config.InputFolder + "/**.puml"
		files, err := filepath.Glob(inputPattern)
		if err != nil {
			return fmt.Errorf("Error finding .puml files: %w", err)
		}

		for _, file := range files {
			// Skip files prefixed with underscore
			if strings.HasPrefix(filepath.Base(file), "_") {
				continue
			}

			outputDir := calculateOutputDirForFile(file, config.InputFolder, config.OutputFolder)
			iw.ExecuteAndTrack(ctx, file, outputDir)
		}
		return nil
	}, application.StartupTaskConfig{Name: "initial generation", AbortOnError: true})

	server := httpserver.New(strconv.Itoa(config.Port), 3*time.Second)

	server.Handle("/output/{name...}", handlers.NewSvgViewHandler(config.OutputFolder, tmpls))
	server.Handle("/ws/{name...}", handlers.NewSVGWSHandler(config.OutputFolder))
	server.Handle("/download/{name...}", handlers.NewDownloadHandler(config.OutputFolder))
	server.Handle("/static/{file}", http.FileServer(http.FS(staticFiles)))
	server.Handle("/", handlers.NewIndexHandler(config.OutputFolder, tmpls))

	app.RegisterService("file watcher", iw)
	app.RegisterService("server", server)

	if err := app.Run(ctx); err != nil {
		log.Fatalln(err)
	}
}
