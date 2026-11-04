// Package player turns a list of tracks into playback: it owns the audio
// stream, the queue, the position, and the small set of modes a listener
// expects (shuffle, repeat, volume, mute, fade).
//
// Nothing here runs on a timer of its own. The application calls Tick from its
// paint, which is also the only place it repaints, so an idle window stays
// idle and a playing one draws exactly as often as it needs to.
package player

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"soda-music/internal/audio"
	"soda-music/internal/catalog"
	"soda-music/internal/lyrics"
)

// RepeatMode is what happens when the current track ends.
type RepeatMode string

const (
	// RepeatOff stops at the end of the queue.
	RepeatOff RepeatMode = "off"
	// RepeatAll wraps around to the first track.
	RepeatAll RepeatMode = "all"
	// RepeatOne plays the current track again.
	RepeatOne RepeatMode = "one"
)

// Player is the playback controller. Its methods are safe to call from any
// goroutine; the fields are guarded by mu.
type Player struct {
	mu sync.Mutex

	engine *audio.Engine
	stream *audio.Stream

	queue []catalog.Track
	index int

	repeat  RepeatMode
	shuffle bool
	order   []int // the shuffled positions, when shuffle is on
	orderAt int

	volume float64
	muted  bool
	fadeMs int

	position  time.Duration
	startedAt time.Time
	playing   bool

	lyricFile lyrics.Lyrics
	version   uint64
	err       error
}

// New makes a player over an audio engine.
func New(engine *audio.Engine) *Player {
	return &Player{engine: engine, index: -1, volume: 0.8, repeat: RepeatOff}
}

// Version increases on every change a view might draw differently. A view can
// compare it against the value it last painted to decide whether to rebuild
// anything it caches.
func (p *Player) Version() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.version
}

func (p *Player) bump() { p.version++ }

// Err reports the last playback failure, or the device error.
func (p *Player) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	if p.engine == nil {
		return errors.New("player: no audio engine")
	}
	return p.engine.Err()
}

// Queue is the list of tracks, in the order they were added.
func (p *Player) Queue() []catalog.Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]catalog.Track(nil), p.queue...)
}

// Index is the position of the current track in the queue, or -1.
func (p *Player) Index() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.index
}

// Current is the track that is loaded, if any.
func (p *Player) Current() (catalog.Track, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index < 0 || p.index >= len(p.queue) {
		return catalog.Track{}, false
	}
	return p.queue[p.index], true
}

// Lyrics are the words of the current track, when a .lrc file sits beside it.
func (p *Player) Lyrics() lyrics.Lyrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lyricFile
}

// Playing reports whether sound is coming out right now.
func (p *Player) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Position is how far into the current track playback has reached.
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.positionLocked()
}

func (p *Player) positionLocked() time.Duration {
	position := p.position
	if p.playing && !p.startedAt.IsZero() {
		position += time.Since(p.startedAt)
	}
	if duration := p.durationLocked(); duration > 0 && position > duration {
		position = duration
	}
	return position
}

func (p *Player) durationLocked() time.Duration {
	if p.stream != nil {
		return p.stream.Duration()
	}
	if p.index >= 0 && p.index < len(p.queue) {
		return p.queue[p.index].Duration
	}
	return 0
}

// Duration is the length of the current track.
func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.durationLocked()
}

// Volume is the level in [0, 1].
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

// Muted reports whether sound is suppressed without changing the level.
func (p *Player) Muted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.muted
}

// Repeat is the current repeat mode.
func (p *Player) Repeat() RepeatMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.repeat
}

// Shuffle reports whether the queue is played in a random order.
func (p *Player) Shuffle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.shuffle
}

// SetVolume sets the level and applies it to the running stream.
func (p *Player) SetVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = v
	p.applyVolumeLocked()
	p.bump()
}

// SetMuted suppresses or restores sound.
func (p *Player) SetMuted(muted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.muted = muted
	p.applyVolumeLocked()
	p.bump()
}

func (p *Player) applyVolumeLocked() {
	if p.stream == nil {
		return
	}
	level := p.volume
	if p.muted {
		level = 0
	}
	p.stream.SetVolume(level)
}

// SetRepeat changes what happens at the end of a track.
func (p *Player) SetRepeat(mode RepeatMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.repeat == mode {
		return
	}
	p.repeat = mode
	p.bump()
}

