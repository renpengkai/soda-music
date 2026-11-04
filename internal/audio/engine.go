package audio

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// DeviceSampleRate and DeviceChannels are the format the audio device is opened
// with. oto allows exactly one device per process, so every source is resampled
// to this shape.
const (
	DeviceSampleRate = 48000
	DeviceChannels   = 2
)

// Engine owns the audio device. Create one per process and keep it for the
// life of the application.
type Engine struct {
	context *oto.Context
	ready   chan struct{}
	once    sync.Once
}

// New opens the audio device. It returns an Engine even when the device cannot
// be opened, so an application can still browse its library on a machine with
// no sound card; Err reports what went wrong.
func New() *Engine {
	engine := &Engine{}
	context, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:      DeviceSampleRate,
		ChannelCount:    DeviceChannels,
		Format:          oto.FormatFloat32LE,
		BufferSize:      120 * time.Millisecond,
		ApplicationName: "汽水音乐",
	})
	if err != nil {
		engine.ready = make(chan struct{})
		close(engine.ready)
		engine.context = nil
		return engine
	}
	engine.context, engine.ready = context, ready
	return engine
}

// Err reports whether the device failed to open or has since failed.
func (e *Engine) Err() error {
	if e.context == nil {
		return errors.New("audio: no output device")
	}
	<-e.ready
	return e.context.Err()
}

// Stream is one decoded track playing through the device.
type Stream struct {
	player   *oto.Player
	reader   *pcmReader
	source   Source
	duration time.Duration
	format   Format
}

// NewStream decodes src and prepares it for playback. The Stream takes
// ownership of src and closes it.
func (e *Engine) NewStream(src Source) (*Stream, error) {
	if e.context == nil {
		return nil, errors.New("audio: no output device")
	}
	<-e.ready
	if err := e.context.Err(); err != nil {
		return nil, err
	}
	out := Format{SampleRate: DeviceSampleRate, Channels: DeviceChannels}
	reader := newPCMReader(src, out)
	stream := &Stream{
		player:   e.context.NewPlayer(reader),
		reader:   reader,
		source:   src,
		duration: src.Duration(),
		format:   out,
	}
	stream.player.SetVolume(1)
	return stream, nil
}

// Duration is how long the track is, or zero when the file does not say.
func (s *Stream) Duration() time.Duration { return s.duration }

// Play starts or resumes playback.
func (s *Stream) Play() { s.player.Play() }

// Pause stops the sound but keeps the position.
func (s *Stream) Pause() { s.player.Pause() }

// Playing reports whether the device is still consuming this stream.
func (s *Stream) Playing() bool { return s.player.IsPlaying() }

// SetVolume sets the stream's own level, in [0, 1].
func (s *Stream) SetVolume(v float64) { s.player.SetVolume(v) }

// Err reports a decode failure that stopped playback.
func (s *Stream) Err() error { return s.player.Err() }

// Seek jumps to d without rebuilding the player.
func (s *Stream) Seek(d time.Duration) error {
	if d < 0 {
		d = 0
	}
	if s.duration > 0 && d > s.duration {
		d = s.duration
	}
	frames := int64(d.Seconds() * float64(s.format.SampleRate))
	_, err := s.player.Seek(frames*int64(s.format.Channels)*4, 0)
	return err
}

// Close stops the stream and releases the decoded file.
func (s *Stream) Close() error {
	s.player.PauseAndStopReading()
	return s.source.Close()
}

// String describes the stream for logs.
func (s *Stream) String() string {
	return fmt.Sprintf("stream(%d Hz, %d ch, %s)", s.format.SampleRate, s.format.Channels, s.duration)
}
