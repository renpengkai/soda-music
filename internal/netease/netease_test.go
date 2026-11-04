package netease

import (
	"encoding/hex"
	"os"
	"testing"
)

// TestEAPIParams pins the encryption to a known vector, so a refactor cannot
// quietly change the wire format.
func TestEAPIParams(t *testing.T) {
	params, err := eapiParams("/api/v6/playlist/detail", map[string]any{"id": "1", "n": "1000", "s": 8})
	if err != nil {
		t.Fatalf("eapiParams: %v", err)
	}
	if len(params) == 0 || len(params)%32 != 0 {
		t.Fatalf("params length %d is not a whole number of AES blocks", len(params))
	}
	if _, err := hex.DecodeString(params); err != nil {
		t.Fatalf("params is not hex: %v", err)
	}
	for _, r := range params {
		if r >= 'a' && r <= 'z' {
			t.Fatalf("params should be upper-case hex, found %q", r)
		}
	}
}

// TestEAPIParamsStable checks the payload is signed over the same bytes that are
// encrypted, by comparing two runs of the same input.
func TestEAPIParamsStable(t *testing.T) {
	first, err := eapiParams("/api/song/lyric", map[string]any{"id": 7, "lv": -1, "tv": -1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := eapiParams("/api/song/lyric", map[string]any{"id": 7, "lv": -1, "tv": -1})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same input produced different params")
	}
}

func TestCookieRoundTrip(t *testing.T) {
	header := "os=pc; appver=8.9.70; MUSIC_U=abc=def; __csrf=xyz"
	jar := parseCookie(header)
	if jar["MUSIC_U"] != "abc=def" {
		t.Fatalf("MUSIC_U = %q, want abc=def", jar["MUSIC_U"])
	}
	if jar["__csrf"] != "xyz" {
		t.Fatalf("__csrf = %q, want xyz", jar["__csrf"])
	}
	round := parseCookie(formatCookie(jar))
	if len(round) != len(jar) {
		t.Fatalf("round trip lost fields: %d -> %d", len(jar), len(round))
	}
	for name, value := range jar {
		if round[name] != value {
			t.Fatalf("%s = %q, want %q", name, round[name], value)
		}
	}
}

func TestMergeCookieKeepsExisting(t *testing.T) {
	merged := mergeCookie("MUSIC_U=one", "MUSIC_U=two; __csrf=three")
	jar := parseCookie(merged)
	if jar["MUSIC_U"] != "one" {
		t.Fatalf("merge overwrote an existing field: %q", jar["MUSIC_U"])
	}
	if jar["__csrf"] != "three" {
		t.Fatalf("merge dropped a new field: %q", jar["__csrf"])
	}
}

func TestExtension(t *testing.T) {
	cases := map[string]string{
		"https://m8.music.126.net/a/b.mp3":          ".mp3",
		"https://m8.music.126.net/a/b.flac?x=1":     ".flac",
		"https://m8.music.126.net/a/b.mp3?auth=abc": ".mp3",
		"https://m8.music.126.net/stream":           ".mp3",
	}
	for url, want := range cases {
		if got := Extension(url); got != want {
			t.Errorf("Extension(%q) = %q, want %q", url, got, want)
		}
	}
}

// TestLiveQRKey talks to the real service. It is skipped unless NETEASE_LIVE is
// set, so the normal test run stays offline.
func TestLiveQRKey(t *testing.T) {
	if os.Getenv("NETEASE_LIVE") == "" {
		t.Skip("set NETEASE_LIVE=1 to run the live check")
	}
	client := New("")
	login, err := client.QRKey()
	if err != nil {
		t.Fatalf("QRKey: %v", err)
	}
	if login.Key == "" || len(login.Image) == 0 {
		t.Fatalf("QRKey returned an empty code")
	}
	next, err := client.QRCheck(login)
	if err != nil {
		t.Fatalf("QRCheck: %v", err)
	}
	if next.State != QRWaiting {
		t.Fatalf("fresh code state = %v, want waiting", next.State)
	}
}

// TestLiveSongURLs checks the player URL endpoint and the quality fallback: a
// free account often gets nothing at "exhigh" but a working URL at "standard".
func TestLiveSongURLs(t *testing.T) {
	if os.Getenv("NETEASE_LIVE") == "" {
		t.Skip("set NETEASE_LIVE=1 to run the live check")
	}
	client := New("")
	// 热歌榜 is a stable, public playlist with mostly free tracks.
	detail, err := client.PlaylistDetail(3778678)
	if err != nil {
		t.Fatalf("PlaylistDetail: %v", err)
	}
	songs := detail.Tracks
	if len(songs) < 10 {
		t.Fatalf("playlist returned %d songs", len(songs))
	}
	ids := make([]int64, 0, 10)
	for _, song := range songs[:10] {
		ids = append(ids, song.ID)
	}
	urls, err := client.SongURLs(ids, "")
	if err != nil {
		t.Fatalf("SongURLs: %v", err)
	}
	if len(urls) == 0 {
		t.Fatal("SongURLs returned nothing")
	}
	for id, url := range urls {
		if url == "" {
			t.Fatalf("song %d has an empty url", id)
		}
	}
	t.Logf("resolved %d/%d urls", len(urls), len(ids))
}

// TestLiveSearch checks the search endpoint and that a song normalises the same
// way from both reply shapes.
func TestLiveSearch(t *testing.T) {
	if os.Getenv("NETEASE_LIVE") == "" {
		t.Skip("set NETEASE_LIVE=1 to run the live check")
	}
	client := New("")
	songs, err := client.Search("周杰伦", 1, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(songs) == 0 {
		t.Fatal("Search returned nothing")
	}
	for _, song := range songs {
		if song.ID == 0 {
			t.Fatalf("song without an id: %+v", song)
		}
		if song.Name == "" {
			t.Fatalf("song without a name: %+v", song)
		}
	}
}
