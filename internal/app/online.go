package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/catalog"
	"soda-music/internal/netease"
)

// onlinePrefix namespaces the identity of a song that lives on the service, so
// it can never collide with the identity of a file on disk.
const onlinePrefix = "netease-"

// onlineMaxRemembered caps how many online songs are kept in the state file, so
// a long listening history cannot grow it without bound.
const onlineMaxRemembered = 600

// onlineTrackID is the identity a service song is remembered under.
func onlineTrackID(remoteID int64) string {
	return onlinePrefix + strconv.FormatInt(remoteID, 10)
}

// songTrack turns a service song into the track the rest of the application
// works with. path is the cached file, which may not exist yet.
func songTrack(song netease.Song, path string) catalog.Track {
	return catalog.Track{
		ID:       onlineTrackID(song.ID),
		Path:     path,
		Title:    song.Name,
		Artist:   song.ArtistNames(),
		Album:    song.AlbumName(),
		Duration: song.Duration,
		Format:   strings.TrimPrefix(filepath.Ext(path), "."),
		Art:      song.Cover() != "",
		Source:   "netease",
		RemoteID: song.ID,
		CoverURL: song.Cover(),
	}
}

// displayTrack is the track shape used to draw a song that has not been
// downloaded yet: everything a row needs, but no file behind it.
func displayTrack(song netease.Song) catalog.Track {
	return songTrack(song, "")
}

// onlineID is the identity of a control on the online screens.
func (m *Model) onlineID(key string) ui.ID {
	if cached, ok := m.ids.online[key]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "online:"+key)
	m.ids.online[key] = identity
	return identity
}

// lookupTrack finds a track by identity, whether it is a file in the library or
// a service song the user has played. Playlists and favourites can hold both.
func (m *Model) lookupTrack(id string) (catalog.Track, bool) {
	if track, ok := m.library.Track(id); ok {
		return track, true
	}
	track, ok := m.onlineTracks[id]
	return track, ok
}

// lookupTracks resolves a list of identities, skipping the ones that are gone.
func (m *Model) lookupTracks(ids []string) []catalog.Track {
	out := make([]catalog.Track, 0, len(ids))
	for _, id := range ids {
		if track, ok := m.lookupTrack(id); ok {
			out = append(out, track)
		}
	}
	return out
}

// rememberOnline keeps a service song in memory and in the state file, so a
// playlist or favourite that points at it still resolves on the next run.
func (m *Model) rememberOnline(track catalog.Track) {
	if !track.Online() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onlineTracks == nil {
		m.onlineTracks = make(map[string]catalog.Track)
	}
	m.onlineTracks[track.ID] = track
	for i, existing := range m.state.OnlineTracks {
		if existing.ID == track.ID {
			m.state.OnlineTracks[i] = track
			return
		}
	}
	m.state.OnlineTracks = append(m.state.OnlineTracks, track)
	if len(m.state.OnlineTracks) > onlineMaxRemembered {
		m.state.OnlineTracks = m.state.OnlineTracks[len(m.state.OnlineTracks)-onlineMaxRemembered:]
	}
}

// ---------------------------------------------------------------------------
// Session

// loadOnline fetches the account, the playlists and the daily picks in the
// background.
func (m *Model) loadOnline() {
	if !m.onlineState.SignedIn() || m.onlineLoading {
		return
	}
	m.onlineLoading = true
	m.onlineError = ""
	m.invalidate()
	go func() {
		client := netease.New(m.onlineState.Cookie)
		account, err := client.Account()
		if err != nil {
			m.publishOnline(&onlineFeed{kind: feedLibrary, err: err.Error()})
			return
		}
		if account == nil {
			m.publishOnline(&onlineFeed{kind: feedLibrary, err: "登录状态已过期，请重新扫码"})
			return
		}
		playlists, err := client.UserPlaylists(account.ID, 100)
		if err != nil {
			m.publishOnline(&onlineFeed{kind: feedLibrary, err: err.Error()})
			return
		}
		daily, _ := client.DailySongs()
		m.publishOnline(&onlineFeed{kind: feedLibrary, account: account, playlists: playlists, daily: daily})
	}()
}

