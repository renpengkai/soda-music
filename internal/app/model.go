package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/audio"
	"soda-music/internal/catalog"
	"soda-music/internal/netease"
	"soda-music/internal/player"
)

// page is which screen the content area is showing.
type page int

const (
	pageDiscover page = iota
	pageSearch
	pageLibrary
	pagePlaylists
	pageSettings
	pageAlbum
	pageArtist
	pagePlaylist
	pageFavorites
	pageRecent
	pageOnline
	pageOnlinePlaylist
)

// libraryTab is the section of the 曲库 screen.
type libraryTab int

const (
	tabTracks libraryTab = iota
	tabAlbums
	tabArtists
	tabGenres
)

// navEntry is one item of the sidebar.
type navEntry struct {
	page  page
	label string
	icon  ui.Icon
}

var navEntries = []navEntry{
	{pageDiscover, "发现", assetsDiscover},
	{pageOnline, "在线音乐", assetsCloud},
	{pageSearch, "搜索", assetsSearch},
	{pageLibrary, "曲库", assetsLibrary},
	{pagePlaylists, "歌单", assetsPlaylist},
	{pageSettings, "设置", assetsSettings},
}

// Model is everything the application remembers while it runs. The paint
// function reads it and the click handlers in that same paint change it; the
// scan worker and the art worker publish into it under mu and ask for a repaint.
type Model struct {
	store  *catalog.Store
	state  catalog.State
	engine *audio.Engine
	player *player.Player
	art    *artCache
	images *imageCache

	online      *netease.Client
	onlineState netease.Session
	onlineLogin *netease.QRLogin
	showLogin   bool
	loginBusy   bool

	// loginQR caches the decoded QR code of the current attempt, so it is only
	// decoded when a new code arrives.
	loginQR    ui.Bitmap
	loginQRKey string

	onlinePlaylists    []netease.Playlist
	onlineDaily        []netease.Song
	onlineSongs        []netease.Song
	onlinePlaylistID   int64
	onlinePlaylistName string
	onlineLoading      bool
	onlineError        string

	onlineQuery     flex.Field
	onlineSearchKey string
	onlineSearch    []netease.Song
	onlineSearching bool

	// onlineFeed is a finished background fetch waiting for the window thread
	// to move it into the model; pending holds the queue changes a download
	// asked for. Both are guarded by mu.
	onlineFeed    *onlineFeed
	pending       []pendingAction
	loginError    string
	loginNextPoll time.Time

	// onlineTracks remembers every service song the user has touched, so a
	// playlist or a favourite that points at one still resolves after a restart.
	onlineTracks map[string]catalog.Track

	invalidator *app.Invalidator
	tree        *ui.Tree
	root        ui.Scope

	view flex.View

	search        flex.Field
	playlistDraft flex.Field

	page       page
	backPage   page
	libraryTab libraryTab
	albumKey   string
	artistName string
	playlistID string

	showNowPlaying bool
	showQueue      bool
	showNewList    bool

	lyricIndex int
	lyricTrack string

	progress        flex.SliderState
	overlayProgress flex.SliderState
	volume          flex.SliderState
	barVolume       flex.SliderState
	fade            flex.SliderState

	addTrackID    string
	menuTrackID   string
	confirmDelete string
	wantAddFolder bool

	library *catalog.Library

	tracks     []catalog.Track
	recent     []catalog.Track
	mostPlayed []catalog.Track
	favorites  []catalog.Track
	history    []catalog.Track
	queue      []catalog.Track

	searchQuery   string
	searchTracks  []catalog.Track
	searchAlbums  []catalog.Album
	searchArtists []catalog.Artist

	systemDark   bool
	reduceMotion bool

	toast      string
	toastUntil time.Time

	scanning  atomic.Bool
	scanCount atomic.Int64

	covers     []coverSlot
	imageSlots []imageSlot
	ids        idCache

	mu             sync.Mutex
	scanned        *catalog.Library
	scannedFolders []string
	dirty          bool
}

// coverSlot is a place reserved for a cover picture. Rects are only known after
// the page has been laid out, so covers are painted once End has run.
type coverSlot struct {
	box    flex.Box
	bitmap ui.Bitmap
	radius float32
}

// imageSlot is a place reserved for a picture that is not tied to a local file,
// such as an online cover or the login QR code.
type imageSlot struct {
	box    flex.Box
	bitmap ui.Bitmap
	radius float32
}

