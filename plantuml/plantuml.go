package plantuml

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/platforma-dev/platforma/log"
)

type PlantUML struct {
	jarPath string
}

func New(jarPath string) *PlantUML {
	return &PlantUML{jarPath: jarPath}
}

func (puml *PlantUML) Execute(ctx context.Context, input, output string) (string, error) {
	return puml.ExecuteWithFormat(ctx, input, output, "svg")
}

func (puml *PlantUML) ExecuteWithFormat(ctx context.Context, input, output, format string) (string, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(output, 0755); err != nil {
		log.ErrorContext(ctx, "failed to create output directory", "output", output, "error", err)
		return "", err
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
	pumlCmd := exec.CommandContext(ctx, "java", javaArgs...)

	pumlOut, err := pumlCmd.CombinedOutput()
	outputText := strings.TrimSpace(string(pumlOut))
	if err != nil {
		switch e := err.(type) {
		case *exec.Error:
			log.ErrorContext(ctx, "failed executing", "error", err)
		case *exec.ExitError:
			log.ErrorContext(ctx, "command exit", "rc", e.ExitCode())
		default:
			log.ErrorContext(ctx, "unexpected error executing plantuml", "error", err)
		}
		if outputText != "" {
			log.InfoContext(ctx, "plantuml output", "output", outputText)
		}
		return outputText, fmt.Errorf("plantuml %s generation failed: %w", format, err)
	}

	if outputText != "" {
		log.InfoContext(ctx, "plantuml output", "output", outputText)
	}

	return outputText, nil
}