// refreshOnline re-fetches the playlists, ignoring the in-flight guard so the
// refresh button always does something.
func (m *Model) refreshOnline() {
	m.onlineLoading = false
	m.loadOnline()
}

// signIn starts a QR login. The code is drawn by the login overlay and polled
// from the paint loop.
func (m *Model) signIn() {
	if m.loginBusy {
		return
	}
	m.loginBusy = true
	m.onlineLogin = nil
	m.loginError = ""
	m.loginQR = ui.Bitmap{}
	m.loginQRKey = ""
	m.showLogin = true
	m.invalidate()
	go func() {
		client := netease.New("")
		login, err := client.QRKey()
		m.mu.Lock()
		m.loginBusy = false
		if err != nil {
			m.loginError = err.Error()
		} else {
			m.loginError = ""
			m.onlineLogin = login
			m.loginNextPoll = time.Now().Add(time.Second)
		}
		m.mu.Unlock()
		m.invalidate()
	}()
}

// pollLogin asks the service whether the code has been scanned, and signs the
// user in once the phone confirms.
func (m *Model) pollLogin() {
	if !m.showLogin {
		return
	}
	m.mu.Lock()
	login := m.onlineLogin
	busy := m.loginBusy
	next := m.loginNextPoll
	m.mu.Unlock()
	if login == nil || busy || m.onlineState.SignedIn() {
		return
	}
	if login.State == netease.QRExpired {
		return
	}
	if time.Now().Before(next) {
		return
	}
	m.mu.Lock()
	m.loginBusy = true
	m.mu.Unlock()
	go func() {
		client := netease.New("")
		next, err := client.QRCheck(login)
		m.mu.Lock()
		m.loginBusy = false
		m.loginNextPoll = time.Now().Add(1500 * time.Millisecond)
		if err != nil {
			m.loginError = err.Error()
		} else {
			m.loginError = ""
			m.onlineLogin = next
		}
		confirmed := err == nil && next.State == netease.QRConfirmed
		m.mu.Unlock()
		if confirmed {
			m.completeLogin(next.Cookie)
		}
		m.invalidate()
	}()
}

// completeLogin stores the cookie the phone handed over and loads the library.
func (m *Model) completeLogin(cookie string) {
	if cookie == "" {
		return
	}
	m.online.SetCookie(cookie)
	m.onlineState.Cookie = cookie
	m.showLogin = false
	m.onlineLogin = nil
	m.toastf("登录成功，正在同步歌单…")
	// The profile is filled in by the playlist fetch that follows.
	go func() {
		client := netease.New(cookie)
		if account, err := client.Account(); err == nil && account != nil {
			m.mu.Lock()
			m.onlineState.UID = account.ID
			m.onlineState.Nickname = account.Nickname
			m.onlineState.Avatar = account.Avatar
			session := m.onlineState
			m.mu.Unlock()
			_ = netease.SaveSession(m.store.Dir, session)
			m.invalidate()
		}
	}()
	m.loadOnline()
}

// signOut forgets the session and clears everything that came with it.
func (m *Model) signOut() {
	m.online.SetCookie("")
	m.onlineState = netease.Session{}
	m.onlinePlaylists = nil
	m.onlineDaily = nil
	m.onlineSongs = nil
	m.onlineSearch = nil
	m.onlineSearchKey = ""
	m.onlineError = ""
	m.showLogin = false
	_ = netease.ClearSession(m.store.Dir)
	m.toastf("已退出网易云音乐")
}

// cancelLogin closes the sign-in dialog and forgets the pending code.
func (m *Model) cancelLogin() {
	m.showLogin = false
	m.onlineLogin = nil
	m.loginError = ""
	m.loginQR = ui.Bitmap{}
	m.loginQRKey = ""
}

