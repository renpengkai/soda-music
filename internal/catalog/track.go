// Package catalog holds the music library: the tracks found on disk, the
// albums and artists they belong to, the user's playlists, and the small
// amount of state the application remembers between runs.
package catalog

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Track is one playable file.
type Track struct {
	ID          string        `json:"id"`
	Path        string        `json:"path"`
	Title       string        `json:"title"`
	Artist      string        `json:"artist"`
	Album       string        `json:"album"`
	AlbumArtist string        `json:"albumArtist,omitempty"`
	Genre       string        `json:"genre,omitempty"`
	Year        int           `json:"year,omitempty"`
	TrackNo     int           `json:"trackNo,omitempty"`
	Disc        int           `json:"disc,omitempty"`
	Duration    time.Duration `json:"duration"`
	Format      string        `json:"format"`
	SampleRate  int           `json:"sampleRate,omitempty"`
	Bitrate     int           `json:"bitrate,omitempty"`
	Size        int64         `json:"size,omitempty"`
	Modified    time.Time     `json:"modified,omitempty"`
	Added       time.Time     `json:"added"`
	Art         bool          `json:"art,omitempty"`

	// Source names where the file came from: empty for a file the user already
	// had, "netease" for a song streamed from the online service and cached on
	// disk. RemoteID is the service's own identity for the song, and CoverURL
	// is the album picture the service served.
	Source   string `json:"source,omitempty"`
	RemoteID int64  `json:"remoteId,omitempty"`
	CoverURL string `json:"coverUrl,omitempty"`
}

// Online reports whether the track came from an online service rather than the
// user's own files.
func (t Track) Online() bool { return t.Source != "" }

// IDFor derives the identity of a track from its path, so the same file keeps
// the same identity across scans and across machines that share a library root.
func IDFor(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	sum := sha1.Sum([]byte(strings.ToLower(filepath.ToSlash(absolute))))
	return hex.EncodeToString(sum[:8])
}

// DisplayArtist is the artist to show for a track, falling back to the album
// artist and then to a placeholder.
func (t Track) DisplayArtist() string {
	switch {
	case t.Artist != "":
		return t.Artist
	case t.AlbumArtist != "":
		return t.AlbumArtist
	}
	return "未知艺术家"
}

// DisplayAlbum is the album to show for a track.
func (t Track) DisplayAlbum() string {
	if t.Album != "" {
		return t.Album
	}
	return "未知专辑"
}

// DisplayTitle is the title to show for a track, falling back to its file name.
func (t Track) DisplayTitle() string {
	if t.Title != "" {
		return t.Title
	}
	return strings.TrimSuffix(filepath.Base(t.Path), filepath.Ext(t.Path))
}

// Album groups the tracks that share an album title and album artist.
type Album struct {
	Key      string
	Name     string
	Artist   string
	Year     int
	Duration time.Duration
	Tracks   []Track
}

// Artist groups everything credited to one performer.
type Artist struct {
	Name     string
	Albums   []string
	Duration time.Duration
	Tracks   []Track
}

// Playlist is a user-made list of tracks.
type Playlist struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TrackIDs  []string  `json:"trackIds"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewPlaylist makes an empty playlist with a fresh identity.
func NewPlaylist(name string, at time.Time) Playlist {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s/%d", name, at.UnixNano())))
	return Playlist{ID: hex.EncodeToString(sum[:8]), Name: name, CreatedAt: at}
}

// DurationOf adds up the length of the tracks that are present.
func DurationOf(tracks []Track) time.Duration {
	var total time.Duration
	for _, track := range tracks {
		total += track.Duration
	}
	return total
}

// SortTracksByTitle orders a copy of tracks the way a list of songs is read.
func SortTracksByTitle(tracks []Track) []Track {
	out := append([]Track(nil), tracks...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := strings.ToLower(out[i].DisplayTitle()), strings.ToLower(out[j].DisplayTitle())
		if left != right {
			return left < right
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// SortTracksByAlbum orders a copy of tracks by album, then disc, then number.
func SortTracksByAlbum(tracks []Track) []Track {
	out := append([]Track(nil), tracks...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if key := strings.ToLower(a.DisplayAlbum()); key != strings.ToLower(b.DisplayAlbum()) {
			return key < strings.ToLower(b.DisplayAlbum())
		}
		if a.Disc != b.Disc {
			return a.Disc < b.Disc
		}
		if a.TrackNo != b.TrackNo {
			return a.TrackNo < b.TrackNo
		}
		return strings.ToLower(a.DisplayTitle()) < strings.ToLower(b.DisplayTitle())
	})
	return out
}
