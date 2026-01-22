package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type DownloadHandler struct {
	outputFolder string
}

func NewDownloadHandler(outputFolder string) *DownloadHandler {
	return &DownloadHandler{outputFolder: outputFolder}
}

func (h *DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ext := r.URL.Query().Get("ext")
	path := filepath.Join(h.outputFolder, name+"."+ext)

	data, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("SVG file not found: " + err.Error()))
		return
	}

	switch ext {
	case "svg":
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.svg", filepath.Base(name)))
	case "png":
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.png", filepath.Base(name)))
	}

	w.Write(data)
}
