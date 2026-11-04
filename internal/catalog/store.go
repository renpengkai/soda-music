package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// State is what the application remembers between runs: the folders it watches,
// the playlists the user made, and the small preferences of the player.
type State struct {
	Folders      []string       `json:"folders"`
	Playlists    []Playlist     `json:"playlists"`
	Favorites    []string       `json:"favorites"`
	History      []string       `json:"history"`
	PlayCounts   map[string]int `json:"playCounts,omitempty"`
	OnlineTracks []Track        `json:"onlineTracks,omitempty"`
	LastTrack    string         `json:"lastTrack,omitempty"`
	LastPosition time.Duration  `json:"lastPosition,omitempty"`
	Volume       float64        `json:"volume"`
	Shuffle      bool           `json:"shuffle"`
	Repeat       string         `json:"repeat"`
	Theme        string         `json:"theme"`
	FadeMs       int            `json:"fadeMs"`
	ShowLyrics   bool           `json:"showLyrics"`
	Updated      time.Time      `json:"updated,omitempty"`
}

// DefaultState is the state of a fresh installation.
func DefaultState() State {
	return State{Volume: 0.8, Repeat: "off", Theme: "system", FadeMs: 120}
}

// Store is the directory the application keeps its files in.
type Store struct {
	Dir string
}

// OpenStore finds the per-user configuration directory and makes sure it
// exists. On Windows that is %AppData%\soda-music, on macOS
// ~/Library/Application Support/soda-music, and on Linux ~/.config/soda-music.
func OpenStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "soda-music")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) path(name string) string { return filepath.Join(s.Dir, name) }

// ArtDir is where extracted cover art is cached.
func (s *Store) ArtDir() string { return filepath.Join(s.Dir, "art") }

// ArtPath is the cached cover for a track, whether or not it exists yet.
func (s *Store) ArtPath(trackID string) string {
	return filepath.Join(s.ArtDir(), trackID+".img")
}

// OnlineDir is where songs streamed from an online service are cached, so a
// song that has been played once does not have to be fetched again.
func (s *Store) OnlineDir() string { return filepath.Join(s.Dir, "online") }

// OnlinePath is the cache file for a song, whether or not it has been
// downloaded yet. The extension is part of the name because the decoder picks a
// parser from it.
func (s *Store) OnlinePath(remoteID int64, extension string) string {
	return filepath.Join(s.OnlineDir(), fmt.Sprintf("%d%s", remoteID, extension))
}

// LoadState reads the saved state, returning the defaults when there is none or
// it cannot be parsed.
func (s *Store) LoadState() State {
	state := DefaultState()
	raw, err := os.ReadFile(s.path("state.json"))
	if err != nil {
		return state
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return DefaultState()
	}
	if state.Volume < 0 || state.Volume > 1 {
		state.Volume = 0.8
	}
	if state.Repeat == "" {
		state.Repeat = "off"
	}
	if state.Theme == "" {
		state.Theme = "system"
	}
	return state
}

// SaveState writes the state atomically, so a crash mid-write cannot leave a
// half-written file behind.
func (s *Store) SaveState(state State) error {
	state.Updated = time.Now()
	return writeJSON(s.path("state.json"), state)
}

// SaveLibrary caches the scanned tracks so the next start does not have to read
// every file again.
func (s *Store) SaveLibrary(folders []string, tracks []Track) error {
	return writeJSON(s.path("library.json"), libraryFile{Folders: folders, Tracks: tracks, Scanned: time.Now()})
}

// LoadLibrary reads the cached scan.
func (s *Store) LoadLibrary() ([]string, []Track, error) {
	raw, err := os.ReadFile(s.path("library.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var file libraryFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, nil, err
	}
	return file.Folders, file.Tracks, nil
}

type libraryFile struct {
	Folders []string  `json:"folders"`
	Tracks  []Track   `json:"tracks"`
	Scanned time.Time `json:"scanned"`
}

// writeJSON writes a value to path through a temporary file in the same
// directory.
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
