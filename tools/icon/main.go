//go:build ignore

// Command icon draws assets/icon.png, the application tile: a rounded square in
// the 汽水音乐 mint, with the two-note mark of a music player on it.
//
//	go run tools/icon/main.go
//
// It is a tool, not part of the program: the build tag keeps it out of the
// binary and out of go build ./...
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	size   = 512
	scale  = 4 // supersampling factor
	radius = 116.0
	corner = radius * scale
	canvas = size * scale
)

func main() {
	big := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	// The tile: a rounded square with a diagonal mint gradient.
	for y := 0; y < canvas; y++ {
		for x := 0; x < canvas; x++ {
			if !insideRounded(float64(x)+0.5, float64(y)+0.5, 0, 0, canvas, canvas, corner) {
				continue
			}
			t := (float64(x)/canvas + float64(y)/canvas) / 2
			big.Set(x, y, gradient(t))
		}
	}
	drawNote(big)
	out := downsample(big, scale)

	file, err := os.Create("assets/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, out); err != nil {
		log.Fatal(err)
	}
}

// gradient is the brand ramp: a bright mint at the top left to a deeper teal at
// the bottom right.
func gradient(t float64) color.RGBA {
	from := [3]float64{0x2A, 0xF5, 0xD0}
	to := [3]float64{0x00, 0x9E, 0x86}
	mix := func(a, b float64) uint8 { return uint8(math.Round(a + (b-a)*t)) }
	return color.RGBA{mix(from[0], to[0]), mix(from[1], to[1]), mix(from[2], to[2]), 255}
}

// insideRounded reports whether a point is inside a rounded rectangle.
func insideRounded(x, y, left, top, width, height, radius float64) bool {
	if x < left || x > left+width || y < top || y > top+height {
		return false
	}
	cx := math.Min(math.Max(x, left+radius), left+width-radius)
	cy := math.Min(math.Max(y, top+radius), top+height-radius)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

// drawNote fills the two-note mark in white.
func drawNote(picture *image.RGBA) {
	ink := color.RGBA{255, 255, 255, 255}
	w := float64(canvas)
	// Two note heads.
	ellipse(picture, 0.355*w, 0.690*w, 0.108*w, 0.082*w, ink)
	ellipse(picture, 0.680*w, 0.635*w, 0.108*w, 0.082*w, ink)
	// Two stems.
	rect(picture, 0.440*w, 0.300*w, 0.032*w, 0.400*w, ink)
	rect(picture, 0.765*w, 0.245*w, 0.032*w, 0.400*w, ink)
	// The beam that joins them.
	quad(picture,
		[2]float64{0.440 * w, 0.300 * w},
		[2]float64{0.797 * w, 0.245 * w},
		[2]float64{0.797 * w, 0.335 * w},
		[2]float64{0.440 * w, 0.390 * w},
		ink)
}

func ellipse(picture *image.RGBA, cx, cy, rx, ry float64, ink color.RGBA) {
	for y := int(cy - ry); y <= int(cy+ry)+1; y++ {
		for x := int(cx - rx); x <= int(cx+rx)+1; x++ {
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			if dx*dx+dy*dy <= 1 {
				set(picture, x, y, ink)
			}
		}
	}
}

func rect(picture *image.RGBA, x, y, width, height float64, ink color.RGBA) {
	for py := int(y); py < int(y+height)+1; py++ {
		for px := int(x); px < int(x+width)+1; px++ {
			if float64(px)+0.5 >= x && float64(px)+0.5 <= x+width && float64(py)+0.5 >= y && float64(py)+0.5 <= y+height {
				set(picture, px, py, ink)
			}
		}
	}
}

// quad fills a convex quadrilateral by testing each pixel against its edges.
func quad(picture *image.RGBA, a, b, c, d [2]float64, ink color.RGBA) {
	points := [][2]float64{a, b, c, d}
	minX, minY := a[0], a[1]
	maxX, maxY := a[0], a[1]
	for _, p := range points {
		minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
		maxX, maxY = math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	for y := int(minY); y <= int(maxY)+1; y++ {
		for x := int(minX); x <= int(maxX)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			inside := true
			for i := range points {
				next := points[(i+1)%len(points)]
				if cross(points[i], next, [2]float64{px, py}) < 0 {
					inside = false
					break
				}
			}
			if inside {
				set(picture, x, y, ink)
			}
		}
	}
}

func cross(a, b, p [2]float64) float64 {
	return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])
}

func set(picture *image.RGBA, x, y int, ink color.RGBA) {
	if x < 0 || y < 0 || x >= canvas || y >= canvas {
		return
	}
	picture.Set(x, y, ink)
}

// downsample averages each scale x scale block of the supersampled picture.
func downsample(picture *image.RGBA, factor int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var red, green, blue, alpha, count uint32
			for sy := 0; sy < factor; sy++ {
				for sx := 0; sx < factor; sx++ {
					r, g, b, a := picture.At(x*factor+sx, y*factor+sy).RGBA()
					red += r >> 8
					green += g >> 8
					blue += b >> 8
					alpha += a >> 8
					count++
				}
			}
			out.Set(x, y, color.RGBA{uint8(red / count), uint8(green / count), uint8(blue / count), uint8(alpha / count)})
		}
	}
	return out
}