// idCache keeps the identity of each row, so a list does not build a new name
// string on every paint.
type idCache struct {
	tracks    map[string]ui.ID
	albums    map[string]ui.ID
	artists   map[string]ui.ID
	playlists map[string]ui.ID
	cards     map[string]ui.ID
	genres    map[string]ui.ID
	favs      map[string]ui.ID
	adds      map[string]ui.ID
	picks     map[string]ui.ID
	queues    map[string]ui.ID
	removes   map[string]ui.ID
	online    map[string]ui.ID
}

func newModel(store *catalog.Store, engine *audio.Engine, invalidator *app.Invalidator, tree *ui.Tree, root ui.Scope) *Model {
	m := &Model{
		store:       store,
		state:       store.LoadState(),
		engine:      engine,
		invalidator: invalidator,
		tree:        tree,
		root:        root,
	}
	m.player = player.New(engine)
	m.art = newArtCache(store, m.invalidate)
	m.images = newImageCache(m.invalidate)
	m.onlineState = netease.LoadSession(store.Dir)
	m.online = netease.New(m.onlineState.Cookie)
	m.ids = idCache{
		tracks:    make(map[string]ui.ID),
		albums:    make(map[string]ui.ID),
		artists:   make(map[string]ui.ID),
		playlists: make(map[string]ui.ID),
		cards:     make(map[string]ui.ID),
		genres:    make(map[string]ui.ID),
		favs:      make(map[string]ui.ID),
		adds:      make(map[string]ui.ID),
		picks:     make(map[string]ui.ID),
		queues:    make(map[string]ui.ID),
		removes:   make(map[string]ui.ID),
		online:    make(map[string]ui.ID),
	}
	m.library = catalog.New(nil)
	m.applyState()
	m.onlineTracks = make(map[string]catalog.Track, len(m.state.OnlineTracks))
	for _, track := range m.state.OnlineTracks {
		m.onlineTracks[track.ID] = track
	}
	return m
}

// invalidate asks the window for a repaint, from any goroutine.
func (m *Model) invalidate() {
	if m.invalidator != nil {
		m.invalidator.Invalidate()
	}
}

// applyState puts the saved preferences onto the player.
func (m *Model) applyState() {
	m.player.SetVolume(m.state.Volume)
	m.player.SetRepeat(player.RepeatMode(m.state.Repeat))
	m.player.SetShuffle(m.state.Shuffle)
	m.player.SetFade(m.state.FadeMs)
	m.volume.Value = float32(m.state.Volume)
	m.fade.Value = float32(m.state.FadeMs)
}

// start loads the cached library and then rescans the folders in the
// background.
func (m *Model) start() {
	m.art.start()
	m.images.start()
	folders, tracks, err := m.store.LoadLibrary()
	if err == nil && len(tracks) > 0 {
		m.setLibrary(catalog.New(tracks), folders)
	}
	if len(m.state.Folders) == 0 && len(folders) > 0 {
		m.state.Folders = folders
	}
	if len(m.state.Folders) > 0 {
		m.rescan()
	}
	m.restoreLast()
	if m.onlineState.SignedIn() {
		m.loadOnline()
	}
}

// setLibrary replaces the library and everything derived from it.
func (m *Model) setLibrary(library *catalog.Library, folders []string) {
	m.library = library
	if len(folders) > 0 {
		m.state.Folders = append([]string(nil), folders...)
	}
	m.art.Register(library.Tracks)
	m.refresh()
}

// refresh rebuilds the lists the screens show. It runs when the library or the
// remembered state changes, never while painting a list.
func (m *Model) refresh() {
	m.tracks = catalog.SortTracksByTitle(m.library.Tracks)

	recent := append([]catalog.Track(nil), m.library.Tracks...)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].Added.After(recent[j].Added) })
	if len(recent) > 12 {
		recent = recent[:12]
	}
	m.recent = recent

	played := make([]catalog.Track, 0, len(m.library.Tracks))
	for _, track := range m.library.Tracks {
		if m.state.PlayCounts[track.ID] > 0 {
			played = append(played, track)
		}
	}
	sort.SliceStable(played, func(i, j int) bool {
		left, right := m.state.PlayCounts[played[i].ID], m.state.PlayCounts[played[j].ID]
		if left != right {
			return left > right
		}
		return played[i].Added.After(played[j].Added)
	})
	if len(played) > 12 {
		played = played[:12]
	}
	m.mostPlayed = played

	m.favorites = m.lookupTracks(m.state.Favorites)
	m.history = m.lookupTracks(m.state.History)
	m.queue = m.player.Queue()
	m.rebuildSearch()
}

