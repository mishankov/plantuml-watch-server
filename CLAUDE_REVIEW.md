# Code Review - PlantUML Watch Server

Analysis performed: 2026-01-22

## Architecture Changes

The codebase has undergone significant refactoring:
- `server/server.go` has been split into separate handler files in `handlers/`
  - `handlers/svgws.go` - WebSocket handler for live SVG updates
  - `handlers/svgview.go` - HTML page serving for diagrams
  - `handlers/index.go` - Index page listing all diagrams
  - `handlers/download.go` - SVG/PNG file downloads
- New framework: `github.com/platforma-dev/platforma` for application lifecycle
- Context propagation added throughout the codebase

---

## Fixed Issues

These issues from the previous review have been resolved:

### 1. Goroutine Closure Bug (FIXED)
**Previous Location**: `inputwatcher/inputwatcher.go:78-89`
**Current Location**: `inputwatcher/inputwatcher.go:193-205`

The goroutine closure bug has been fixed by passing variables as function parameters:
```go
go func(watchedFile string, watchedOutputDir string) {
    for {
        err := WatchFile(ctx, watchedFile)
        // ...
        iw.ExecuteAndTrack(ctx, watchedFile, watchedOutputDir)
    }
}(file, outputDir)
```

### 2. Application Crash on Walk Error (FIXED)
**Previous Location**: `inputwatcher/inputwatcher.go:63`
**Current Location**: `inputwatcher/inputwatcher.go:174-176`

Now uses `log.ErrorContext` instead of `log.Fatalln`:
```go
if err != nil {
    log.ErrorContext(ctx, "error getting files", "error", err)
}
```

### 3. Ignored Callback Error in Walk (FIXED)
**Previous Location**: `inputwatcher/inputwatcher.go:54-59`
**Current Location**: `inputwatcher/inputwatcher.go:77-80`

The `getSvgFilesInDir` callback now handles errors:
```go
err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
    if err != nil {
        return nil // Skip directories we can't access
    }
    // ...
})
```

### 4. Initial File Read Error Handling (FIXED)
**Previous Location**: Upgrade happened before file read validation
**Current Location**: `handlers/svgws.go:29-34`

Now checks if the file exists before attempting WebSocket upgrade:
```go
svg, err := os.ReadFile(svgFullPath)
if err != nil {
    w.WriteHeader(404)
    w.Write([]byte("Error getting SVG: " + err.Error()))
    return
}
```

### 5. No Graceful Shutdown (FIXED)
**Previous Location**: `main.go`

The Platforma framework now handles graceful shutdown with context cancellation. The application uses `application.New()` and `app.Run(ctx)` which properly manages service lifecycle.

### 6. Write After Failed Upgrade - Early Return (PARTIALLY FIXED)
**Previous Location**: `server/server.go:82-85`
**Current Location**: `handlers/svgws.go:42-47`

