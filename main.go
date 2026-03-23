package main

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mishankov/plantuml-watch-server/config"
	"github.com/mishankov/plantuml-watch-server/handlers"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/mishankov/plantuml-watch-server/plantuml"
	"github.com/platforma-dev/platforma/application"
	"github.com/platforma-dev/platforma/httpserver"
	"github.com/platforma-dev/platforma/log"
)

//go:embed static
var staticFiles embed.FS

//go:embed templates
var templateFiles embed.FS

func main() {
	ctx := context.Background()
	app := application.New()

	config, err := config.NewFromCLIArgs()
	if err != nil {
		log.ErrorContext(ctx, "failed to load config", "error", err)
		return
	}

	puml := plantuml.New(config.PlantUMLPath)
	iw := inputwatcher.New(config.InputFolder, config.OutputFolder, puml, config.Parallelism)

	// Preparing termplates
	tmpls, err := template.New("").ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.ErrorContext(ctx, "failed to parse templates", "error", err)
		return
	}

	app.OnStartFunc(func(ctx context.Context) error {
		// Remove all stale outputs
		os.RemoveAll(config.OutputFolder + "/")

		iw.RenderFiles(ctx, iw.GetFiles(ctx))
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
		log.InfoContext(ctx, "application exited", "error", err)
	}
}
