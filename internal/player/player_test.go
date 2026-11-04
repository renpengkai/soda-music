package player

import (
	"testing"
	"time"

	"soda-music/internal/catalog"
)

func queueOf(n int) []catalog.Track {
	out := make([]catalog.Track, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, catalog.Track{
			ID:       string(rune('a' + i)),
			Path:     "/missing/" + string(rune('a'+i)) + ".flac",
			Title:    string(rune('a' + i)),
			Duration: time.Minute,
		})
	}
	return out
}

// A player with no audio engine still tracks the queue, which is what the view
// draws; only the sound is missing.
func TestQueueNavigationWithoutEngine(t *testing.T) {
	p := New(nil)
	list := queueOf(3)
	if err := p.SetQueue(list, 0); err == nil {
		t.Fatal("SetQueue with no engine should report the load failure")
	}
	if p.Index() != 0 {
		t.Fatalf("index = %d, want 0", p.Index())
	}
	if cur, ok := p.Current(); !ok || cur.ID != "a" {
		t.Fatalf("current = %+v, ok = %v", cur, ok)
	}

	_ = p.Next()
	if p.Index() != 1 {
		t.Fatalf("after Next index = %d, want 1", p.Index())
	}
	// RepeatOff stops at the end instead of wrapping.
	_ = p.Next()
	_ = p.Next()
	if p.Index() != 2 {
		t.Fatalf("index past the end = %d, want 2", p.Index())
	}

	p.SetRepeat(RepeatAll)
	_ = p.Next()
	if p.Index() != 0 {
		t.Fatalf("RepeatAll wrap index = %d, want 0", p.Index())
	}
}

func TestRemoveKeepsCurrentTrack(t *testing.T) {
	p := New(nil)
	_ = p.SetQueue(queueOf(4), 2)
	p.Remove(0)
	if p.Index() != 1 {
		t.Fatalf("index after removing an earlier track = %d, want 1", p.Index())
	}
	if cur, _ := p.Current(); cur.ID != "c" {
		t.Fatalf("current after remove = %q, want c", cur.ID)
	}
	if len(p.Queue()) != 3 {
		t.Fatalf("queue length = %d, want 3", len(p.Queue()))
	}
}

func TestVolumeClampsAndMute(t *testing.T) {
	p := New(nil)
	p.SetVolume(2)
	if p.Volume() != 1 {
		t.Fatalf("volume = %v, want 1", p.Volume())
	}
	p.SetVolume(-1)
	if p.Volume() != 0 {
		t.Fatalf("volume = %v, want 0", p.Volume())
	}
	p.SetMuted(true)
	if !p.Muted() {
		t.Fatal("muted should be true")
	}
}

func TestInsertAfterAndEnqueue(t *testing.T) {
	p := New(nil)
	_ = p.SetQueue(queueOf(2), 0)
	p.InsertAfter(0, []catalog.Track{{ID: "x", Path: "/missing/x.flac"}})
	got := p.Queue()
	if len(got) != 3 || got[1].ID != "x" {
		t.Fatalf("queue after InsertAfter = %+v", got)
	}
	_ = p.Enqueue([]catalog.Track{{ID: "y", Path: "/missing/y.flac"}}, false)
	if len(p.Queue()) != 4 {
		t.Fatalf("queue after Enqueue = %d", len(p.Queue()))
	}
}