// loginBitmap decodes the QR code of the current attempt, caching the result so
// it is only decoded when a new code arrives.
func (m *Model) loginBitmap(login *netease.QRLogin) ui.Bitmap {
	if login == nil || len(login.Image) == 0 {
		return ui.Bitmap{}
	}
	if m.loginQRKey == login.Key && m.loginQR.Width > 0 {
		return m.loginQR
	}
	bitmap, err := decodeBitmap(login.Image)
	if err != nil {
		return ui.Bitmap{}
	}
	m.loginQRKey = login.Key
	m.loginQR = bitmap
	return bitmap
}

// ---------------------------------------------------------------------------
// Feeds

// onlineKind says which fetch a feed answers, so a slow reply cannot overwrite
// a newer one of a different kind.
type onlineKind int

const (
	feedLibrary onlineKind = iota
	feedDetail
	feedSearch
)

// onlineFeed is what a background fetch produced, handed to the window thread
// through mu.
type onlineFeed struct {
	kind      onlineKind
	account   *netease.Account
	playlists []netease.Playlist
	daily     []netease.Song
	detail    *netease.PlaylistDetail
	search    []netease.Song
	searchKey string
	err       string
}

// publishOnline hands a finished fetch to the window thread.
func (m *Model) publishOnline(feed *onlineFeed) {
	m.mu.Lock()
	m.onlineFeed = feed
	m.mu.Unlock()
	m.invalidate()
}

// applyOnlineFeed moves a finished fetch into the model. It runs on the window
// thread, from wake.
func (m *Model) applyOnlineFeed() {
	m.mu.Lock()
	feed := m.onlineFeed
	m.onlineFeed = nil
	m.mu.Unlock()
	if feed == nil {
		return
	}
	switch feed.kind {
	case feedLibrary:
		m.onlineLoading = false
		if feed.err != "" {
			m.onlineError = feed.err
			return
		}
		m.onlineError = ""
		if feed.account != nil {
			m.onlineState.UID = feed.account.ID
			m.onlineState.Nickname = feed.account.Nickname
			m.onlineState.Avatar = feed.account.Avatar
			_ = netease.SaveSession(m.store.Dir, m.onlineState)
		}
		m.onlinePlaylists = feed.playlists
		m.onlineDaily = feed.daily
	case feedDetail:
		m.onlineLoading = false
		if feed.err != "" {
			m.onlineError = feed.err
			m.toastf("%s", feed.err)
			return
		}
		if feed.detail != nil {
			m.onlineSongs = feed.detail.Tracks
		}
	case feedSearch:
		m.onlineSearching = false
		if feed.err != "" {
			m.onlineError = feed.err
			return
		}
		m.onlineSearchKey = feed.searchKey
		m.onlineSearch = feed.search
	}
}

// openOnlinePlaylist loads a playlist's songs in the background.
func (m *Model) openOnlinePlaylist(id int64, name string) {
	m.backPage = m.page
	m.onlinePlaylistID = id
	m.onlinePlaylistName = name
	m.onlineSongs = nil
	m.onlineLoading = true
	m.page = pageOnlinePlaylist
	m.invalidate()
	go func() {
		client := netease.New(m.onlineState.Cookie)
		detail, err := client.PlaylistDetail(id)
		if err != nil {
			m.publishOnline(&onlineFeed{kind: feedDetail, err: err.Error()})
			return
		}
		m.publishOnline(&onlineFeed{kind: feedDetail, detail: detail})
	}()
}

// searchOnline looks for songs on the service.
func (m *Model) searchOnline(keyword string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		m.onlineSearch = nil
		m.onlineSearchKey = ""
		return
	}
	if !m.onlineState.SignedIn() {
		m.toastf("请先登录网易云音乐")
		m.showLogin = true
		return
	}
	m.onlineSearching = true
	m.invalidate()
	go func() {
		client := netease.New(m.onlineState.Cookie)
		songs, err := client.Search(keyword, 1, 40)
		if err != nil {
			m.publishOnline(&onlineFeed{kind: feedSearch, err: err.Error(), searchKey: keyword})
			return
		}
		if len(songs) > 0 {
			if detailed, err := client.SongDetail(songIDs(songs)); err == nil && len(detailed) > 0 {
				songs = detailed
			}
		}
		m.publishOnline(&onlineFeed{kind: feedSearch, search: songs, searchKey: keyword})
	}()
}

