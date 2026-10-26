package mascot

import (
	"regexp"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func opts() Options { return Options{Profile: termenv.TrueColor, Dark: true} }

func TestFrameSizeAndCrop(t *testing.T) {
	f := Static(Idle, .3)
	full := f.Lines(opts())
	if len(full) != Rows {
		t.Fatalf("rows = %d, want %d", len(full), Rows)
	}
	for _, l := range full {
		if got := len([]rune(plain(l))); got != Cols {
			t.Fatalf("full width = %d, want %d", got, Cols)
		}
	}
	o := opts()
	o.Crop = true
	for _, l := range f.Lines(o) {
		if got := len([]rune(plain(l))); got != CropCols {
			t.Fatalf("crop width = %d, want %d", got, CropCols)
		}
	}
}

// The cat must be symmetric when it looks straight ahead; an asymmetric head
// was the first thing that looked wrong in review.
func TestIdleCatIsSymmetric(t *testing.T) {
	f := Static(Idle, 0)
	for y := 0; y < H; y++ {
		for x := cropX0; x < cropX0+CropCols/2; x++ {
			if f.Visible(x, y) != f.Visible(W-1-x, y) {
				t.Fatalf("pixel (%d,%d) differs from its mirror", x, y)
			}
		}
	}
}

func TestEveryMoodDrawsAndIsDeterministic(t *testing.T) {
	for _, m := range []Mood{Idle, Busy, Happy, Error, Sleep} {
		a := strings.Join(Static(m, 1.3).Lines(opts()), "\n")
		b := strings.Join(Static(m, 1.3).Lines(opts()), "\n")
		if a != b {
			t.Fatalf("%s: two draws of the same instant differ", m)
		}
		if strings.TrimSpace(plain(a)) == "" {
			t.Fatalf("%s: nothing drawn", m)
		}
	}
}

func TestBusyFilesFlyHigherThanTheEars(t *testing.T) {
	// At mid-flight a file is above the cat's head; find a busy instant whose
	// top rows hold a drawn pixel that is not part of the cat.
	found := false
	for ts := 0.0; ts < 4 && !found; ts += .05 {
		f := Static(Busy, ts)
		for x := 0; x < W && !found; x++ {
			for y := 0; y < 6; y++ {
				if f.Visible(x, y) {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("no file ever crossed the area above the ears")
	}
}

func TestAnimEasesInsteadOfSnapping(t *testing.T) {
	a := NewAnim()
	first := strings.Join(a.Step(Idle, .07).Lines(opts()), "\n")
	var changed bool
	for i := 0; i < 30; i++ {
		if strings.Join(a.Step(Happy, .07).Lines(opts()), "\n") != first {
			changed = true
		}
	}
	if !changed {
		t.Fatal("happy mood never changed the picture")
	}
	// A huge dt (suspended terminal) must not blow the springs up.
	for i := 0; i < 5; i++ {
		a.Step(Busy, 60)
	}
	if got := len(a.Step(Busy, .07).Lines(opts())); got != Rows {
		t.Fatalf("rows after a long pause = %d", got)
	}
}

func TestLowColorProfilesStillRender(t *testing.T) {
	for _, p := range []termenv.Profile{termenv.ANSI256, termenv.ANSI} {
		o := opts()
		o.Profile = p
		if strings.TrimSpace(plain(strings.Join(Static(Happy, .4).Lines(o), ""))) == "" {
			t.Fatalf("profile %v drew nothing", p)
		}
	}
}

func BenchmarkAnimStepAndRender(b *testing.B) {
	a := NewAnim()
	o := opts()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = a.Step(Busy, .07).Lines(o)
	}
}
