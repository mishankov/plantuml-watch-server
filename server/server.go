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
}

func New(staticFS, templatesFS fs.FS, outputFolder string) *Server {
	// Preparing termplates
	tmpls, err := template.New("").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Fatalln(err)
	}

	return &Server{staticFS: staticFS, templates: tmpls, outputFolder: outputFolder}
}

func (s *Server) Serve() {
	http.HandleFunc("/output/{name}", s.handleOutput)
	http.HandleFunc("/ws/{name}", s.handleWSOutput)
	http.Handle("/static/{file}", http.FileServer(http.FS(s.staticFS)))
	http.HandleFunc("/", s.handleIndex)

	log.Println("http://localhost:8080/")
	if err := http.ListenAndServe(":8080", nil); err != nil {
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

		log.Println("SVG changed:", svgFullPath)

		svg, _ := os.ReadFile(svgFullPath)
		if len(svg) != 0 {
			ws.WriteMessage(1, svg)
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	files := []string{}
	err := filepath.Walk("/output", func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".svg") {
			files = append(files, strings.ReplaceAll(path, ".svg", ""))
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
