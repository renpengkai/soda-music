package app

import (
	"net/http"
	"sync"
	"time"

	"github.com/vivaui/viva/ui"

	"soda-music/internal/netease"
)

// imageCache decodes the pictures that come from the online service - album
// covers and the profile avatar - the same way artCache handles embedded art:
// a worker does the network and JPEG work, and the paint only ever reads a
// ready bitmap.
type imageCache struct {
	invalidate func()

	mu      sync.Mutex
	entries map[string]*imageEntry
	queue   chan string
	once    sync.Once
	closed  chan struct{}

	http *http.Client
}

type imageEntry struct {
	bitmap  ui.Bitmap
	ready   bool
	failed  bool
	pending bool
}

func newImageCache(invalidate func()) *imageCache {
	return &imageCache{
		invalidate: invalidate,
		entries:    make(map[string]*imageEntry),
		queue:      make(chan string, 64),
		closed:     make(chan struct{}),
		http:       &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *imageCache) start() {
	c.once.Do(func() { go c.work() })
}

// Bitmap is the picture at url, or the zero bitmap while it is still being
// fetched. An empty url is a miss, not a request.
func (c *imageCache) Bitmap(url string) ui.Bitmap {
	if url == "" {
		return ui.Bitmap{}
	}
	c.mu.Lock()
	entry := c.entries[url]
	if entry == nil {
		entry = &imageEntry{}
		c.entries[url] = entry
	}
	if entry.ready {
		bitmap := entry.bitmap
		c.mu.Unlock()
		return bitmap
	}
	if !entry.pending && !entry.failed {
		entry.pending = true
		c.mu.Unlock()
		select {
		case c.queue <- url:
		default:
			c.mu.Lock()
			entry.pending = false
			c.mu.Unlock()
		}
		return ui.Bitmap{}
	}
	c.mu.Unlock()
	return ui.Bitmap{}
}

// Cached reports whether a picture has already been decoded, so a card can stop
// waiting for one.
func (c *imageCache) Cached(url string) bool {
	if url == "" {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[url]
	return entry != nil && (entry.ready || entry.failed)
}

func (c *imageCache) work() {
	for {
		select {
		case <-c.closed:
			return
		case url := <-c.queue:
			c.decode(url)
		}
	}
}

func (c *imageCache) close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

func (c *imageCache) decode(url string) {
	raw, err := netease.Fetch(url)
	if err != nil {
		c.fail(url)
		return
	}
	bitmap, err := decodeBitmap(raw)
	if err != nil {
		c.fail(url)
		return
	}
	c.mu.Lock()
	entry := c.entries[url]
	if entry == nil {
		entry = &imageEntry{}
		c.entries[url] = entry
	}
	entry.bitmap = bitmap
	entry.ready = true
	entry.pending = false
	c.mu.Unlock()
	if c.invalidate != nil {
		c.invalidate()
	}
}

func (c *imageCache) fail(url string) {
	c.mu.Lock()
	entry := c.entries[url]
	if entry == nil {
		entry = &imageEntry{}
		c.entries[url] = entry
	}
	entry.failed = true
	entry.pending = false
	c.mu.Unlock()
}