The code now returns early after error, though the write to `w` after upgrade attempt is still technically invalid (see New Issues #4).

---

## Critical Issues

### 1. Path Traversal Vulnerability (handlers/svgview.go:25, handlers/svgws.go:27)
**Location**: `handlers/svgview.go:25`, `handlers/svgws.go:27`
**Severity**: Critical (Security)

User input from `r.PathValue("name")` is used directly in file paths without sanitization:
```go
svgFullPath := fmt.Sprintf(h.outputFolder+"/%v.svg", svgName)
```

**Impact**: An attacker could use `../../../etc/passwd` to read arbitrary files from the filesystem.

**Fix**: Sanitize the path using `filepath.Clean()` and validate it's within the output folder:
```go
svgName := filepath.Clean(r.PathValue("name"))
svgFullPath := filepath.Join(s.outputFolder, svgName+".svg")

// Validate the path is within output folder
if !strings.HasPrefix(svgFullPath, s.outputFolder) {
    w.WriteHeader(400)
    w.Write([]byte("Invalid path"))
    return
}
```

### 2. Panic on Unexpected Error (plantuml/plantuml.go:53)
**Location**: `plantuml/plantuml.go:53`
**Severity**: Critical

The default case in the error switch panics:
```go
switch e := err.(type) {
case *exec.Error:
    log.ErrorContext(ctx, "failed executing", "error", err)
case *exec.ExitError:
    log.ErrorContext(ctx, "command exit", "rc", e.ExitCode())
default:
    panic(err)  // Crashes the entire application
}
```

**Impact**: Any unexpected error type from command execution will crash the entire application.

**Fix**: Log the error instead of panicking:
```go
default:
    log.ErrorContext(ctx, "unexpected error executing plantuml", "error", err)
```

---

## High Severity Issues

### 3. WebSocket CORS Bypass (handlers/svgws.go:39)
**Location**: `handlers/svgws.go:39`
**Severity**: High (Security)

```go
CheckOrigin: func(_ *http.Request) bool { return true }
```

**Impact**: Any website can connect to the WebSocket, potentially leading to CSRF attacks. Malicious websites could connect to the local server and read diagram contents.

**Fix**: Implement proper origin checking or remove the CheckOrigin override to use the default (same-origin policy):
```go
// Remove CheckOrigin to use default same-origin policy
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}
```

### 4. Race Condition on fileToSvgMap (inputwatcher/inputwatcher.go)
**Location**: `inputwatcher/inputwatcher.go:47, 142, 158, 227`
**Severity**: High

The `fileToSvgMap` field is accessed concurrently by multiple goroutines without synchronization:
- Field definition at line 47: `fileToSvgMap map[string]map[string]bool`
- Read access at line 142: `oldSvgs := iw.fileToSvgMap[inputFile]`
- Write access at line 158: `iw.fileToSvgMap[inputFile] = generatedSvgs`
- Delete access at line 227: `delete(iw.fileToSvgMap, oldFile)`

**Impact**: Concurrent map access causes race conditions that can lead to data corruption or runtime panics.

**Fix**: Use a `sync.RWMutex` to protect map access:
```go
type InputWatcher struct {
    // ...
    fileToSvgMap   map[string]map[string]bool
    fileToSvgMutex sync.RWMutex
}
```

---

## Medium Severity Issues

### 5. Ignored Error on SVG Read in Loop (handlers/svgws.go:70)
**Location**: `handlers/svgws.go:70`
**Severity**: Medium

```go
svg, _ := os.ReadFile(svgFullPath) // Error ignored
if len(svg) != 0 {
    ws.WriteMessage(1, svg)
}
```

**Impact**: If the file read fails (deleted, permission issue), an empty/zero-length buffer is checked instead of handling the error. The WebSocket connection stays open but stops sending updates.

**Fix**: Check the error and close the WebSocket connection or send an error message:
```go
svg, err := os.ReadFile(svgFullPath)
if err != nil {
    log.ErrorContext(ctx, "Error reading SVG:", "error", err)
    break
}
```

### 6. Unchecked WebSocket Write Errors (handlers/svgws.go:60, 73)
**Location**: `handlers/svgws.go:60, 73`
**Severity**: Medium

```go
ws.WriteMessage(1, svg) // Error not checked
```

**Impact**: If the client disconnects or the write fails, the server continues trying to watch and send updates, wasting resources.

**Fix**: Check write errors and break the loop:
```go
if err := ws.WriteMessage(websocket.TextMessage, svg); err != nil {
    log.ErrorContext(ctx, "Error writing to WebSocket:", "error", err)
    break
}
```

### 7. Potential Index Out of Bounds Panic (handlers/index.go:26)
**Location**: `handlers/index.go:26`
**Severity**: Medium

```go
path = path[1:] // Panics if path is empty
```

**Impact**: If the output folder is the root directory or path manipulation results in an empty string, this panics and crashes the server.

**Fix**: Check `len(path) > 0` before slicing:
```go
if len(path) > 0 {
    path = path[1:]
}
```

### 8. Destructive Startup Operation (main.go:67)
**Location**: `main.go:67`
**Severity**: Medium

```go
os.RemoveAll(config.OutputFolder + "/")
```

**Impact**: Completely removes the output directory on every startup, destroying any manual files or data users might have placed there. This is unexpected behavior for a watch server.

**Fix**: Only remove `.svg` and `.png` files, or make this behavior configurable:
```go
// Remove only generated files
filepath.Walk(config.OutputFolder, func(path string, info fs.FileInfo, err error) error {
    if err == nil && (strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".png")) {
        os.Remove(path)
    }
    return nil
})
```

### 9. Goroutine Leak on File Deletion (inputwatcher/inputwatcher.go:193-205)
**Location**: `inputwatcher/inputwatcher.go:193-205`
**Severity**: Medium

When a `.puml` file is deleted, the watcher goroutine for that file continues running indefinitely until it encounters an error (like the file not existing on the next stat call).

**Impact**: While the goroutine will eventually exit when the file disappears, there's no explicit cancellation mechanism. This could lead to delayed cleanup and resource waste.

**Fix**: Use a map of cancellation functions to explicitly cancel watcher goroutines when files are deleted:
```go
type InputWatcher struct {
    // ...
    watcherCancels map[string]context.CancelFunc
}
```

### 10. Write to ResponseWriter After Upgrade Attempt (handlers/svgws.go:44-45)
**Location**: `handlers/svgws.go:44-45`
**Severity**: Medium

```go
ws, err := upgrader.Upgrade(w, r, nil)
if err != nil {
    w.WriteHeader(500)
    w.Write([]byte("Couldn't upgrade to WebSocker. Error: " + err.Error()))
    return
}
```

**Impact**: After `Upgrade()` is called, the connection may already be hijacked even if an error is returned. Writing to `w` after this is invalid and won't work properly.

**Fix**: Only log the error; don't attempt to write to the ResponseWriter:
```go
ws, err := upgrader.Upgrade(w, r, nil)
if err != nil {
    log.ErrorContext(ctx, "WebSocket upgrade failed", "error", err)
    return
}
```

---

## Low Severity Issues

### 11. Magic Numbers for WebSocket Message Types (handlers/svgws.go:60, 73)
**Location**: `handlers/svgws.go:60, 73`
**Severity**: Low

```go
ws.WriteMessage(1, svg) // Should use websocket.TextMessage or websocket.BinaryMessage
```

**Impact**: Code readability and maintainability. The constant `1` should be `websocket.TextMessage` (or `websocket.BinaryMessage` for SVG).

**Fix**: Use named constants:
```go
ws.WriteMessage(websocket.TextMessage, svg)
```

### 12. Typo in Error Message (handlers/svgws.go:45)
**Location**: `handlers/svgws.go:45`
**Severity**: Trivial

```go
"Couldn't upgrade to WebSocker. Error: " // "WebSocker" should be "WebSocket"
```

---

## Summary

- **Critical**: 2 issues (path traversal, panic on error)
- **High**: 2 issues (CORS bypass, race condition)
- **Medium**: 6 issues (error handling, resource management, goroutine leaks)
- **Low**: 2 issues (code quality, typo)

**Fixed since last review**: 6 issues

## Recommended Priority

The most urgent issues to fix are:
1. **Path traversal vulnerability** - Security risk allowing arbitrary file access
2. **Panic on unexpected error** - Can crash entire application
3. **Race condition on fileToSvgMap** - Can cause data corruption or panics
4. **CORS bypass** - Security risk for WebSocket connections
5. **Goroutine leak on file deletion** - Resource leak over time

## Testing Recommendations

1. Add unit tests for file watching logic with concurrent access
2. Add integration tests for WebSocket updates
3. Test with multiple files in nested directories
4. Test error scenarios (permission denied, disk full, missing files)
5. Test path traversal attempts in all HTTP handlers
6. Test concurrent file changes to verify thread safety
7. Run with `-race` flag to detect race conditions
