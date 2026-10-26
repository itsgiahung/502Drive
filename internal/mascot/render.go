package mascot

import (
	"math"
	"strconv"

	"github.com/muesli/termenv"
)

const (
	// MiniCols and MiniRows are the text size of the small cat.
	MiniCols    = 18
	MiniRows    = 7
	CompactRows = 6
	miniY0      = 12
	// StripCols and StripRows are the text size of the file-transfer strip.
	StripCols = 56
	StripRows = 4
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
	// Mini crops tightly around the small cat (NewMini / StaticMini).
	Mini bool
	// Compact fits the same mini artwork into six text rows for short terminals.
	Compact bool
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
	c := p.c
	if bg.r > 200 {
		c = mix(c, cOutline, .12)
	}
	return mix(bg, c, math.Min(1, p.a)), true
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
	var buf [32]byte
	b := append(buf[:0], "\x1b["...)
	if p.o.Profile == termenv.TrueColor {
		if bg {
			b = append(b, "48;2;"...)
		} else {
			b = append(b, "38;2;"...)
		}
		for i, v := range [...]uint32{uint32(c.r), uint32(c.g), uint32(c.b)} {
			if i > 0 {
				b = append(b, ';')
			}
			b = strconv.AppendUint(b, uint64(v), 10)
		}
	} else if p.o.Profile == termenv.ANSI || p.o.Profile == termenv.ANSI256 {
		n := 256
		if p.o.Profile == termenv.ANSI {
			n = 16
		}
		idx := nearestTerminalColor(c, n)
		if n == 256 {
			if bg {
				b = append(b, "48;5;"...)
			} else {
				b = append(b, "38;5;"...)
			}
			b = strconv.AppendInt(b, int64(idx), 10)
		} else {
			code := 30 + idx
			if idx >= 8 {
				code = 90 + idx - 8
			}
			if bg {
				code += 10
			}
			b = strconv.AppendInt(b, int64(code), 10)
		}
	} else {
		return ""
	}

	b = append(b, 'm')
	s := string(b)
	p.cache[key] = s
	return s
}

// Lines renders the frame as Rows lines of half-block text. Two pixel rows
// share one text row: the top pixel is the foreground of "▀", the bottom the
// background.
func (f *Frame) Lines(o Options) []string {
	x0, x1, y0, y1 := 0, W, 0, H
	switch {
	case o.Mini:
		x0, x1, y0, y1 = int(cx)-MiniCols/2, int(cx)+MiniCols/2, miniY0, miniY0+MiniRows*2
	case o.Crop:
		x0, x1 = cropX0, cropX0+CropCols
	}
	if o.Mini && o.Compact {
		return f.compact(o)
	}
	return f.lines(o, x0, x1, y0, y1)
}

// StripLines renders a Strip frame.
func (f *Frame) StripLines(o Options) []string {
	return f.lines(o, 0, StripCols, 0, StripRows*2)
}

func (f *Frame) lines(o Options, x0, x1, y0, y1 int) []string {
	bgc := o.background()
	pt := &painter{o: o, cache: map[uint32]string{}}
	out := make([]string, 0, (y1-y0)/2)
	var buf [W * 64]byte
	for y := y0; y < y1; y += 2 {
		b := buf[:0]
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
				b = append(b, "\x1b[49m"...)
			}
			if bg != "" && bg != lastBG {
				b = append(b, bg...)
			}
			if fg != "" && fg != lastFG {
				b = append(b, fg...)
			}
			if fg != "" {
				lastFG = fg
			}
			lastBG = bg
			b = append(b, ch...)
		}
		b = append(b, "\x1b[0m"...)
		out = append(out, string(b))
	}
	return out
}

// Visible reports whether any pixel is drawn in column x of the full scene;
// tests use it to check the cat is symmetric.
func (f *Frame) Visible(x, y int) bool {
	return x >= 0 && x < W && y >= 0 && y < H && f.cv[y][x].a >= .12
}

// Compact uses the same native pixels in a shorter viewport; no bitmap resize.
func (f *Frame) compact(o Options) []string {
	cv := f.compactCanvas()
	ff := Frame{cv: &cv}
	return ff.lines(o, int(cx)-MiniCols/2, int(cx)+MiniCols/2, H-CompactRows*2, H)
}
func (f *Frame) compactCanvas() (cv canvas) {
	if f.p.sx <= 0 {
		return *f.cv
	}
	cv.cat(f.mood, f.t, f.p, f.face, f.accent, 1, true)
	return
}
