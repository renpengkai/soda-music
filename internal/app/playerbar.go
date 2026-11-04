package app

import (
	"time"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/player"
)

// playerBarHeight is the height of the transport strip along the bottom edge.
const playerBarHeight = 78

// playerBar is the transport at the bottom of the window: what is playing on the
// left, the controls and the progress in the middle, and the volume and the
// panels on the right. It reads the player and records the clicks; the actions
// themselves live in the small methods below.
func (m *Model) playerBar(f *flex.Frame, theme ui.Theme, size ui.Size) {
	track, hasTrack := m.player.Current()
	position := m.player.Position()
	duration := m.player.Duration()
	if duration <= 0 && hasTrack {
		duration = track.Duration
	}
	if !m.progress.Dragging() {
		m.progress.Value = fraction(position, duration)
	}
	if !m.barVolume.Dragging() {
		m.barVolume.Value = float32(m.player.Volume())
	}

	f.Row().FillWidth().Height(playerBarHeight).PadSides(12, 18, 12, 18).Items(flex.Center).Gap(14).Background(theme.Card).Border(1, theme.Border).Children(func() {
		// What is playing. The whole tile opens the now-playing view.
		now := f.Row().Width(250).Items(flex.Center).Gap(12).Pad(6).Radius(10).HoverBackground(theme.Hover).Hit(m.rowID("now-playing"), "正在播放")
		now.Children(func() {
			if hasTrack {
				m.coverSlot(f, track, 46, 8)
			} else {
				f.Column().Size(46, 46).Center().Radius(8).Background(theme.Muted).Children(func() {
					f.Icon(assetsMusic).Size(20, 20).Color(theme.MutedForeground)
				})
			}
			f.Column().Grow(1).Gap(2).Children(func() {
				if hasTrack {
					f.Text(track.DisplayTitle()).Bold().Ellipsis()
					f.Text(track.DisplayArtist()).FontSize(12).Muted().Ellipsis()
				} else {
					f.Text("未在播放").Bold().Muted().Ellipsis()
					f.Text("从曲库里挑一首开始").FontSize(12).Muted().Ellipsis()
				}
			})
		})
		if now.Clicked() {
			m.showNowPlaying = true
		}

		// The transport and the progress.
		f.Column().Grow(1).Shrink(1).Gap(4).Children(func() {
			f.Row().FillWidth().Items(flex.Center).Justify(flex.Center).Gap(4).Children(func() {
				m.iconButton(f, theme, shuffleID, assetsShuffle, "随机播放", 32, m.player.Shuffle(), m.toggleShuffle)
				m.iconButton(f, theme, previousID, assetsPrevious, "上一首", 34, false, m.previousTrack)
				playIcon, playLabel := assetsPlay, "播放"
				if m.player.Playing() {
					playIcon, playLabel = assetsPause, "暂停"
				}
				m.iconButton(f, theme, playPauseID, playIcon, playLabel, 40, true, m.togglePlay)
				m.iconButton(f, theme, nextID, assetsNext, "下一首", 34, false, m.nextTrack)
				repeatIcon := assetsRepeat
				if m.player.Repeat() == player.RepeatOne {
					repeatIcon = assetsRepeatOne
				}
				m.iconButton(f, theme, repeatID, repeatIcon, "循环模式", 32, m.player.Repeat() != player.RepeatOff, m.cycleRepeat)
			})
			f.Row().FillWidth().Items(flex.Center).Gap(10).Children(func() {
				f.Text(formatDuration(position)).FontSize(11).Muted().Width(44).TextEnd()
				if f.Slider(progressID, &m.progress).Range(0, 1).Step(0.001).Grow(1).Changed() {
					if duration > 0 {
						m.player.Seek(time.Duration(float64(m.progress.Value) * float64(duration)))
					}
				}
				f.Text(formatDuration(duration)).FontSize(11).Muted().Width(44)
			})
		})

		// Volume and the panels.
		f.Row().Width(216).Items(flex.Center).Justify(flex.End).Gap(6).Children(func() {
			volumeIcon := assetsVolume
			if m.player.Muted() || m.player.Volume() <= 0 {
				volumeIcon = assetsVolumeX
			}
			m.iconButton(f, theme, muteID, volumeIcon, "静音", 30, false, m.toggleMute)
			if f.Slider(volumeID, &m.barVolume).Range(0, 1).Step(0.01).Width(84).Changed() {
				m.player.SetMuted(false)
				m.player.SetVolume(float64(m.barVolume.Value))
				m.state.Volume = float64(m.barVolume.Value)
				m.volume.Value = m.barVolume.Value
				m.touch()
			}
			m.iconButton(f, theme, lyricsID, assetsMic, "歌词", 30, m.showNowPlaying, func() {
				m.showNowPlaying = !m.showNowPlaying
			})
			m.iconButton(f, theme, queueID, assetsQueue, "播放队列", 30, m.showQueue, func() {
				m.showQueue = !m.showQueue
			})
		})
	})
}

// fraction is part divided by whole, in [0, 1], and zero when whole is not
// positive.
func fraction(part, whole time.Duration) float32 {
	if whole <= 0 {
		return 0
	}
	value := float32(float64(part) / float64(whole))
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// togglePlay starts, pauses or resumes the current track.
func (m *Model) togglePlay() {
	m.player.Toggle()
	m.afterPlayStart()
}

// nextTrack moves to the following track.
func (m *Model) nextTrack() {
	if err := m.player.Next(); err != nil {
		m.toastf("无法播放：%v", err)
	}
	m.afterPlayStart()
}

// previousTrack restarts the current track, or moves to the one before it.
func (m *Model) previousTrack() {
	if err := m.player.Previous(); err != nil {
		m.toastf("无法播放：%v", err)
	}
	m.afterPlayStart()
}

// toggleShuffle turns the random order on or off.
func (m *Model) toggleShuffle() {
	m.player.SetShuffle(!m.player.Shuffle())
	m.state.Shuffle = m.player.Shuffle()
	m.touch()
}

// cycleRepeat steps through off, list and one.
func (m *Model) cycleRepeat() {
	switch m.player.Repeat() {
	case player.RepeatOff:
		m.player.SetRepeat(player.RepeatAll)
	case player.RepeatAll:
		m.player.SetRepeat(player.RepeatOne)
	default:
		m.player.SetRepeat(player.RepeatOff)
	}
	m.state.Repeat = string(m.player.Repeat())
	m.touch()
}

// toggleMute suppresses or restores the sound.
func (m *Model) toggleMute() {
	m.player.SetMuted(!m.player.Muted())
}
