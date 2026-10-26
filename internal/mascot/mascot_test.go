package mascot

import (
	"math"
	"reflect"
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

func TestWaitKeepsTheHeadIntact(t *testing.T) {
	f := StaticMini(Wait, .65)
	earY := -1
	ears := 0
	for y, row := range f.cv {
		for _, p := range row {
			if p.a == 1 && p.c == cBlush {
				if earY >= 0 && earY != y {
					t.Fatal("waiting pose shears the head between the ears")
				}
				earY = y
				ears++
			}
		}
	}
	if ears != 2 {
		t.Fatalf("inner ears = %d, want 2", ears)
	}
}

func TestEveryMoodDrawsAndIsDeterministic(t *testing.T) {
	for _, m := range []Mood{Idle, Busy, Happy, Error, Wait, Sleep} {
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

// A paused terminal must not feed negative/invalid time into the integrator.
func TestSpringHandlesTimeAndIsIndependentOfTickSize(t *testing.T) {
	for _, dt := range []float64{-60, 0, .05, 60, math.MaxFloat64, math.NaN(), math.Inf(1)} {
		s := spring{pos: 1}
		got := s.step(2, dt)
		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got) > 3 {
			t.Fatalf("dt %g: unstable spring %g", dt, got)
		}
	}
	a, b := spring{}, spring{}
	a.step(1, .2)
	for range 4 {
		b.step(1, .05)
	}
	if math.Abs(a.pos-b.pos) > 1e-9 {
		t.Fatalf("same elapsed time changed spring position: %g vs %g", a.pos, b.pos)
	}
}

func TestStaticHandlesNegativeAndHugeTime(t *testing.T) {
	for _, ts := range []float64{-1e300, -3, 0, 1e300, math.NaN(), math.Inf(1)} {
		for _, mood := range []Mood{Idle, Busy, Happy, Error, Wait, Sleep} {
			for _, f := range []*Frame{Static(mood, ts), StaticMini(mood, ts), Strip(ts)} {
				for _, row := range f.cv {
					for _, p := range row {
						if math.IsNaN(p.a) || math.IsNaN(p.c.r) || math.IsInf(p.c.r, 0) {
							t.Fatalf("%s at %g produced an invalid pixel", mood, ts)
						}
					}
				}
			}
		}
	}
}

func TestMiniAndStripDimensionsAllProfiles(t *testing.T) {
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI} {
		for _, dark := range []bool{false, true} {
			o := Options{Profile: profile, Dark: dark, Mini: true}
			for _, mood := range []Mood{Idle, Busy, Happy, Error, Wait, Sleep} {
				assertDimensions(t, StaticMini(mood, 1.1).Lines(o), MiniCols, MiniRows)
				o.Compact = true
				assertDimensions(t, StaticMini(mood, 1.1).Lines(o), MiniCols, CompactRows)
				o.Compact = false
			}
			assertDimensions(t, Strip(.7).StripLines(o), StripCols, StripRows)
		}
	}
}

func TestLowColorKeepsEyesDistinctFromFur(t *testing.T) {
	for _, n := range []int{16, 256} {
		if nearestTerminalColor(cEye, n) == nearestTerminalColor(cFur, n) || nearestTerminalColor(cEye, n) == nearestTerminalColor(cFurShade, n) {
			t.Fatalf("%d-colour palette merged eyes with fur", n)
		}
	}
}

func assertDimensions(t *testing.T, lines []string, cols, rows int) {
	t.Helper()
	if len(lines) != rows {
		t.Fatalf("height = %d, want %d", len(lines), rows)
	}
	for _, line := range lines {
		if got := len([]rune(plain(line))); got != cols {
			t.Fatalf("width = %d, want %d", got, cols)
		}
		for _, ch := range plain(line) {
			if ch != ' ' && ch != '▀' && ch != '▄' {
				t.Fatalf("unexpected terminal glyph %q", ch)
			}
		}
	}
}

func TestAnimDeterministicThroughMoodChanges(t *testing.T) {
	a, b := NewMini(), NewMini()
	for _, mood := range []Mood{Idle, Busy, Happy, Idle, Error, Wait, Sleep} {
		for range 12 {
			x := a.Step(mood, 1.0/15).Lines(opts())
			y := b.Step(mood, 1.0/15).Lines(opts())
			if !reflect.DeepEqual(x, y) {
				t.Fatalf("%s: identical inputs diverged", mood)
			}
		}
	}
}

func TestMiniUsesSolidPixelsAndASmallPalette(t *testing.T) {
	for _, mood := range previewMoods {
		for _, ts := range []float64{0, .6, 1.4, 3} {
			colors := map[rgb]bool{}
			for _, row := range StaticMini(mood, ts).cv {
				for _, p := range row {
					if p.a != 0 && p.a != 1 {
						t.Fatalf("%s: translucent pixel blurs the sprite", mood)
					}
					if p.a == 1 {
						colors[p.c] = true
					}
				}
			}
			if len(colors) > 9 {
				t.Fatalf("%s: %d colors at native size", mood, len(colors))
			}
		}
	}
}

func TestMiniFitsSevenTextRows(t *testing.T) {
	o := opts()
	o.Mini = true
	if got := len(StaticMini(Idle, 0).Lines(o)); got > 7 {
		t.Fatalf("mini is still %d rows tall", got)
	}
}

func TestMoodsAreDistinctWithoutTheirColors(t *testing.T) {
	seen := map[[H][W]bool]Mood{}
	for _, mood := range previewMoods {
		var silhouette [H][W]bool
		f := StaticMini(mood, .65)
		for y := range silhouette {
			for x := range silhouette[y] {
				silhouette[y][x] = f.Visible(x, y)
			}
		}
		if other, ok := seen[silhouette]; ok {
			t.Fatalf("%s and %s only differ in color", mood, other)
		}
		seen[silhouette] = mood
	}
}

func TestCompactKeepsWholeMiniInSixRows(t *testing.T) {
	o := opts()
	o.Mini, o.Compact = true, true
	for _, mood := range previewMoods {
		assertDimensions(t, StaticMini(mood, .6).Lines(o), MiniCols, CompactRows)
	}
}

func TestAnimationSeedAndElapsedTime(t *testing.T) {
	a, b, other := NewMini(), NewMini(), NewMini()
	a.Seed, b.Seed, other.Seed = 42, 42, 99
	for range 4 {
		a.Step(Idle, .25)
	}
	var bf, otherf *Frame
	for range 20 {
		bf = b.Step(Idle, .05)
		otherf = other.Step(Idle, .05)
	}
	if math.Abs(a.sx.pos-b.sx.pos) > .0001 || math.Abs(a.face.gx-b.face.gx) > .001 {
		t.Fatalf("tick size changed motion: sx=%g/%g gaze=%g/%g", a.sx.pos, b.sx.pos, a.face.gx, b.face.gx)
	}
	for range 200 {
		if !reflect.DeepEqual(bf.cv, otherf.cv) {
			return
		}
		bf, otherf = b.Step(Idle, .05), other.Step(Idle, .05)
	}
	t.Fatal("different seeds never changed the animation")
}

func TestMoodTransitionStartsWithPreviousPose(t *testing.T) {
	a := NewMini()
	before := *a.Step(Idle, .25).cv
	a.Step(Sleep, 0)
	// Decorations can change immediately, but the pose and face must wait for dt.
	if a.curl.pos != 0 || a.face.blink > .5 {
		t.Fatal("sleep snapped straight to its final pose")
	}
	if reflect.DeepEqual(before, *a.Step(Sleep, .25).cv) {
		t.Fatal("sleep never transitioned")
	}
}

func TestFrameRemainsASnapshotAfterNextStep(t *testing.T) {
	a := NewMini()
	f := a.Step(Idle, .1)
	before := f.Lines(opts())
	a.Step(Happy, .25)
	if !reflect.DeepEqual(before, f.Lines(opts())) {
		t.Fatal("next Step mutated an earlier Frame")
	}
}

func TestMiniCropKeepsTheHeadAndEarsDuringMotion(t *testing.T) {
	for _, mood := range previewMoods {
		for tick := 0; tick < 1200; tick++ {
			ts := float64(tick) / 15
			cv := StaticMini(mood, ts).cv
			for y, row := range cv {
				for x, pixel := range row {
					if pixel.a >= .12 && (y < miniY0 || x < int(cx)-MiniCols/2 || x >= int(cx)+MiniCols/2) {
						t.Fatalf("%s %.2fs: pixel at (%d,%d) leaves mini crop", mood, ts, x, y)
					}
				}
			}
		}
	}
}

func BenchmarkMiniStepAndRender(b *testing.B) {
	a := NewMini()
	o := opts()
	o.Mini = true
	b.ReportAllocs()
	for range b.N {
		_ = a.Step(Busy, 1.0/15).Lines(o)
	}
}

func BenchmarkMiniLowColor(b *testing.B) {
	for _, profile := range []termenv.Profile{termenv.ANSI256, termenv.ANSI} {
		b.Run(profile.Name(), func(b *testing.B) {
			a := NewMini()
			o := Options{Profile: profile, Dark: true, Mini: true}
			b.ReportAllocs()
			for range b.N {
				_ = a.Step(Busy, 1.0/15).Lines(o)
			}
		})
	}
}

func BenchmarkStripAndRender(b *testing.B) {
	o := opts()
	b.ReportAllocs()
	for i := range b.N {
		_ = Strip(float64(i) / 15).StripLines(o)
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
