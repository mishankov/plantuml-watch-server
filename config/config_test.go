package config

import (
	"runtime"
	"testing"
)

func TestNewFromArgsUsesDefaultParallelism(t *testing.T) {
	cfg, err := NewFromArgs(nil)
	if err != nil {
		t.Fatalf("NewFromArgs returned error: %v", err)
	}

	if cfg.Parallelism != runtime.NumCPU() {
		t.Fatalf("expected parallelism %d, got %d", runtime.NumCPU(), cfg.Parallelism)
	}
}

func TestNewFromArgsRejectsInvalidParallelism(t *testing.T) {
	testCases := [][]string{
		{"-parallelism=0"},
		{"-parallelism=-1"},
	}

	for _, args := range testCases {
		if _, err := NewFromArgs(args); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}
