# Code Review - PlantUML Watch Server

Analysis performed: 2026-01-21

## Critical Issues

### 1. Goroutine Closure Bug (inputwatcher/inputwatcher.go:78-89)
**Location**: `inputwatcher/inputwatcher.go:78-89`
**Severity**: Critical

The goroutine captures the loop variable `file` by reference, not by value:
```go
for _, file := range files {
    if !slices.Contains(oldFiles, file) {
        go func() {
            for {
                err := WatchFile(ctx, file) // BUG: 'file' captured by reference
```

**Impact**: All goroutines will watch the same file (the last file in the loop), instead of each watching their own file. This breaks the entire file watching mechanism.

**Fix**: Capture the variable by value:
```go
go func(f string) {
    for {
        err := WatchFile(ctx, f)
        if err != nil {
            log.Println("Stopped watchFile:", err)
            break
        }
        log.Println("File changed:", f)
        iw.pulm.Execute(f, iw.outputPath)
    }
}(file)
```

### 2. Path Traversal Vulnerability (server/server.go:48, 65)
**Location**: `server/server.go:48, 65`
**Severity**: Critical (Security)

User input from `r.PathValue("name")` is used directly in file paths without sanitization:
```go
svgName := r.PathValue("name")
svgFullPath := fmt.Sprintf(s.outputFolder+"/%v.svg", svgName)
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

## High Severity Issues

### 3. WebSocket CORS Bypass (server/server.go:78)
**Location**: `server/server.go:78`
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

### 4. Goroutine Leak (inputwatcher/inputwatcher.go:69-101)
**Location**: `inputwatcher/inputwatcher.go:69-101`
**Severity**: High

Goroutines are spawned for each file but never cleaned up. If a file is deleted and recreated, multiple goroutines will watch the same file.

**Impact**: Memory leak, wasted CPU cycles, duplicate PlantUML executions. Over time, this could accumulate hundreds of leaked goroutines.

**Fix**: Track goroutines and properly cancel them when files are deleted. Consider using a sync.WaitGroup or maintaining a map of active watchers that can be cancelled.

### 5. Application Crash on Directory Walk Error (inputwatcher/inputwatcher.go:63)
**Location**: `inputwatcher/inputwatcher.go:63`
**Severity**: High

```go
if err != nil {
    log.Fatalln(err) // Crashes entire application
}
```

**Impact**: A single filesystem error (permission denied, disk full, etc.) crashes the entire server.

**Fix**: Return the error and handle it gracefully in the caller:
```go
func (iw *InputWatcher) GetFiles() ([]string, error) {
    files := []string{}
    err := filepath.Walk(iw.inputPath, func(path string, info fs.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if strings.HasSuffix(path, ".puml") {
            files = append(files, path)
        }
        return nil
    })

    return files, err
}
```

## Medium Severity Issues

### 6. Ignored Error on SVG Read (server/server.go:109)
**Location**: `server/server.go:109`
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
    log.Println("Error reading SVG:", err)
    break
}
if len(svg) != 0 {
    if err := ws.WriteMessage(websocket.BinaryMessage, svg); err != nil {
        log.Println("Error writing to WebSocket:", err)
        break
    }
}
```

### 7. Unchecked WebSocket Write Errors (server/server.go:99, 112)
**Location**: `server/server.go:99, 112`
**Severity**: Medium

```go
ws.WriteMessage(1, svg) // Error not checked
```

**Impact**: If the client disconnects or the write fails, the server continues trying to watch and send updates, wasting resources.

**Fix**: Check write errors and break the loop:
```go
if err := ws.WriteMessage(websocket.BinaryMessage, svg); err != nil {
    log.Println("Error writing to WebSocket:", err)
    break
}
```

### 8. Writing After Failed Upgrade (server/server.go:82-85)
**Location**: `server/server.go:82-85`
**Severity**: Medium

```go
ws, err := upgrader.Upgrade(w, r, nil)
if err != nil {
    w.WriteHeader(500) // Invalid: connection already hijacked
    w.Write([]byte("Couldn't upgrade to WebSocker. Error: " + err.Error()))
    return
}
```

