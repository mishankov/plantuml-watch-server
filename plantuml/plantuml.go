package plantuml

import (
	"log"
	"os"
	"os/exec"
)

type PlantUML struct {
	jarPath string
}

func New(jarPath string) *PlantUML {
	return &PlantUML{jarPath: jarPath}
}

func (puml *PlantUML) Execute(input, output string) {
	puml.ExecuteWithFormat(input, output, "svg")
}

func (puml *PlantUML) ExecuteWithFormat(input, output, format string) {
	// Ensure output directory exists
	if err := os.MkdirAll(output, 0755); err != nil {
		log.Printf("Failed to create output directory %s: %v", output, err)
		return
	}

	// Map format to PlantUML flag
	var formatFlag string
	switch format {
	case "svg":
		formatFlag = "-tsvg"
	case "png":
		formatFlag = "-tpng"
	default:
		log.Printf("Unknown format %s, defaulting to SVG", format)
		formatFlag = "-tsvg"
	}

	javaArgs := []string{"-jar", puml.jarPath, "-o", output, formatFlag, input}
	pumlCmd := exec.Command("java", javaArgs...)

	pumlOut, err := pumlCmd.CombinedOutput()
	if err != nil {
		switch e := err.(type) {
		case *exec.Error:
			log.Println("failed executing:", err)
		case *exec.ExitError:
			log.Println("command exit rc =", e.ExitCode())
		default:
			panic(err)
		}
	}

	if len(pumlOut) != 0 {
		log.Println(string(pumlOut))
	}
}