func songIDs(songs []netease.Song) []int64 {
	ids := make([]int64, 0, len(songs))
	for _, song := range songs {
		ids = append(ids, song.ID)
	}
	return ids
}

// ---------------------------------------------------------------------------
// Playback

// playOnline caches the chosen song and starts it, then caches the rest of the
// list in the background and queues them as they arrive.
func (m *Model) playOnline(songs []netease.Song, index int) {
	if len(songs) == 0 || index < 0 || index >= len(songs) {
		return
	}
	if !m.onlineState.SignedIn() {
		m.toastf("请先登录网易云音乐")
		m.showLogin = true
		return
	}
	selected := songs[index]
	m.toastf("正在缓存《%s》…", selected.Name)
	order := make([]int, 0, len(songs))
	for i := index; i < len(songs); i++ {
		order = append(order, i)
	}
	for i := 0; i < index; i++ {
		order = append(order, i)
	}
	go func() {
		client := netease.New(m.onlineState.Cookie)
		urls, err := client.SongURLs(songIDs(songs), "exhigh")
		if err != nil {
			m.toastf("无法播放：%v", err)
			m.invalidate()
			return
		}
		first := true
		for _, at := range order {
			song := songs[at]
			url, ok := urls[song.ID]
			if !ok {
				continue
			}
			track, err := m.cacheSong(client, song, url)
			if err != nil {
				continue
			}
			if first {
				first = false
				m.postPending(pendingAction{replace: true, tracks: []catalog.Track{track}, index: 0})
			} else {
				m.postPending(pendingAction{tracks: []catalog.Track{track}})
			}
		}
		if first {
			m.toastf("这些歌曲暂时无法播放，可能需要会员")
			m.invalidate()
		}
	}()
}

// cacheSong downloads a song into the cache, along with its cover, and returns
// the track the player can open. A song already in the cache is not fetched
// again.
func (m *Model) cacheSong(client *netease.Client, song netease.Song, url string) (catalog.Track, error) {
	extension := netease.Extension(url)
	path := m.store.OnlinePath(song.ID, extension)
	if _, err := os.Stat(path); err != nil {
		if err := netease.Download(url, path); err != nil {
			return catalog.Track{}, err
		}
	}
	track := songTrack(song, path)
	m.cacheCover(track)
	m.rememberOnline(track)
	return track, nil
}

// cacheCover writes the album picture beside the other cached art, so the
// picture survives a restart without another request.
func (m *Model) cacheCover(track catalog.Track) {
	if track.CoverURL == "" {
		return
	}
	path := m.store.ArtPath(track.ID)
	if _, err := os.Stat(path); err == nil {
		return
	}
	raw, err := netease.Fetch(track.CoverURL)
	if err != nil {
		return
	}
	_ = os.MkdirAll(m.store.ArtDir(), 0o755)
	_ = os.WriteFile(path, raw, 0o644)
}

// pendingAction is a queue change a background download asked for.
type pendingAction struct {
	replace bool
	tracks  []catalog.Track
	index   int
}

// postPending hands a queue change to the window thread.
func (m *Model) postPending(action pendingAction) {
	m.mu.Lock()
	m.pending = append(m.pending, action)
	m.mu.Unlock()
	m.invalidate()
}

// takeActions drains the queue changes the window thread has not applied yet.
func (m *Model) takeActions() []pendingAction {
	m.mu.Lock()
	defer m.mu.Unlock()
	actions := m.pending
	m.pending = nil
	return actions
}

// applyActions runs the queue changes on the window thread.
func (m *Model) applyActions() {
	for _, action := range m.takeActions() {
		if action.replace {
			m.playTracks(action.tracks, action.index)
			continue
		}
		if err := m.player.Enqueue(action.tracks, false); err != nil {
			continue
		}
		m.queue = m.player.Queue()
	}
}

// ---------------------------------------------------------------------------
// Pages

