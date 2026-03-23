package handlers

import (
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileNode represents a file or folder in the hierarchy
type FileNode struct {
	Name     string      // Display name (folder name or filename)
	Path     string      // Full path for links (empty for folders)
	IsFolder bool        // true for folders, false for files
	Children []*FileNode // Child nodes (for folders)
}

type IndexHandler struct {
	outputFolder string
	templates    *template.Template
}

func NewIndexHandler(outputFolder string, templates *template.Template) *IndexHandler {
	return &IndexHandler{outputFolder: outputFolder, templates: templates}
}

func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		renderErrorPage(w, r, h.templates, http.StatusNotFound, "The page you requested was not found.")
		return
	}

	if _, err := os.Stat(h.outputFolder); err != nil {
		if os.IsNotExist(err) {
			if err := renderHTMLTemplate(w, h.templates, "index.html", []*FileNode{}); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}

		renderErrorPage(w, r, h.templates, http.StatusInternalServerError, "Unable to load the diagrams list.")
		return
	}

	files := []string{}
	err := filepath.Walk(h.outputFolder, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".svg") {
			path = strings.ReplaceAll(path, ".svg", "")
			path = strings.ReplaceAll(path, h.outputFolder, "")
			path = path[1:]

			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			if err := renderHTMLTemplate(w, h.templates, "index.html", []*FileNode{}); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			return
		}

		renderErrorPage(w, r, h.templates, http.StatusInternalServerError, "Unable to load the diagrams list.")
		return
	}

	// Build the file tree from flat list
	root := buildFileTree(files)

	if err := renderHTMLTemplate(w, h.templates, "index.html", root); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// buildFileTree converts a flat list of file paths into a hierarchical tree
func buildFileTree(files []string) []*FileNode {
	root := &FileNode{IsFolder: true, Children: []*FileNode{}}

	for _, filePath := range files {
		parts := strings.Split(filePath, string(filepath.Separator))
		currentNode := root

		for i, part := range parts {
			isLastPart := i == len(parts)-1

			if isLastPart {
				// This is a file
				currentNode.Children = append(currentNode.Children, &FileNode{
					Name:     part,
					Path:     filePath,
					IsFolder: false,
				})
			} else {
				// This is a folder - find or create it
				var folderNode *FileNode
				for _, child := range currentNode.Children {
					if child.IsFolder && child.Name == part {
						folderNode = child
						break
					}
				}
				if folderNode == nil {
					folderNode = &FileNode{
						Name:     part,
						IsFolder: true,
						Children: []*FileNode{},
					}
					currentNode.Children = append(currentNode.Children, folderNode)
				}
				currentNode = folderNode
			}
		}
	}

	// Sort the tree: folders first, then files, alphabetically within each group
	sortFileTree(root)

	return root.Children
}

// sortFileTree recursively sorts the tree nodes
func sortFileTree(node *FileNode) {
	if node.Children == nil {
		return
	}

	sort.Slice(node.Children, func(i, j int) bool {
		// Folders come before files
		if node.Children[i].IsFolder != node.Children[j].IsFolder {
			return node.Children[i].IsFolder
		}
		// Alphabetical within same type
		return node.Children[i].Name < node.Children[j].Name
	})

	// Recursively sort children
	for _, child := range node.Children {
		if child.IsFolder {
			sortFileTree(child)
		}
	}
}
