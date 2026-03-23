package handlers

import (
	"html/template"
	"net/http"
	"os"
)

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
	files, err := collectSVGFiles(h.outputFolder)
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

	root := buildFileTree(files, "")

	if err := renderHTMLTemplate(w, h.templates, "index.html", root); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
