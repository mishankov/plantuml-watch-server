package handlers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildFileTreeMarksActivePathAndSortsNodes(t *testing.T) {
	t.Parallel()

	tree := buildFileTree([]string{
		"zeta",
		"alpha",
		"folder/sub/leaf",
		"folder/one",
		"folder/two",
	}, "folder/sub/leaf")

	rootNames := []string{tree[0].Name, tree[1].Name, tree[2].Name}
	if want := []string{"folder", "alpha", "zeta"}; !reflect.DeepEqual(rootNames, want) {
		t.Fatalf("unexpected root order: got %v want %v", rootNames, want)
	}

	folder := tree[0]
	if !folder.IsFolder || !folder.HasActiveDescendant {
		t.Fatalf("expected folder node to be marked with active descendant: %#v", folder)
	}
	if folder.Path != "folder" {
		t.Fatalf("expected folder path to be preserved, got %#v", folder)
	}

	subfolder := folder.Children[0]
	if subfolder.Name != "sub" || !subfolder.HasActiveDescendant {
		t.Fatalf("expected subfolder to stay expanded for active file: %#v", subfolder)
	}
	if subfolder.Path != "folder/sub" {
		t.Fatalf("expected subfolder path to be preserved, got %#v", subfolder)
	}

	leaf := subfolder.Children[0]
	if leaf.Path != "folder/sub/leaf" || !leaf.Active {
		t.Fatalf("expected active leaf node, got %#v", leaf)
	}
}

func TestCollectSVGFilesUsesRelativeSlashSeparatedPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nestedDir := filepath.Join(root, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "top.svg"), []byte("svg"), 0o644); err != nil {
		t.Fatalf("write top svg failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "child.svg"), []byte("svg"), 0o644); err != nil {
		t.Fatalf("write nested svg failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "ignore.txt"), []byte("txt"), 0o644); err != nil {
		t.Fatalf("write ignore file failed: %v", err)
	}

	files, err := collectSVGFiles(root)
	if err != nil {
		t.Fatalf("collectSVGFiles failed: %v", err)
	}

	if want := []string{"nested/child", "top"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected collected files: got %v want %v", files, want)
	}
}
