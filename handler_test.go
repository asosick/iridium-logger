package iridiumlogs

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iridiumgo/iridium/network/wrapper"
)

func TestNewHandlerValidatesSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources []Source
		want    string
	}{
		{name: "none", want: "at least one source"},
		{name: "unsafe ID", sources: []Source{{ID: "../../etc", Path: "/tmp/app.log"}}, want: "invalid source ID"},
		{name: "missing path", sources: []Source{{ID: "app"}}, want: "has no path"},
		{name: "duplicate", sources: []Source{{ID: "app", Path: "a"}, {ID: "app", Path: "b"}}, want: "duplicate source ID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewHandler(Config{Sources: test.sources})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewHandler() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestHandlerRejectsBrowserSuppliedPaths(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Source{ID: "app", Path: "/private/logs/app.log"})
	request := httptest.NewRequest(http.MethodGet, "http://example.test/stream?source=../../etc/passwd", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if strings.Contains(response.Body.String(), "/private/logs") {
		t.Fatal("response exposed a configured filesystem path")
	}
}

func TestRegisterRouteUsesIridiumPathPattern(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Source{ID: "app", Path: "/private/logs/app.log"})
	mux := &recordingMux{}
	handler.RegisterRoute(mux, "/stream")
	if mux.pattern != "/stream" || mux.handler != handler {
		t.Fatalf("registered route = %q, %#v", mux.pattern, mux.handler)
	}
}

func TestHandlerResolvesDirectoryChildren(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	logPath := filepath.Join(directory, "app current.log")
	if err := os.WriteFile(logPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Config{Directories: []DirectorySource{{ID: "logs", Path: directory}}})
	if err != nil {
		t.Fatal(err)
	}

	source, ok := handler.resolveSource("logs", "app current.log")
	if !ok || source.Path != logPath {
		t.Fatalf("resolved source = %#v, %v", source, ok)
	}
	for _, filename := range []string{"../outside.log", "subdirectory/app.log", ".", "missing.log"} {
		if _, ok := handler.resolveSource("logs", filename); ok {
			t.Fatalf("unsafe or missing filename %q was accepted", filename)
		}
	}
}

func TestHandlerRejectsDirectorySymlinks(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.log")
	if err := os.WriteFile(target, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "linked.log")); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Config{Directories: []DirectorySource{{ID: "logs", Path: directory}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.resolveSource("logs", "linked.log"); ok {
		t.Fatal("directory source followed a symbolic link")
	}
}

func TestHandlerStreamsInitialAndAppendedLines(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "app.log")
	if err := os.WriteFile(logPath, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := newTestHandler(t, Source{ID: "app", Path: logPath})
	handler.pollInterval = 10 * time.Millisecond
	reader, writer := io.Pipe()
	response := &streamResponseWriter{header: make(http.Header), writer: writer}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "http://example.test/stream?source=app&lines=10", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		_ = writer.Close()
		close(done)
	}()

	stream := bufio.NewReader(reader)
	if got := readLineEvent(t, stream); got != "first" {
		t.Fatalf("initial line = %q", got)
	}
	if response.header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Content-Type = %q", response.header.Get("Content-Type"))
	}

	appendFile(t, logPath, "second\n")
	if got := readLineEvent(t, stream); got != "second" {
		t.Fatalf("appended line = %q", got)
	}

	cancel()
	_ = reader.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after request cancellation")
	}
}

type streamResponseWriter struct {
	header http.Header
	writer *io.PipeWriter
	status int
}

type recordingMux struct {
	pattern string
	handler http.Handler
}

func (m *recordingMux) Handle(pattern string, handler http.Handler) {
	m.pattern = pattern
	m.handler = handler
}

func (m *recordingMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.Handle(pattern, http.HandlerFunc(handler))
}

func (m *recordingMux) WithMiddleware(...func(http.Handler) http.Handler) wrapper.IMux {
	return m
}

func (w *streamResponseWriter) Header() http.Header             { return w.header }
func (w *streamResponseWriter) Write(value []byte) (int, error) { return w.writer.Write(value) }
func (w *streamResponseWriter) WriteHeader(status int)          { w.status = status }
func (w *streamResponseWriter) Flush()                          {}

func readLineEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event stream: %v", err)
		}
		if line != "event: line\n" {
			continue
		}
		data, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event data: %v", err)
		}
		return strings.Trim(strings.TrimPrefix(strings.TrimSpace(data), "data: "), `"`)
	}
}

func newTestHandler(t *testing.T, sources ...Source) *Handler {
	t.Helper()
	handler, err := NewHandler(Config{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
