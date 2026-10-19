package loader

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// buf is a bytes.Buffer safe for the loader goroutine and the test to share.
type buf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestQuickStopDrawsNothing(t *testing.T) {
	var w buf
	l := Start(&w, "fetching", time.Hour)
	l.Stop()
	if w.String() != "" {
		t.Fatalf("drew %q before the delay", w.String())
	}
}

func TestDrawsTwoRowsThenErases(t *testing.T) {
	var w buf
	l := Start(&w, "cali is fetching skills…", 0)
	time.Sleep(3 * tickEvery)
	l.Stop()
	out := w.String()
	if !strings.Contains(out, "cali is fetching skills…") || !strings.ContainsAny(out, "▀▄") {
		t.Fatalf("no sprite or message in %q", out)
	}
	if !strings.HasSuffix(out, erase) {
		t.Fatalf("output does not end erased: %q", out)
	}
	l.Stop() // a second Stop is a no-op
}

func TestWriteClearsFrameFirst(t *testing.T) {
	var w buf
	l := Start(&w, "fetching", 0)
	time.Sleep(2 * tickEvery)
	if _, err := l.Write([]byte("warning: x\n")); err != nil {
		t.Fatal(err)
	}
	l.Stop()
	if !strings.Contains(w.String(), erase+"warning: x\n") {
		t.Fatalf("warning not written on a cleared frame: %q", w.String())
	}
}

func TestFrameWalksAndSnows(t *testing.T) {
	var s state
	first := s.rows("m")
	for i := 0; i < 12; i++ {
		s.advance()
	}
	later := s.rows("m")
	if first == later {
		t.Fatal("frame did not change as she walked")
	}
	if !strings.ContainsAny(later[0]+later[1], "*·") {
		t.Fatalf("no snow behind her: %q", later)
	}
	for i := 0; i < 200; i++ {
		s.advance()
		if s.x > track {
			t.Fatalf("x = %d walked past the %d-column track", s.x, track)
		}
	}
}
