package audio

import (
	"io"
	"math"
	"time"
)

// resampler converts any Source to the one format the audio device was opened
// with. It interpolates with a Catmull-Rom spline, which is cheap and keeps
// high frequencies cleaner than straight linear interpolation.
type resampler struct {
	src     Source
	out     Format
	ratio   float64
	pending []float32 // interleaved source frames not yet consumed
	scratch []float32
	pos     float64
	eof     bool
}

func newResampler(src Source, out Format) *resampler {
	in := src.Format()
	ratio := 1.0
	if in.SampleRate > 0 {
		ratio = float64(in.SampleRate) / float64(out.SampleRate)
	}
	return &resampler{src: src, out: out, ratio: ratio}
}

func (r *resampler) inChannels() int { return r.src.Format().Channels }

func (r *resampler) frames() int {
	channels := r.inChannels()
	if channels == 0 {
		return 0
	}
	return len(r.pending) / channels
}

// Read fills p with interleaved frames in the output format. len(p) must be a
// whole number of output frames.
func (r *resampler) Read(p []float32) (int, error) {
	channels := r.out.Channels
	if channels == 0 {
		return 0, io.EOF
	}
	written := 0
	for written+channels <= len(p) {
		if err := r.fill(int(r.pos) + 3); err != nil && r.frames() == 0 {
			return written, err
		}
		if r.frames() == 0 {
			return written, io.EOF
		}
		index := int(r.pos)
		frac := r.pos - float64(index)
		for c := 0; c < channels; c++ {
			p[written] = r.sample(index, frac, c)
			written++
		}
		r.pos += r.ratio
		if drop := int(r.pos); drop > 0 {
			r.drop(drop)
			r.pos -= float64(drop)
		}
	}
	return written, nil
}

// sample reads channel c at frame index with the fractional position frac,
// interpolating between the neighbouring frames.
func (r *resampler) sample(index int, frac float64, c int) float32 {
	in := r.inChannels()
	if c >= in {
		c = in - 1
	}
	at := func(i int) float32 {
		if i < 0 {
			i = 0
		}
		if last := r.frames() - 1; i > last {
			i = last
		}
		return r.pending[i*in+c]
	}
	p0, p1, p2, p3 := at(index-1), at(index), at(index+1), at(index+2)
	t := float32(frac)
	a := 2 * p1
	b := p2 - p0
	cc := 2*p0 - 5*p1 + 4*p2 - p3
	d := -p0 + 3*p1 - 3*p2 + p3
	return 0.5 * (a + b*t + cc*t*t + d*t*t*t)
}

// fill reads from the source until at least need frames are pending.
func (r *resampler) fill(need int) error {
	channels := r.inChannels()
	if channels == 0 {
		return io.EOF
	}
	const block = 4096
	for !r.eof && r.frames() < need {
		if cap(r.scratch) < block*channels {
			r.scratch = make([]float32, block*channels)
		}
		n, err := r.src.Read(r.scratch)
		if n > 0 {
			r.pending = append(r.pending, r.scratch[:n]...)
		}
		if err != nil {
			r.eof = true
			if n == 0 {
				return err
			}
		}
		if n == 0 && err == nil {
			return nil
		}
	}
	return nil
}

// drop discards frames from the front of the pending buffer.
func (r *resampler) drop(frames int) {
	channels := r.inChannels()
	if channels == 0 {
		return
	}
	if frames >= r.frames() {
		r.pending = r.pending[:0]
		return
	}
	r.pending = r.pending[frames*channels:]
}

// Seek moves the underlying source and forgets everything buffered.
func (r *resampler) Seek(d time.Duration) error {
	if err := r.src.Seek(d); err != nil {
		return err
	}
	r.pending = r.pending[:0]
	r.pos = 0
	r.eof = false
	return nil
}

// pcmReader adapts a resampler to the byte stream oto reads: little-endian
// float32, and seekable so the player can jump within a track.
type pcmReader struct {
	resampler *resampler
	frames    []float32
	bytes     []byte
	offset    int
	position  int64
}

func newPCMReader(src Source, out Format) *pcmReader {
	return &pcmReader{
		resampler: newResampler(src, out),
		frames:    make([]float32, 2048),
		bytes:     make([]byte, 0, 2048*4),
	}
}

func (p *pcmReader) Read(b []byte) (int, error) {
	if p.offset >= len(p.bytes) {
		n, err := p.resampler.Read(p.frames)
		if n == 0 {
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		p.bytes = p.bytes[:n*4]
		for i, value := range p.frames[:n] {
			bits := math.Float32bits(value)
			p.bytes[i*4] = byte(bits)
			p.bytes[i*4+1] = byte(bits >> 8)
			p.bytes[i*4+2] = byte(bits >> 16)
			p.bytes[i*4+3] = byte(bits >> 24)
		}
		p.offset = 0
	}
	n := copy(b, p.bytes[p.offset:])
	p.offset += n
	p.position += int64(n)
	return n, nil
}

// Seek implements io.Seeker over the output byte stream, so oto can jump
// without the application rebuilding the player.
func (p *pcmReader) Seek(offset int64, whence int) (int64, error) {
	const bytesPerFrame = 4
	target := offset
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		target = p.position + offset
	case io.SeekEnd:
		duration := p.resampler.src.Duration()
		total := int64(duration.Seconds()*float64(p.resampler.out.SampleRate)) * int64(p.resampler.out.Channels) * bytesPerFrame
		target = total + offset
	default:
		return p.position, io.ErrUnexpectedEOF
	}
	if target < 0 {
		target = 0
	}
	frames := target / int64(p.resampler.out.Channels*bytesPerFrame)
	d := time.Duration(float64(frames) / float64(p.resampler.out.SampleRate) * float64(time.Second))
	if err := p.resampler.Seek(d); err != nil {
		return p.position, err
	}
	p.bytes = p.bytes[:0]
	p.offset = 0
	p.position = frames * int64(p.resampler.out.Channels*bytesPerFrame)
	return p.position, nil
}
