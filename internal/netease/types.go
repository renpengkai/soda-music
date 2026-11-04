package netease

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Artist is one performer credited on a song.
type Artist struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Album is the record a song belongs to.
type Album struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	CoverURL string `json:"picUrl"`
}

// Song is one track, normalised so the two shapes the API returns - the search
// shape and the playlist shape - read the same way.
type Song struct {
	ID       int64
	Name     string
	Duration time.Duration
	Artists  []Artist
	Album    Album
	Fee      int
}

// wireSong is the union of the fields the API uses for a song. Search replies
// with artists/album/duration; playlist and detail replies use ar/al/dt.
type wireSong struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Dt       int64    `json:"dt"`
	Duration int64    `json:"duration"`
	Ar       []Artist `json:"ar"`
	Artists  []Artist `json:"artists"`
	Al       Album    `json:"al"`
	Album    Album    `json:"album"`
	Fee      int      `json:"fee"`
}

// UnmarshalJSON folds the two song shapes into one.
func (s *Song) UnmarshalJSON(raw []byte) error {
	var wire wireSong
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	s.ID = wire.ID
	s.Name = wire.Name
	s.Fee = wire.Fee
	switch {
	case wire.Dt > 0:
		s.Duration = time.Duration(wire.Dt) * time.Millisecond
	case wire.Duration > 0:
		s.Duration = time.Duration(wire.Duration) * time.Millisecond
	}
	if len(wire.Ar) > 0 {
		s.Artists = wire.Ar
	} else {
		s.Artists = wire.Artists
	}
	if wire.Al.Name != "" || wire.Al.ID != 0 {
		s.Album = wire.Al
	} else {
		s.Album = wire.Album
	}
	return nil
}

// ArtistNames joins the credited performers the way a list of songs reads.
func (s Song) ArtistNames() string {
	names := make([]string, 0, len(s.Artists))
	for _, artist := range s.Artists {
		if artist.Name != "" {
			names = append(names, artist.Name)
		}
	}
	if len(names) == 0 {
		return "未知艺术家"
	}
	return strings.Join(names, " / ")
}

// AlbumName is the album title, with a placeholder when the API omits it.
func (s Song) AlbumName() string {
	if s.Album.Name != "" {
		return s.Album.Name
	}
	return "未知专辑"
}

// Cover is the album picture, empty when the API did not send one.
func (s Song) Cover() string { return s.Album.CoverURL }

// Playable reports whether the song can be streamed. Songs the service marks as
// paid-only, or that have no album, are still listed but cannot be played
// without a subscription.
func (s Song) Playable() bool { return s.ID != 0 }

// Playlist is one of the user's lists, or one the service recommends.
type Playlist struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	CoverURL    string `json:"coverImgUrl"`
	Description string `json:"description"`
	TrackCount  int    `json:"trackCount"`
	PlayCount   int64  `json:"playCount"`
	CreatorID   int64  `json:"userId"`
	Subscribed  bool   `json:"subscribed"`
}

// PlaylistDetail is a playlist together with its songs.
type PlaylistDetail struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	CoverURL    string `json:"coverImgUrl"`
	Description string `json:"description"`
	TrackCount  int    `json:"trackCount"`
	CreatorID   int64  `json:"userId"`
	Tracks      []Song `json:"tracks"`
}

// Account is the signed-in profile.
type Account struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatarUrl"`
}

// Lyric is the words of a song: the original lines and, when it exists, a
// translation to show underneath.
type Lyric struct {
	Original    string `json:"-"`
	Translation string `json:"-"`
}

// Empty reports whether there are no words to show.
func (l Lyric) Empty() bool { return strings.TrimSpace(l.Original) == "" }

// qrStatus is the state of a login QR code.
type qrStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Cookie  string `json:"cookie"`
}

// idList renders song ids the way the player-url endpoint expects them: a JSON
// array inside a string.
func idList(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
