package app

import (
	"fmt"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/catalog"
	"soda-music/internal/player"
)

// iconButton is the small square icon control used by the header, transport and
// panel chrome. It is deliberately a hit region rather than a Button so the icon
// keeps its own optical size and colour.
func (m *Model) iconButton(f *flex.Frame, theme ui.Theme, id ui.ID, icon ui.Icon, label string, size float32, active bool, onClick func()) {
	box := f.Row().Size(size, size).Items(flex.Center).Justify(flex.Center).Radius(size/2).Hit(id, label)
	if active {
		box = box.Background(theme.Accent)
	} else {
		box = box.HoverBackground(theme.Hover)
	}
	box.Children(func() {
		color := theme.Foreground
		if active {
			color = theme.AccentForeground
		}
		f.Icon(icon).Size(size*0.5, size*0.5).Color(color)
	})
	if box.Clicked() {
		onClick()
	}
}

// coverSlot reserves the place a cover will be painted after layout. Cover art is
// decoded on a worker, so the bitmap may be empty on the first frames.
func (m *Model) coverSlot(f *flex.Frame, track catalog.Track, size, radius float32) flex.Box {
	slot := f.Slot().Size(size, size)
	var bitmap ui.Bitmap
	if track.Online() {
		// A service song carries its own picture URL and has no file to read
		// embedded art from.
		bitmap = m.images.Bitmap(track.CoverURL)
	} else {
		bitmap = m.art.Bitmap(track)
	}
	m.covers = append(m.covers, coverSlot{box: slot, bitmap: bitmap, radius: radius})
	return slot
}

// imageBox reserves the place a picture that is not tied to a local file will be
// painted after layout, such as an online cover or the login QR code. The bitmap
// may be empty on the first frames while the worker fetches it.
func (m *Model) imageBox(f *flex.Frame, bitmap ui.Bitmap, size, radius float32) flex.Box {
	slot := f.Slot().Size(size, size)
	m.imageSlots = append(m.imageSlots, imageSlot{box: slot, bitmap: bitmap, radius: radius})
	return slot
}

func (m *Model) sectionTitle(f *flex.Frame, title, subtitle string) {
	f.Row().FillWidth().Items(flex.End).Gap(8).Children(func() {
		f.Column().Grow(1).Gap(2).Children(func() {
			f.Text(title).FontSize(17).Bold()
			if subtitle != "" {
				f.Text(subtitle).FontSize(12).Muted()
			}
		})
	})
}

func (m *Model) emptyState(f *flex.Frame, theme ui.Theme) {
	f.Column().FillWidth().MinHeight(360).Center().Gap(14).Children(func() {
		f.Row().Size(72, 72).Items(flex.Center).Justify(flex.Center).Radius(36).Background(theme.Accent).Children(func() {
			f.Icon(assetsMusic).Size(32, 32).Color(theme.AccentForeground)
		})
		f.Text("还没有音乐").FontSize(22).Bold()
		f.Text("添加一个音乐文件夹，汽水音乐会自动扫描 MP3、FLAC 和 WAV。").Muted().TextCenter()
		if f.Button(emptyAddFolderID, "添加音乐文件夹").Clicked() {
			m.wantAddFolder = true
		}
	})
}

