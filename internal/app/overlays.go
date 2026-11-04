package app

import (
	"fmt"
	"time"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/catalog"
	"soda-music/internal/lyrics"
)

// lyricLineHeight is how tall one lyric line is, used both for the row and for
// scrolling the current line into view.
const lyricLineHeight = 34

// overlays declares everything that floats over the page: the now-playing view,
// the queue, the dialogs and the toast. It runs after the page so these paint on
// top of it.
func (m *Model) overlays(f *flex.Frame, theme ui.Theme, size ui.Size) {
	if m.showNowPlaying {
		m.nowPlayingOverlay(f, theme, size)
	}
	if m.showQueue {
		m.queuePanel(f, theme, size)
	}
	if m.addTrackID != "" {
		m.addToPlaylistDialog(f, theme)
	}
	if m.showNewList {
		m.newPlaylistDialog(f, theme)
	}
	if m.confirmDelete != "" {
		m.deleteConfirmDialog(f, theme)
	}
	if m.showLogin {
		m.loginDialog(f, theme)
	}
	m.toastPill(f, theme, size)
}

// nowPlayingOverlay is the full-window view of the current track: a large cover,
// the words, and the transport.
func (m *Model) nowPlayingOverlay(f *flex.Frame, theme ui.Theme, size ui.Size) {
	track, hasTrack := m.player.Current()
	position := m.player.Position()
	duration := m.player.Duration()
	if duration <= 0 && hasTrack {
		duration = track.Duration
	}
	if !m.overlayProgress.Dragging() {
		m.overlayProgress.Value = fraction(position, duration)
	}

	f.Popup().At(0, 0).Fill().Background(theme.Background).Children(func() {
		f.Column().Fill().PadSides(20, 40, 18, 40).Gap(14).Children(func() {
			f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
				f.Text("正在播放").FontSize(13).Muted().Grow(1)
				m.iconButton(f, theme, closeOverlayID, assetsChevronDown, "收起", 34, false, func() {
					m.showNowPlaying = false
				})
			})
			f.Row().Grow(1).Shrink(1).FillWidth().Gap(40).Children(func() {
				f.Column().Width(340).Grow(1).Gap(18).Justify(flex.Center).Children(func() {
					if hasTrack {
						m.coverSlot(f, track, 320, 18)
					} else {
						f.Column().Size(320, 320).Center().Radius(18).Background(theme.Muted).Children(func() {
							f.Icon(assetsMusic).Size(96, 96).Color(theme.MutedForeground)
						})
					}
					f.Column().Gap(4).Children(func() {
						if hasTrack {
							f.Text(track.DisplayTitle()).FontSize(24).Bold().Ellipsis()
							f.Text(track.DisplayArtist()).Muted().Ellipsis()
							f.Text(track.DisplayAlbum()).FontSize(12).Muted().Ellipsis()
						} else {
							f.Text("未在播放").FontSize(24).Bold()
							f.Text("从曲库里挑一首开始").Muted()
						}
					})
				})
				m.lyricsPanel(f, theme, position, track.ID)
			})
			f.Column().FillWidth().Gap(8).Children(func() {
				f.Row().FillWidth().Items(flex.Center).Justify(flex.Center).Gap(10).Children(func() {
					m.iconButton(f, theme, overlayPreviousID, assetsPrevious, "上一首", 40, false, m.previousTrack)
					playIcon, playLabel := assetsPlay, "播放"
					if m.player.Playing() {
						playIcon, playLabel = assetsPause, "暂停"
					}
					m.iconButton(f, theme, overlayPlayPauseID, playIcon, playLabel, 48, true, m.togglePlay)
					m.iconButton(f, theme, overlayNextID, assetsNext, "下一首", 40, false, m.nextTrack)
				})
				f.Row().FillWidth().Items(flex.Center).Gap(10).Children(func() {
					f.Text(formatDuration(position)).FontSize(11).Muted().Width(44).TextEnd()
					if f.Slider(overlayProgressID, &m.overlayProgress).Range(0, 1).Step(0.001).Grow(1).Changed() {
						if duration > 0 {
							m.player.Seek(time.Duration(float64(m.overlayProgress.Value) * float64(duration)))
						}
					}
					f.Text(formatDuration(duration)).FontSize(11).Muted().Width(44)
				})
			})
		})
	})
}

