package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vivaui/viva/ui"

	"soda-music/assets"
)

// paintCover draws a cover picture inside a rounded rectangle. The canvas has no
// rounded image clip, so the picture is composed from one-pixel horizontal
// strips, each as wide as the rounded rectangle is at that row - the same
// technique the framework uses for a round avatar. A track without art gets a
// tinted tile with a music note instead.
func paintCover(canvas *ui.Canvas, theme ui.Theme, bounds ui.Rect, bitmap ui.Bitmap, radius float32) {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	if bitmap.Width <= 0 || bitmap.Height <= 0 {
		paintCoverPlaceholder(canvas, theme, bounds, radius)
		return
	}
	canvas.Rounded(bounds, radius, theme.Muted)
	side := float32(min(bitmap.Width, bitmap.Height))
	source := ui.Rect{
		X:      (float32(bitmap.Width) - side) / 2,
		Y:      (float32(bitmap.Height) - side) / 2,
		Width:  side,
		Height: side,
	}
	// The picture is center-cropped to a square and then scaled to the tile.
	scale := side / min(bounds.Width, bounds.Height)
	height := bounds.Height
	for top := float32(0); top < height; top++ {
		rowHeight := float32(1)
		if height-top < 1 {
			rowHeight = height - top
		}
		left := roundedInset(top+rowHeight/2, height, radius)
		right := roundedInset(height-(top+rowHeight/2), height, radius)
		width := bounds.Width - left - right
		if width <= 0 {
			continue
		}
		canvas.DrawBitmapRegion(bitmap,
			ui.Rect{
				X:      source.X + left*scale,
				Y:      source.Y + top*scale,
				Width:  width * scale,
				Height: rowHeight * scale,
			},
			ui.Rect{
				X:      bounds.X + left,
				Y:      bounds.Y + top,
				Width:  width,
				Height: rowHeight,
			})
	}
}

// roundedInset is how far the rounded rectangle's edge is from the straight edge
// at distance d from the nearer end.
func roundedInset(d, height, radius float32) float32 {
	if radius <= 0 {
		return 0
	}
	if radius > height/2 {
		radius = height / 2
	}
	if d >= radius && d <= height-radius {
		return 0
	}
	var dy float32
	if d < radius {
		dy = radius - d
	} else {
		dy = d - (height - radius)
	}
	chord := radius*radius - dy*dy
	if chord <= 0 {
		return radius
	}
	return radius - float32(math.Sqrt(float64(chord)))
}

// paintCoverPlaceholder is the tile shown for a track with no embedded art.
func paintCoverPlaceholder(canvas *ui.Canvas, theme ui.Theme, bounds ui.Rect, radius float32) {
	canvas.Rounded(bounds, radius, theme.Muted)
	side := min(bounds.Width, bounds.Height)
	icon := side * 0.42
	area := ui.Rect{
		X:      bounds.X + (bounds.Width-icon)/2,
		Y:      bounds.Y + (bounds.Height-icon)/2,
		Width:  icon,
		Height: icon,
	}
	tint := theme.MutedForeground
	tint.Alpha *= 0.7
	canvas.DrawIcon(assets.Music, area, tint)
}

// formatDuration writes a length the way a track list does: m:ss, or h:mm:ss
// when it is an hour or more.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second) / time.Second)
	hours := total / 3600
	minutes := total % 3600 / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// formatTotal writes a total length for a header, such as "1 小时 12 分钟".
func formatTotal(d time.Duration) string {
	if d <= 0 {
		return "0 分钟"
	}
	minutes := int(d.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	hours := minutes / 60
	rest := minutes % 60
	if rest == 0 {
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, rest)
}

// formatCount writes a count with a thousands separator.
func formatCount(n int) string {
	text := fmt.Sprintf("%d", n)
	if len(text) <= 3 {
		return text
	}
	var out strings.Builder
	lead := len(text) % 3
	if lead > 0 {
		out.WriteString(text[:lead])
	}
	for i := lead; i < len(text); i += 3 {
		if out.Len() > 0 {
			out.WriteString(",")
		}
		out.WriteString(text[i : i+3])
	}
	return out.String()
}

// plural picks between a singular and a plural word for a count.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
