package app

import (
	"fmt"
	"strings"
	"time"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/app/iconpng"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/assets"
	"soda-music/internal/audio"
	"soda-music/internal/catalog"
)

var (
	appTree = ui.Tree{}
	appRoot = appTree.Feature("soda-music")

	themeScope      = appTree.Sub(appRoot, appTree.Name("theme"))
	libraryTabScope = appTree.Sub(appRoot, appTree.Name("library-tabs"))
	dialogScopeID   = appTree.ID(appRoot, "dialog-scope")
	addScopeID      = appTree.ID(appRoot, "add-scope")
	confirmScopeID  = appTree.ID(appRoot, "confirm-scope")

	contentScroll = appTree.ID(appRoot, "content-scroll")

	queueScroll  = appTree.ID(appRoot, "queue-scroll")
	lyricsScroll = appTree.ID(appRoot, "lyrics-scroll")

	searchFieldID       = appTree.ID(appRoot, "search-field")
	playlistFieldID     = appTree.ID(appRoot, "playlist-field")
	progressID          = appTree.ID(appRoot, "progress")
	overlayProgressID   = appTree.ID(appRoot, "overlay-progress")
	volumeID            = appTree.ID(appRoot, "volume")
	fadeID              = appTree.ID(appRoot, "fade")
	backID              = appTree.ID(appRoot, "back")
	addFolderID         = appTree.ID(appRoot, "add-folder")
	settingsAddFolderID = appTree.ID(appRoot, "settings-add-folder")
	emptyAddFolderID    = appTree.ID(appRoot, "empty-add-folder")
	newPlaylistID       = appTree.ID(appRoot, "new-playlist")
	emptyNewPlaylistID  = appTree.ID(appRoot, "empty-new-playlist")
	addNewPlaylistID    = appTree.ID(appRoot, "add-new-playlist")
	playPauseID         = appTree.ID(appRoot, "play-pause")
	overlayPlayPauseID  = appTree.ID(appRoot, "overlay-play-pause")
	previousID          = appTree.ID(appRoot, "previous")
	overlayPreviousID   = appTree.ID(appRoot, "overlay-previous")
	nextID              = appTree.ID(appRoot, "next")
	overlayNextID       = appTree.ID(appRoot, "overlay-next")
	shuffleID           = appTree.ID(appRoot, "shuffle")
	repeatID            = appTree.ID(appRoot, "repeat")
	settingsVolumeID    = appTree.ID(appRoot, "settings-volume")
	settingsFadeID      = appTree.ID(appRoot, "settings-fade")
	settingsShuffleID   = appTree.ID(appRoot, "settings-shuffle")
	settingsRepeatID    = appTree.ID(appRoot, "settings-repeat")
	muteID              = appTree.ID(appRoot, "mute")
	lyricsID            = appTree.ID(appRoot, "lyrics")
	queueID             = appTree.ID(appRoot, "queue")
	closeOverlayID      = appTree.ID(appRoot, "close-overlay")
	closeQueueID        = appTree.ID(appRoot, "close-queue")
	clearHistoryID      = appTree.ID(appRoot, "clear-history")
	createPlaylistID    = appTree.ID(appRoot, "create-playlist")
	cancelPlaylistID    = appTree.ID(appRoot, "cancel-playlist")
	newPlaylistFieldID  = appTree.ID(appRoot, "new-playlist-field")
	newPlaylistCreateID = appTree.ID(appRoot, "new-playlist-create")
	newPlaylistCancelID = appTree.ID(appRoot, "new-playlist-cancel")
	confirmDeleteID     = appTree.ID(appRoot, "confirm-delete")
	cancelDeleteID      = appTree.ID(appRoot, "cancel-delete")
	dismissMenuID       = appTree.ID(appRoot, "dismiss-menu")
	dismissAddID        = appTree.ID(appRoot, "dismiss-add")

	navIDs = map[page]ui.ID{
		pageDiscover:  appTree.ID(appRoot, "nav-discover"),
		pageSearch:    appTree.ID(appRoot, "nav-search"),
		pageLibrary:   appTree.ID(appRoot, "nav-library"),
		pagePlaylists: appTree.ID(appRoot, "nav-playlists"),
		pageSettings:  appTree.ID(appRoot, "nav-settings"),
	}
)

// rowID names a control that may appear more than once on one page, such as a
// track in both 最近添加 and 常听. The tree interns the path, so a warm page
// builds no new identity after the first paint.
func (m *Model) rowID(name string) ui.ID {
	return m.tree.ID(m.root, name)
}

