package handlers

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type SvgViewHandler struct {
	outputFolder string
	templates    *template.Template
}

type SvgViewData struct {
	Diagram string
	Tree    []*FileNode
}

func NewSvgViewHandler(outputFolder string, templates *template.Template) *SvgViewHandler {
	return &SvgViewHandler{
		outputFolder: outputFolder,
		templates:    templates,
	}
}

func (h *SvgViewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svgName := filepath.Clean(r.PathValue("name"))
	svgFullPath := filepath.Join(h.outputFolder, svgName+".svg")

	// Validate the path is within output folder
	absOutputFolder, err := filepath.Abs(h.outputFolder)
	if err != nil {
		renderErrorPage(w, r, h.templates, http.StatusInternalServerError, "Unable to resolve the diagrams directory.")
		return
	}

	absFullPath, err := filepath.Abs(svgFullPath)
	if err != nil {
		renderErrorPage(w, r, h.templates, http.StatusInternalServerError, "Unable to resolve the requested diagram.")
		return
	}

	if !strings.HasPrefix(absFullPath, absOutputFolder+string(filepath.Separator)) {
		renderErrorPage(w, r, h.templates, http.StatusBadRequest, "The requested diagram path is invalid.")
		return
	}

	_, err = os.ReadFile(svgFullPath)
	if err != nil {
		renderErrorPage(w, r, h.templates, http.StatusNotFound, "The requested diagram could not be found.")
		return
	}

	files, err := collectSVGFiles(h.outputFolder)
	if err != nil {
		if os.IsNotExist(err) {
			renderErrorPage(w, r, h.templates, http.StatusNotFound, "The requested diagram could not be found.")
			return
		}

		renderErrorPage(w, r, h.templates, http.StatusInternalServerError, "Unable to load the diagrams list.")
		return
	}

	data := SvgViewData{
		Diagram: svgName,
		Tree:    buildFileTree(files, svgName),
	}

	if err := renderHTMLTemplate(w, h.templates, "output.html", data); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
