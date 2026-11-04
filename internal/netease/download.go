package netease

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mediaClient fetches song files and cover art. It has no timeout on the whole
// request - a song can take a while - but it does cap how long a stalled
// connection may sit idle.
var mediaClient = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	},
}

// Download streams a URL to path, writing through a temporary file so a failed
// transfer never leaves a half-written song behind.
func Download(rawURL, path string) error {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", referer)

	response, err := mediaClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &HTTPError{URL: rawURL, Status: response.StatusCode}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".part"
	file, err := os.Create(temp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		file.Close()
		os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	return os.Rename(temp, path)
}

// Fetch reads a small resource, such as a cover picture, into memory.
func Fetch(rawURL string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", referer)
	response, err := mediaClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &HTTPError{URL: rawURL, Status: response.StatusCode}
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}

// HTTPError is a response that was not 200.
type HTTPError struct {
	URL    string
	Status int
}

func (e *HTTPError) Error() string {
	return "netease: " + e.URL + " 返回 " + http.StatusText(e.Status)
}

// Extension guesses a file extension from a media URL, so a downloaded song is
// decoded with the right parser. The service serves MP3 and FLAC.
func Extension(rawURL string) string {
	lower := strings.ToLower(rawURL)
	if index := strings.IndexAny(lower, "?#"); index >= 0 {
		lower = lower[:index]
	}
	switch {
	case strings.HasSuffix(lower, ".flac"):
		return ".flac"
	case strings.HasSuffix(lower, ".wav"):
		return ".wav"
	default:
		return ".mp3"
	}
}
