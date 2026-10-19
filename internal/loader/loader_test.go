package loader

import (
	"bytes"
	"os"
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

// waitDrawn waits until the loader has drawn a frame on w.
func waitDrawn(t *testing.T, w *buf) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if strings.ContainsAny(w.String(), "▀▄") {
			return
		}
	}
	t.Fatal("no frame drawn within 5s")
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
	waitDrawn(t, &w)
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
	waitDrawn(t, &w)
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

func TestInterruptErasesAndExits(t *testing.T) {
	reg, code := make(chan chan<- os.Signal, 1), make(chan int, 1)
	oldNotify, oldStop, oldExit := notify, stopNotify, exit
	notify = func(c chan<- os.Signal) { reg <- c }
	stopNotify = func(chan<- os.Signal) {}
	exit = func(c int) { code <- c }
	defer func() { notify, stopNotify, exit = oldNotify, oldStop, oldExit }()
	var w buf
	l := Start(&w, "fetching", 0)
	intr := <-reg
	waitDrawn(t, &w)
	intr <- os.Interrupt
	if c := <-code; c != 130 {
		t.Fatalf("exit code = %d, want 130", c)
	}
	l.Stop()
	if out := w.String(); !strings.HasSuffix(out, erase) || strings.Count(out[strings.LastIndex(out, "▀"):], erase) != 1 {
		t.Fatalf("frame not erased exactly once after the interrupt: %q", out)
	}
}