// trackRow is the common row used by every list of songs. context keeps the
// identity unique when the same track is shown in two sections on one page.
func (m *Model) trackRow(f *flex.Frame, theme ui.Theme, track catalog.Track, index int, tracks []catalog.Track, context string, onRemove func()) {
	id := m.trackID(track.ID)
	if context != "" {
		id = m.rowID("track:" + context + ":" + track.ID)
	}
	row := f.Row().Height(56).FillWidth().PadX(10).Gap(10).Items(flex.Center).Radius(8).HoverBackground(theme.Hover).Hit(id, track.DisplayTitle())
	row.Children(func() {
		f.Text(fmt.Sprintf("%d", index+1)).Width(24).FontSize(12).Muted().TextEnd()
		m.coverSlot(f, track, 40, 7)
		f.Column().Grow(1).Gap(2).Children(func() {
			f.Text(track.DisplayTitle()).Ellipsis().Bold()
			f.Text(track.DisplayArtist()).FontSize(12).Muted().Ellipsis()
		})
		f.Text(track.DisplayAlbum()).Width(150).FontSize(12).Muted().Ellipsis()
		f.Text(formatDuration(track.Duration)).Width(52).FontSize(12).Muted().TextEnd()

		fav := f.Row().Size(30, 30).Items(flex.Center).Justify(flex.Center).Radius(15).HoverBackground(theme.Hover).Hit(m.favID(track.ID), "喜欢")
		fav.Children(func() {
			if m.isFavorite(track.ID) {
				f.Icon(assetsHeartFilled).Size(16, 16).Color(theme.Primary)
			} else {
				f.Icon(assetsHeart).Size(16, 16).Color(theme.MutedForeground)
			}
		})
		if fav.Clicked() {
			m.toggleFavorite(track.ID)
		}

		more := f.Row().Size(30, 30).Items(flex.Center).Justify(flex.Center).Radius(15).HoverBackground(theme.Hover).Hit(m.addID(track.ID), "更多")
		more.Children(func() { f.Icon(assetsMore).Size(16, 16).Color(theme.MutedForeground) })
		if more.Clicked() {
			if m.menuTrackID == track.ID {
				m.menuTrackID = ""
			} else {
				m.menuTrackID = track.ID
			}
		}
		if m.menuTrackID == track.ID {
			f.Popup().BelowBox(more, 6, flex.End).Width(190).Pad(6).Background(theme.Popover).Border(1, theme.Border).Radius(theme.Metrics.RadiusControl).Shadow(ui.ShadowMD).Children(func() {
				if f.Button(m.rowID("menu-next:"+track.ID), "下一首播放").Ghost().Clicked() {
					m.playNext([]catalog.Track{track})
					m.menuTrackID = ""
				}
				if f.Button(m.rowID("menu-add:"+track.ID), "加入歌单…").Ghost().Clicked() {
					m.addTrackID = track.ID
					m.menuTrackID = ""
				}
				if onRemove != nil {
					if f.Button(m.rowID("menu-remove:"+track.ID), "从歌单移除").Ghost().Destructive().Clicked() {
						onRemove()
						m.menuTrackID = ""
					}
				}
			})
		}
	})
	if row.Clicked() {
		m.playTrackList(tracks, index)
	}
}

func (m *Model) trackList(f *flex.Frame, theme ui.Theme, tracks []catalog.Track, context string, onRemove func(string)) {
	if len(tracks) == 0 {
		f.Text("这里还没有歌曲。").Muted().PadY(16)
		return
	}
	f.Column().FillWidth().Gap(2).Children(func() {
		for index, track := range tracks {
			track := track
			remove := func() {}
			if onRemove != nil {
				remove = func() { onRemove(track.ID) }
			}
			m.trackRow(f, theme, track, index, tracks, context, remove)
		}
	})
}

func (m *Model) albumCard(f *flex.Frame, theme ui.Theme, album catalog.Album) {
	card := f.Column().Width(168).Gap(8).Hit(m.albumID(album.Key), album.Name)
	card.Children(func() {
		if len(album.Tracks) > 0 {
			m.coverSlot(f, album.Tracks[0], 168, 12)
		} else {
			f.Slot().Size(168, 168)
		}
		f.Text(album.Name).Bold().Ellipsis()
		f.Text(album.Artist).FontSize(12).Muted().Ellipsis()
	})
	if card.Clicked() {
		m.openAlbum(album.Key)
	}
}

func (m *Model) artistCard(f *flex.Frame, theme ui.Theme, artist catalog.Artist) {
	card := f.Column().Width(168).Gap(8).Hit(m.artistID(artist.Name), artist.Name)
	card.Children(func() {
		if len(artist.Tracks) > 0 {
			m.coverSlot(f, artist.Tracks[0], 168, 84)
		} else {
			f.Slot().Size(168, 168)
		}
		f.Text(artist.Name).Bold().Ellipsis()
		f.Text(fmt.Sprintf("%d 首歌曲", len(artist.Tracks))).FontSize(12).Muted()
	})
	if card.Clicked() {
		m.openArtist(artist.Name)
	}
}

func (m *Model) playlistCard(f *flex.Frame, theme ui.Theme, playlist catalog.Playlist) {
	tracks := m.lookupTracks(playlist.TrackIDs)
	card := f.Column().Width(168).Gap(8).Hit(m.cardID(playlist.ID), playlist.Name)
	card.Children(func() {
		if len(tracks) > 0 {
			m.coverSlot(f, tracks[0], 168, 12)
		} else {
			f.Column().Size(168, 168).Items(flex.Center).Justify(flex.Center).Radius(12).Background(theme.Muted).Children(func() {
				f.Icon(assetsPlaylist).Size(42, 42).Color(theme.MutedForeground)
			})
		}
		f.Text(playlist.Name).Bold().Ellipsis()
		f.Text(fmt.Sprintf("%d 首歌曲", len(playlist.TrackIDs))).FontSize(12).Muted()
	})
	if card.Clicked() {
		m.openPlaylist(playlist.ID)
	}
}