// SetShuffle turns random order on or off, keeping the current track.
func (p *Player) SetShuffle(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shuffle == on {
		return
	}
	p.shuffle = on
	if on {
		p.buildOrderLocked()
	}
	p.bump()
}

// SetFade is how long a track takes to reach full volume, in milliseconds.
func (p *Player) SetFade(ms int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fadeMs = ms
	p.bump()
}

// Fade is the fade length in milliseconds.
func (p *Player) Fade() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fadeMs
}

// SetQueue replaces the queue and starts the track at index. An out-of-range
// index starts nothing.
func (p *Player) SetQueue(tracks []catalog.Track, index int) error {
	p.mu.Lock()
	p.queue = append([]catalog.Track(nil), tracks...)
	if p.shuffle {
		p.buildOrderLocked()
	}
	p.mu.Unlock()
	return p.PlayAt(index)
}

// Enqueue appends tracks to the queue, starting playback when nothing is
// playing yet.
func (p *Player) Enqueue(tracks []catalog.Track, playNow bool) error {
	if len(tracks) == 0 {
		return nil
	}
	p.mu.Lock()
	empty := p.index < 0
	p.queue = append(p.queue, tracks...)
	if p.shuffle {
		p.buildOrderLocked()
	}
	p.mu.Unlock()
	if playNow && empty {
		return p.PlayAt(p.Index() + 1)
	}
	return nil
}

// InsertAfter puts tracks directly after position, for "play next".
func (p *Player) InsertAfter(position int, tracks []catalog.Track) {
	if len(tracks) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if position < 0 || position >= len(p.queue) {
		p.queue = append(p.queue, tracks...)
	} else {
		at := position + 1
		rest := append([]catalog.Track(nil), p.queue[at:]...)
		p.queue = append(append(p.queue[:at:at], tracks...), rest...)
	}
	if p.shuffle {
		p.buildOrderLocked()
	}
	p.bump()
}

// Remove takes the track at index out of the queue. Removing the current track
// stops playback; the caller decides what to play next.
func (p *Player) Remove(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.queue) {
		return
	}
	if index == p.index {
		p.closeStreamLocked()
		p.index = -1
		p.position = 0
		p.playing = false
	}
	p.queue = append(p.queue[:index:index], p.queue[index+1:]...)
	if p.index > index {
		p.index--
	}
	if p.shuffle {
		p.buildOrderLocked()
	}
	p.bump()
}

// Clear empties the queue and stops playback.
func (p *Player) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeStreamLocked()
	p.queue = nil
	p.index = -1
	p.position = 0
	p.playing = false
	p.bump()
}

// PlayAt loads and plays the track at index.
func (p *Player) PlayAt(index int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.queue) {
		return errors.New("player: no such track")
	}
	if p.index == index && p.stream != nil {
		p.playing = true
		p.startedAt = time.Now()
		p.stream.Play()
		p.applyVolumeLocked()
		p.bump()
		return nil
	}
	return p.loadLocked(index, true)
}

func (p *Player) loadLocked(index int, play bool) error {
	p.closeStreamLocked()
	track := p.queue[index]
	source, err := audio.Open(track.Path)
	if err != nil {
		p.err = err
		p.index = index
		p.position = 0
		p.playing = false
		p.bump()
		return err
	}
	stream, err := p.engine.NewStream(source)
	if err != nil {
		source.Close()
		p.err = err
		p.index = index
		p.position = 0
		p.playing = false
		p.bump()
		return err
	}
	p.err = nil
	p.stream = stream
	p.index = index
	p.position = 0
	p.startedAt = time.Now()
	p.playing = play
	p.lyricFile = lyrics.Load(track.Path)
	if play {
		stream.Play()
	}
	p.applyVolumeLocked()
	p.bump()
	return nil
}

func (p *Player) closeStreamLocked() {
	if p.stream != nil {
		p.stream.Close()
		p.stream = nil
	}
}

// Toggle pauses or resumes.
func (p *Player) Toggle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		index := p.index
		if index < 0 {
			index = p.nextIndexLocked(1)
		}
		if index >= 0 && index < len(p.queue) {
			p.loadLocked(index, true)
		}
		return
	}
	if p.playing {
		p.position = p.positionLocked()
		p.startedAt = time.Time{}
		p.playing = false
		p.stream.Pause()
	} else {
		p.startedAt = time.Now()
		p.playing = true
		p.stream.Play()
		p.applyVolumeLocked()
	}
	p.bump()
}

