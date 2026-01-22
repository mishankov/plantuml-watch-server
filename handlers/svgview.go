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
		w.WriteHeader(500)
		w.Write([]byte("Internal server error"))
		return
	}

	absFullPath, err := filepath.Abs(svgFullPath)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Internal server error"))
		return
	}

	if !strings.HasPrefix(absFullPath, absOutputFolder+string(filepath.Separator)) {
		w.WriteHeader(400)
		w.Write([]byte("Invalid path"))
		return
	}

	_, err = os.ReadFile(svgFullPath)
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("Error getting SVG: " + err.Error()))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	h.templates.ExecuteTemplate(w, "output.html", svgName)
}
