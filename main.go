package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
)

func runPlantUML(input, output string) {
	javaArgs := []string{"-jar", "plantuml.jar", "-o", output, "-tsvg", input}
	pumlCmd := exec.Command("java", javaArgs...)

	pumlOut, err := pumlCmd.CombinedOutput()
	if err != nil {
		switch e := err.(type) {
		case *exec.Error:
			log.Println("failed executing:", err)
		case *exec.ExitError:
			log.Println("command exit rc =", e.ExitCode())
		default:
			panic(err)
		}
	}

	if len(pumlOut) != 0 {
		log.Println(string(pumlOut))
	}
}

func server() {
	// Handler function to return SVGs
	http.HandleFunc("/output/{name}", func(w http.ResponseWriter, r *http.Request) {
		svgName := r.PathValue("name")

		_, err := os.ReadFile(fmt.Sprintf("/output/%v.svg", svgName))
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Error getting SVG: " + err.Error()))
			return
		}

		tmplFile := "templates/output.html"
		tmpl, err := template.ParseFiles(tmplFile)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Error getting template: " + err.Error()))
			return
		}

		w.Header().Add("Content-Type", "text/html")
		tmpl.Execute(w, svgName)
	})

	// Hanler function to stream updates
	http.HandleFunc("/ws/{name}", func(w http.ResponseWriter, r *http.Request) {
		_, cancel := context.WithCancel(r.Context())
		defer cancel()

		svgName := r.PathValue("name")
		svgFullPath := fmt.Sprintf("/output/%v.svg", svgName)

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
					log.Println("Couldn't get NextReader:", err)
					ws.Close()
					cancel()
					break
				}
			}
		}()

		ws.WriteMessage(1, svg)

		for {
			// TODO: stop watch if context is done
			err := watchFile(svgFullPath)
			if err != nil {
				log.Println(err)
				break
			}

			log.Println("SVG changed:", svgFullPath)

			svg, _ := os.ReadFile(svgFullPath)
			if len(svg) != 0 {
				ws.WriteMessage(1, svg)
			}
		}
	})

	// Home handler function
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmplFile := "templates/home.html"
		tmpl, err := template.ParseFiles(tmplFile)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Template not found. Error: " + err.Error()))
			return
		}

		files := []string{}
		err = filepath.Walk("/output", func(path string, info fs.FileInfo, err error) error {
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
		tmpl.Execute(w, files)
	})

	log.Println("http://localhost:8080/")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalln(err)
	}
}

func main() {
	os.RemoveAll("/output/")
	// os.MkdirAll("/output/", 0777)

	// Generate initial SVGs
	runPlantUML("/input/*.puml", "/output")

	// Watch input changes
	go (&InputWatcher{}).Watch()

	// Run server
	server()
}
