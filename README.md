# PlantUML Watch Server

[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/mishankov/plantuml-watch-server/ci.yml)](https://github.com/mishankov/plantuml-watch-server/actions/workflows/ci.yml)

This tool makes it easy to see changes in PlantUML files in real-time. It watches for changes in PlantUML files in a specified directory and generates SVG files for them. The generated SVG files are updated live in the browser.

## Usage

PlantUML Watch Server can be run both as a standalone executable or Docker container.

### Standalone Executable

#### Installation

- **Go Install:**  
  Install via Go install command:
  ```bash
  go install github.com/mishankov/plantuml-watch-server@latest
  ```
- **Download Latest Release:**  
  Navigate to the [GitHub Releases page](https://github.com/mishankov/plantuml-watch-server/releases) and download the executable for your platform.

#### Running the Executable

Run the executable with the command line options below.

#### Command Line Parameters

- `-plantumlPath [path]`  
  Specifies the path to the PlantUML jar file. Default: `plantuml.jar`.
- `-input [path]`  
  Specifies the directory to watch for PlantUML file changes. Default: `input`.
- `-output [path]`  
  Specifies the target directory for generated outputs. Default: `output`.
- `-port [number]`  
  Specifies the port number for the HTTP server. Default: `8080`.
- `-h`  
  Prints the help message.

Example:
```bash
plantuml-watch-server -plantumlPath="/path/to/plantuml.jar" -input="./diagrams" -output="./output" -port=8080
```

### Docker

#### Running with Docker

Run the container:
```bash
docker run -d \
  --name plantuml-watch-server \
  -p 8080:8080 \
  -v /path/to/input:/input \
  -v /path/to/output:/output \
  ghcr.io/mishankov/plantuml-watch-server:latest
```

#### Running with Docker Compose

Example `compose.yml`:
```yaml
services:
  plantuml-watch-server:
    image: ghcr.io/mishankov/plantuml-watch-server:latest
    ports:
      - "8080:8080"
    volumes: 
      - /path/to/input:/input # folder with .puml files
      - /path/to/output:/output # [optional] folder with output SVGs
```

### Accessing the Web Interface
Open your browser and navigate to `http://localhost:8080` (or other specified port) to see list of generated diagrams. Click on a diagram to view it. It will be updated live as you make changes to the PlantUML file.