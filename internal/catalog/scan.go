package catalog

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"soda-music/internal/audio"
)

// Scan walks the given folders and returns every playable file it finds, with
// the tags and length the file itself reports. It never fails because one file
// is unreadable: a file that cannot be parsed is still listed, under its file
// name, so the user can see what is there.
//
// previous is the library as it was, used to keep the "added" timestamp of
// files that have not changed. progress, when not nil, is called with the number
// of files scanned so far.
func Scan(folders []string, previous *Library, progress func(done int)) []Track {
	tracks := make([]Track, 0, 256)
	seen := make(map[string]bool)
	addedAt := make(map[string]time.Time)
	if previous != nil {
		for _, track := range previous.Tracks {
			addedAt[track.ID] = track.Added
		}
	}
	for _, folder := range folders {
		root, err := filepath.Abs(folder)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if entry.IsDir() {
				if path != root && strings.HasPrefix(name, ".") {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(name, ".") || !audio.Playable(path) {
				return nil
			}
			key := strings.ToLower(path)
			if seen[key] {
				return nil
			}
			seen[key] = true
			track := readTrack(path, addedAt)
			tracks = append(tracks, track)
			if progress != nil {
				progress(len(tracks))
			}
			return nil
		})
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		left, right := strings.ToLower(tracks[i].Path), strings.ToLower(tracks[j].Path)
		return left < right
	})
	return tracks
}

// readTrack describes one file: its tags when they can be read, and the shape
// of its audio.
func readTrack(path string, addedAt map[string]time.Time) Track {
	info, statErr := os.Stat(path)
	track := Track{
		ID:    IDFor(path),
		Path:  path,
		Added: time.Now(),
	}
	if statErr == nil {
		track.Size = info.Size()
		track.Modified = info.ModTime()
	}
	if at, ok := addedAt[track.ID]; ok {
		track.Added = at
	}
	track.Format = strings.TrimPrefix(strings.ToUpper(filepath.Ext(path)), ".")
	readTags(path, &track)
	if probed, err := audio.Probe(path); err == nil {
		track.Duration = probed.Duration
		track.SampleRate = probed.Format.SampleRate
		track.Bitrate = probed.Bitrate
	}
	return track
}

// readTags fills in what the file's own metadata says.
func readTags(path string, track *Track) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return
	}
	track.Title = strings.TrimSpace(metadata.Title())
	track.Artist = strings.TrimSpace(metadata.Artist())
	track.Album = strings.TrimSpace(metadata.Album())
	track.AlbumArtist = strings.TrimSpace(metadata.AlbumArtist())
	track.Genre = strings.TrimSpace(metadata.Genre())
	track.Year = metadata.Year()
	if number, total := metadata.Track(); number > 0 {
		track.TrackNo = number
		_ = total
	}
	if number, total := metadata.Disc(); number > 0 {
		track.Disc = number
		_ = total
	}
	if metadata.Picture() != nil {
		track.Art = true
	}
}

// ErrNoFolders reports a scan that was asked for nothing.
var ErrNoFolders = errors.New("catalog: no folders to scan")
