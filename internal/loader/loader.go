// Package loader draws the wait animation of slow commands in one of two
// styles. Snowdrift is one line of plain ASCII redrawn with a carriage return,
// so it works on any terminal. Princess is a 3x4-pixel ice princess walking
// across two terminal rows, leaving a snow trail, beside a spinner and a
// message; it uses half-block glyphs, 24-bit color and cursor movement, so the
// caller shows it only on a color terminal.
package loader

import (
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Style is the look of the loader.
type Style int

const (
	Snowdrift Style = iota
	Princess
)

const (
	tickEvery = 90 * time.Millisecond
	track     = 12 // columns she walks before starting over
	// erase clears both Princess rows and leaves the cursor at the start of the first.
	erase      = "\r\x1b[2K\n\x1b[2K\x1b[1A\r"
	driftTrack = 7 // cells the Snowdrift flake bounces across
)

// Colors as "r;g;b" for the 24-bit SGR sequences.
const (
	ice     = "133;183;235"
	snowNew = "230;241;251"
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

// notify, stopNotify and exit are signal.Notify, signal.Stop and os.Exit;
// tests replace them.
var (
	notify     = func(c chan<- os.Signal) { signal.Notify(c, os.Interrupt) }
	stopNotify = signal.Stop
	exit       = os.Exit
)

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

// Width is the column count the loader needs for msg in style: for Princess
// the walking track, the sprite, the spinner and msg on the second row; for
// Snowdrift the bracketed track and msg spelled in ASCII.
func Width(msg string, style Style) int {
	if style == Snowdrift {
		return driftTrack + 3 + len(ascii(msg))
	}
	return track + 6 + utf8.RuneCountInString(msg)
}

// ascii spells the ellipsis of msg as three dots, for terminals without Unicode.
func ascii(msg string) string { return strings.ReplaceAll(msg, "…", "...") }

// drift is the Snowdrift animation: a flake bouncing in a bracketed track,
// a dot in the cell it just left.
type drift struct {
	tick int
	msg  string
}

// at is the flake's cell at tick t, going back and forth between the track ends.
func at(t int) int {
	m := t % (2*driftTrack - 2)
	if m < driftTrack {
		return m
	}
	return 2*driftTrack - 2 - m
}

func (d *drift) line() string {
	cells := []byte(strings.Repeat(" ", driftTrack))
	if d.tick > 0 {
		cells[at(d.tick-1)] = '.'
	}
	cells[at(d.tick)] = '*'
	return "[" + string(cells) + "] " + d.msg
}

func (d *drift) frame() string { return "\r" + d.line() }
func (d *drift) advance()      { d.tick++ }

// princess adapts state to the frame and advance of an animation.
type princess struct {
	s   state
	msg string
}

func (p *princess) frame() string {
	rows := p.s.rows(p.msg)
	return "\r\x1b[2K" + rows[0] + "\n\x1b[2K" + rows[1] + "\x1b[1A\r"
}
func (p *princess) advance() { p.s.advance() }

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
						glyph = fg(ice) + "·\x1b[0m"
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
	wipe  string // clears a drawn frame and returns the cursor to its start
	drawn bool
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

// Start draws msg and the animation in style on w every tick, beginning after
// delay so a quick command never flickers, until Stop.
func Start(w io.Writer, msg string, delay time.Duration, style Style) *Loader {
	l := &Loader{w: w, wipe: erase, done: make(chan struct{})}
	var a interface {
		frame() string
		advance()
	} = &princess{msg: msg}
	if style == Snowdrift {
		a = &drift{msg: ascii(msg)}
		l.wipe = "\r" + strings.Repeat(" ", Width(msg, Snowdrift)) + "\r"
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		select {
		case <-l.done:
			return
		case <-time.After(delay):
		}
		// Ctrl-C while a frame is drawn: erase it, then exit as the default handler would.
		intr := make(chan os.Signal, 1)
		notify(intr)
		defer stopNotify(intr)
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			l.mu.Lock()
			_, _ = io.WriteString(l.w, a.frame())
			l.drawn = true
			l.mu.Unlock()
			a.advance()
			select {
			case <-l.done:
				return
			case <-intr:
				l.mu.Lock()
				_, _ = io.WriteString(l.w, l.wipe)
				l.drawn = false
				l.mu.Unlock()
				exit(130)
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
		_, _ = io.WriteString(l.w, l.wipe)
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
			_, _ = io.WriteString(l.w, l.wipe)
		}
	})
}