func (m *Model) pageDiscover(f *flex.Frame, theme ui.Theme) {
	if m.library.Length() == 0 {
		m.emptyState(f, theme)
		return
	}
	f.Column().FillWidth().Gap(4).Children(func() {
		f.Text("晚上好，欢迎回来").FontSize(26).Bold()
		f.Text("你的本地音乐，已经准备好播放。").Muted()
	})
	if len(m.recent) > 0 {
		m.sectionTitle(f, "最近添加", "刚刚加入曲库的音乐")
		f.Grid(180).Gap(16).Children(func() {
			for i, track := range m.recent {
				if i >= 6 {
					break
				}
				track := track
				card := f.Column().Width(168).Gap(8).Hit(m.rowID("recent-card:"+track.ID), track.DisplayTitle())
				card.Children(func() {
					m.coverSlot(f, track, 168, 12)
					f.Text(track.DisplayTitle()).Bold().Ellipsis()
					f.Text(track.DisplayArtist()).FontSize(12).Muted().Ellipsis()
				})
				if card.Clicked() {
					m.playTracks(m.recent, i)
				}
			}
		})
	}
	if len(m.mostPlayed) > 0 {
		m.sectionTitle(f, "常听", "播放次数最多的歌曲")
		m.trackList(f, theme, m.mostPlayed, "discover-most", nil)
	}
	if len(m.favorites) > 0 {
		m.sectionTitle(f, "喜欢的歌曲", "你的收藏")
		m.trackList(f, theme, m.favorites, "discover-fav", nil)
	}
}

func (m *Model) pageSearch(f *flex.Frame, theme ui.Theme) {
	field := f.Field(&m.search, searchFieldID, "搜索", "搜索歌曲、专辑或艺术家")
	if field.Submitted() {
		m.syncSearch()
	}
	if strings.TrimSpace(m.search.Text()) == "" {
		f.Column().FillWidth().MinHeight(260).Center().Gap(10).Children(func() {
			f.Icon(assetsSearch).Size(42, 42).Color(theme.MutedForeground)
			f.Text("搜索你的音乐库").FontSize(18).Bold()
			f.Text("输入歌曲名、专辑或艺术家。").Muted()
		})
		return
	}
	if len(m.searchTracks) == 0 && len(m.searchAlbums) == 0 && len(m.searchArtists) == 0 {
		f.Text(fmt.Sprintf("没有找到与“%s”相关的内容。", m.searchQuery)).Muted().PadY(16)
		return
	}
	if len(m.searchTracks) > 0 {
		m.sectionTitle(f, "歌曲", fmt.Sprintf("%d 个结果", len(m.searchTracks)))
		m.trackList(f, theme, m.searchTracks, "search-track", nil)
	}
	if len(m.searchAlbums) > 0 {
		m.sectionTitle(f, "专辑", "")
		f.Grid(180).Gap(16).Children(func() {
			for _, album := range m.searchAlbums {
				m.albumCard(f, theme, album)
			}
		})
	}
	if len(m.searchArtists) > 0 {
		m.sectionTitle(f, "艺术家", "")
		f.Grid(180).Gap(16).Children(func() {
			for _, artist := range m.searchArtists {
				m.artistCard(f, theme, artist)
			}
		})
	}
}

func (m *Model) pageLibrary(f *flex.Frame, theme ui.Theme) {
	tabs := f.Tabs(libraryTabScope, []string{"歌曲", "专辑", "艺术家", "流派"}, int(m.libraryTab)).Line()
	if index, ok := tabs.TabClicked(); ok {
		m.libraryTab = libraryTab(index)
	}
	switch m.libraryTab {
	case tabTracks:
		m.trackList(f, theme, m.tracks, "library-track", nil)
	case tabAlbums:
		f.Grid(180).Gap(16).Children(func() {
			for _, album := range m.library.Albums {
				m.albumCard(f, theme, album)
			}
		})
	case tabArtists:
		f.Grid(180).Gap(16).Children(func() {
			for _, artist := range m.library.Artists {
				m.artistCard(f, theme, artist)
			}
		})
	case tabGenres:
		genres := m.library.Genres()
		if len(genres) == 0 {
			f.Text("歌曲里还没有流派标签。").Muted().PadY(16)
			return
		}
		f.Grid(220).Gap(12).Children(func() {
			for _, genre := range genres {
				genre := genre
				card := f.Row().Height(64).PadX(16).Items(flex.Center).Gap(12).Radius(12).Background(theme.Card).Border(1, theme.Border).Hit(m.genreID(genre), genre)
				card.Children(func() {
					f.Icon(assetsDisc).Size(22, 22).Color(theme.Primary)
					f.Text(genre).Bold().Grow(1).Ellipsis()
				})
				if card.Clicked() {
					tracks := make([]catalog.Track, 0)
					for _, track := range m.library.Tracks {
						if strings.EqualFold(track.Genre, genre) {
							tracks = append(tracks, track)
						}
					}
					m.playTracks(tracks, 0)
				}
			}
		})
	}
}

