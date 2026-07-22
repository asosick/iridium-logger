package iridiumlogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iridiumgo/iridium/network/wrapper"
)

const (
	defaultInitialLines    = 250
	defaultMaxInitialLines = 2_000
	defaultMaxLineBytes    = 256 * 1024
	defaultPollInterval    = 250 * time.Millisecond
	defaultHeartbeat       = 15 * time.Second
	defaultMaxConnections  = 50
)

var sourceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Source maps a stable browser-visible ID to a server-owned log file.
type Source struct {
	ID   string
	Path string
}

// DirectorySource maps a stable browser-visible ID to a server-owned directory.
// Requests may select a direct child by filename, but never provide a path.
type DirectorySource struct {
	ID   string
	Path string
}

// Config controls the exposed sources and streaming resource limits.
type Config struct {
	Sources         []Source
	Directories     []DirectorySource
	PollInterval    time.Duration
	Heartbeat       time.Duration
	MaxInitialLines int
	MaxLineBytes    int
	MaxConnections  int
}

// Handler streams configured log files to FormLogViewer fields.
type Handler struct {
	sources         map[string]Source
	directories     map[string]DirectorySource
	pollInterval    time.Duration
	heartbeat       time.Duration
	maxInitialLines int
	maxLineBytes    int
	connections     chan struct{}
}

// NewHandler creates the HTTP side of the plugin. Files do not need to exist yet.
func NewHandler(config Config) (*Handler, error) {
	if len(config.Sources) == 0 && len(config.Directories) == 0 {
		return nil, errors.New("iridium logs: at least one source or directory is required")
	}
	directories := make(map[string]DirectorySource, len(config.Directories))
	for _, directory := range config.Directories {
		if !sourceIDPattern.MatchString(directory.ID) {
			return nil, fmt.Errorf("iridium logs: invalid directory ID %q", directory.ID)
		}
		if directory.Path == "" {
			return nil, fmt.Errorf("iridium logs: directory %q has no path", directory.ID)
		}
		if _, exists := directories[directory.ID]; exists {
			return nil, fmt.Errorf("iridium logs: duplicate directory ID %q", directory.ID)
		}
		absolutePath, err := filepath.Abs(directory.Path)
		if err != nil {
			return nil, fmt.Errorf("iridium logs: resolve directory %q: %w", directory.ID, err)
		}
		directory.Path = absolutePath
		directories[directory.ID] = directory
	}

	sources := make(map[string]Source, len(config.Sources))
	for _, source := range config.Sources {
		if !sourceIDPattern.MatchString(source.ID) {
			return nil, fmt.Errorf("iridium logs: invalid source ID %q", source.ID)
		}
		if source.Path == "" {
			return nil, fmt.Errorf("iridium logs: source %q has no path", source.ID)
		}
		if _, exists := sources[source.ID]; exists {
			return nil, fmt.Errorf("iridium logs: duplicate source ID %q", source.ID)
		}
		sources[source.ID] = source
	}

	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.Heartbeat <= 0 {
		config.Heartbeat = defaultHeartbeat
	}
	if config.MaxInitialLines <= 0 {
		config.MaxInitialLines = defaultMaxInitialLines
	}
	if config.MaxLineBytes <= 0 {
		config.MaxLineBytes = defaultMaxLineBytes
	}
	if config.MaxConnections <= 0 {
		config.MaxConnections = defaultMaxConnections
	}

	return &Handler{
		sources:         sources,
		directories:     directories,
		pollInterval:    config.PollInterval,
		heartbeat:       config.Heartbeat,
		maxInitialLines: config.MaxInitialLines,
		maxLineBytes:    config.MaxLineBytes,
		connections:     make(chan struct{}, config.MaxConnections),
	}, nil
}

// RegisterRoute adds the stream to an Iridium mux. The mux determines which
// panel authentication and page middleware protect it.
func (h *Handler) RegisterRoute(mux wrapper.IMux, route string) {
	// Iridium's scoped mux prefixes paths before delegating to http.ServeMux.
	// Method validation stays in ServeHTTP so the prefix is applied correctly.
	mux.Handle(route, h)
}

// ServeHTTP serves a single SSE stream selected by the source query parameter.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source, ok := h.resolveSource(r.URL.Query().Get("directory"), r.URL.Query().Get("source"))
	if !ok {
		http.Error(w, "unknown log source", http.StatusNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is unsupported", http.StatusInternalServerError)
		return
	}
	select {
	case h.connections <- struct{}{}:
		defer func() { <-h.connections }()
	default:
		http.Error(w, "too many live log connections", http.StatusServiceUnavailable)
		return
	}

	initialLines := defaultInitialLines
	if requested, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && requested >= 0 {
		initialLines = min(requested, h.maxInitialLines)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	tail := newFileTail(source.Path, h.maxLineBytes)
	initial, err := tail.open(initialLines)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeEvent(w, "problem", "Unable to open the log source.")
	} else {
		for _, line := range initial {
			writeEvent(w, "line", line)
		}
	}
	writeEvent(w, "ready", source.ID)
	flusher.Flush()
	defer tail.close()

	poll := time.NewTicker(h.pollInterval)
	heartbeat := time.NewTicker(h.heartbeat)
	defer poll.Stop()
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			lines, rotated, err := tail.readAvailable()
			if err != nil {
				writeEvent(w, "problem", "The log source is temporarily unavailable.")
				flusher.Flush()
				continue
			}
			if rotated {
				writeEvent(w, "rotation", true)
			}
			for _, line := range lines {
				writeEvent(w, "line", line)
			}
			if rotated || len(lines) > 0 {
				flusher.Flush()
			}
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (h *Handler) resolveSource(directoryID, sourceID string) (Source, bool) {
	if directoryID == "" {
		source, ok := h.sources[sourceID]
		return source, ok
	}
	directory, ok := h.directories[directoryID]
	if !ok || sourceID == "" || sourceID == "." || filepath.Base(sourceID) != sourceID || strings.ContainsRune(sourceID, 0) {
		return Source{}, false
	}
	path := filepath.Join(directory.Path, sourceID)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Source{}, false
	}
	return Source{ID: sourceID, Path: path}, true
}

func writeEvent(w io.Writer, event string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, strings.TrimSpace(string(data)))
}
