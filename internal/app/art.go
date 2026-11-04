package app

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sync"

	"github.com/dhowden/tag"
	"github.com/vivaui/viva/ui"

	"soda-music/internal/catalog"
)

// artMaxSide is the longest side a decoded cover is scaled down to. Covers are
// drawn at a few hundred pixels at most, so keeping the full original would only
// cost memory.
const artMaxSide = 512

// artCache decodes the cover art of the tracks that are on screen. Decoding a
// JPEG is far too slow to do while painting, so a worker goroutine does it and
// asks for a repaint when a cover becomes ready. The worker blocks when there is
// nothing to do, so an idle window stays idle.
type artCache struct {
	store      *catalog.Store
	invalidate func()

	mu      sync.Mutex
	entries map[string]*artEntry
	paths   map[string]string
	nextID  uint64
	queue   chan string
	once    sync.Once
	closed  chan struct{}
}

type artEntry struct {
	bitmap  ui.Bitmap
	ready   bool
	failed  bool
	pending bool
}

func newArtCache(store *catalog.Store, invalidate func()) *artCache {
	return &artCache{
		store:      store,
		invalidate: invalidate,
		entries:    make(map[string]*artEntry),
		queue:      make(chan string, 64),
		closed:     make(chan struct{}),
	}
}

// start launches the decode worker.
func (c *artCache) start() {
	c.once.Do(func() { go c.work() })
}

// Bitmap is the cover of a track, or the zero bitmap when there is none yet. A
// call with a track that has art and no cached cover queues the decode.
func (c *artCache) Bitmap(track catalog.Track) ui.Bitmap {
	if !track.Art {
		return ui.Bitmap{}
	}
	c.mu.Lock()
	entry := c.entries[track.ID]
	if entry == nil {
		entry = &artEntry{}
		c.entries[track.ID] = entry
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
		case c.queue <- track.ID:
		default:
			// The queue is full; the next paint asks again.
			c.mu.Lock()
			entry.pending = false
			c.mu.Unlock()
		}
		return ui.Bitmap{}
	}
	c.mu.Unlock()
	return ui.Bitmap{}
}

// Ready reports whether the cover of a track has been decoded. A track without
// art counts as ready, so a card can stop waiting for one.
func (c *artCache) Ready(track catalog.Track) bool {
	if !track.Art {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[track.ID]
	return entry != nil && (entry.ready || entry.failed)
}

func (c *artCache) work() {
	for {
		select {
		case <-c.closed:
			return
		case id := <-c.queue:
			c.decode(id)
		}
	}
}

func (c *artCache) close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

// decode reads the cover of the track with this identity and stores it. The
// extracted bytes are cached on disk, so a second run does not have to parse the
// audio file's tags again.
func (c *artCache) decode(id string) {
	raw, err := os.ReadFile(c.store.ArtPath(id))
	if err != nil {
		raw = nil
	}
	if raw == nil {
		path := c.lookupPath(id)
		if path == "" {
			c.fail(id)
			return
		}
		raw, err = extractArt(path)
		if err != nil || len(raw) == 0 {
			c.fail(id)
			return
		}
		_ = os.MkdirAll(c.store.ArtDir(), 0o755)
		_ = os.WriteFile(c.store.ArtPath(id), raw, 0o644)
	}
	bitmap, err := decodeBitmap(raw)
	if err != nil {
		c.fail(id)
		return
	}
	c.mu.Lock()
	entry := c.entries[id]
	if entry == nil {
		entry = &artEntry{}
		c.entries[id] = entry
	}
	entry.bitmap = bitmap
	entry.ready = true
	entry.pending = false
	c.mu.Unlock()
	if c.invalidate != nil {
		c.invalidate()
	}
}

func (c *artCache) fail(id string) {
	c.mu.Lock()
	entry := c.entries[id]
	if entry == nil {
		entry = &artEntry{}
		c.entries[id] = entry
	}
	entry.failed = true
	entry.pending = false
	c.mu.Unlock()
}

// lookupPath finds the file of a track by identity, from the paths the cache was
// told about. Paths are registered as the library is indexed.
func (c *artCache) lookupPath(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paths[id]
}

// Register tells the cache where a track's file is, so the worker can extract
// its cover. It is called when the library changes.
func (c *artCache) Register(tracks []catalog.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paths == nil {
		c.paths = make(map[string]string, len(tracks))
	}
	for _, track := range tracks {
		if track.Art {
			c.paths[track.ID] = track.Path
		}
	}
}

// extractArt reads the embedded picture of a music file.
func extractArt(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return nil, err
	}
	picture := metadata.Picture()
	if picture == nil || len(picture.Data) == 0 {
		return nil, os.ErrNotExist
	}
	return picture.Data, nil
}

// decodeBitmap turns encoded image bytes into the RGBA buffer the canvas draws,
// scaled down to artMaxSide.
func decodeBitmap(raw []byte) (ui.Bitmap, error) {
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ui.Bitmap{}, err
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return ui.Bitmap{}, os.ErrInvalid
	}
	scale := 1
	for width/scale > artMaxSide || height/scale > artMaxSide {
		scale++
	}
	targetWidth, targetHeight := max(width/scale, 1), max(height/scale, 1)
	picture := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	if scale == 1 {
		draw.Draw(picture, picture.Bounds(), source, bounds.Min, draw.Src)
	} else {
		boxDownscale(picture, source, bounds, scale)
	}
	return ui.Bitmap{
		Pix:    picture.Pix,
		Width:  targetWidth,
		Height: targetHeight,
		Stride: picture.Stride,
	}, nil
}

// boxDownscale averages scale x scale blocks of the source into one pixel.
func boxDownscale(target *image.RGBA, source image.Image, bounds image.Rectangle, scale int) {
	for y := 0; y < target.Rect.Dy(); y++ {
		for x := 0; x < target.Rect.Dx(); x++ {
			var red, green, blue, alpha, count uint32
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px := bounds.Min.X + x*scale + sx
					py := bounds.Min.Y + y*scale + sy
					if px >= bounds.Max.X || py >= bounds.Max.Y {
						continue
					}
					r, g, b, a := source.At(px, py).RGBA()
					red += r >> 8
					green += g >> 8
					blue += b >> 8
					alpha += a >> 8
					count++
				}
			}
			if count == 0 {
				continue
			}
			at := target.PixOffset(x, y)
			target.Pix[at] = uint8(red / count)
			target.Pix[at+1] = uint8(green / count)
			target.Pix[at+2] = uint8(blue / count)
			target.Pix[at+3] = uint8(alpha / count)
		}
	}
}
