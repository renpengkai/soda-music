package catalog

import (
	"sort"
	"strings"
	"time"
)

// Library is the scanned music on disk, indexed for the screens that need it.
// Build one with New and replace it with Scan's result; the indexes are derived
// from the tracks, so a Library is never edited in place.
type Library struct {
	Tracks  []Track
	Albums  []Album
	Artists []Artist

	byID map[string]int
}

// New indexes tracks into a Library.
func New(tracks []Track) *Library {
	library := &Library{Tracks: append([]Track(nil), tracks...)}
	library.reindex()
	return library
}

func (l *Library) reindex() {
	l.byID = make(map[string]int, len(l.Tracks))
	for i, track := range l.Tracks {
		l.byID[track.ID] = i
	}
	l.buildAlbums()
	l.buildArtists()
}

// Track looks a track up by identity.
func (l *Library) Track(id string) (Track, bool) {
	if l == nil {
		return Track{}, false
	}
	index, ok := l.byID[id]
	if !ok {
		return Track{}, false
	}
	return l.Tracks[index], true
}

// TracksByID resolves a list of identities, dropping the ones that are gone.
func (l *Library) TracksByID(ids []string) []Track {
	out := make([]Track, 0, len(ids))
	for _, id := range ids {
		if track, ok := l.Track(id); ok {
			out = append(out, track)
		}
	}
	return out
}

// Length is how many tracks the library holds.
func (l *Library) Length() int {
	if l == nil {
		return 0
	}
	return len(l.Tracks)
}

// Duration is the total playing time of the library.
func (l *Library) Duration() time.Duration {
	if l == nil {
		return 0
	}
	return DurationOf(l.Tracks)
}

// Album looks an album up by its key.
func (l *Library) Album(key string) (Album, bool) {
	for _, album := range l.Albums {
		if album.Key == key {
			return album, true
		}
	}
	return Album{}, false
}

// Artist looks an artist up by name.
func (l *Library) Artist(name string) (Artist, bool) {
	for _, artist := range l.Artists {
		if artist.Name == name {
			return artist, true
		}
	}
	return Artist{}, false
}

// Search returns the tracks, albums and artists matching a query, matching
// case-insensitively on any of the fields a listener would type.
func (l *Library) Search(query string, limit int) (tracks []Track, albums []Album, artists []Artist) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" || l == nil {
		return nil, nil, nil
	}
	for _, track := range l.Tracks {
		if trackMatches(track, query) {
			tracks = append(tracks, track)
			if limit > 0 && len(tracks) >= limit {
				break
			}
		}
	}
	for _, album := range l.Albums {
		if strings.Contains(strings.ToLower(album.Name), query) || strings.Contains(strings.ToLower(album.Artist), query) {
			albums = append(albums, album)
			if limit > 0 && len(albums) >= limit {
				break
			}
		}
	}
	for _, artist := range l.Artists {
		if strings.Contains(strings.ToLower(artist.Name), query) {
			artists = append(artists, artist)
			if limit > 0 && len(artists) >= limit {
				break
			}
		}
	}
	return tracks, albums, artists
}

func trackMatches(track Track, query string) bool {
	for _, field := range []string{track.Title, track.Artist, track.Album, track.AlbumArtist, track.Genre, track.Path} {
		if field != "" && strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// Genres lists the distinct genres in the library, most common first.
func (l *Library) Genres() []string {
	if l == nil {
		return nil
	}
	counts := make(map[string]int)
	for _, track := range l.Tracks {
		if track.Genre != "" {
			counts[track.Genre]++
		}
	}
	out := make([]string, 0, len(counts))
	for genre := range counts {
		out = append(out, genre)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

func (l *Library) buildAlbums() {
	type bucket struct {
		album Album
	}
	groups := make(map[string]*bucket)
	order := make([]string, 0, len(l.Tracks))
	for _, track := range l.Tracks {
		name := track.DisplayAlbum()
		artist := track.AlbumArtist
		if artist == "" {
			artist = track.DisplayArtist()
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(artist)
		group := groups[key]
		if group == nil {
			group = &bucket{album: Album{Key: key, Name: name, Artist: artist, Year: track.Year}}
			groups[key] = group
			order = append(order, key)
		}
		if group.album.Year == 0 {
			group.album.Year = track.Year
		}
		group.album.Tracks = append(group.album.Tracks, track)
		group.album.Duration += track.Duration
	}
	l.Albums = make([]Album, 0, len(order))
	for _, key := range order {
		album := groups[key].album
		album.Tracks = SortTracksByAlbum(album.Tracks)
		l.Albums = append(l.Albums, album)
	}
	sort.SliceStable(l.Albums, func(i, j int) bool {
		left, right := strings.ToLower(l.Albums[i].Name), strings.ToLower(l.Albums[j].Name)
		if left != right {
			return left < right
		}
		return strings.ToLower(l.Albums[i].Artist) < strings.ToLower(l.Albums[j].Artist)
	})
}

func (l *Library) buildArtists() {
	groups := make(map[string]*Artist)
	order := make([]string, 0)
	for _, track := range l.Tracks {
		name := track.DisplayArtist()
		artist := groups[name]
		if artist == nil {
			artist = &Artist{Name: name}
			groups[name] = artist
			order = append(order, name)
		}
		artist.Tracks = append(artist.Tracks, track)
		artist.Duration += track.Duration
		album := track.DisplayAlbum()
		found := false
		for _, existing := range artist.Albums {
			if existing == album {
				found = true
				break
			}
		}
		if !found {
			artist.Albums = append(artist.Albums, album)
		}
	}
	l.Artists = make([]Artist, 0, len(order))
	for _, name := range order {
		artist := groups[name]
		sort.Strings(artist.Albums)
		artist.Tracks = SortTracksByAlbum(artist.Tracks)
		l.Artists = append(l.Artists, *artist)
	}
	sort.SliceStable(l.Artists, func(i, j int) bool {
		return strings.ToLower(l.Artists[i].Name) < strings.ToLower(l.Artists[j].Name)
	})
}