func (m *Model) pagePlaylists(f *flex.Frame, theme ui.Theme) {
	f.Row().FillWidth().Items(flex.Center).Children(func() {
		f.Text("你的播放列表").FontSize(17).Bold().Grow(1)
		if f.Button(newPlaylistID, "新建歌单").Clicked() {
			m.showNewList = true
		}
	})
	if len(m.state.Playlists) == 0 {
		f.Column().FillWidth().MinHeight(240).Center().Gap(10).Children(func() {
			f.Icon(assetsPlaylist).Size(42, 42).Color(theme.MutedForeground)
			f.Text("还没有歌单").FontSize(18).Bold()
			f.Text("创建一个歌单，把喜欢的音乐收在一起。").Muted()
			if f.Button(emptyNewPlaylistID, "新建歌单").Clicked() {
				m.showNewList = true
			}
		})
		return
	}
	f.Grid(180).Gap(16).Children(func() {
		for _, playlist := range m.state.Playlists {
			m.playlistCard(f, theme, playlist)
		}
	})
}

func (m *Model) pageSettings(f *flex.Frame, theme ui.Theme) {
	f.Column().FillWidth().Gap(18).Children(func() {
		f.Text("音乐文件夹").FontSize(17).Bold()
		f.Column().FillWidth().Gap(8).Children(func() {
			if len(m.state.Folders) == 0 {
				f.Text("还没有添加音乐文件夹。").Muted()
			}
			for _, folder := range m.state.Folders {
				f.Row().FillWidth().Pad(12).Gap(10).Items(flex.Center).Radius(10).Background(theme.Card).Border(1, theme.Border).Children(func() {
					f.Icon(assetsFolder).Size(18, 18).Color(theme.Primary)
					f.Text(folder).Grow(1).Ellipsis().FontSize(13)
				})
			}
			if f.Button(settingsAddFolderID, "添加音乐文件夹").Outline().Clicked() {
				m.wantAddFolder = true
			}
		})
		f.Divider()
		f.Text("外观").FontSize(17).Bold()
		labels := []string{"跟随系统", "浅色", "深色"}
		selected := 0
		switch m.state.Theme {
		case "light":
			selected = 1
		case "dark":
			selected = 2
		}
		if index, ok := f.Choice(themeScope, labels, selected).ChosenIndex(); ok {
			switch index {
			case 1:
				m.state.Theme = "light"
			case 2:
				m.state.Theme = "dark"
			default:
				m.state.Theme = "system"
			}
			m.touch()
		}
		f.Divider()
		f.Text("播放").FontSize(17).Bold()
		f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
			f.Text("音量").Width(70)
			if f.Slider(settingsVolumeID, &m.volume).Range(0, 1).Step(0.01).Changed() {
				m.player.SetVolume(float64(m.volume.Value))
				m.state.Volume = float64(m.volume.Value)
				m.touch()
			}
			f.Text(fmt.Sprintf("%d%%", int(m.volume.Value*100))).Width(48).Muted().TextEnd()
		})
		f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
			f.Text("淡入淡出").Width(70)
			if f.Slider(settingsFadeID, &m.fade).Range(0, 1000).Step(10).Changed() {
				m.player.SetFade(int(m.fade.Value))
				m.state.FadeMs = int(m.fade.Value)
				m.touch()
			}
			f.Text(fmt.Sprintf("%d ms", int(m.fade.Value))).Width(64).Muted().TextEnd()
		})
		f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
			f.Text("随机播放").Grow(1)
			sw := f.Switch(settingsShuffleID, m.player.Shuffle())
			if sw.Clicked() {
				m.player.SetShuffle(!m.player.Shuffle())
				m.state.Shuffle = m.player.Shuffle()
				m.touch()
			}
		})
		f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
			f.Text("循环模式").Grow(1)
			label := "关闭"
			switch m.player.Repeat() {
			case player.RepeatAll:
				label = "列表循环"
			case player.RepeatOne:
				label = "单曲循环"
			}
			if f.Button(settingsRepeatID, label).Outline().Clicked() {
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
		})
		f.Divider()
		if f.Button(clearHistoryID, "清除播放历史").Outline().Clicked() {
			m.state.History = nil
			m.refresh()
			m.touch()
			m.toastf("播放历史已清除")
		}
	})
}

