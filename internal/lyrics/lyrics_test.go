package lyrics

import (
	"testing"
	"time"
)

func TestParseAndActive(t *testing.T) {
	doc := "[ti:夜航]\n[offset:500]\n[00:01.00]第一句\n[00:03.50]第二句\n[00:06.00]第三句\n"
	got := Parse(doc)
	if len(got.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(got.Lines))
	}
	if got.Offset != 500*time.Millisecond {
		t.Fatalf("offset = %v, want 500ms", got.Offset)
	}
	if got.Lines[0].Text != "第一句" {
		t.Fatalf("first line = %q", got.Lines[0].Text)
	}
	if want := 1500 * time.Millisecond; got.Lines[0].At != want {
		t.Fatalf("first At = %v, want %v", got.Lines[0].At, want)
	}
	cases := []struct {
		position time.Duration
		want     int
	}{
		{0, -1},
		{1500 * time.Millisecond, 0},
		{4 * time.Second, 1},
		{10 * time.Second, 2},
	}
	for _, tc := range cases {
		if got := Active(got.Lines, tc.position); got != tc.want {
			t.Errorf("Active(%v) = %d, want %d", tc.position, got, tc.want)
		}
	}
}

func TestParseTranslationAndMultipleStamps(t *testing.T) {
	doc := "[00:01.00][00:05.00]副歌\n[00:01.00]chorus\n"
	got := Parse(doc)
	if len(got.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(got.Lines))
	}
	if got.Lines[0].Translation != "chorus" {
		t.Fatalf("translation = %q", got.Lines[0].Translation)
	}
	if got.Lines[0].Text != "副歌" || got.Lines[1].Text != "副歌" {
		t.Fatalf("texts = %q, %q", got.Lines[0].Text, got.Lines[1].Text)
	}
}

func TestParseIgnoresUntimedLines(t *testing.T) {
	got := Parse("just a note\n[ar:someone]\n")
	if !got.Empty() {
		t.Fatalf("expected empty, got %+v", got.Lines)
	}
}
