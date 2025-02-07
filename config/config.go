package config

import "flag"

type Config struct {
	PlantUMLPath string
	InputFolder  string
	OutputFolder string
	Port         int
}

func NewFromCLIArgs() *Config {
	plantUMLPath := flag.String("plantumlPath", "plantuml.jar", "path to plantuml.jar")
	inputFolder := flag.String("input", "input", "input folder")
	outputFolder := flag.String("output", "output", "output folder")
	port := flag.Int("port", 8080, "server port")

	flag.Parse()

	return &Config{
		PlantUMLPath: *plantUMLPath,
		InputFolder:  *inputFolder,
		OutputFolder: *outputFolder,
		Port:         *port,
	}
}
