// Package lyrics reads the .lrc lyric files that ship beside a music file and
// pairs the lines with the moment they should be shown.
package lyrics

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Line is one lyric line and the moment it becomes the current one.
type Line struct {
	At   time.Duration
	Text string
	// Translation is the same line in another language, when the file carries
	// one at the same timestamp.
	Translation string
}

// Lyrics is a parsed lyric file.
type Lyrics struct {
	Lines []Line
	// Offset shifts every timestamp, from the file's [offset:] tag.
	Offset time.Duration
}

// Empty reports whether there is nothing to show.
func (l Lyrics) Empty() bool { return len(l.Lines) == 0 }

// Parse reads an LRC document. It accepts the common forms of a timestamp -
// [mm:ss], [mm:ss.xx], [mm:ss.xxx] and [mm:ss:xx] - several tags on one line,
// and the [offset:] and [ti:]/[ar:] metadata tags. Lines that carry no
// timestamp are ignored, because they are usually metadata or a translation
// block the file could not anchor.
func Parse(text string) Lyrics {
	var out Lyrics
	seen := make(map[time.Duration]int)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if tag, value, ok := metadata(line); ok {
				if tag == "offset" {
					if ms, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
						out.Offset = time.Duration(ms) * time.Millisecond
					}
				}
				continue
			}
		}
		stamps, rest := stamps(line)
		if len(stamps) == 0 {
			continue
		}
		rest = strings.TrimSpace(rest)
		for _, at := range stamps {
			if index, ok := seen[at]; ok {
				// The same timestamp twice is a translation: keep it beside the
				// original instead of adding a second line.
				if out.Lines[index].Translation == "" && rest != "" && rest != out.Lines[index].Text {
					out.Lines[index].Translation = rest
				}
				continue
			}
			seen[at] = len(out.Lines)
			out.Lines = append(out.Lines, Line{At: at, Text: rest})
		}
	}
	sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[i].At < out.Lines[j].At })
	if out.Offset != 0 {
		for i := range out.Lines {
			out.Lines[i].At += out.Offset
			if out.Lines[i].At < 0 {
				out.Lines[i].At = 0
			}
		}
	}
	return out
}

// metadata reads a [tag:value] line.
func metadata(line string) (tag, value string, ok bool) {
	end := strings.IndexByte(line, ']')
	if end < 0 {
		return "", "", false
	}
	body := line[1:end]
	colon := strings.IndexByte(body, ':')
	if colon < 0 {
		return "", "", false
	}
	tag = strings.ToLower(strings.TrimSpace(body[:colon]))
	value = body[colon+1:]
	switch tag {
	case "offset", "ti", "ar", "al", "by", "re", "ve", "length":
		return tag, value, true
	}
	return "", "", false
}

// stamps pulls every leading [mm:ss(.xx)] timestamp off a line and returns the
// rest of it.
func stamps(line string) ([]time.Duration, string) {
	var out []time.Duration
	for strings.HasPrefix(line, "[") {
		end := strings.IndexByte(line, ']')
		if end < 0 {
			break
		}
		at, ok := parseStamp(line[1:end])
		if !ok {
			break
		}
		out = append(out, at)
		line = line[end+1:]
	}
	return out, line
}

// parseStamp reads mm:ss, mm:ss.xx, mm:ss.xxx or mm:ss:xx.
func parseStamp(body string) (time.Duration, bool) {
	body = strings.TrimSpace(body)
	colon := strings.LastIndexByte(body, ':')
	if colon < 0 {
		return 0, false
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(body[:colon]))
	if err != nil {
		return 0, false
	}
	secondsField := strings.TrimSpace(body[colon+1:])
	whole, fraction := secondsField, ""
	if dot := strings.IndexAny(secondsField, ".:"); dot >= 0 {
		whole, fraction = secondsField[:dot], secondsField[dot+1:]
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(whole))
	if err != nil {
		return 0, false
	}
	var millis int
	if fraction != "" {
		digits := fraction
		if len(digits) > 3 {
			digits = digits[:3]
		}
		for len(digits) < 3 {
			digits += "0"
		}
		millis, _ = strconv.Atoi(digits)
	}
	return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second + time.Duration(millis)*time.Millisecond, true
}

// Active returns the index of the line that should be highlighted at position,
// or -1 before the first line.
func Active(lines []Line, position time.Duration) int {
	if len(lines) == 0 || position < lines[0].At {
		return -1
	}
	index := sort.Search(len(lines), func(i int) bool { return lines[i].At > position }) - 1
	if index < 0 {
		return -1
	}
	return index
}

// Load reads the lyric file beside path: song.lrc first, then song.LRC. It
// returns an empty Lyrics when there is none.
func Load(path string) Lyrics {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	for _, candidate := range []string{base + ".lrc", base + ".LRC"} {
		raw, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return Parse(string(raw))
	}
	return Lyrics{}
}