// rebuildSearch recomputes the search results for the current query.
func (m *Model) rebuildSearch() {
	query := strings.TrimSpace(m.search.Text())
	m.searchQuery = query
	if query == "" {
		m.searchTracks, m.searchAlbums, m.searchArtists = nil, nil, nil
		return
	}
	tracks, albums, artists := m.library.Search(query, 200)
	m.searchTracks, m.searchAlbums, m.searchArtists = tracks, albums, artists
}

// syncSearch recomputes the results when the field has changed since the last
// paint.
func (m *Model) syncSearch() {
	if strings.TrimSpace(m.search.Text()) != m.searchQuery {
		m.rebuildSearch()
	}
}

// rescan walks the music folders in the background. Progress and the finished
// library are published under mu; OnWake applies them on the window thread.
func (m *Model) rescan() {
	if !m.scanning.CompareAndSwap(false, true) {
		return
	}
	folders := append([]string(nil), m.state.Folders...)
	previous := m.library
	m.scanCount.Store(0)
	m.invalidate()
	go func() {
		tracks := catalog.Scan(folders, previous, func(done int) {
			m.scanCount.Store(int64(done))
			m.invalidate()
		})
		library := catalog.New(tracks)
		m.mu.Lock()
		m.scanned = library
		m.scannedFolders = folders
		m.mu.Unlock()
		_ = m.store.SaveLibrary(folders, tracks)
		m.invalidate()
	}()
}

// wake runs on the window thread before a repaint that something asked for. It
// moves what the workers produced into the model.
func (m *Model) wake() {
	m.mu.Lock()
	scanned, folders := m.scanned, m.scannedFolders
	m.scanned, m.scannedFolders = nil, nil
	m.mu.Unlock()
	if scanned != nil {
		m.scanning.Store(false)
		m.setLibrary(scanned, folders)
		m.toastf("已扫描 %s 首歌曲", formatCount(len(scanned.Tracks)))
	}
	// A finished online fetch and the queue changes a download asked for are
	// applied here, on the window thread, so the player is never touched from a
	// worker. The sign-in poll also runs here, so its result lands on the same
	// thread that draws it.
	m.applyOnlineFeed()
	m.applyActions()
	m.pollLogin()
	// Tick advances the queue when a track has finished. It runs here, on the
	// window thread, so Paint only reads the player's state.
	if m.player.Tick() {
		m.queue = m.player.Queue()
		m.afterPlayStart()
	}
}

// saveState writes the remembered state, marking it clean.
func (m *Model) saveState() {
	m.mu.Lock()
	m.dirty = false
	state := m.state
	m.mu.Unlock()
	_ = m.store.SaveState(state)
}

// touch marks the state as needing a write. The write happens when playback
// changes track, when a setting changes, and when the window closes, so a
// continuously dragged slider does not write on every frame.
func (m *Model) touch() {
	m.mu.Lock()
	m.dirty = true
	m.mu.Unlock()
}

// flush writes the state if anything changed since the last write.
func (m *Model) flush() {
	m.mu.Lock()
	dirty := m.dirty
	m.mu.Unlock()
	if dirty {
		m.saveState()
	}
}

// toastf shows a short message over the player bar.
func (m *Model) toastf(format string, args ...any) {
	m.toast = fmt.Sprintf(format, args...)
	m.toastUntil = time.Now().Add(3 * time.Second)
}

// toastVisible is the message to draw, if one is still current.
func (m *Model) toastVisible() string {
	if m.toast == "" || time.Now().After(m.toastUntil) {
		return ""
	}
	return m.toast
}

// theme is the active theme.
func (m *Model) theme() ui.Theme {
	return themeFor(m.state.Theme, m.systemDark)
}

// trackID is the identity of a track's row.
func (m *Model) trackID(id string) ui.ID {
	if cached, ok := m.ids.tracks[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "track:"+id)
	m.ids.tracks[id] = identity
	return identity
}

func (m *Model) albumID(key string) ui.ID {
	if cached, ok := m.ids.albums[key]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "album:"+key)
	m.ids.albums[key] = identity
	return identity
}

func (m *Model) artistID(name string) ui.ID {
	if cached, ok := m.ids.artists[name]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "artist:"+name)
	m.ids.artists[name] = identity
	return identity
}

func (m *Model) playlistIDOf(id string) ui.ID {
	if cached, ok := m.ids.playlists[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "playlist:"+id)
	m.ids.playlists[id] = identity
	return identity
}

