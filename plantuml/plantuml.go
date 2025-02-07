package plantuml

import (
	"log"
	"os/exec"
)

type PlantUML struct {
	jarPath string
}

func New(jarPath string) *PlantUML {
	return &PlantUML{jarPath: jarPath}
}

func (puml *PlantUML) Execute(input, output string) {
	javaArgs := []string{"-jar", puml.jarPath, "-o", output, "-tsvg", input}
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
