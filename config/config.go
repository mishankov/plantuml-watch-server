package config

import (
	"flag"
	"path/filepath"
)

type Config struct {
	PlantUMLPath string
	InputFolder  string
	OutputFolder string
	Port         int
}

func NewFromCLIArgs() (*Config, error) {
	plantUMLPath := flag.String("plantumlPath", "plantuml.jar", "path to plantuml.jar")
	inputFolder := flag.String("input", "input", "input folder")
	outputFolder := flag.String("output", "output", "output folder")
	port := flag.Int("port", 8080, "server port")

	flag.Parse()

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