// lyricsPanel is the right half of the now-playing view: the words of the
// current track, with the line that is being sung picked out and kept in view.
func (m *Model) lyricsPanel(f *flex.Frame, theme ui.Theme, position time.Duration, trackID string) {
	lyric := m.player.Lyrics()
	f.Column().Grow(1).Shrink(1).Gap(10).Children(func() {
		f.Text("歌词").FontSize(13).Muted()
		if lyric.Empty() {
			f.Column().Grow(1).Center().Gap(10).Children(func() {
				f.Icon(assetsMic).Size(30, 30).Color(theme.MutedForeground)
				f.Text("这首歌还没有歌词").Muted()
				f.Text("把同名的 .lrc 文件放在歌曲旁边即可显示。").FontSize(12).Muted().TextCenter()
			})
			return
		}
		active := lyrics.Active(lyric.Lines, position-lyric.Offset)
		if active != m.lyricIndex || trackID != m.lyricTrack {
			m.lyricIndex, m.lyricTrack = active, trackID
			if state := m.view.ScrollState(lyricsScroll); state != nil {
				state.ScrollTo(float32(active)*lyricLineHeight - 140)
			}
		}
		f.ScrollArea(lyricsScroll).Grow(1).Children(func() {
			f.Column().FillWidth().PadY(120).Gap(2).Children(func() {
				for i, line := range lyric.Lines {
					i, line := i, line
					height := float32(lyricLineHeight)
					if line.Translation != "" {
						height = lyricLineHeight * 3 / 2
					}
					box := f.Column().Height(height).FillWidth().Justify(flex.Center)
					if i == active {
						box = box.Background(theme.Accent).Radius(8).PadX(10)
					}
					box.Children(func() {
						color := theme.MutedForeground
						size := float32(15)
						if i == active {
							color = theme.AccentForeground
							size = 18
						}
						f.Text(line.Text).FontSize(size).Color(color).Ellipsis()
						if line.Translation != "" {
							f.Text(line.Translation).FontSize(12).Muted().Ellipsis()
						}
					})
				}
			})
		})
	})
}