// cardID is the identity of a playlist's card on the 歌单 screen. It is a
// different name from the sidebar's row for the same playlist, because both are
// on screen at once.
func (m *Model) cardID(id string) ui.ID {
	if cached, ok := m.ids.cards[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "playlist-card:"+id)
	m.ids.cards[id] = identity
	return identity
}

func (m *Model) genreID(name string) ui.ID {
	if cached, ok := m.ids.genres[name]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "genre:"+name)
	m.ids.genres[name] = identity
	return identity
}

// favID is the identity of the heart button on a track's row.
func (m *Model) favID(id string) ui.ID {
	if cached, ok := m.ids.favs[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "fav:"+id)
	m.ids.favs[id] = identity
	return identity
}

// addID is the identity of the add-to-playlist button on a track's row.
func (m *Model) addID(id string) ui.ID {
	if cached, ok := m.ids.adds[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "add:"+id)
	m.ids.adds[id] = identity
	return identity
}

// pickID is the identity of a playlist's row inside the add-to-playlist dialog.
func (m *Model) pickID(id string) ui.ID {
	if cached, ok := m.ids.picks[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "pick:"+id)
	m.ids.picks[id] = identity
	return identity
}

// queueID is the identity of a track's row inside the queue panel.
func (m *Model) queueID(id string) ui.ID {
	if cached, ok := m.ids.queues[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "queue:"+id)
	m.ids.queues[id] = identity
	return identity
}

// removeID is the identity of the remove button on a playlist's own page.
func (m *Model) removeID(id string) ui.ID {
	if cached, ok := m.ids.removes[id]; ok {
		return cached
	}
	identity := m.tree.ID(m.root, "remove:"+id)
	m.ids.removes[id] = identity
	return identity
}

// restoreLast puts the last played track back in the player, paused, at the
// position it was left.
func (m *Model) restoreLast() {
	if m.state.LastTrack == "" || m.library == nil {
		return
	}
	tracks := m.library.Tracks
	index := -1
	for i, track := range tracks {
		if track.ID == m.state.LastTrack {
			index = i
			break
		}
	}
	if index < 0 {
		// The last track may have been a service song that was cached on disk.
		if track, ok := m.onlineTracks[m.state.LastTrack]; ok && track.Path != "" {
			if _, err := os.Stat(track.Path); err == nil {
				m.player.Restore([]catalog.Track{track}, 0, m.state.LastPosition)
			}
		}
		return
	}
	// The queue is the whole library in album order, so "next" goes somewhere
	// sensible before the user has chosen anything.
	queue := catalog.SortTracksByAlbum(tracks)
	at := 0
	for i, track := range queue {
		if track.ID == m.state.LastTrack {
			at = i
			break
		}
	}
	m.player.Restore(queue, at, m.state.LastPosition)
}

// rememberPlayback stores what is playing, for the next run.
func (m *Model) rememberPlayback() {
	track, ok := m.player.Current()
	if !ok {
		return
	}
	m.state.LastTrack = track.ID
	m.state.LastPosition = m.player.Position()
	m.touch()
}

// setPage navigates to a top-level screen.
func (m *Model) setPage(next page) {
	if next != m.page {
		m.backPage = m.page
	}
	m.page = next
	if next == pageSearch {
		m.syncSearch()
	}
}

// openAlbum shows an album's tracks.
func (m *Model) openAlbum(key string) {
	m.backPage = m.page
	m.albumKey = key
	m.page = pageAlbum
}

// openArtist shows an artist's tracks.
func (m *Model) openArtist(name string) {
	m.backPage = m.page
	m.artistName = name
	m.page = pageArtist
}

// openPlaylist shows a playlist's tracks.
func (m *Model) openPlaylist(id string) {
	m.backPage = m.page
	m.playlistID = id
	m.page = pagePlaylist
}

// goBack returns to the screen a detail page was opened from.
func (m *Model) goBack() {
	m.page = m.backPage
}

// openFavorites shows the tracks the user marked with a heart.
func (m *Model) openFavorites() {
	m.backPage = m.page
	m.page = pageFavorites
}

// openRecent shows the tracks added to the library most recently.
func (m *Model) openRecent() {
	m.backPage = m.page
	m.page = pageRecent
}

// playTracks replaces the queue with tracks and starts at index.
func (m *Model) playTracks(tracks []catalog.Track, index int) {
	if len(tracks) == 0 {
		return
	}
	if err := m.player.SetQueue(tracks, index); err != nil {
		m.toastf("无法播放：%v", err)
	}
	m.afterPlayStart()
}