// config describes the window and connects the model to Viva's event loop.
func (m *Model) config() vivaapp.Config {
	icon, err := iconpng.Decode(assets.IconPNG)
	if err != nil {
		icon = vivaapp.WindowIcon{}
	}
	return vivaapp.Config{
		Title:       "汽水音乐",
		Class:       "soda-music",
		Icon:        icon,
		Width:       1180,
		Height:      760,
		MinWidth:    900,
		MinHeight:   600,
		Tree:        m.tree,
		Paint:       m.paint,
		OnWake:      m.wake,
		OnScroll:    m.view.Wheel,
		OnKey:       m.onKey,
		TextInput:   flex.Fields(&m.search, &m.playlistDraft),
		Invalidator: m.invalidator,
		OnCloseRequest: func(allow func()) {
			m.rememberPlayback()
			m.flush()
			allow()
		},
	}
}

// Run opens the native window. It is kept here rather than in main so the
// application can be driven by a test or by the Viva headless runner.
func Run() error {
	store, err := catalog.OpenStore()
	if err != nil {
		return err
	}
	engine := audioNewEngine()
	invalidator := &vivaapp.Invalidator{}
	m := newModel(store, engine, invalidator, &appTree, appRoot)
	m.systemDark = vivaapp.SystemAppearance() == vivaapp.AppearanceDark
	m.reduceMotion = vivaapp.ReduceMotion()
	stopAppearance := vivaapp.WatchSystemAppearance(func() {
		m.systemDark = vivaapp.SystemAppearance() == vivaapp.AppearanceDark
		m.invalidate()
	})
	m.start()

	config := m.config()
	config.OnClosed = func() {
		stopAppearance()
		m.art.close()
		m.player.Close()
	}
	var canvas ui.Canvas
	return vivaapp.RunCanvas(config, &canvas)
}

// paint is the single place that declares the whole window. It reads state and
// records click actions; the model is never mutated by a worker while painting.
func (m *Model) paint(canvas *ui.Canvas, size ui.Size) {
	m.syncSearch()
	canvas.SetReduceMotion(m.reduceMotion)
	theme := m.theme()
	canvas.Fill(ui.Rect{Width: size.Width, Height: size.Height}, theme.Background)

	m.covers = m.covers[:0]
	m.view.Draw(canvas, theme, ui.Rect{Width: size.Width, Height: size.Height}, func(f *flex.Frame) {
		f.Column().Fill().Children(func() {
			f.Row().Grow(1).FillWidth().Children(func() {
				m.sidebar(f, theme)
				f.Column().Grow(1).FillHeight().Children(func() {
					m.header(f, theme, size)
					m.content(f, theme, size)
				})
			})
			m.playerBar(f, theme, size)
		})
		// Popups are declared after the page so they paint above it. The layer
		// keeps the cover slots, which are painted after Draw, underneath them.
		f.PopupLayer(1)
		m.overlays(f, theme, size)
	})

	for _, slot := range m.covers {
		paintCover(canvas, theme, slot.box.Rect(), slot.bitmap, slot.radius)
	}

	if m.player.Playing() {
		canvas.RequestFrameIn(200 * time.Millisecond)
	}
	if m.toastVisible() != "" {
		canvas.RequestFrameIn(100 * time.Millisecond)
	}
	if m.wantAddFolder {
		m.wantAddFolder = false
		canvas.AfterPaint(func() {
			path, err := vivaapp.OpenDirectory(vivaapp.OpenDirectoryOptions{Title: "选择音乐文件夹"})
			if err != nil || strings.TrimSpace(path) == "" {
				return
			}
			m.addFolder(path)
		})
	}
}

func audioNewEngine() *audio.Engine {
	return audio.New()
}

// sidebar is the persistent navigation column.
func (m *Model) sidebar(f *flex.Frame, theme ui.Theme) {
	f.Column().Width(232).FillHeight().Background(theme.CardElevated).Pad(16).Gap(6).Children(func() {
		f.Row().Items(flex.Center).Gap(10).PadY(8).Children(func() {
			f.Icon(assetsSparkles).Size(24, 24).Color(theme.Primary)
			f.Text("汽水音乐").FontSize(20).Bold()
		})
		f.Divider()
		for _, entry := range navEntries {
			entry := entry
			active := m.page == entry.page
			m.navButton(f, theme, navIDs[entry.page], entry.icon, entry.label, active, func() {
				m.setPage(entry.page)
			})
		}
		f.Text("我的音乐").FontSize(12).Muted().PadY(6)
		m.navButton(f, theme, appTree.ID(appRoot, "nav-favorites"), assetsHeart, "喜欢的歌曲", m.page == pageFavorites, m.openFavorites)
		m.navButton(f, theme, appTree.ID(appRoot, "nav-recent"), assetsClock, "最近播放", m.page == pageRecent, m.openRecent)

		f.Text("我的歌单").FontSize(12).Muted().PadY(6)
		for i, playlist := range m.state.Playlists {
			if i >= 7 {
				break
			}
			playlist := playlist
			m.navButton(f, theme, m.playlistIDOf(playlist.ID), assetsPlaylist, playlist.Name, m.page == pagePlaylist && m.playlistID == playlist.ID, func() {
				m.openPlaylist(playlist.ID)
			})
		}
		f.Spacer()
		f.Divider()
		if m.scanning.Load() {
			f.Text(fmt.Sprintf("正在扫描 %s 首", formatCount(int(m.scanCount.Load())))).FontSize(12).Muted()
		} else {
			f.Text(fmt.Sprintf("曲库 %s 首", formatCount(m.library.Length()))).FontSize(12).Muted()
		}
	})
}