// queuePanel is the list of what will play next, along the right edge.
func (m *Model) queuePanel(f *flex.Frame, theme ui.Theme, size ui.Size) {
	queue := m.queue
	index := m.player.Index()
	height := size.Height - playerBarHeight
	if height < 200 {
		height = 200
	}
	f.Popup().At(size.Width-380, 0).Width(380).Height(height).Background(theme.Popover).Border(1, theme.Border).Children(func() {
		f.Column().Fill().Pad(16).Gap(10).Children(func() {
			f.Row().FillWidth().Items(flex.Center).Gap(10).Children(func() {
				f.Text("播放队列").FontSize(16).Bold().Grow(1)
				f.Text(fmt.Sprintf("%d 首", len(queue))).FontSize(12).Muted()
				m.iconButton(f, theme, closeQueueID, assetsClose, "关闭", 30, false, func() {
					m.showQueue = false
				})
			})
			f.Divider()
			if len(queue) == 0 {
				f.Column().Grow(1).Center().Gap(8).Children(func() {
					f.Icon(assetsQueue).Size(32, 32).Color(theme.MutedForeground)
					f.Text("队列是空的").Muted()
				})
				return
			}
			f.ScrollArea(queueScroll).Grow(1).Children(func() {
				f.Column().FillWidth().Gap(2).Children(func() {
					for i, track := range queue {
						i, track := i, track
						current := i == index
						row := f.Row().Height(52).FillWidth().PadX(8).Gap(10).Items(flex.Center).Radius(8).Hit(m.queueID(fmt.Sprintf("%d:%s", i, track.ID)), track.DisplayTitle())
						if current {
							row = row.Background(theme.Accent)
						} else {
							row = row.HoverBackground(theme.Hover)
						}
						row.Children(func() {
							f.Text(fmt.Sprintf("%d", i+1)).Width(22).FontSize(12).Muted().TextEnd()
							m.coverSlot(f, track, 36, 6)
							f.Column().Grow(1).Gap(2).Children(func() {
								f.Text(track.DisplayTitle()).FontSize(13).Ellipsis()
								f.Text(track.DisplayArtist()).FontSize(11).Muted().Ellipsis()
							})
							if current {
								f.Icon(assetsEqualizer).Size(16, 16).Color(theme.Primary)
							}
							remove := f.Row().Size(26, 26).Center().Radius(13).HoverBackground(theme.Hover).Hit(m.removeID(fmt.Sprintf("queue:%d", i)), "从队列移除")
							remove.Children(func() {
								f.Icon(assetsClose).Size(14, 14).Color(theme.MutedForeground)
							})
							if remove.Clicked() {
								m.player.Remove(i)
								m.queue = m.player.Queue()
							}
						})
						if row.Clicked() {
							if err := m.player.PlayAt(i); err != nil {
								m.toastf("无法播放：%v", err)
							}
							m.afterPlayStart()
						}
					}
				})
			})
		})
	})
}

// addToPlaylistDialog asks which playlist a track should join.
func (m *Model) addToPlaylistDialog(f *flex.Frame, theme ui.Theme) {
	track, _ := m.lookupTrack(m.addTrackID)
	d := f.Dialog(flex.DialogSpec{
		Title:       "加入歌单",
		Description: track.DisplayTitle(),
		Width:       420,
		Scope:       addScopeID,
		DismissID:   dismissAddID,
		CloseID:     dismissAddID,
	})
	d.Body(func() {
		if len(m.state.Playlists) == 0 {
			f.Text("还没有歌单。先新建一个，再把这首歌放进去。").Muted()
			return
		}
		f.Column().FillWidth().Gap(2).Children(func() {
			for _, playlist := range m.state.Playlists {
				playlist := playlist
				row := f.Row().Height(40).FillWidth().PadX(10).Gap(10).Items(flex.Center).Radius(8).HoverBackground(theme.Hover).Hit(m.pickID(playlist.ID), playlist.Name)
				row.Children(func() {
					f.Icon(assetsPlaylist).Size(16, 16).Color(theme.MutedForeground)
					f.Text(playlist.Name).Grow(1).Ellipsis()
					f.Text(fmt.Sprintf("%d 首", len(playlist.TrackIDs))).FontSize(12).Muted()
				})
				if row.Clicked() {
					m.addToPlaylist(playlist.ID, []catalog.Track{track})
					m.addTrackID = ""
				}
			}
		})
	})
	d.Footer(func() {
		if f.Button(addNewPlaylistID, "新建歌单").Outline().Clicked() {
			m.addTrackID = ""
			m.showNewList = true
		}
	})
	if d.Dismissed() {
		m.addTrackID = ""
	}
}

// newPlaylistDialog asks for the name of a new playlist.
func (m *Model) newPlaylistDialog(f *flex.Frame, theme ui.Theme) {
	d := f.Dialog(flex.DialogSpec{
		Title:          "新建歌单",
		Description:    "给歌单起一个名字。",
		Width:          400,
		Scope:          dialogScopeID,
		DismissID:      newPlaylistCancelID,
		CloseID:        newPlaylistCancelID,
		InitialFocusID: newPlaylistFieldID,
	})
	d.Body(func() {
		if f.Field(&m.playlistDraft, newPlaylistFieldID, "歌单名称", "例如：深夜循环").Submitted() {
			m.createPlaylist(m.playlistDraft.Text())
		}
	})
	d.Footer(func() {
		if f.Button(newPlaylistCancelID, "取消").Outline().Clicked() {
			m.playlistDraft.SetText("")
			m.showNewList = false
		}
		if f.Button(newPlaylistCreateID, "创建").Clicked() {
			m.createPlaylist(m.playlistDraft.Text())
		}
	})
	if d.Dismissed() {
		m.showNewList = false
	}
}

