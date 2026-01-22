package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
)

type Server struct {
	staticFS     fs.FS
	templates    *template.Template
	outputFolder string
	port         int
}

func New(staticFS, templatesFS fs.FS, outputFolder string, port int) *Server {
	// Preparing termplates
	tmpls, err := template.New("").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Fatalln(err)
	}

	return &Server{staticFS: staticFS, templates: tmpls, outputFolder: outputFolder, port: port}
}

func (s *Server) Serve() {
	http.HandleFunc("/output/{name...}", s.handleOutput)
	http.HandleFunc("/ws/{name...}", s.handleWSOutput)
	http.HandleFunc("/download/svg/{name...}", s.handleDownloadSVG)
	http.HandleFunc("/download/png/{name...}", s.handleDownloadPNG)
	http.Handle("/static/{file}", http.FileServer(http.FS(s.staticFS)))
	http.HandleFunc("/", s.handleIndex)

	log.Printf("http://localhost:%v/", s.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%v", s.port), nil); err != nil {
		log.Fatalln(err)
	}
}

func (s *Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	svgName := r.PathValue("name")

	_, err := os.ReadFile(fmt.Sprintf(s.outputFolder+"/%v.svg", svgName))
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("Error getting SVG: " + err.Error()))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	s.templates.ExecuteTemplate(w, "output.html", svgName)
}

func (s *Server) handleWSOutput(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	svgName := r.PathValue("name")
	svgFullPath := fmt.Sprintf(s.outputFolder+"/%v.svg", svgName)

	svg, err := os.ReadFile(svgFullPath)
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("Error getting SVG: " + err.Error()))
		return
	}

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(_ *http.Request) bool { return true },
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Couldn't upgrade to WebSocker. Error: " + err.Error()))
		return
	}

	go func() {
		for {
			if _, _, err := ws.NextReader(); err != nil {
				log.Println("WebSocket connection aborted:", err)
				ws.Close()
				cancel()
				break
			}
		}
	}()

	ws.WriteMessage(1, svg)

	log.Println("Started watching diagram:", svgFullPath)
	for {
		err := inputwatcher.WatchFile(ctx, svgFullPath)
		if err != nil {
			log.Println("Stopped watching diagram "+svgFullPath+":", err)
			break
		}

		svg, _ := os.ReadFile(svgFullPath)
		if len(svg) != 0 {
			log.Println("SVG changed:", svgFullPath)
			ws.WriteMessage(1, svg)
		}
	}
}

func (s *Server) handleDownloadSVG(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svgPath := filepath.Join(s.outputFolder, name+".svg")

	svgData, err := os.ReadFile(svgPath)
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("SVG file not found: " + err.Error()))
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.svg", filepath.Base(name)))
	w.Write(svgData)
}

func (s *Server) handleDownloadPNG(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pngPath := filepath.Join(s.outputFolder, name+".png")

	pngData, err := os.ReadFile(pngPath)
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("PNG file not found: " + err.Error()))
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.png", filepath.Base(name)))
	w.Write(pngData)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	files := []string{}
	err := filepath.Walk(s.outputFolder, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".svg") {
			path = strings.ReplaceAll(path, ".svg", "")
			path = strings.ReplaceAll(path, s.outputFolder, "")
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
	s.templates.ExecuteTemplate(w, "index.html", files)
}
