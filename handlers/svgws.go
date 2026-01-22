package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/mishankov/plantuml-watch-server/inputwatcher"
	"github.com/platforma-dev/platforma/log"
)

type SVGWSHandler struct {
	outputFolder string
}

func NewSVGWSHandler(outputFolder string) *SVGWSHandler {
	return &SVGWSHandler{outputFolder: outputFolder}
}

func (h *SVGWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	svgName := r.PathValue("name")
	svgFullPath := fmt.Sprintf(h.outputFolder+"/%v.svg", svgName)

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
				log.ErrorContext(ctx, "WebSocket connection aborted", "error", err)
				ws.Close()
				cancel()
				break
			}
		}
	}()

	ws.WriteMessage(1, svg)

	log.InfoContext(ctx, "Started watching diagram", "svg", svgFullPath)
	for {
		err := inputwatcher.WatchFile(ctx, svgFullPath)
		if err != nil {
			log.ErrorContext(ctx, "Stopped watching diagram", "svg", svgFullPath, "error", err)
			break
		}

		svg, _ := os.ReadFile(svgFullPath)
		if len(svg) != 0 {
			log.InfoContext(ctx, "SVG changed", "svg", svgFullPath)
			ws.WriteMessage(1, svg)
		}
	}
}
