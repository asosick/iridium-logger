package iridiumlogs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileTailReadsCompletedLines(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tail := newFileTail(logPath, 1024)
	t.Cleanup(tail.close)

	initial, err := tail.open(2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initial, []string{"two", "three"}) {
		t.Fatalf("initial = %#v", initial)
	}

	appendFile(t, logPath, "partial")
	lines, _, err := tail.readAvailable()
	if err != nil || len(lines) != 0 {
		t.Fatalf("partial read = %#v, %v", lines, err)
	}
	appendFile(t, logPath, " line\nfive\n")
	lines, _, err = tail.readAvailable()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{"partial line", "five"}) {
		t.Fatalf("appended lines = %#v", lines)
	}
}

func TestFileTailFollowsRotationAndTruncation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	logPath := filepath.Join(directory, "app.log")
	if err := os.WriteFile(logPath, []byte("old content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tail := newFileTail(logPath, 1024)
	t.Cleanup(tail.close)
	if _, err := tail.open(1); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(logPath, filepath.Join(directory, "app.log.1")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("after rotation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, rotated, err := tail.readAvailable()
	if err != nil || !rotated || !reflect.DeepEqual(lines, []string{"after rotation"}) {
		t.Fatalf("rotation read = %#v, rotated %v, error %v", lines, rotated, err)
	}

	if err := os.WriteFile(logPath, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, rotated, err = tail.readAvailable()
	if err != nil || !rotated || !reflect.DeepEqual(lines, []string{"new"}) {
		t.Fatalf("truncation read = %#v, rotated %v, error %v", lines, rotated, err)
	}
}

func appendFile(t *testing.T, path, value string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(value); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
