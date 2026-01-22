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

The code now returns early after error, though the write to `w` after upgrade attempt is still technically invalid (see Medium Issues #10).

### 7. WebSocket CORS Bypass (FIXED)
**Previous Location**: `handlers/svgws.go:39`

The `CheckOrigin` override has been removed. The upgrader now uses the default same-origin policy.

### 8. Race Condition on fileToSvgMap (FIXED)
**Previous Location**: `inputwatcher/inputwatcher.go:47, 142, 158, 227`

A `sync.RWMutex` has been added to protect concurrent access to `fileToSvgMap`. All read operations use `RLock()`/`RUnlock()` and all write operations use `Lock()`/`Unlock()`.

### 9. Path Traversal Vulnerability (FIXED)
**Previous Location**: `handlers/svgview.go:25`, `handlers/svgws.go:27`
**Current Location**: `handlers/svgview.go:24-46`, `handlers/svgws.go:27-49`

Both handlers now sanitize the path using `filepath.Clean()` and `filepath.Join()`, then validate that the absolute path is within the output folder:
```go
svgName := filepath.Clean(r.PathValue("name"))
svgFullPath := filepath.Join(h.outputFolder, svgName+".svg")

// Validate the path is within output folder
absOutputFolder, err := filepath.Abs(h.outputFolder)
// ... error handling ...

absFullPath, err := filepath.Abs(svgFullPath)
// ... error handling ...

if !strings.HasPrefix(absFullPath, absOutputFolder+string(filepath.Separator)) {
    w.WriteHeader(400)
    w.Write([]byte("Invalid path"))
    return
}
```

### 10. Panic on Unexpected Error (FIXED)
**Previous Location**: `plantuml/plantuml.go:53`
**Current Location**: `plantuml/plantuml.go:53`

The default case now logs the error instead of panicking:
```go
default:
    log.ErrorContext(ctx, "unexpected error executing plantuml", "error", err)
```

---

## Medium Severity Issues

### 1. Ignored Error on SVG Read in Loop (handlers/svgws.go:70)
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

### 2. Unchecked WebSocket Write Errors (handlers/svgws.go:60, 73)
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

### 3. Potential Index Out of Bounds Panic (handlers/index.go:26)
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

### 4. Destructive Startup Operation (main.go:67)
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

### 5. Goroutine Leak on File Deletion (inputwatcher/inputwatcher.go:193-205)
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

### 6. Write to ResponseWriter After Upgrade Attempt (handlers/svgws.go:44-45)
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

### 7. Magic Numbers for WebSocket Message Types (handlers/svgws.go:60, 73)
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

### 8. Typo in Error Message (handlers/svgws.go:45)
**Location**: `handlers/svgws.go:45`
**Severity**: Trivial

```go
"Couldn't upgrade to WebSocker. Error: " // "WebSocker" should be "WebSocket"
```

---

## Summary

- **Critical**: 0 issues (all fixed)
- **High**: 0 issues (all fixed)
- **Medium**: 6 issues (error handling, resource management, goroutine leaks)
- **Low**: 2 issues (code quality, typo)

**Fixed since last review**: 10 issues
**Fixed in this update**: 2 critical issues (path traversal, panic on error) and 2 high severity issues (CORS bypass, race condition)

## Recommended Priority

All critical and high severity issues have been fixed! The remaining issues to address are:
1. **Goroutine leak on file deletion** - Resource leak over time
2. **Unchecked WebSocket write errors** - Resource waste when clients disconnect
3. **Destructive startup operation** - Unexpected deletion of user files
4. **Ignored error on SVG read in loop** - WebSocket stays open but stops sending updates
5. **Potential index out of bounds panic** - Edge case that could crash server

## Testing Recommendations

1. Add unit tests for file watching logic with concurrent access
2. Add integration tests for WebSocket updates
3. Test with multiple files in nested directories
4. Test error scenarios (permission denied, disk full, missing files)
5. Test path traversal attempts in all HTTP handlers
6. Test concurrent file changes to verify thread safety
7. Run with `-race` flag to detect race conditions
