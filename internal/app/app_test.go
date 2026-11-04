package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/catalog"
	"soda-music/internal/netease"
)

// sampleTracks is a small, deterministic library so a screenshot always shows
// the same rows.
func sampleTracks() []catalog.Track {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	specs := []struct {
		title  string
		artist string
		album  string
		genre  string
	}{
		{"夜航", "陈一", "潮汐", "电子"},
		{"汽水", "林二", "气泡", "流行"},
		{"夏日回声", "苏三", "潮汐", "电子"},
		{"旧磁带", "陈一", "潮汐", "电子"},
		{"月光航线", "苏三", "夜行", "氛围"},
		{"清晨地铁", "林二", "气泡", "流行"},
	}
	tracks := make([]catalog.Track, 0, len(specs))
	for i, spec := range specs {
		tracks = append(tracks, catalog.Track{
			ID:       catalog.IDFor("/music/" + spec.title + ".flac"),
			Path:     "/music/" + spec.title + ".flac",
			Title:    spec.title,
			Artist:   spec.artist,
			Album:    spec.album,
			Genre:    spec.genre,
			TrackNo:  i + 1,
			Duration: time.Duration(180+i*11) * time.Second,
			Format:   "FLAC",
			Added:    now.Add(time.Duration(i) * time.Hour),
		})
	}
	return tracks
}

func newTestModel(t *testing.T) *Model {
	t.Helper()
	store := &catalog.Store{Dir: t.TempDir()}
	m := newModel(store, nil, &vivaapp.Invalidator{}, &appTree, appRoot)
	m.state = catalog.DefaultState()
	m.state.Playlists = []catalog.Playlist{
		catalog.NewPlaylist("深夜循环", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		catalog.NewPlaylist("通勤", time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)),
	}
	tracks := sampleTracks()
	m.state.Favorites = []string{tracks[0].ID, tracks[2].ID}
	m.state.History = []string{tracks[1].ID, tracks[0].ID}
	m.state.PlayCounts = map[string]int{tracks[0].ID: 12, tracks[1].ID: 5, tracks[2].ID: 3}
	m.setLibrary(catalog.New(tracks), nil)
	return m
}

// screenshotDir is where the verification images are written.
func screenshotDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "outputs", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// screenCase is one state of the window that is painted and photographed.
type screenCase struct {
	name  string
	setup func(m *Model)
}

func screenCases() []screenCase {
	return []screenCase{
		{"01-discover-empty", func(m *Model) {
			m.setLibrary(catalog.New(nil), nil)
			m.page = pageDiscover
		}},
		{"02-library", func(m *Model) {
			m.page = pageLibrary
			m.libraryTab = tabTracks
		}},
		{"03-albums", func(m *Model) {
			m.page = pageLibrary
			m.libraryTab = tabAlbums
		}},
		{"04-playlists", func(m *Model) {
			m.page = pagePlaylists
		}},
		{"05-settings", func(m *Model) {
			m.page = pageSettings
		}},
		{"06-now-playing", func(m *Model) {
			m.showNowPlaying = true
			_ = m.player.SetQueue(sampleTracks(), 0)
		}},
		{"07-queue", func(m *Model) {
			m.showQueue = true
			_ = m.player.SetQueue(sampleTracks(), 0)
		}},
		{"08-new-playlist", func(m *Model) {
			m.showNewList = true
		}},
		{"09-add-to-playlist", func(m *Model) {
			m.addTrackID = sampleTracks()[0].ID
		}},
		{"10-dark-library", func(m *Model) {
			m.state.Theme = "dark"
			m.page = pageLibrary
		}},
		{"11-online-signed-out", func(m *Model) {
			m.page = pageOnline
			m.onlineState = netease.Session{}
		}},
		{"12-online-library", func(m *Model) {
			m.page = pageOnline
			m.onlineState = netease.Session{Cookie: "test", UID: 42, Nickname: "测试用户"}
			m.onlinePlaylists = samplePlaylists()
			m.onlineDaily = sampleSongs()
		}},
		{"13-online-playlist", func(m *Model) {
			m.page = pageOnlinePlaylist
			m.onlineState = netease.Session{Cookie: "test", UID: 42, Nickname: "测试用户"}
			m.onlinePlaylistName = "每日推荐"
			m.onlineSongs = sampleSongs()
		}},
		{"14-login-dialog", func(m *Model) {
			m.page = pageOnline
			m.onlineState = netease.Session{}
			m.showLogin = true
		}},
		{"15-login-qr", func(m *Model) {
			m.page = pageOnline
			m.onlineState = netease.Session{}
			m.showLogin = true
			m.onlineLogin = &netease.QRLogin{
				Key:     "sample-key",
				Image:   sampleQRPNG(),
				State:   netease.QRWaiting,
				Message: "请使用网易云音乐 App 扫码",
			}
		}},
	}
}

