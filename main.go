package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
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

	log.Println(string(pumlOut))
}

func main() {
	// Generate initial SVGs
	runPlantUML("/input/*.puml", "/output")

	// Handler function to return SVGs
	http.HandleFunc("/output/{name}", func(w http.ResponseWriter, r *http.Request) {
		svgName := r.PathValue("name")

		svg, err := os.ReadFile(fmt.Sprintf("/output/%v.svg", svgName))
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("SVG not found. Error: " + err.Error()))
			return
		}

		w.Header().Add("Content-Type", "text/html")
		w.Write(svg)
	})

	log.Println("http://localhost:8080/")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalln(err)
	}
}