func (m *Model) pageAlbum(f *flex.Frame, theme ui.Theme) {
	album, ok := m.library.Album(m.albumKey)
	if !ok {
		f.Text("专辑不存在。").Muted()
		return
	}
	f.Row().FillWidth().Gap(18).Children(func() {
		if len(album.Tracks) > 0 {
			m.coverSlot(f, album.Tracks[0], 180, 14)
		}
		f.Column().Grow(1).Justify(flex.Center).Gap(8).Children(func() {
			f.Text(album.Name).FontSize(24).Bold()
			f.Text(album.Artist).Muted()
			f.Text(fmt.Sprintf("%d 首歌曲 · %s", len(album.Tracks), formatTotal(album.Duration))).FontSize(12).Muted()
			f.Row().Gap(8).Children(func() {
				if f.Button(m.rowID("play-album"), "播放").Clicked() {
					m.playTracks(album.Tracks, 0)
				}
				if f.Button(m.rowID("shuffle-album"), "随机播放").Outline().Clicked() {
					m.player.SetShuffle(true)
					m.state.Shuffle = true
					m.playTracks(album.Tracks, 0)
					m.touch()
				}
			})
		})
	})
	m.trackList(f, theme, album.Tracks, "album", nil)
}

func (m *Model) pageArtist(f *flex.Frame, theme ui.Theme) {
	artist, ok := m.library.Artist(m.artistName)
	if !ok {
		f.Text("艺术家不存在。").Muted()
		return
	}
	f.Row().FillWidth().Gap(18).Items(flex.Center).Children(func() {
		if len(artist.Tracks) > 0 {
			m.coverSlot(f, artist.Tracks[0], 120, 60)
		}
		f.Column().Grow(1).Gap(6).Children(func() {
			f.Text(artist.Name).FontSize(24).Bold()
			f.Text(fmt.Sprintf("%d 首歌曲 · %s", len(artist.Tracks), formatTotal(artist.Duration))).Muted()
			if f.Button(m.rowID("play-artist"), "播放全部").Clicked() {
				m.playTracks(artist.Tracks, 0)
			}
		})
	})
	m.trackList(f, theme, artist.Tracks, "artist", nil)
}

func (m *Model) pagePlaylist(f *flex.Frame, theme ui.Theme) {
	playlist, ok := m.playlist(m.playlistID)
	if !ok {
		f.Text("歌单不存在。").Muted()
		return
	}
	tracks := m.lookupTracks(playlist.TrackIDs)
	f.Row().FillWidth().Items(flex.Center).Children(func() {
		f.Column().Grow(1).Gap(4).Children(func() {
			f.Text(playlist.Name).FontSize(24).Bold()
			f.Text(fmt.Sprintf("%d 首歌曲", len(tracks))).Muted()
		})
		if len(tracks) > 0 {
			if f.Button(m.rowID("play-playlist"), "播放").Clicked() {
				m.playTracks(tracks, 0)
			}
		}
		if f.Button(m.rowID("delete-playlist"), "删除歌单").Outline().Destructive().Clicked() {
			m.confirmDelete = playlist.ID
		}
	})
	m.trackList(f, theme, tracks, "playlist", func(trackID string) {
		m.removeFromPlaylist(playlist.ID, trackID)
	})
}

func (m *Model) pageFavorites(f *flex.Frame, theme ui.Theme) {
	if len(m.favorites) == 0 {
		f.Text("还没有喜欢的歌曲。点击歌曲右侧的心形即可收藏。").Muted().PadY(16)
		return
	}
	m.trackList(f, theme, m.favorites, "favorites", nil)
}

func (m *Model) pageRecent(f *flex.Frame, theme ui.Theme) {
	if len(m.recent) == 0 {
		f.Text("还没有最近添加的歌曲。").Muted().PadY(16)
		return
	}
	m.trackList(f, theme, m.recent, "recent", nil)
}
