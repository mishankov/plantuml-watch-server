package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFromArgsParsesFlagsWithoutSubcommand(t *testing.T) {
	cfg, err := NewFromArgs([]string{"-input=./in", "-output=./out", "-port=9999"})
	if err != nil {
		t.Fatalf("NewFromArgs returned error: %v", err)
	}

	assertConfigPaths(t, cfg, "./in", "./out")
	if cfg.Port != 9999 {
		t.Fatalf("expected port 9999, got %d", cfg.Port)
	}
}

func TestNewFromArgsParsesFlagsAfterRun(t *testing.T) {
	cfg, err := NewFromArgs([]string{"-input=./in", "-output=./out", "-port=9999"})
	if err != nil {
		t.Fatalf("NewFromArgs returned error: %v", err)
	}

	assertConfigPaths(t, cfg, "./in", "./out")
	if cfg.Port != 9999 {
		t.Fatalf("expected port 9999, got %d", cfg.Port)
	}
}

func TestNewFromCLIArgsParsesFlagsAfterRun(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	os.Args = []string{"plantuml-watch-server", "run", "-input=./in", "-output=./out", "-port=9999"}

	cfg, err := NewFromCLIArgs()
	if err != nil {
		t.Fatalf("NewFromCLIArgs returned error: %v", err)
	}

	assertConfigPaths(t, cfg, "./in", "./out")
	if cfg.Port != 9999 {
		t.Fatalf("expected port 9999, got %d", cfg.Port)
	}
}

func TestNewFromCLIArgsDefaultsWithoutFlags(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	os.Args = []string{"plantuml-watch-server", "run"}

	cfg, err := NewFromCLIArgs()
	if err != nil {
		t.Fatalf("NewFromCLIArgs returned error: %v", err)
	}

	expectedInput, err := filepath.Abs("input")
	if err != nil {
		t.Fatalf("filepath.Abs(input): %v", err)
	}
	expectedOutput, err := filepath.Abs("output")
	if err != nil {
		t.Fatalf("filepath.Abs(output): %v", err)
	}

	if cfg.PlantUMLPath != "plantuml.jar" {
		t.Fatalf("expected default PlantUMLPath, got %q", cfg.PlantUMLPath)
	}
	if cfg.InputFolder != expectedInput {
		t.Fatalf("expected input folder %q, got %q", expectedInput, cfg.InputFolder)
	}
	if cfg.OutputFolder != expectedOutput {
		t.Fatalf("expected output folder %q, got %q", expectedOutput, cfg.OutputFolder)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Port)
	}
}

func TestNewFromArgsHelp(t *testing.T) {
	cfg, err := NewFromArgs([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config on help, got %#v", cfg)
	}
}

func assertConfigPaths(t *testing.T, cfg *Config, inputPath, outputPath string) {
	t.Helper()

	expectedInput, err := filepath.Abs(inputPath)
	if err != nil {
		t.Fatalf("filepath.Abs(%s): %v", inputPath, err)
	}
	expectedOutput, err := filepath.Abs(outputPath)
	if err != nil {
		t.Fatalf("filepath.Abs(%s): %v", outputPath, err)
	}

	if cfg.InputFolder != expectedInput {
		t.Fatalf("expected input folder %q, got %q", expectedInput, cfg.InputFolder)
	}
	if cfg.OutputFolder != expectedOutput {
		t.Fatalf("expected output folder %q, got %q", expectedOutput, cfg.OutputFolder)
	}
}
