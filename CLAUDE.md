# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PlantUML Watch Server is a Go application that watches PlantUML files in a directory, automatically generates SVG diagrams, and serves them via a web interface with live reload functionality using WebSockets.

## Development Commands

### Building
```bash
# Build executable to ./build/ directory
go build -o ./build/ -v .

# Build for current platform
go build .

# Build Docker image
docker build -t plantuml-watch-server .
```

### Running
```bash
# Run with default parameters
go run .

# Run with custom parameters
go run . -plantumlPath="/path/to/plantuml.jar" -input="./diagrams" -output="./output" -port=8080

# Run with Docker
docker run -d --name plantuml-watch-server -p 8080:8080 -v /path/to/input:/input -v /path/to/output:/output ghcr.io/mishankov/plantuml-watch-server:latest
```

### Testing
Note: This project currently has no test files. When adding tests, use standard Go testing:
```bash
go test ./...
```

## Architecture

### Main Components

**main.go**: Entry point that orchestrates the application lifecycle
- Parses CLI configuration
- Cleans stale output directory
- Generates initial SVGs for all `.puml` files
- Starts input watcher in a goroutine
- Starts HTTP server (blocking)

**config/config.go**: CLI argument parsing using `flag` package
- Converts relative paths to absolute paths
- Default values: `plantuml.jar`, `input/`, `output/`, port 8080

**plantuml/plantuml.go**: PlantUML jar execution wrapper
- Executes `java -jar plantuml.jar -o <output> -tsvg <input>`
- Logs combined output and errors
- Handles both single files and glob patterns (`**.puml`)

**inputwatcher/inputwatcher.go**: File system monitoring
- `WatchFile()`: Polls a single file for size/modtime changes every 100ms
- `Watch()`: Main loop that discovers new `.puml` files and spawns goroutines to watch each file
- Triggers PlantUML regeneration on file changes
- Supports nested directories (uses `filepath.Walk`)

**server/server.go**: HTTP server with WebSocket support
- `/`: Lists all available SVG diagrams (index)
- `/output/{name}`: Serves HTML page for a specific diagram
- `/ws/{name}`: WebSocket endpoint that streams SVG updates in real-time
- `/static/{file}`: Serves embedded static files
- Uses `embed.FS` for static files and templates
- WebSocket watches the output SVG file and pushes updates to connected clients

### Key Data Flow

1. Application starts, clears output directory
2. Initial SVG generation for all `.puml` files in input directory
3. InputWatcher continuously scans for new/changed `.puml` files (every 100ms)
4. When a `.puml` file changes, PlantUML regenerates the corresponding `.svg`
5. Server watches `.svg` files via WebSocket connections
6. When an `.svg` changes, the server pushes the new content to all connected clients
7. Browser updates the diagram in real-time without page refresh

### Dependencies

- `github.com/gorilla/websocket v1.5.3`: WebSocket implementation for real-time updates
- Go 1.23.0: Uses standard library for file watching, HTTP server, template rendering

### Embedded Assets

The application embeds static files and templates at build time using `//go:embed`:
- `static/`: CSS, JavaScript, and other static assets
- `templates/`: HTML templates (`index.html`, `output.html`)

## CI/CD

The `.github/workflows/ci.yml` workflow:
- Builds executables for Ubuntu, Windows, and macOS
- Uploads build artifacts with 5-day retention
- Builds and pushes Docker images for linux/amd64 and linux/arm64 to GitHub Container Registry
- Triggers on pushes to `main`, tags matching `v*.*.*`, and pull requests

## Requirements

- Java runtime (to execute PlantUML jar)
- PlantUML jar file (default location: `plantuml.jar` in working directory)
- Go 1.23.0+ (for development)
