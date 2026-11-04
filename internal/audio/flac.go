package audio

import (
	"io"
	"os"
	"time"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
)

// flacSource decodes FLAC, including seeking through the stream's own seek
// table.
type flacSource struct {
	file    *os.File
	stream  *flac.Stream
	format  Format
	length  time.Duration
	scale   float32
	skip    int
	current *frame.Frame
	at      int
}

func newFLACSource(file *os.File) (*flacSource, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	stream, err := flac.NewSeek(file)
	if err != nil {
		return nil, err
	}
	info := stream.Info
	source := &flacSource{
		file:   file,
		stream: stream,
		format: Format{SampleRate: int(info.SampleRate), Channels: int(info.NChannels)},
		scale:  float32(1 / float64(int64(1)<<(info.BitsPerSample-1))),
	}
	if info.NSamples > 0 {
		source.length = time.Duration(float64(info.NSamples) / float64(info.SampleRate) * float64(time.Second))
	}
	return source, nil
}

func (s *flacSource) Format() Format { return s.format }

func (s *flacSource) Duration() time.Duration { return s.length }

func (s *flacSource) Read(p []float32) (int, error) {
	channels := s.format.Channels
	written := 0
	for written+channels <= len(p) {
		if s.current == nil || s.at >= len(s.current.Subframes[0].Samples) {
			next, err := s.stream.ParseNext()
			if err != nil {
				if written > 0 {
					return written, nil
				}
				return 0, err
			}
			s.current, s.at = next, 0
			if s.skip > 0 {
				if s.skip >= int(next.BlockSize) {
					s.skip -= int(next.BlockSize)
					s.current = nil
					continue
				}
				s.at, s.skip = s.skip, 0
			}
		}
		available := len(s.current.Subframes[0].Samples) - s.at
		want := (len(p) - written) / channels
		if available > want {
			available = want
		}
		for i := 0; i < available; i++ {
			for c := 0; c < channels; c++ {
				p[written] = float32(s.current.Subframes[c].Samples[s.at+i]) * s.scale
				written++
			}
		}
		s.at += available
	}
	return written, nil
}

func (s *flacSource) Seek(d time.Duration) error {
	if d < 0 {
		d = 0
	}
	sample := uint64(d.Seconds() * float64(s.format.SampleRate))
	first, err := s.stream.Seek(sample)
	if err != nil {
		return err
	}
	s.current, s.at = nil, 0
	if sample > first {
		s.skip = int(sample - first)
	} else {
		s.skip = 0
	}
	return nil
}

func (s *flacSource) Close() error { return s.file.Close() }
