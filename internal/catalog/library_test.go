package catalog

import (
	"path/filepath"
	"testing"
	"time"
)

func sampleLibrary() *Library {
	return New([]Track{
		{ID: "a", Path: "/m/夜航.flac", Title: "夜航", Artist: "陈一", Album: "潮汐", Genre: "电子", Duration: time.Minute, Added: time.Unix(3, 0)},
		{ID: "b", Path: "/m/汽水.mp3", Title: "汽水", Artist: "林二", Album: "气泡", Genre: "流行", Duration: 2 * time.Minute, Added: time.Unix(1, 0)},
		{ID: "c", Path: "/m/夏日回声.flac", Title: "夏日回声", Artist: "苏三", Album: "潮汐", Genre: "电子", Duration: 3 * time.Minute, Added: time.Unix(2, 0)},
	})
}

func TestLibraryIndex(t *testing.T) {
	lib := sampleLibrary()
	if lib.Length() != 3 {
		t.Fatalf("length = %d", lib.Length())
	}
	if got := lib.Duration(); got != 6*time.Minute {
		t.Fatalf("duration = %v", got)
	}
	if _, ok := lib.Track("b"); !ok {
		t.Fatal("track b missing")
	}
	ids := lib.TracksByID([]string{"c", "missing", "a"})
	if len(ids) != 2 || ids[0].ID != "c" || ids[1].ID != "a" {
		t.Fatalf("TracksByID = %+v", ids)
	}
	if genres := lib.Genres(); len(genres) != 2 {
		t.Fatalf("genres = %v", genres)
	}
}

func TestLibrarySearch(t *testing.T) {
	lib := sampleLibrary()
	tracks, albums, _ := lib.Search("潮汐", 10)
	if len(tracks) != 2 {
		t.Fatalf("tracks for 潮汐 = %d, want 2", len(tracks))
	}
	// 潮汐 is by two different artists, so it forms two albums.
	if len(albums) != 2 {
		t.Fatalf("albums for 潮汐 = %d, want 2", len(albums))
	}
	tracks, _, artists := lib.Search("苏三", 10)
	if len(tracks) != 1 || len(artists) != 1 {
		t.Fatalf("search 苏三 = %d tracks, %d artists", len(tracks), len(artists))
	}
	if _, _, artists = lib.Search("", 10); len(artists) != 0 {
		t.Fatalf("empty query returned %d artists", len(artists))
	}
}

func TestSortTracksByTitle(t *testing.T) {
	lib := sampleLibrary()
	sorted := SortTracksByTitle(lib.Tracks)
	// Titles sort by UTF-8 byte order, so 夏 (U+590F) leads 夜 (U+591C).
	if sorted[0].Title != "夏日回声" {
		t.Fatalf("first title = %q", sorted[0].Title)
	}
	// The original slice must be untouched.
	if lib.Tracks[0].Title != "夜航" {
		t.Fatal("SortTracksByTitle modified its input")
	}
}

func TestIDForIsStable(t *testing.T) {
	first := IDFor(filepath.Join("music", "song.flac"))
	second := IDFor(filepath.Join("music", "song.flac"))
	if first != second {
		t.Fatalf("IDFor is not stable: %q vs %q", first, second)
	}
	if first == IDFor(filepath.Join("music", "other.flac")) {
		t.Fatal("different paths share an identity")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := &Store{Dir: dir}
	state := DefaultState()
	state.Volume = 0.42
	state.Favorites = []string{"a", "b"}
	state.Playlists = []Playlist{NewPlaylist("深夜", time.Unix(10, 0))}
	if err := store.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got := store.LoadState()
	if got.Volume != 0.42 || len(got.Favorites) != 2 || len(got.Playlists) != 1 {
		t.Fatalf("LoadState = %+v", got)
	}
	tracks := sampleLibrary().Tracks
	if err := store.SaveLibrary([]string{dir}, tracks); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	folders, loaded, err := store.LoadLibrary()
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}
	if len(folders) != 1 || len(loaded) != len(tracks) {
		t.Fatalf("LoadLibrary = %d folders, %d tracks", len(folders), len(loaded))
	}
}

func TestScanMissingFolderIsEmpty(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if got := Scan([]string{missing}, nil, nil); len(got) != 0 {
		t.Fatalf("Scan of a missing folder = %d tracks", len(got))
	}
}
