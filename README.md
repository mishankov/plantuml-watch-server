# PlantUML Watch Server

A tool for watching source files and rendering PlantUML diagrams automatically.

## Usage

PlantUML Watch Server can be run both as a standalone executable or containerized with Docker.

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
  Specifies the target directory for generated outputs. Default: /output`.
- `-port [number]`  
  Specifies the port number for the HTTP server. Default: `8080`.
- `-h`  
  Prints the help message.

Example:
```bash
plantuml-watch-server -input="./diagrams" -output="./output" -port=8080 -verbose
```

### Docker

#### Running with Docker

Run the container:
```bash
docker run -d \
  --name plantuml-watch-server \
  -p 8080:8080 \
  -v ./input:/path/to/input \
  -v ./output:/path/to/output \
  ghcr.io/mishankov/plantuml-watch-server:latest
```

#### Running with Docker Compose

Example `docker-compose.yml`:
```yaml
services:
  plantuml-watch-server:
    image: ghcr.io/mishankov/plantuml-watch-server:latest
    ports:
      - "8080:8080"
    volumes: 
      - ./input:/path/to/input # folder with .puml files
      - ./output:/path/to/output # [optional] folder with output SVGs
```
