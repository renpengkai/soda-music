package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

// wavSource decodes uncompressed RIFF/WAVE audio: 8, 16, 24 and 32-bit integer
// PCM and 32-bit IEEE float.
type wavSource struct {
	file    *os.File
	format  Format
	length  time.Duration
	bits    int
	float   bool
	dataAt  int64
	dataEnd int64
	pos     int64
	scratch []byte
}

func newWAVSource(file *os.File) (*wavSource, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, fmt.Errorf("wav: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: not a RIFF/WAVE file")
	}
	source := &wavSource{file: file}
	chunk := make([]byte, 8)
	for {
		if _, err := io.ReadFull(file, chunk); err != nil {
			return nil, fmt.Errorf("wav: no audio data: %w", err)
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[0:4]) {
		case "fmt ":
			body := make([]byte, size)
			if _, err := io.ReadFull(file, body); err != nil {
				return nil, fmt.Errorf("wav: %w", err)
			}
			if len(body) < 16 {
				return nil, fmt.Errorf("wav: short fmt chunk")
			}
			kind := binary.LittleEndian.Uint16(body[0:])
			source.format.Channels = int(binary.LittleEndian.Uint16(body[2:]))
			source.format.SampleRate = int(binary.LittleEndian.Uint32(body[4:]))
			source.bits = int(binary.LittleEndian.Uint16(body[14:]))
			switch kind {
			case 1:
			case 3:
				source.float = true
			case 0xFFFE:
				if len(body) >= 26 {
					source.float = binary.LittleEndian.Uint16(body[24:]) == 3
				}
			default:
				return nil, fmt.Errorf("wav: unsupported codec %d", kind)
			}
			if size%2 == 1 {
				if _, err := file.Seek(1, io.SeekCurrent); err != nil {
					return nil, err
				}
			}
		case "data":
			if offset, err := file.Seek(0, io.SeekCurrent); err == nil {
				source.dataAt = offset
			}
			source.dataEnd = source.dataAt + size
			if source.format.Channels == 0 || source.format.SampleRate == 0 {
				return nil, fmt.Errorf("wav: data before fmt")
			}
			if source.bits == 0 {
				return nil, fmt.Errorf("wav: unknown sample size")
			}
			frames := size / int64(source.format.Channels*source.bits/8)
			source.length = time.Duration(float64(frames) / float64(source.format.SampleRate) * float64(time.Second))
			return source, nil
		default:
			if _, err := file.Seek(size+size%2, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}
}

func (s *wavSource) Format() Format { return s.format }

func (s *wavSource) Duration() time.Duration { return s.length }

func (s *wavSource) Read(p []float32) (int, error) {
	width := s.bits / 8
	channels := s.format.Channels
	written := 0
	for written+channels <= len(p) {
		if s.pos >= s.dataEnd {
			return written, io.EOF
		}
		need := channels * width
		if cap(s.scratch) < need {
			s.scratch = make([]byte, need)
		}
		buf := s.scratch[:need]
		n, err := io.ReadFull(s.file, buf)
		if n < need {
			if written > 0 {
				return written, nil
			}
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		s.pos += int64(need)
		for c := 0; c < channels; c++ {
			p[written] = decodeWAVSample(buf[c*width:(c+1)*width], s.bits, s.float)
			written++
		}
	}
	return written, nil
}

func (s *wavSource) Seek(d time.Duration) error {
	if d < 0 {
		d = 0
	}
	frames := int64(d.Seconds() * float64(s.format.SampleRate))
	offset := s.dataAt + frames*int64(s.format.Channels*s.bits/8)
	if offset > s.dataEnd {
		offset = s.dataEnd
	}
	if _, err := s.file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	s.pos = offset
	return nil
}

func (s *wavSource) Close() error { return s.file.Close() }

// decodeWAVSample turns one sample of width bytes into a float32 in [-1, 1].
func decodeWAVSample(b []byte, bits int, float bool) float32 {
	switch {
	case float && bits == 32:
		return math.Float32frombits(binary.LittleEndian.Uint32(b))
	case bits == 8:
		return (float32(b[0]) - 128) / 128
	case bits == 16:
		return float32(int16(binary.LittleEndian.Uint16(b))) / 32768
	case bits == 24:
		value := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if value&0x800000 != 0 {
			value -= 1 << 24
		}
		return float32(value) / 8388608
	case bits == 32:
		return float32(float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648)
	}
	return 0
}