// Pause stops the sound but keeps the position.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || !p.playing {
		return
	}
	p.position = p.positionLocked()
	p.startedAt = time.Time{}
	p.playing = false
	p.stream.Pause()
	p.bump()
}

// Seek jumps to d in the current track.
func (p *Player) Seek(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		return
	}
	if d < 0 {
		d = 0
	}
	if duration := p.durationLocked(); duration > 0 && d > duration {
		d = duration
	}
	if err := p.stream.Seek(d); err != nil {
		p.err = err
	}
	p.position = d
	if p.playing {
		p.startedAt = time.Now()
	}
	p.bump()
}

// Next moves to the following track. It wraps when repeat is on or the queue
// has been asked to loop.
func (p *Player) Next() error {
	p.mu.Lock()
	index := p.nextIndexLocked(1)
	p.mu.Unlock()
	if index < 0 {
		p.Pause()
		return nil
	}
	return p.PlayAt(index)
}

// Previous moves to the previous track, or restarts the current one when it is
// more than three seconds in.
func (p *Player) Previous() error {
	p.mu.Lock()
	position := p.positionLocked()
	index := p.nextIndexLocked(-1)
	p.mu.Unlock()
	if position > 3*time.Second {
		p.Seek(0)
		return nil
	}
	if index < 0 {
		p.Seek(0)
		return nil
	}
	return p.PlayAt(index)
}

// nextIndexLocked is the queue position step away from the current one, honouring
// shuffle and repeat. It returns -1 when playback should stop.
func (p *Player) nextIndexLocked(step int) int {
	if len(p.queue) == 0 {
		return -1
	}
	if p.shuffle && len(p.order) == len(p.queue) {
		at := p.orderAt + step
		if at >= 0 && at < len(p.order) {
			p.orderAt = at
			return p.order[at]
		}
		if p.repeat == RepeatAll {
			if step > 0 {
				p.orderAt = 0
			} else {
				p.orderAt = len(p.order) - 1
			}
			return p.order[p.orderAt]
		}
		if step > 0 {
			return -1
		}
		return p.order[0]
	}
	index := p.index
	if index < 0 {
		if step > 0 {
			return 0
		}
		return len(p.queue) - 1
	}
	next := index + step
	switch {
	case next >= 0 && next < len(p.queue):
		return next
	case p.repeat == RepeatAll:
		if step > 0 {
			return 0
		}
		return len(p.queue) - 1
	default:
		return -1
	}
}

// Tick advances the queue when a track has finished. The application calls it
// once per paint; it reports whether anything changed.
func (p *Player) Tick() bool {
	p.mu.Lock()
	if p.stream == nil || !p.playing {
		p.mu.Unlock()
		return false
	}
	finished := !p.stream.Playing()
	duration := p.durationLocked()
	position := p.positionLocked()
	repeat := p.repeat
	p.mu.Unlock()
	if !finished {
		return false
	}
	if duration > 0 && position+250*time.Millisecond < duration {
		// The device stalled rather than reaching the end.
		return false
	}
	if repeat == RepeatOne {
		p.Seek(0)
		if err := p.PlayAt(p.Index()); err != nil {
			return false
		}
		return true
	}
	if err := p.Next(); err != nil {
		return false
	}
	return true
}

// Stop halts playback and unloads the current track.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeStreamLocked()
	p.position = 0
	p.playing = false
	p.startedAt = time.Time{}
	p.lyricFile = lyrics.Lyrics{}
	p.bump()
}

// Close releases the audio stream. The engine outlives the player.
func (p *Player) Close() {
	p.Stop()
}

func (p *Player) buildOrderLocked() {
	n := len(p.queue)
	p.order = make([]int, n)
	for i := range p.order {
		p.order[i] = i
	}
	rand.Shuffle(n, func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	p.orderAt = 0
	for at, index := range p.order {
		if index == p.index {
			p.orderAt = at
			break
		}
	}
}

// Restore puts a saved track and position back without starting playback.
func (p *Player) Restore(tracks []catalog.Track, index int, position time.Duration) {
	p.mu.Lock()
	p.queue = append([]catalog.Track(nil), tracks...)
	p.index = index
	p.mu.Unlock()
	if index < 0 || index >= len(tracks) {
		return
	}
	p.mu.Lock()
	if err := p.loadLocked(index, false); err == nil {
		p.position = position
		if err := p.stream.Seek(position); err != nil {
			p.err = err
		}
	}
	p.mu.Unlock()
}
