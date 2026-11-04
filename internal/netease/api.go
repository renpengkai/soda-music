package netease

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Account returns the signed-in profile, or nil when the cookie is not a
// session.
func (c *Client) Account() (*Account, error) {
	raw, err := c.call("/api/w/nuser/account/get", nil)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code    int      `json:"code"`
		Account *Account `json:"account"`
		Profile *Account `json:"profile"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Profile != nil && reply.Profile.ID != 0 {
		return reply.Profile, nil
	}
	if reply.Account != nil && reply.Account.ID != 0 {
		return reply.Account, nil
	}
	return nil, nil
}

// UserPlaylists returns the playlists the user created or collected.
func (c *Client) UserPlaylists(uid int64, limit int) ([]Playlist, error) {
	if limit <= 0 {
		limit = 100
	}
	raw, err := c.call("/api/user/playlist", map[string]any{
		"uid": uid, "limit": limit, "offset": 0, "includeVideo": true,
	})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code     int        `json:"code"`
		Playlist []Playlist `json:"playlist"`
		More     bool       `json:"more"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Code != 200 {
		return nil, fmt.Errorf("netease: 无法获取歌单（%d）", reply.Code)
	}
	return reply.Playlist, nil
}

// PlaylistDetail returns a playlist with its songs.
func (c *Client) PlaylistDetail(id int64) (*PlaylistDetail, error) {
	raw, err := c.call("/api/v6/playlist/detail", map[string]any{
		"id": strconv.FormatInt(id, 10), "n": 1000, "s": 8,
	})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code     int             `json:"code"`
		Playlist *PlaylistDetail `json:"playlist"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Code != 200 || reply.Playlist == nil {
		return nil, fmt.Errorf("netease: 无法获取歌单内容（%d）", reply.Code)
	}
	return reply.Playlist, nil
}

// SongDetail fills in the album art and length the search endpoint leaves out.
func (c *Client) SongDetail(ids []int64) ([]Song, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	entries := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": id})
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	raw, err := c.call("/api/v3/song/detail", map[string]any{"c": string(encoded)})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code  int    `json:"code"`
		Songs []Song `json:"songs"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	return reply.Songs, nil
}

// Search looks for songs, albums or artists. kind is 1 for songs, 10 for
// albums and 100 for artists.
func (c *Client) Search(keyword string, kind, limit int) ([]Song, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 30
	}
	raw, err := c.call("/api/search/get", map[string]any{
		"s": keyword, "type": kind, "limit": limit, "offset": 0,
	})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code   int `json:"code"`
		Result struct {
			Songs []Song `json:"songs"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Code != 200 {
		return nil, fmt.Errorf("netease: 搜索失败（%d）", reply.Code)
	}
	return reply.Result.Songs, nil
}

// SongURLs resolves the stream address of each song. Songs that need a
// subscription, or that the service has taken down, are simply absent from the
// result. level is one of "standard", "higher", "exhigh", "lossless".
func (c *Client) SongURLs(ids []int64, level string) (map[int64]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if level == "" {
		level = "exhigh"
	}
	// Try the requested quality first, then fall back to lower tiers. A free
	// account often gets nothing at "exhigh" but a working URL at "standard".
	levels := []string{level}
	for _, fallback := range []string{"exhigh", "higher", "standard"} {
		if fallback != level {
			levels = append(levels, fallback)
		}
	}
	var lastErr error
	for _, lv := range levels {
		urls, err := c.songURLsAt(ids, lv)
		if err != nil {
			lastErr = err
			continue
		}
		if len(urls) > 0 {
			return urls, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("netease: 没有可播放的地址，可能需要会员或已下架")
}

// songURLsAt queries the player URL endpoint for a single quality level.
func (c *Client) songURLsAt(ids []int64, level string) (map[int64]string, error) {
	raw, err := c.call("/api/song/enhance/player/url/v1", map[string]any{
		"ids": idList(ids), "level": level, "encodeType": "mp3",
	})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code int `json:"code"`
		Data []struct {
			ID  int64  `json:"id"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	urls := make(map[int64]string, len(reply.Data))
	for _, item := range reply.Data {
		if item.URL != "" {
			urls[item.ID] = item.URL
		}
	}
	return urls, nil
}

// Lyrics returns the words of a song.
func (c *Client) Lyrics(id int64) (Lyric, error) {
	raw, err := c.call("/api/song/lyric", map[string]any{"id": id, "lv": -1, "tv": -1})
	if err != nil {
		return Lyric{}, err
	}
	var reply struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		TLRC struct {
			Lyric string `json:"lyric"`
		} `json:"tlyric"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return Lyric{}, err
	}
	return Lyric{Original: reply.LRC.Lyric, Translation: reply.TLRC.Lyric}, nil
}

// DailySongs returns the songs the service picked for the user today.
func (c *Client) DailySongs() ([]Song, error) {
	raw, err := c.call("/api/v3/discovery/recommend/songs", map[string]any{})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code int `json:"code"`
		Data struct {
			DailySongs []Song `json:"dailySongs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Code != 200 {
		return nil, fmt.Errorf("netease: 无法获取每日推荐（%d）", reply.Code)
	}
	return reply.Data.DailySongs, nil
}
