// Package audio decodes music files to PCM and plays them through the system
// audio device. It is deliberately cgo-free: every decoder here is pure Go, so
// the same code cross-compiles for macOS, Windows and Linux from any machine.
package audio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Format is the shape of a PCM stream: how many frames a second it has and how
// many channels those frames carry.
type Format struct {
	SampleRate int
	Channels   int
}

// Source is a decoded audio file. Read returns interleaved float32 samples in
// [-1, 1]; the count it returns is always a whole number of frames.
type Source interface {
	Format() Format
	Duration() time.Duration
	Read(p []float32) (int, error)
	Seek(d time.Duration) error
	Close() error
}

// ErrUnsupported reports a file whose format this package cannot decode.
var ErrUnsupported = errors.New("audio: unsupported format")

// Playable reports whether a path has an extension Open can decode.
func Playable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac", ".wav", ".wave":
		return true
	}
	return false
}

// Open decodes the file at path. The caller closes the returned Source.
func Open(path string) (Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	source, err := Decode(file, filepath.Ext(path))
	if err != nil {
		file.Close()
		return nil, err
	}
	return source, nil
}

// Decode wraps an already-open file. The extension picks the decoder; the file
// must stay open for as long as the Source is used, and closing the Source
// closes it.
func Decode(file *os.File, extension string) (Source, error) {
	switch strings.ToLower(extension) {
	case ".mp3":
		return newMP3Source(file)
	case ".flac":
		return newFLACSource(file)
	case ".wav", ".wave":
		return newWAVSource(file)
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, extension)
}

// readBytes fills p from r, reporting how much arrived before an error. A short
// read at the end of a stream is not an error by itself.
func readBytes(r io.Reader, p []byte) (int, error) {
	read := 0
	for read < len(p) {
		n, err := r.Read(p[read:])
		read += n
		if err != nil {
			return read, err
		}
		if n == 0 {
			return read, io.ErrNoProgress
		}
	}
	return read, nil
}
