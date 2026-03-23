package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	PlantUMLPath string
	InputFolder  string
	OutputFolder string
	Port         int
	Parallelism  int
}

func NewFromCLIArgs() (*Config, error) {
	return NewFromArgs(os.Args[1:])
}

func NewFromArgs(args []string) (*Config, error) {
	flags := flag.NewFlagSet("plantuml-watch-server", flag.ContinueOnError)

	plantUMLPath := flags.String("plantumlPath", "plantuml.jar", "path to plantuml.jar")
	inputFolder := flags.String("input", "input", "input folder")
	outputFolder := flags.String("output", "output", "output folder")
	port := flags.Int("port", 8080, "server port")
	parallelism := flags.Int("parallelism", runtime.NumCPU(), "maximum number of diagrams to render in parallel")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if *parallelism < 1 {
		return nil, fmt.Errorf("parallelism must be at least 1")
	}

	inputFolderStr, err := filepath.Abs(*inputFolder)
	if err != nil {
		return nil, err
	}

	outputFolderStr, err := filepath.Abs(*outputFolder)
	if err != nil {
		return nil, err
	}

	return &Config{
		PlantUMLPath: *plantUMLPath,
		InputFolder:  inputFolderStr,
		OutputFolder: outputFolderStr,
		Port:         *port,
		Parallelism:  *parallelism,
	}, nil
}
