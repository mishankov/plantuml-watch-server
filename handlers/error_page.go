package handlers

import (
	"bytes"
	"html/template"
	"net/http"
)

type ErrorPageData struct {
	StatusCode  int
	Title       string
	Message     string
	RequestPath string
	BackURL     string
	BackLabel   string
}

func renderHTMLTemplate(w http.ResponseWriter, templates *template.Template, name string, data any) error {
	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, name, data); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(rendered.Bytes())
	return err
}

func renderErrorPage(w http.ResponseWriter, r *http.Request, templates *template.Template, statusCode int, message string) {
	data := ErrorPageData{
		StatusCode:  statusCode,
		Title:       errorPageTitle(statusCode),
		Message:     message,
		RequestPath: r.URL.Path,
		BackURL:     "/",
		BackLabel:   "Diagrams list",
	}

	var rendered bytes.Buffer
	if err := templates.ExecuteTemplate(&rendered, "error.html", data); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(rendered.Bytes())
}

func errorPageTitle(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "Bad request"
	case http.StatusNotFound:
		return "Not found"
	case http.StatusInternalServerError:
		return "Internal server error"
	default:
		if title := http.StatusText(statusCode); title != "" {
			return title
		}
		return "Request error"
	}
}