// playNext puts tracks directly after the current one.
func (m *Model) playNext(tracks []catalog.Track) {
	m.player.InsertAfter(m.player.Index(), tracks)
	m.queue = m.player.Queue()
	m.toastf("已加入下一首播放")
}

// enqueue appends tracks to the queue.
func (m *Model) enqueue(tracks []catalog.Track) {
	if err := m.player.Enqueue(tracks, true); err != nil {
		m.toastf("无法播放：%v", err)
	}
	m.queue = m.player.Queue()
	m.afterPlayStart()
}

// afterPlayStart records the play in the counts and history.
func (m *Model) afterPlayStart() {
	track, ok := m.player.Current()
	if !ok {
		return
	}
	if m.state.PlayCounts == nil {
		m.state.PlayCounts = make(map[string]int)
	}
	m.state.PlayCounts[track.ID]++
	m.state.History = prependID(m.state.History, track.ID, 200)
	m.state.LastTrack = track.ID
	m.state.LastPosition = 0
	m.queue = m.player.Queue()
	m.refresh()
	m.touch()
}

// prependID puts id at the front of a list, removing an earlier occurrence.
func prependID(list []string, id string, limit int) []string {
	out := make([]string, 0, len(list)+1)
	out = append(out, id)
	for _, existing := range list {
		if existing == id {
			continue
		}
		out = append(out, existing)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// toggleFavorite adds or removes a track from 喜欢的歌曲.
func (m *Model) toggleFavorite(id string) {
	for i, existing := range m.state.Favorites {
		if existing == id {
			m.state.Favorites = append(m.state.Favorites[:i:i], m.state.Favorites[i+1:]...)
			m.refresh()
			m.touch()
			return
		}
	}
	m.state.Favorites = append([]string{id}, m.state.Favorites...)
	m.refresh()
	m.touch()
}

// isFavorite reports whether a track is in 喜欢的歌曲.
func (m *Model) isFavorite(id string) bool {
	for _, existing := range m.state.Favorites {
		if existing == id {
			return true
		}
	}
	return false
}

// createPlaylist makes an empty playlist and opens it.
func (m *Model) createPlaylist(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		m.toastf("请先给歌单起个名字")
		return
	}
	playlist := catalog.NewPlaylist(name, time.Now())
	m.state.Playlists = append(m.state.Playlists, playlist)
	m.playlistDraft.SetText("")
	m.showNewList = false
	m.refresh()
	m.touch()
	m.openPlaylist(playlist.ID)
}

// playlist looks a playlist up by identity.
func (m *Model) playlist(id string) (catalog.Playlist, bool) {
	for _, playlist := range m.state.Playlists {
		if playlist.ID == id {
			return playlist, true
		}
	}
	return catalog.Playlist{}, false
}

// addToPlaylist appends tracks to a playlist, skipping the ones already there.
func (m *Model) addToPlaylist(id string, tracks []catalog.Track) {
	for i := range m.state.Playlists {
		playlist := &m.state.Playlists[i]
		if playlist.ID != id {
			continue
		}
		for _, track := range tracks {
			if !containsID(playlist.TrackIDs, track.ID) {
				playlist.TrackIDs = append(playlist.TrackIDs, track.ID)
			}
		}
		m.refresh()
		m.touch()
		m.toastf("已加入歌单《%s》", playlist.Name)
		return
	}
}

// removeFromPlaylist takes a track out of a playlist.
func (m *Model) removeFromPlaylist(id, trackID string) {
	for i := range m.state.Playlists {
		playlist := &m.state.Playlists[i]
		if playlist.ID != id {
			continue
		}
		out := playlist.TrackIDs[:0]
		for _, existing := range playlist.TrackIDs {
			if existing != trackID {
				out = append(out, existing)
			}
		}
		playlist.TrackIDs = out
		m.refresh()
		m.touch()
		return
	}
}

// deletePlaylist removes a playlist.
func (m *Model) deletePlaylist(id string) {
	for i := range m.state.Playlists {
		if m.state.Playlists[i].ID == id {
			m.state.Playlists = append(m.state.Playlists[:i:i], m.state.Playlists[i+1:]...)
			m.refresh()
			m.touch()
			if m.page == pagePlaylist {
				m.setPage(pagePlaylists)
			}
			return
		}
	}
}

func containsID(list []string, id string) bool {
	for _, existing := range list {
		if existing == id {
			return true
		}
	}
	return false
}
