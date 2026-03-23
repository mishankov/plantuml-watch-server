package handlers

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platforma-dev/platforma/httpserver"
)

func TestServerRendersIndexAtRoot(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDir, "sequence.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatalf("write svg: %v", err)
	}

	server := newTestServer(t, outputDir)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	server.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}
	if !strings.Contains(body, `/output/sequence`) {
		t.Fatalf("expected index page to link diagram, body: %s", body)
	}
}

func TestServerRendersNotFoundPageForUnknownRoute(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)

	server.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
	assertHTMLBodyContains(t, body, "Not found", "Home", "/missing", "The page you requested was not found.")
}

func TestServerRendersNotFoundPageForMissingDiagram(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/output/does-not-exist", nil)

	server.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
	assertHTMLBodyContains(t, body, "Not found", "Home", "/output/does-not-exist", "The requested diagram could not be found.")
}

func TestSvgViewHandlerRejectsInvalidPathWithErrorPage(t *testing.T) {
	t.Parallel()

	handler := NewSvgViewHandler(t.TempDir(), testTemplates(t))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/output/../secret", nil)
	request.SetPathValue("name", "../secret")

	handler.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.StatusCode)
	}
	assertHTMLBodyContains(t, body, "Bad request", "/output/../secret", "The requested diagram path is invalid.")
}

func TestIndexHandlerRendersInternalErrorPageWhenOutputFolderFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	blockingFile := filepath.Join(tempDir, "output")
	if err := os.WriteFile(blockingFile, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	handler := NewIndexHandler(filepath.Join(blockingFile, "child"), testTemplates(t))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.StatusCode)
	}
	assertHTMLBodyContains(t, body, "Internal server error", "/", "Unable to load the diagrams list.")
}

func TestIndexHandlerRendersEmptyStateWhenOutputFolderIsMissing(t *testing.T) {
	t.Parallel()

	handler := NewIndexHandler(filepath.Join(t.TempDir(), "missing"), testTemplates(t))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
	assertHTMLBodyContains(t, body, "No diagrams found")
}

func TestServerKeepsDownloadErrorsNonHTML(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/download/missing?ext=svg", nil)

	server.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
	if strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Fatalf("expected non-html error body, got %s", body)
	}
	if !strings.Contains(body, "SVG file not found") {
		t.Fatalf("expected download error message, got %s", body)
	}
}

func TestServerKeepsWebsocketErrorsNonHTML(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ws/missing", nil)

	server.ServeHTTP(recorder, request)

	response := recorder.Result()
	body := readBody(t, response)

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.StatusCode)
	}
	if strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Fatalf("expected non-html error body, got %s", body)
	}
	if !strings.Contains(body, "Error getting SVG") {
		t.Fatalf("expected websocket error message, got %s", body)
	}
}

func newTestServer(t *testing.T, outputDir string) *httpserver.HTTPServer {
	t.Helper()

	templates := testTemplates(t)
	server := httpserver.New("", 0)
	server.Handle("/output/{name...}", NewSvgViewHandler(outputDir, templates))
	server.Handle("/ws/{name...}", NewSVGWSHandler(outputDir))
	server.Handle("/download/{name...}", NewDownloadHandler(outputDir))
	server.Handle("/", NewIndexHandler(outputDir, templates))
	return server
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()

	pattern := filepath.Join("..", "templates", "*.html")
	templates, err := template.ParseGlob(pattern)
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return templates
}

func assertHTMLBodyContains(t *testing.T, body string, expected ...string) {
	t.Helper()

	if !strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Fatalf("expected html body, got %s", body)
	}

	for _, value := range expected {
		if !strings.Contains(body, value) {
			t.Fatalf("expected body to contain %q, got %s", value, body)
		}
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
