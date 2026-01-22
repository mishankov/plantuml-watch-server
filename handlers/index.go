package handlers

import (
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

type IndexHandler struct {
	outputFolder string
	templates    *template.Template
}

func NewIndexHandler(outputFolder string, templates *template.Template) *IndexHandler {
	return &IndexHandler{outputFolder: outputFolder, templates: templates}
}

func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	files := []string{}
	err := filepath.Walk(h.outputFolder, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".svg") {
			path = strings.ReplaceAll(path, ".svg", "")
			path = strings.ReplaceAll(path, h.outputFolder, "")
			path = path[1:]

			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("Output not found. Error: " + err.Error()))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	h.templates.ExecuteTemplate(w, "index.html", files)
}
