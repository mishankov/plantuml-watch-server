package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
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
	svgName := r.PathValue("name")

	_, err := os.ReadFile(fmt.Sprintf(h.outputFolder+"/%v.svg", svgName))
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("Error getting SVG: " + err.Error()))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	h.templates.ExecuteTemplate(w, "output.html", svgName)
}
