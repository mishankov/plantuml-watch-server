package plantuml

import (
	"context"
	"os"
	"os/exec"

	"github.com/platforma-dev/platforma/log"
)

type PlantUML struct {
	jarPath string
}

func New(jarPath string) *PlantUML {
	return &PlantUML{jarPath: jarPath}
}

func (puml *PlantUML) Execute(ctx context.Context, input, output string) {
	puml.ExecuteWithFormat(ctx, input, output, "svg")
}

func (puml *PlantUML) ExecuteWithFormat(ctx context.Context, input, output, format string) {
	// Ensure output directory exists
	if err := os.MkdirAll(output, 0755); err != nil {
		log.ErrorContext(ctx, "failed to create output directory", "output", output, "error", err)
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
		log.WarnContext(ctx, "unknown format, defaulting to SVG", "format", format)
		formatFlag = "-tsvg"
	}

	javaArgs := []string{"-jar", puml.jarPath, "-o", output, formatFlag, input}
	pumlCmd := exec.Command("java", javaArgs...)

	pumlOut, err := pumlCmd.CombinedOutput()
	if err != nil {
		switch e := err.(type) {
		case *exec.Error:
			log.ErrorContext(ctx, "failed executing", "error", err)
		case *exec.ExitError:
			log.ErrorContext(ctx, "command exit", "rc", e.ExitCode())
		default:
			log.ErrorContext(ctx, "unexpected error executing plantuml", "error", err)
		}
	}

	if len(pumlOut) != 0 {
		log.InfoContext(ctx, "plantuml output", "output", string(pumlOut))
	}
}
