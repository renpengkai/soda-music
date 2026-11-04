package audio

import (
	"encoding/binary"
	"io"
	"os"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

// mp3Source decodes MPEG audio. go-mp3 always hands back 16-bit little-endian
// stereo, whatever the file's own channel layout was.
type mp3Source struct {
	file    *os.File
	decoder *mp3.Decoder
	scratch []byte
	format  Format
	length  time.Duration
}

func newMP3Source(file *os.File) (*mp3Source, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return nil, err
	}
	source := &mp3Source{
		file:    file,
		decoder: decoder,
		format:  Format{SampleRate: decoder.SampleRate(), Channels: 2},
	}
	if bytes := decoder.Length(); bytes > 0 {
		frames := float64(bytes) / 4
		source.length = time.Duration(frames / float64(source.format.SampleRate) * float64(time.Second))
	}
	return source, nil
}

func (s *mp3Source) Format() Format { return s.format }

func (s *mp3Source) Duration() time.Duration { return s.length }

func (s *mp3Source) Read(p []float32) (int, error) {
	need := len(p) * 2
	if cap(s.scratch) < need {
		s.scratch = make([]byte, need)
	}
	buf := s.scratch[:need]
	n, err := readBytes(s.decoder, buf)
	values := n / 2
	for i := 0; i < values; i++ {
		sample := int16(binary.LittleEndian.Uint16(buf[i*2:]))
		p[i] = float32(sample) / 32768
	}
	if err != nil && values == 0 {
		return 0, err
	}
	return values, nil
}

func (s *mp3Source) Seek(d time.Duration) error {
	if d < 0 {
		d = 0
	}
	frames := int64(d.Seconds() * float64(s.format.SampleRate))
	_, err := s.decoder.Seek(frames*4, io.SeekStart)
	return err
}

func (s *mp3Source) Close() error { return s.file.Close() }