func (m *Model) pageOnline(f *flex.Frame, theme ui.Theme) {
	if !m.onlineState.SignedIn() {
		m.onlineSignInPrompt(f, theme)
		return
	}
	m.onlineHeader(f, theme)
	field := f.Field(&m.onlineQuery, m.onlineID("search-field"), "搜索在线音乐", "搜索网易云音乐上的歌曲")
	if field.Submitted() {
		m.searchOnline(m.onlineQuery.Text())
	}
	if strings.TrimSpace(m.onlineQuery.Text()) != "" {
		m.onlineSearchResults(f, theme)
		return
	}
	if m.onlineLoading {
		f.Text("正在同步你的歌单…").Muted().PadY(16)
	}
	if m.onlineError != "" {
		f.Text(m.onlineError).Muted().PadY(8)
	}
	if len(m.onlinePlaylists) > 0 {
		m.sectionTitle(f, "我的歌单", fmt.Sprintf("%d 个歌单", len(m.onlinePlaylists)))
		f.Grid(180).Gap(16).Children(func() {
			for _, playlist := range m.onlinePlaylists {
				m.onlinePlaylistCard(f, theme, playlist)
			}
		})
	}
	if len(m.onlineDaily) > 0 {
		m.sectionTitle(f, "每日推荐", "根据你的口味挑选")
		m.onlineSongList(f, theme, m.onlineDaily, "daily")
	}
	if !m.onlineLoading && len(m.onlinePlaylists) == 0 && len(m.onlineDaily) == 0 && m.onlineError == "" {
		f.Text("这个账号还没有歌单。").Muted().PadY(16)
	}
}

func (m *Model) onlineSignInPrompt(f *flex.Frame, theme ui.Theme) {
	f.Column().FillWidth().MinHeight(320).Center().Gap(14).Children(func() {
		f.Row().Size(72, 72).Items(flex.Center).Justify(flex.Center).Radius(36).Background(theme.Accent).Children(func() {
			f.Icon(assetsCloud).Size(32, 32).Color(theme.Primary)
		})
		f.Text("登录网易云音乐").FontSize(22).Bold()
		f.Text("扫码登录后，就能在这里浏览你的歌单、每日推荐，并直接播放。").Muted().TextCenter()
		if f.Button(m.onlineID("sign-in"), "扫码登录").Clicked() {
			m.signIn()
		}
		f.Text("登录凭据只保存在这台电脑上。").FontSize(12).Muted()
	})
}

func (m *Model) onlineHeader(f *flex.Frame, theme ui.Theme) {
	f.Row().FillWidth().Items(flex.Center).Gap(12).Children(func() {
		if bitmap := m.images.Bitmap(m.onlineState.Avatar); bitmap.Width > 0 {
			m.imageBox(f, bitmap, 40, 20)
		} else {
			f.Row().Size(40, 40).Items(flex.Center).Justify(flex.Center).Radius(20).Background(theme.Accent).Children(func() {
				f.Icon(assetsCloud).Size(20, 20).Color(theme.Primary)
			})
		}
		f.Column().Grow(1).Gap(2).Children(func() {
			name := m.onlineState.Nickname
			if name == "" {
				name = "已登录"
			}
			f.Text(name).Bold()
			f.Text("网易云音乐").FontSize(12).Muted()
		})
		m.iconButton(f, theme, m.onlineID("refresh"), assetsRefresh, "刷新", 34, false, m.refreshOnline)
		m.iconButton(f, theme, m.onlineID("sign-out"), assetsLogout, "退出登录", 34, false, m.signOut)
	})
}

func (m *Model) onlinePlaylistCard(f *flex.Frame, theme ui.Theme, playlist netease.Playlist) {
	card := f.Column().Width(168).Gap(8).Hit(m.onlineID("playlist:"+strconv.FormatInt(playlist.ID, 10)), playlist.Name)
	card.Children(func() {
		m.imageBox(f, m.images.Bitmap(playlist.CoverURL), 168, 12)
		f.Text(playlist.Name).Bold().Ellipsis()
		f.Text(fmt.Sprintf("%d 首歌曲", playlist.TrackCount)).FontSize(12).Muted()
	})
	if card.Clicked() {
		m.openOnlinePlaylist(playlist.ID, playlist.Name)
	}
}