func (m *Model) navButton(f *flex.Frame, theme ui.Theme, id ui.ID, icon ui.Icon, label string, active bool, onClick func()) {
	row := f.Row().Height(36).PadX(10).Gap(10).Items(flex.Center).Radius(8).Hit(id, label)
	if active {
		row = row.Background(theme.Accent)
	} else {
		row = row.HoverBackground(theme.Hover)
	}
	row.Children(func() {
		color := theme.Foreground
		if active {
			color = theme.AccentForeground
		}
		f.Icon(icon).Size(16, 16).Color(color)
		f.Text(label).Grow(1).Ellipsis().Color(color)
	})
	if row.Clicked() {
		onClick()
	}
}

// header is the title strip above the scrolling page.
func (m *Model) header(f *flex.Frame, theme ui.Theme, size ui.Size) {
	f.Row().FillWidth().Height(72).PadSides(24, 24, 24, 24).Items(flex.Center).Gap(12).Background(theme.Background).Children(func() {
		if m.page == pageAlbum || m.page == pageArtist || m.page == pagePlaylist || m.page == pageFavorites || m.page == pageRecent {
			m.iconButton(f, theme, backID, assetsChevronLeft, "返回", 34, false, m.goBack)
		}
		f.Column().Grow(1).Gap(2).Children(func() {
			f.Text(pageTitle(m.page)).FontSize(20).Bold()
			f.Text(m.pageSubtitle()).FontSize(12).Muted().Ellipsis()
		})
		if m.page == pageSettings {
			if f.Button(addFolderID, "添加音乐文件夹").Outline().Clicked() {
				m.wantAddFolder = true
			}
		} else if m.library.Length() > 0 {
			f.Text(fmt.Sprintf("%s 首歌曲", formatCount(m.library.Length()))).FontSize(12).Muted()
		}
	})
}

func (m *Model) content(f *flex.Frame, theme ui.Theme, size ui.Size) {
	f.ScrollArea(contentScroll).Grow(1).FillWidth().Children(func() {
		f.Column().FillWidth().PadSides(24, 24, 24, 24).Gap(18).Children(func() {
			switch m.page {
			case pageDiscover:
				m.pageDiscover(f, theme)
			case pageSearch:
				m.pageSearch(f, theme)
			case pageLibrary:
				m.pageLibrary(f, theme)
			case pagePlaylists:
				m.pagePlaylists(f, theme)
			case pageSettings:
				m.pageSettings(f, theme)
			case pageAlbum:
				m.pageAlbum(f, theme)
			case pageArtist:
				m.pageArtist(f, theme)
			case pagePlaylist:
				m.pagePlaylist(f, theme)
			case pageFavorites:
				m.pageFavorites(f, theme)
			case pageRecent:
				m.pageRecent(f, theme)
			}
		})
	})
}

func pageTitle(p page) string {
	switch p {
	case pageDiscover:
		return "发现"
	case pageSearch:
		return "搜索"
	case pageLibrary:
		return "曲库"
	case pagePlaylists:
		return "歌单"
	case pageSettings:
		return "设置"
	case pageAlbum:
		return "专辑"
	case pageArtist:
		return "艺术家"
	case pagePlaylist:
		return "歌单"
	case pageFavorites:
		return "喜欢的歌曲"
	case pageRecent:
		return "最近添加"
	}
	return "汽水音乐"
}

func (m *Model) pageSubtitle() string {
	switch m.page {
	case pageDiscover:
		return "从你的本地音乐库开始"
	case pageSearch:
		return "搜索歌曲、专辑和艺术家"
	case pageLibrary:
		return "按歌曲、专辑、艺术家和流派浏览"
	case pagePlaylists:
		return "整理属于你的播放列表"
	case pageSettings:
		return "音乐文件夹、外观与播放偏好"
	case pageAlbum:
		if album, ok := m.library.Album(m.albumKey); ok {
			return fmt.Sprintf("%s · %s", album.Artist, formatTotal(album.Duration))
		}
	case pageArtist:
		if artist, ok := m.library.Artist(m.artistName); ok {
			return fmt.Sprintf("%d 张专辑 · %s", len(artist.Albums), formatTotal(artist.Duration))
		}
	case pagePlaylist:
		if playlist, ok := m.playlist(m.playlistID); ok {
			return fmt.Sprintf("%d 首歌曲", len(playlist.TrackIDs))
		}
	case pageFavorites:
		return fmt.Sprintf("%d 首歌曲", len(m.favorites))
	case pageRecent:
		return fmt.Sprintf("%d 首歌曲", len(m.recent))
	}
	return ""
}
