// Package loader draws the wait animation of slow commands: a 3x4-pixel ice
// princess walking across two terminal rows, leaving a snow trail, beside a
// spinner and a message. It uses half-block glyphs and 24-bit color, so the
// caller shows it only on a color terminal.
package loader

import (
	"io"
	"strings"
	"sync"
	"time"
)

const (
	tickEvery = 90 * time.Millisecond
	track     = 12 // columns she walks before starting over
	// erase clears both rows and leaves the cursor at the start of the first.
	erase = "\r\x1b[2K\n\x1b[2K\x1b[1A\r"
)

// Colors as "r;g;b" for the 24-bit SGR sequences.
const (
	ice     = "133;183;235"
	snowNew = "230;241;251"
	snowOld = ice
	spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
)

var palette = map[byte]string{
	'h': "250;238;218", // platinum braid
	's': "245;196;179", // skin
	'd': "55;138;221",  // gown
	'c': "181;212;244", // cape
}

// frames are the two walking poses, 3 pixels wide and 4 tall; two pixel rows
// make one terminal row.
var frames = [2][4]string{
	{".sh", "cdh", "cdd", "cdd"},
	{".sh", "cdh", "cdd", "dd."},
}

type flake struct{ x, row, age int }

// state is the animation; advance moves it one tick.
type state struct {
	tick, x int
	flakes  []flake
}

func (s *state) advance() {
	s.tick++
	for i := range s.flakes {
		s.flakes[i].age++
	}
	kept := s.flakes[:0]
	for _, f := range s.flakes {
		if f.age < 10 {
			kept = append(kept, f)
		}
	}
	s.flakes = kept
	if s.tick%2 != 0 {
		return
	}
	if s.x > 1 {
		s.flakes = append(s.flakes, flake{x: s.x - 1, row: (s.tick / 2) % 2})
	}
	if s.x = (s.x + 1) % (track + 1); s.x == 0 {
		s.flakes = nil
	}
}

func fg(c string) string { return "\x1b[38;2;" + c + "m" }

// cell renders a top and a bottom pixel ('.' is empty) as one glyph.
func cell(top, bottom byte) string {
	t, b := palette[top], palette[bottom]
	switch {
	case t != "" && b != "":
		return fg(t) + "\x1b[48;2;" + b + "m▀\x1b[0m"
	case t != "":
		return fg(t) + "▀\x1b[0m"
	case b != "":
		return fg(b) + "▄\x1b[0m"
	}
	return " "
}

// rows renders the two terminal rows of the current frame.
func (s *state) rows(msg string) [2]string {
	pose := frames[(s.tick/2)%2]
	var out [2]string
	for r := 0; r < 2; r++ {
		var b strings.Builder
		for i := 0; i < s.x; i++ {
			glyph := " "
			for _, f := range s.flakes {
				if f.x == i && f.row == r {
					if f.age < 5 {
						glyph = fg(snowNew) + "*\x1b[0m"
					} else {
						glyph = fg(snowOld) + "·\x1b[0m"
					}
				}
			}
			b.WriteString(glyph)
		}
		for c := 0; c < 3; c++ {
			b.WriteString(cell(pose[r*2][c], pose[r*2+1][c]))
		}
		out[r] = b.String()
	}
	spin := []rune(spinner)
	out[1] += strings.Repeat(" ", track-s.x+1) + fg(ice) + string(spin[s.tick%len(spin)]) + "\x1b[0m " + msg
	return out
}

// Loader is a running animation. Write through it while it runs so a line
// of output lands on a cleared frame instead of inside one.
type Loader struct {
	mu    sync.Mutex
	w     io.Writer
	drawn bool
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

// Start draws msg and the animation on w every tick, beginning after delay so
// a quick command never flickers, until Stop.
func Start(w io.Writer, msg string, delay time.Duration) *Loader {
	l := &Loader{w: w, done: make(chan struct{})}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		select {
		case <-l.done:
			return
		case <-time.After(delay):
		}
		var s state
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			l.mu.Lock()
			rows := s.rows(msg)
			_, _ = io.WriteString(l.w, "\r\x1b[2K"+rows[0]+"\n\x1b[2K"+rows[1]+"\x1b[1A\r")
			l.drawn = true
			l.mu.Unlock()
			s.advance()
			select {
			case <-l.done:
				return
			case <-t.C:
			}
		}
	}()
	return l
}

// Write clears the frame, then writes p; the next tick redraws below it.
func (l *Loader) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.drawn {
		_, _ = io.WriteString(l.w, erase)
		l.drawn = false
	}
	return l.w.Write(p)
}

// Stop ends the animation and erases it. Calling it again does nothing.
func (l *Loader) Stop() {
	l.once.Do(func() {
		close(l.done)
		l.wg.Wait()
		if l.drawn {
			_, _ = io.WriteString(l.w, erase)
		}
	})
}
