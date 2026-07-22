package iridiumlogs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
)

const (
	maxInitialReadBytes = 8 * 1024 * 1024
	maxReadBatchBytes   = 1024 * 1024
)

type fileTail struct {
	path         string
	file         *os.File
	info         os.FileInfo
	offset       int64
	pending      []byte
	maxLineBytes int
}

func newFileTail(path string, maxLineBytes int) *fileTail {
	return &fileTail{path: path, maxLineBytes: maxLineBytes}
}

func (t *fileTail) open(lineCount int) ([]string, error) {
	file, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	t.close()
	t.file = file
	t.info = info
	t.offset = info.Size()
	t.pending = nil
	if lineCount == 0 || info.Size() == 0 {
		return nil, nil
	}
	return readLastLines(file, info.Size(), lineCount, t.maxLineBytes)
}

func (t *fileTail) readAvailable() ([]string, bool, error) {
	current, err := os.Stat(t.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.close()
			return nil, false, nil
		}
		return nil, false, err
	}

	rotated := false
	if t.file == nil || t.info == nil || !os.SameFile(t.info, current) {
		if _, err := t.open(0); err != nil {
			return nil, false, err
		}
		// Start at zero after replacement so lines written during rotation are retained.
		t.offset = 0
		rotated = true
	} else if current.Size() < t.offset {
		if _, err := t.file.Seek(0, io.SeekStart); err != nil {
			return nil, false, err
		}
		t.offset = 0
		t.pending = nil
		rotated = true
	}

	if current.Size() <= t.offset {
		return nil, rotated, nil
	}
	if _, err := t.file.Seek(t.offset, io.SeekStart); err != nil {
		return nil, rotated, err
	}
	available := min(current.Size()-t.offset, int64(maxReadBatchBytes))
	chunk := make([]byte, available)
	read, err := io.ReadFull(t.file, chunk)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, rotated, err
	}
	t.offset += int64(read)
	t.info = current
	t.pending = append(t.pending, chunk[:read]...)

	lastNewline := bytes.LastIndexByte(t.pending, '\n')
	if lastNewline < 0 {
		if len(t.pending) > t.maxLineBytes {
			t.pending = append(append([]byte(nil), t.pending[:t.maxLineBytes]...), []byte("… [truncated]")...)
		}
		return nil, rotated, nil
	}
	complete := t.pending[:lastNewline]
	t.pending = append([]byte(nil), t.pending[lastNewline+1:]...)
	return splitLines(complete, t.maxLineBytes), rotated, nil
}

func (t *fileTail) close() {
	if t.file != nil {
		_ = t.file.Close()
	}
	t.file = nil
	t.info = nil
}

func readLastLines(file *os.File, size int64, count, maxLineBytes int) ([]string, error) {
	readSize := min(size, int64(maxInitialReadBytes))
	buffer := make([]byte, readSize)
	if _, err := file.ReadAt(buffer, size-readSize); err != nil && err != io.EOF {
		return nil, err
	}
	if size > readSize {
		if firstNewline := bytes.IndexByte(buffer, '\n'); firstNewline >= 0 {
			buffer = buffer[firstNewline+1:]
		}
	}
	buffer = bytes.TrimSuffix(buffer, []byte{'\n'})
	lines := splitLines(buffer, maxLineBytes)
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return lines, nil
}

func splitLines(buffer []byte, maxLineBytes int) []string {
	if len(buffer) == 0 {
		return nil
	}
	rawLines := bytes.Split(buffer, []byte{'\n'})
	lines := make([]string, 0, len(rawLines))
	for _, raw := range rawLines {
		raw = bytes.TrimSuffix(raw, []byte{'\r'})
		if len(raw) > maxLineBytes {
			raw = append(append([]byte(nil), raw[:maxLineBytes]...), []byte("… [truncated]")...)
		}
		lines = append(lines, strings.ToValidUTF8(string(raw), "�"))
	}
	return lines
}
