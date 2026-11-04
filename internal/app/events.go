package app

import (
	"time"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// onKey maps the window's keys onto playback and the overlays. It is given to
// Config.OnKey, so it sees every key before the focused control does; a key a
// text field owns is left for the field.
func (m *Model) onKey(event ui.KeyEvent) ui.EventResult {
	if event.Release || m.typing(event.Target) {
		return ui.EventResult{}
	}
	// A focused slider owns the arrows, Home and End.
	for _, slider := range []*flex.SliderState{&m.volume, &m.barVolume, &m.fade} {
		if result := slider.Key(event); result.Handled {
			m.syncVolume()
			return result
		}
	}
	switch event.Key {
	case ui.KeyEscape:
		if !m.dismiss() {
			return ui.EventResult{}
		}
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.KeySpace:
		m.togglePlay()
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.KeyLeft:
		if event.Modifiers&(ui.ModifierControl|ui.ModifierMeta) != 0 {
			m.previousTrack()
		} else {
			m.seekBy(-5 * time.Second)
		}
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.KeyRight:
		if event.Modifiers&(ui.ModifierControl|ui.ModifierMeta) != 0 {
			m.nextTrack()
		} else {
			m.seekBy(5 * time.Second)
		}
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.KeyUp:
		m.adjustVolume(0.05)
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.KeyDown:
		m.adjustVolume(-0.05)
		return ui.EventResult{Handled: true, Repaint: true}
	}
	return ui.EventResult{}
}

// typing reports whether the focused control is a text field, which owns every
// printable key and the arrows.
func (m *Model) typing(target ui.ID) bool {
	switch target {
	case searchFieldID, playlistFieldID, newPlaylistFieldID, m.onlineID("search-field"):
		return true
	}
	return false
}

// dismiss closes the innermost thing that is open, and reports whether it closed
// anything.
func (m *Model) dismiss() bool {
	switch {
	case m.showLogin:
		m.cancelLogin()
	case m.addTrackID != "":
		m.addTrackID = ""
	case m.showNewList:
		m.showNewList = false
	case m.confirmDelete != "":
		m.confirmDelete = ""
	case m.menuTrackID != "":
		m.menuTrackID = ""
	case m.showQueue:
		m.showQueue = false
	case m.showNowPlaying:
		m.showNowPlaying = false
	default:
		return false
	}
	return true
}

// seekBy moves the position by delta, kept inside the track.
func (m *Model) seekBy(delta time.Duration) {
	duration := m.player.Duration()
	if duration <= 0 {
		return
	}
	position := m.player.Position() + delta
	if position < 0 {
		position = 0
	}
	if position > duration {
		position = duration
	}
	m.player.Seek(position)
}

// adjustVolume moves the level by delta, kept in [0, 1], and remembers it.
func (m *Model) adjustVolume(delta float64) {
	value := m.player.Volume() + delta
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	m.player.SetMuted(false)
	m.player.SetVolume(value)
	m.state.Volume = value
	m.volume.Value = float32(value)
	m.barVolume.Value = float32(value)
	m.touch()
}

// syncVolume copies a change a slider made back onto the player.
func (m *Model) syncVolume() {
	if m.volume.Value == m.barVolume.Value {
		return
	}
	value := float64(m.volume.Value)
	m.player.SetVolume(value)
	m.state.Volume = value
	m.barVolume.Value = m.volume.Value
	m.touch()
}
