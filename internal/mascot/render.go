package mascot

import (
	"fmt"
	"math"
	"strings"

	"github.com/muesli/termenv"
)

const (
	// Cols and Rows are the text size of a full scene (cat plus both folders).
	Cols = W
	Rows = H / 2
	// CropCols is the width of the cat-only crop for narrow terminals.
	CropCols = 34
	cropX0   = 13
)

// Options describe the terminal the frame is drawn for.
type Options struct {
	Profile termenv.Profile
	// Dark is the assumed terminal background for translucent pixels (shadow,
	// dimmed folders).
	Dark bool
	// Crop draws only the cat, without the folders and most of the flight path.
	Crop bool
}

func (o Options) background() rgb {
	if o.Dark {
		return rgb{30, 30, 46}
	}
	return rgb{244, 244, 250}
}

func final(p px, bg rgb) (rgb, bool) {
	if p.a < .12 {
		return rgb{}, false
	}
	return mix(bg, p.c, math.Min(1, p.a)), true
}

type painter struct {
	o     Options
	cache map[uint32]string
}

func (p *painter) seq(c rgb, bg bool) string {
	key := uint32(c.r)<<16 | uint32(c.g)<<8 | uint32(c.b)
	if bg {
		key |= 1 << 24
	}
	if s, ok := p.cache[key]; ok {
		return s
	}
	col := p.o.Profile.Color(fmt.Sprintf("#%02x%02x%02x", int(c.r), int(c.g), int(c.b)))
	s := ""
	if col != nil {
		s = "\x1b[" + col.Sequence(bg) + "m"
	}
	p.cache[key] = s
	return s
}

// Lines renders the frame as Rows lines of half-block text. Two pixel rows
// share one text row: the top pixel is the foreground of "▀", the bottom the
// background.
func (f *Frame) Lines(o Options) []string {
	x0, x1 := 0, W
	if o.Crop {
		x0, x1 = cropX0, cropX0+CropCols
	}
	bgc := o.background()
	pt := &painter{o: o, cache: map[uint32]string{}}
	out := make([]string, 0, Rows)
	for y := 0; y < H; y += 2 {
		var sb strings.Builder
		var lastFG, lastBG string
		for x := x0; x < x1; x++ {
			top, tok := final(f.cv[y][x], bgc)
			bot, bok := final(f.cv[y+1][x], bgc)
			var fg, bg, ch string
			switch {
			case tok && bok:
				fg, bg, ch = pt.seq(top, false), pt.seq(bot, true), "▀"
			case tok:
				fg, ch = pt.seq(top, false), "▀"
			case bok:
				fg, ch = pt.seq(bot, false), "▄"
			default:
				ch = " "
			}
			if bg == "" && lastBG != "" {
				sb.WriteString("\x1b[49m")
			}
			if bg != "" && bg != lastBG {
				sb.WriteString(bg)
			}
			if fg != "" && fg != lastFG {
				sb.WriteString(fg)
			}
			if fg != "" {
				lastFG = fg
			}
			lastBG = bg
			sb.WriteString(ch)
		}
		sb.WriteString("\x1b[0m")
		out = append(out, sb.String())
	}
	return out
}

// Visible reports whether any pixel is drawn in column x of the full scene;
// tests use it to check the cat is symmetric.
func (f *Frame) Visible(x, y int) bool {
	return x >= 0 && x < W && y >= 0 && y < H && f.cv[y][x].a >= .12
}