// sampleQRPNG renders a deterministic QR code so the sign-in dialog can be
// photographed with a real bitmap in place.
func sampleQRPNG() []byte {
	code, err := qrcode.New("https://music.163.com/login?codekey=sample-key", qrcode.Medium)
	if err != nil {
		panic(err)
	}
	code.DisableBorder = false
	bitmap := code.Bitmap()
	side := len(bitmap)
	scale := 8
	picture := image.NewRGBA(image.Rect(0, 0, side*scale, side*scale))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			shade := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if bitmap[y][x] {
				shade = color.RGBA{R: 0x0B, G: 0x1A, B: 0x17, A: 255}
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					picture.Set(x*scale+dx, y*scale+dy, shade)
				}
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		panic(err)
	}
	return out.Bytes()
}

// sampleSongs is a small, deterministic list of service songs for the online
// screenshots.
func sampleSongs() []netease.Song {
	specs := []struct {
		id     int64
		title  string
		artist string
		album  string
	}{
		{101, "海边的信", "周屿", "潮汐"},
		{102, "夜行列车", "林声", "夜行"},
		{103, "气泡水", "苏禾", "气泡"},
	}
	songs := make([]netease.Song, 0, len(specs))
	for _, spec := range specs {
		song := netease.Song{ID: spec.id, Name: spec.title, Duration: 3*time.Minute + 20*time.Second}
		song.Artists = []netease.Artist{{ID: spec.id, Name: spec.artist}}
		song.Album = netease.Album{ID: spec.id, Name: spec.album}
		songs = append(songs, song)
	}
	return songs
}

// samplePlaylists is a small, deterministic list of service playlists.
func samplePlaylists() []netease.Playlist {
	return []netease.Playlist{
		{ID: 201, Name: "我喜欢的音乐", TrackCount: 128},
		{ID: 202, Name: "通勤路上", TrackCount: 42},
		{ID: 203, Name: "深夜循环", TrackCount: 66},
	}
}

// TestLayout drives the real paint path for every screen without a window. It
// fails if a page panics or a control loses its identity, and it is the only
// test that needs no offscreen renderer.
func TestLayout(t *testing.T) {
	for _, tc := range screenCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			tc.setup(m)
			session, err := headless.Open(m.config(), nil, headless.Options{Scale: 1, ReferenceText: true})
			if err != nil {
				t.Skipf("headless session unavailable: %v", err)
			}
			defer session.Close()
			if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
				t.Fatalf("settle: %v", err)
			}
			if len(session.Texts()) == 0 {
				t.Fatal("the window painted no text")
			}
		})
	}
}

// TestScreenshots writes the images used to review the layout by eye. It skips
// when the platform has no offscreen renderer, so it never fails a headless
// build machine.
func TestScreenshots(t *testing.T) {
	dir := screenshotDir(t)
	for _, tc := range screenCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			tc.setup(m)
			session, err := headless.Open(m.config(), nil, headless.Options{Scale: 1, ReferenceText: true})
			if err != nil {
				t.Skipf("headless session unavailable: %v", err)
			}
			defer session.Close()
			if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
				t.Fatalf("settle: %v", err)
			}
			path := filepath.Join(dir, tc.name+".png")
			if err := session.Screenshot(path); err != nil {
				if errors.Is(err, headless.ErrNoRenderer) {
					t.Skipf("no offscreen renderer: %v", err)
				}
				t.Fatalf("screenshot: %v", err)
			}
		})
	}
}

// TestNavIDsCoverEveryEntry guards a failure mode that is invisible in a
// screenshot: a sidebar entry that is drawn but has no hit id, so clicking it
// does nothing. Every entry must have a distinct, non-zero id.
func TestNavIDsCoverEveryEntry(t *testing.T) {
	seen := make(map[ui.ID]string, len(navEntries))
	for _, entry := range navEntries {
		id, ok := navIDs[entry.page]
		if !ok {
			t.Fatalf("导航项 %q（page %v）在 navIDs 里没有对应条目，点击不会生效", entry.label, entry.page)
		}
		if id.IsZero() {
			t.Fatalf("导航项 %q（page %v）映射到零值 id，点击不会生效", entry.label, entry.page)
		}
		if prev, dup := seen[id]; dup {
			t.Fatalf("导航项 %q 与 %q 共用了同一个 id %v", prev, entry.label, id)
		}
		seen[id] = entry.label
	}
}