func (m *Model) onlineSearchResults(f *flex.Frame, theme ui.Theme) {
	if m.onlineSearching {
		f.Text("正在搜索…").Muted().PadY(16)
		return
	}
	if len(m.onlineSearch) == 0 {
		f.Text(fmt.Sprintf("没有找到与“%s”相关的歌曲。", strings.TrimSpace(m.onlineQuery.Text()))).Muted().PadY(16)
		return
	}
	m.sectionTitle(f, "搜索结果", fmt.Sprintf("%d 个结果", len(m.onlineSearch)))
	m.onlineSongList(f, theme, m.onlineSearch, "search")
}

func (m *Model) pageOnlinePlaylist(f *flex.Frame, theme ui.Theme) {
	f.Row().FillWidth().Items(flex.Center).Gap(16).Children(func() {
		f.Column().Grow(1).Gap(4).Children(func() {
			f.Text(m.onlinePlaylistName).FontSize(24).Bold().Ellipsis()
			f.Text(fmt.Sprintf("%d 首歌曲 · 来自网易云音乐", len(m.onlineSongs))).Muted()
		})
		if len(m.onlineSongs) > 0 {
			if f.Button(m.onlineID("play-all"), "播放全部").Clicked() {
				m.playOnline(m.onlineSongs, 0)
			}
		}
	})
	if m.onlineLoading {
		f.Text("正在加载歌单…").Muted().PadY(16)
		return
	}
	if len(m.onlineSongs) == 0 {
		f.Text("这个歌单还没有歌曲。").Muted().PadY(16)
		return
	}
	m.onlineSongList(f, theme, m.onlineSongs, "playlist")
}

// onlineSongList draws the songs of a playlist, the daily picks or a search
// result. context keeps the row identities apart when two lists share a page.
func (m *Model) onlineSongList(f *flex.Frame, theme ui.Theme, songs []netease.Song, context string) {
	f.Column().FillWidth().Gap(2).Children(func() {
		for index, song := range songs {
			song := song
			m.onlineSongRow(f, theme, song, index, songs, context)
		}
	})
}

func (m *Model) onlineSongRow(f *flex.Frame, theme ui.Theme, song netease.Song, index int, songs []netease.Song, context string) {
	key := context + ":" + strconv.FormatInt(song.ID, 10)
	row := f.Row().Height(56).FillWidth().PadX(10).Gap(10).Items(flex.Center).Radius(8).HoverBackground(theme.Hover).Hit(m.onlineID("song:"+key), song.Name)
	row.Children(func() {
		f.Text(fmt.Sprintf("%d", index+1)).Width(24).FontSize(12).Muted().TextEnd()
		m.imageBox(f, m.images.Bitmap(song.Cover()), 40, 7)
		f.Column().Grow(1).Gap(2).Children(func() {
			f.Text(song.Name).Ellipsis().Bold()
			f.Text(song.ArtistNames()).FontSize(12).Muted().Ellipsis()
		})
		f.Text(song.AlbumName()).Width(150).FontSize(12).Muted().Ellipsis()
		f.Text(formatDuration(song.Duration)).Width(52).FontSize(12).Muted().TextEnd()

		trackID := onlineTrackID(song.ID)
		fav := f.Row().Size(30, 30).Items(flex.Center).Justify(flex.Center).Radius(15).HoverBackground(theme.Hover).Hit(m.favID(trackID), "喜欢")
		fav.Children(func() {
			if m.isFavorite(trackID) {
				f.Icon(assetsHeartFilled).Size(16, 16).Color(theme.Primary)
			} else {
				f.Icon(assetsHeart).Size(16, 16).Color(theme.MutedForeground)
			}
		})
		if fav.Clicked() {
			m.rememberOnline(displayTrack(song))
			m.toggleFavorite(trackID)
		}

		add := f.Row().Size(30, 30).Items(flex.Center).Justify(flex.Center).Radius(15).HoverBackground(theme.Hover).Hit(m.addID(trackID), "加入歌单")
		add.Children(func() {
			f.Icon(assetsMore).Size(16, 16).Color(theme.MutedForeground)
		})
		if add.Clicked() {
			m.rememberOnline(displayTrack(song))
			m.addTrackID = trackID
		}

		cache := f.Row().Size(30, 30).Items(flex.Center).Justify(flex.Center).Radius(15).HoverBackground(theme.Hover).Hit(m.onlineID("cache:"+key), "缓存到本地")
		cache.Children(func() {
			f.Icon(assetsDownload).Size(16, 16).Color(theme.MutedForeground)
		})
		if cache.Clicked() {
			m.cacheOnlineSong(song)
		}
	})
	if row.Clicked() {
		m.playOnline(songs, index)
	}
}

