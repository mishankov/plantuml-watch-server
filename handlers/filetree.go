package handlers

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FileNode represents a file or folder in the hierarchy.
type FileNode struct {
	Name                string
	Path                string
	IsFolder            bool
	Active              bool
	HasActiveDescendant bool
	Children            []*FileNode
}

func collectSVGFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info == nil || info.IsDir() || !strings.HasSuffix(path, ".svg") {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		fileWithoutExt := strings.TrimSuffix(relPath, ".svg")
		files = append(files, filepath.ToSlash(fileWithoutExt))
		return nil
	})

	return files, err
}

// buildFileTree converts a flat list of file paths into a hierarchical tree.
func buildFileTree(files []string, activePath string) []*FileNode {
	root := &FileNode{IsFolder: true, Children: []*FileNode{}}

	for _, filePath := range files {
		parts := strings.Split(filePath, "/")
		currentNode := root

		for i, part := range parts {
			isLastPart := i == len(parts)-1

			if isLastPart {
				currentNode.Children = append(currentNode.Children, &FileNode{
					Name:     part,
					Path:     filePath,
					IsFolder: false,
				})
				continue
			}

			var folderNode *FileNode
			for _, child := range currentNode.Children {
				if child.IsFolder && child.Name == part {
					folderNode = child
					break
				}
			}
			if folderNode == nil {
				folderPath := strings.Join(parts[:i+1], "/")
				folderNode = &FileNode{
					Name:     part,
					Path:     folderPath,
					IsFolder: true,
					Children: []*FileNode{},
				}
				currentNode.Children = append(currentNode.Children, folderNode)
			}
			currentNode = folderNode
		}
	}

	sortFileTree(root)
	markActivePath(root, filepath.ToSlash(activePath))

	return root.Children
}

func markActivePath(node *FileNode, activePath string) bool {
	if node == nil {
		return false
	}

	if !node.IsFolder {
		node.Active = node.Path == activePath
		return node.Active
	}

	for _, child := range node.Children {
		if markActivePath(child, activePath) {
			node.HasActiveDescendant = true
		}
	}

	return node.HasActiveDescendant
}

// sortFileTree recursively sorts the tree nodes.
func sortFileTree(node *FileNode) {
	if node.Children == nil {
		return
	}

	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsFolder != node.Children[j].IsFolder {
			return node.Children[i].IsFolder
		}
		return node.Children[i].Name < node.Children[j].Name
	})

	for _, child := range node.Children {
		if child.IsFolder {
			sortFileTree(child)
		}
	}
}