**Impact**: After `Upgrade()` returns an error, the connection is already hijacked, so writing to `w` is invalid and won't work. The Upgrade function handles the error response itself.

**Fix**: Only log the error:
```go
ws, err := upgrader.Upgrade(w, r, nil)
if err != nil {
    log.Println("WebSocket upgrade failed:", err)
    return
}
```

### 9. Destructive Startup Operation (main.go:34)
**Location**: `main.go:34`
**Severity**: Medium

```go
os.RemoveAll(config.OutputFolder + "/")
```

**Impact**: Completely removes the output directory on every startup, destroying any manual files or data users might have placed there. This is unexpected behavior for a watch server.

**Fix**: Only remove `.svg` files, or make this behavior configurable:
```go
// Remove only .svg files
filepath.Walk(config.OutputFolder, func(path string, info fs.FileInfo, err error) error {
    if err == nil && strings.HasSuffix(path, ".svg") {
        os.Remove(path)
    }
    return nil
})
```

### 10. Potential Index Out of Bounds Panic (server/server.go:123)
**Location**: `server/server.go:123`
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

## Low Severity Issues

### 11. Magic Numbers for WebSocket Message Types (server/server.go:99, 112)
**Location**: `server/server.go:99, 112`
**Severity**: Low

```go
ws.WriteMessage(1, svg) // Should use websocket.TextMessage or websocket.BinaryMessage
```

**Impact**: Code readability and maintainability. The constant `1` should be `websocket.TextMessage` (or `websocket.BinaryMessage` for SVG).

**Fix**: Use named constants:
```go
ws.WriteMessage(websocket.BinaryMessage, svg)
```

### 12. Ignored Callback Error in Walk (inputwatcher/inputwatcher.go:54-59)
**Location**: `inputwatcher/inputwatcher.go:54-59`
**Severity**: Low

The callback ignores the `err` parameter from `filepath.Walk`:
```go
err := filepath.Walk(iw.inputPath, func(path string, info fs.FileInfo, err error) error {
    if strings.HasSuffix(path, ".puml") {
        files = append(files, path)
    }
    return nil
})
```

**Impact**: Errors during directory traversal (permission denied, deleted files) are silently ignored, leading to incomplete file lists.

**Fix**: Return the error from the callback:
```go
err := filepath.Walk(iw.inputPath, func(path string, info fs.FileInfo, err error) error {
    if err != nil {
        return err
    }
    if strings.HasSuffix(path, ".puml") {
        files = append(files, path)
    }
    return nil
})
```

### 13. No Graceful Shutdown
**Location**: `main.go`
**Severity**: Low

The application doesn't handle SIGTERM/SIGINT signals for graceful shutdown.

**Impact**: Goroutines are abruptly terminated on shutdown; WebSocket connections aren't closed cleanly. This can leave PlantUML processes running.

**Fix**: Implement signal handling and context cancellation:
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down gracefully...")
    cancel()
}()
```

### 14. Typo in Error Message (server/server.go:84)
**Location**: `server/server.go:84`
**Severity**: Trivial

```go
"Couldn't upgrade to WebSocker. Error: " // "WebSocker" should be "WebSocket"
```

## Summary

- **Critical**: 2 issues (goroutine bug, path traversal)
- **High**: 3 issues (CORS, goroutine leak, crash on error)
- **Medium**: 6 issues (error handling, resource management)
- **Low**: 4 issues (code quality, maintainability)

## Recommended Priority

The most urgent issues to fix are:
1. **Goroutine closure bug** - Breaks core functionality
2. **Path traversal vulnerability** - Security risk
3. **CORS bypass** - Security risk
4. **Goroutine leak** - Resource leak over time
5. **Application crash on error** - Stability issue

## Testing Recommendations

1. Add unit tests for file watching logic
2. Add integration tests for WebSocket updates
3. Test with multiple files in nested directories
4. Test error scenarios (permission denied, disk full, missing files)
5. Test path traversal attempts in the HTTP handlers
6. Load test to verify goroutine cleanup