// cacheOnlineSong downloads one song without playing it.
func (m *Model) cacheOnlineSong(song netease.Song) {
	if !m.onlineState.SignedIn() {
		m.toastf("请先登录网易云音乐")
		return
	}
	m.toastf("正在缓存《%s》…", song.Name)
	go func() {
		client := netease.New(m.onlineState.Cookie)
		urls, err := client.SongURLs([]int64{song.ID}, "exhigh")
		if err != nil {
			m.toastf("缓存失败：%v", err)
			m.invalidate()
			return
		}
		url, ok := urls[song.ID]
		if !ok {
			m.toastf("这首歌暂时无法缓存")
			m.invalidate()
			return
		}
		if _, err := m.cacheSong(client, song, url); err != nil {
			m.toastf("缓存失败：%v", err)
			m.invalidate()
			return
		}
		m.toastf("《%s》已缓存", song.Name)
		m.invalidate()
	}()
}

// ---------------------------------------------------------------------------
// Playing a remembered service song

// playTrackList plays a list that may mix local files and service songs. A local
// track plays straight away; a song whose cached file is gone is fetched again
// first, then started on the window thread.
func (m *Model) playTrackList(tracks []catalog.Track, index int) {
	if index < 0 || index >= len(tracks) {
		return
	}
	track := tracks[index]
	if !track.Online() {
		m.playTracks(tracks, index)
		return
	}
	if track.Path != "" {
		if _, err := os.Stat(track.Path); err == nil {
			m.playTracks(tracks, index)
			return
		}
	}
	m.fetchOnline(track)
}

// fetchOnline re-downloads a remembered service song whose cached file is gone,
// then starts it.
func (m *Model) fetchOnline(track catalog.Track) {
	if !m.onlineState.SignedIn() {
		m.toastf("请先登录网易云音乐")
		m.showLogin = true
		return
	}
	m.toastf("正在缓存《%s》…", track.DisplayTitle())
	go func() {
		client := netease.New(m.onlineState.Cookie)
		urls, err := client.SongURLs([]int64{track.RemoteID}, "exhigh")
		if err != nil {
			m.toastf("无法播放：%v", err)
			m.invalidate()
			return
		}
		url, ok := urls[track.RemoteID]
		if !ok {
			m.toastf("这首歌暂时无法播放，可能需要会员")
			m.invalidate()
			return
		}
		cached, err := m.cacheSong(client, songFromTrack(track), url)
		if err != nil {
			m.toastf("无法播放：%v", err)
			m.invalidate()
			return
		}
		m.postPending(pendingAction{replace: true, tracks: []catalog.Track{cached}, index: 0})
	}()
}

// songFromTrack rebuilds the service shape of a remembered song, so it can go
// through the same download path as a song that was just fetched.
func songFromTrack(track catalog.Track) netease.Song {
	song := netease.Song{ID: track.RemoteID, Name: track.Title, Duration: track.Duration}
	song.Album = netease.Album{Name: track.Album, CoverURL: track.CoverURL}
	if track.Artist != "" {
		song.Artists = []netease.Artist{{Name: track.Artist}}
	}
	return song
}