// TestClickEveryNavEntry drives the real hit-testing path: it clicks each
// sidebar entry by its label and checks the model switched to that page.
func TestClickEveryNavEntry(t *testing.T) {
	for _, entry := range navEntries {
		entry := entry
		t.Run(entry.label, func(t *testing.T) {
			m := newTestModel(t)
			session, err := headless.Open(m.config(), nil, headless.Options{Scale: 1, ReferenceText: true})
			if err != nil {
				t.Skipf("headless session unavailable: %v", err)
			}
			defer session.Close()
			if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
				t.Fatalf("settle: %v", err)
			}
			if err := session.ClickLabel(entry.label); err != nil {
				t.Fatalf("点击 %q 失败: %v", entry.label, err)
			}
			if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
				t.Fatalf("settle: %v", err)
			}
			if m.page != entry.page {
				t.Fatalf("点击 %q 之后 page = %v，期望 %v", entry.label, m.page, entry.page)
			}
		})
	}
}

// TestLoginQRDrawsOnDialogLayer guards the sign-in code against a z-order bug:
// the QR bitmap must be painted on the dialog's own compositing layer, not on
// layer zero after the frame. On layer zero the dialog surface and its scrim,
// which paint above the page, veil or hide the code.
func TestLoginQRDrawsOnDialogLayer(t *testing.T) {
	m := newTestModel(t)
	m.page = pageOnline
	m.onlineState = netease.Session{}
	m.showLogin = true
	m.onlineLogin = &netease.QRLogin{
		Key:     "sample-key",
		Image:   sampleQRPNG(),
		State:   netease.QRWaiting,
		Message: "请使用网易云音乐 App 扫码",
	}
	session, err := headless.Open(m.config(), nil, headless.Options{Scale: 1, ReferenceText: true})
	if err != nil {
		t.Skipf("headless session unavailable: %v", err)
	}
	defer session.Close()
	if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
		t.Fatalf("settle: %v", err)
	}
	snapshot := session.Snapshot()
	// The code is the one bitmap big enough to be the QR square. Drawing it as
	// many one-pixel strips, the way the cover painter does, is the bug this
	// test exists to catch.
	bitmaps := 0
	var code *vivaapp.Label
	for index := range snapshot.Labels {
		label := &snapshot.Labels[index]
		if label.BitmapWidth == 0 {
			continue
		}
		bitmaps++
		if label.Bounds.Width >= 100 && label.Bounds.Height >= 100 {
			code = label
		}
	}
	if code == nil {
		t.Fatalf("登录对话框里没有画出整块二维码位图（找到 %d 个位图标签）；它应当是一张图片，而不是逐行拼出来的", bitmaps)
	}
	if code.Layer == 0 {
		t.Fatalf("二维码画在第 0 层（bounds %v），会被对话框表面和遮罩盖住", code.Bounds)
	}
}

// TestLoginDialogStartsFetchWhenOpened guards the sign-in dialog against being
// opened without a code being fetched. Several actions open it directly (a
// search or a playback that needs an account); if none of them starts the
// fetch, the dialog sits on "正在获取二维码…" and no code ever appears.
func TestLoginDialogStartsFetchWhenOpened(t *testing.T) {
	m := newTestModel(t)
	m.page = pageOnline
	m.onlineState = netease.Session{}
	// Open the dialog the way the search and playback paths do: without going
	// through signIn.
	m.showLogin = true
	session, err := headless.Open(m.config(), nil, headless.Options{Scale: 1, ReferenceText: true})
	if err != nil {
		t.Skipf("headless session unavailable: %v", err)
	}
	defer session.Close()
	if err := session.Settle(5*time.Millisecond, 2*time.Second); err != nil {
		t.Fatalf("settle: %v", err)
	}
	m.mu.Lock()
	login := m.onlineLogin
	loginErr := m.loginError
	m.mu.Unlock()
	if !m.loginBusy && login == nil && loginErr == "" {
		t.Fatal("登录对话框打开了，却没有开始获取二维码，会一直停在「正在获取二维码…」")
	}
}
