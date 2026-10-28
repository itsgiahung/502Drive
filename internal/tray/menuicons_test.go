package tray

import (
	"image"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEveryMenuGlyphIsUsedAndDraws(t *testing.T) {
	used := map[string]bool{}
	for _, v := range []View{
		BuildView(Snapshot{}, false, ""),
		BuildView(Snapshot{Status: Status{Ready: true, QueuePaused: true}}, true, ""),
		BuildView(Snapshot{Status: Status{Ready: true, AccountStatus: "connected"}, Update: "v9.0.0"}, true, ""),
	} {
		for _, it := range v.Items {
			if it.Icon != "" {
				used[it.Icon] = true
			}
			for _, c := range it.Children {
				if c.Icon != "" {
					used[c.Icon] = true
				}
			}
		}
	}
	for key := range used {
		img := glyphImage(key, 32)
		if img == nil {
			t.Fatalf("glyph %q is not defined", key)
		}
		painted := 0
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 0 {
				painted++
			}
		}
		if painted < 40 {
			t.Fatalf("glyph %q paints only %d pixels", key, painted)
		}
	}
	if len(used) < 8 {
		t.Fatalf("only %d glyphs in use: %v", len(used), used)
	}
}

// The Linux tray library mangles semi-transparent pixels, so the icon it gets
// must be fully opaque or fully transparent.
func TestTrayPNGHasHardEdges(t *testing.T) {
	for _, st := range AllStates {
		data, err := trayPNG(st, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		img, err := decodeIcon(data)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a != 0 && a != 0xffff {
					t.Fatalf("state %s: pixel %d,%d is semi-transparent (%d)", st, x, y, a)
				}
			}
		}
	}
	var _ image.Image
}

func TestFirstRowIsPaddedToTheSubmenuWidth(t *testing.T) {
	v := BuildView(snapOf(0, 0, 0, "connected"), true, "")
	if n := utf8.RuneCountInString(menuRow(t, v, "status").Title); n < minRowWidth {
		t.Fatalf("first row is %d wide, want at least %d", n, minRowWidth)
	}
	long := BuildView(snapOf(1, 0, 0, "connected", Job{SourceName: strings.Repeat("x", 200), State: "running", Completed: 123, Total: ptr(456)}), true, "")
	if n := utf8.RuneCountInString(menuRow(t, long, "job:0").Title); n > minRowWidth+4 {
		t.Fatalf("a job row is %d wide and would widen the menu", n)
	}
}

func ptr(n int) *int { return &n }
