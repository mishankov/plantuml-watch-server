package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/platforma-dev/platforma/log"
)

type SourceHandler struct {
	inputWatcher *inputwatcher.InputWatcher
}

type sourceResponse struct {
	Diagram    string `json:"diagram"`
	SourcePath string `json:"sourcePath"`
	Content    string `json:"content,omitempty"`
	Saved      bool   `json:"saved,omitempty"`
	CompileOK  bool   `json:"compileOk,omitempty"`
	Message    string `json:"message,omitempty"`
}

type sourceUpdateRequest struct {
	Content string `json:"content"`
}

func NewSourceHandler(inputWatcher *inputwatcher.InputWatcher) *SourceHandler {
	return &SourceHandler{inputWatcher: inputWatcher}
}

func (h *SourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handlePut(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *SourceHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	diagram := filepath.Clean(r.PathValue("name"))
	sourcePath, content, err := h.inputWatcher.ReadSourceForOutput(diagram)
	if err != nil {
		h.writeSourceError(w, r, diagram, err, "failed to load source")
		return
	}

	writeJSON(w, http.StatusOK, sourceResponse{
		Diagram:    diagram,
		SourcePath: sourcePath,
		Content:    content,
	})
}

func (h *SourceHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	diagram := filepath.Clean(r.PathValue("name"))

	var req sourceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	log.InfoContext(r.Context(), "saving diagram source", "diagram", diagram)

	sourcePath, result, err := h.inputWatcher.WriteSourceForOutput(r.Context(), diagram, req.Content)
	if err != nil {
		h.writeSourceError(w, r, diagram, err, "failed to save source")
		return
	}

	if result.OK {
		log.InfoContext(r.Context(), "saved diagram source", "diagram", diagram, "source", sourcePath)
	} else {
		log.WarnContext(r.Context(), "diagram source saved with compile error", "diagram", diagram, "source", sourcePath, "message", result.Message)
	}

	writeJSON(w, http.StatusOK, sourceResponse{
		Diagram:    diagram,
		SourcePath: sourcePath,
		Saved:      true,
		CompileOK:  result.OK,
		Message:    result.Message,
	})
}

func (h *SourceHandler) writeSourceError(w http.ResponseWriter, r *http.Request, diagram string, err error, message string) {
	if errors.Is(err, inputwatcher.ErrOutputNotTracked) {
		log.WarnContext(r.Context(), message, "diagram", diagram, "error", err)
		http.Error(w, "diagram source not found", http.StatusNotFound)
		return
	}

	log.ErrorContext(r.Context(), message, "diagram", diagram, "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