// deleteConfirmDialog asks before a playlist is removed.
func (m *Model) deleteConfirmDialog(f *flex.Frame, theme ui.Theme) {
	name := "这个歌单"
	if playlist, ok := m.playlist(m.confirmDelete); ok {
		name = "《" + playlist.Name + "》"
	}
	d := f.Dialog(flex.DialogSpec{
		Title:       "删除歌单？",
		Description: fmt.Sprintf("%s 将从汽水音乐里移除，音乐文件不会被删除。", name),
		Width:       400,
		Scope:       confirmScopeID,
		DismissID:   cancelDeleteID,
		CloseID:     cancelDeleteID,
	})
	d.Footer(func() {
		if f.Button(cancelDeleteID, "取消").Outline().Clicked() {
			m.confirmDelete = ""
		}
		if f.Button(confirmDeleteID, "删除").Destructive().Clicked() {
			m.deletePlaylist(m.confirmDelete)
			m.confirmDelete = ""
		}
	})
	if d.Dismissed() {
		m.confirmDelete = ""
	}
}

// loginDialog shows the QR code a phone scans to sign in, together with the
// state of the poll underneath it.
func (m *Model) loginDialog(f *flex.Frame, theme ui.Theme) {
	d := f.Dialog(flex.DialogSpec{
		Title:       "扫码登录网易云音乐",
		Description: "打开网易云音乐 App，扫描二维码并在手机上确认。",
		Width:       380,
		Scope:       loginScopeID,
		DismissID:   dismissLoginID,
		CloseID:     dismissLoginID,
	})
	d.Body(func() {
		f.Column().FillWidth().Items(flex.Center).Gap(12).Children(func() {
			login := m.onlineLogin
			if login == nil {
				if m.loginError != "" {
					f.Text(m.loginError).Muted().TextCenter()
					return
				}
				f.Text("正在获取二维码…").Muted().PadY(24)
				return
			}
			if bitmap := m.loginBitmap(login); bitmap.Width > 0 {
				m.imageBox(f, bitmap, 220, 10)
			} else {
				f.Column().Size(220, 220).Center().Radius(10).Background(theme.Muted).Children(func() {
					f.Icon(assetsQrcode).Size(64, 64).Color(theme.MutedForeground)
				})
			}
			message := login.Message
			if message == "" {
				message = "请使用网易云音乐 App 扫码"
			}
			f.Text(message).FontSize(13).Muted().TextCenter()
			if m.loginError != "" {
				f.Text(m.loginError).FontSize(12).Muted().TextCenter()
			}
		})
	})
	d.Footer(func() {
		if f.Button(refreshLoginID, "刷新二维码").Outline().Clicked() {
			m.signIn()
		}
		if f.Button(dismissLoginID, "取消").Outline().Clicked() {
			m.cancelLogin()
		}
	})
	if d.Dismissed() {
		m.cancelLogin()
	}
}

// toastPill is the short message that appears over the player bar.
func (m *Model) toastPill(f *flex.Frame, theme ui.Theme, size ui.Size) {
	message := m.toastVisible()
	if message == "" {
		return
	}
	y := size.Height - playerBarHeight - 64
	if y < 0 {
		y = 0
	}
	f.Popup().At(0, y).FillWidth().Items(flex.Center).Children(func() {
		f.Row().PadSides(8, 16, 8, 16).Radius(16).Background(theme.Foreground).Shadow(ui.ShadowMD).Children(func() {
			f.Text(message).FontSize(13).Color(theme.Background)
		})
	})
}
