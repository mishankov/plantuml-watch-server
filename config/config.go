package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	PlantUMLPath string
	InputFolder  string
	OutputFolder string
	Port         int
}

func NewFromCLIArgs() (*Config, error) {
	if len(os.Args) < 3 {
		return NewFromArgs(nil)
	}

	return NewFromArgs(os.Args[2:])
}

func NewFromArgs(args []string) (*Config, error) {
	flagSet := flag.NewFlagSet("plantuml-watch-server", flag.ContinueOnError)

	plantUMLPath := flagSet.String("plantumlPath", "plantuml.jar", "path to plantuml.jar")
	inputFolder := flagSet.String("input", "input", "input folder")
	outputFolder := flagSet.String("output", "output", "output folder")
	port := flagSet.Int("port", 8080, "server port")

	if err := flagSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, err
		}

		return nil, fmt.Errorf("parse flags: %w", err)
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
	}, nil
}
