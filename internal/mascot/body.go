package mascot

import "math"

// The small cat is authored at its final pixel size. No resampling or alpha
// edges: one source pixel is one terminal half-cell (two in the full scene).
var catHead = []string{
	"..b........b..",
	"..bpbbbbbbpb..",
	".bbbbbbbbbbbb.",
	".bbbbbbbbbbbb.",
	".bbbbbbbbbbbb.",
	"..bbbbbbbbbb..",
	"...vvvvvvvv...",
}

var catBody = []string{
	"...voooooov...",
	"...bbbbbbbb...",
	"...bb....bb...",
}

var sleepingCat = []string{
	"...bb....bb...",
	"..bbbbbbbbbb..",
	".bbbbbbbbbbbb.",
	".bbbbbbbbbbbb.",
	"..vvvvvvvvvv..",
}

type sprite struct {
	cv          *canvas
	x, y, scale int
}

func (s sprite) pixel(x, y int, c rgb) {
	for dy := 0; dy < s.scale; dy++ {
		for dx := 0; dx < s.scale; dx++ {
			s.cv.blend(s.x+x*s.scale+dx, s.y+y*s.scale+dy, c, 1)
		}
	}
}

func (s sprite) stamp(rows []string, y0 int, accent rgb) {
	for y, row := range rows {
		for x, ch := range row {
			var c rgb
			switch ch {
			case 'b':
				c = cFur
			case 'v':
				c = cFurShade
			case 'p':
				c = cBlush
			case 'o':
				c = accent
			default:
				continue
			}
			s.pixel(x, y+y0, c)
		}
	}
}

func (s sprite) sparkle(x, y int, c rgb, large bool) {
	s.pixel(x, y, c)
	if large {
		s.pixel(x-1, y, c)
		s.pixel(x+1, y, c)
		s.pixel(x, y-1, c)
		s.pixel(x, y+1, c)
	}
}

func (cv *canvas) cat(m Mood, t float64, p pose, f face, accent rgb, size int, compact bool) {
	lift := int(math.Round(p.lift * 2))
	if compact {
		lift = min(lift, 1)
	}
	b := sprite{cv: cv, x: int(cx) - 7*size + int(math.Round(p.shake))*size, y: ground - (10+lift)*size, scale: size}
	if compact {
		b.y++
	}
	if p.curl > .6 {
		bed := b
		bed.y += 4 * size
		bed.stamp(sleepingCat, 0, accent)
		for _, x := range []int{3, 4, 9, 10} {
			bed.pixel(x, 2, cEye)
		}
		bed.pixel(6, 3, cEye)
		bed.pixel(7, 3, cEye)
		for x := 5; x < 11; x++ {
			bed.pixel(x, 5, cFurShade)
		}
		bed.pixel(11, 4, cFurShade)
		bed.pixel(11, 3, cFurShade)
		z := b
		z.y -= int(frac(t*.35)*2) * size
		for y, row := range glyphZ {
			for x, ch := range row {
				if ch == '#' {
					z.pixel(12+x, y, cFur)
				}
			}
		}
		return
	}

	// A curled tail stays behind the body, and is tucked away at rest at t=0.
	if p.tail > .3 {
		for _, xy := range [][2]int{{10, 9}, {11, 9}, {12, 9}, {12, 8}} {
			b.pixel(xy[0], xy[1], cFurShade)
		}
		tip := 7
		if math.Sin(t*2.5) > 0 {
			tip = 6
		}
		b.pixel(13, tip, cFurShade)
		b.pixel(13, tip+1, cFurShade)
	}
	b.stamp(catBody, 7, accent)
	b.pixel(6, 6, cFur)
	b.pixel(7, 6, cFur)
	h := b
	scale := MiniScale
	if size == 2 {
		scale = S
	}
	if m == Idle {
		switch {
		case p.sy > scale*1.045:
			h.y -= size
		case p.sy < scale*.95:
			h.y += size
		}
	} else if m == Happy && p.sy < scale*.95 && lift == 0 {
		h.y += size
	}
	if m == Busy && p.head > .15 {
		h.y += size
	}
	if p.tilt > .1 {
		h.x += size
	} else if p.tilt < -.1 {
		h.x -= size
	}
	h.stamp(catHead, 0, accent)
	h.face(f)

	if m == Busy {
		// Two alternating paws over a single row of keys read as typing.
		left, right := 8, 9
		if p.paw > 0 {
			left, right = right, left
		}
		for _, x := range []int{3, 4} {
			b.pixel(x, left, cHi)
		}
		for _, x := range []int{9, 10} {
			b.pixel(x, right, cHi)
		}
		for x := 3; x <= 10; x++ {
			c := cGold
			if x%2 == 0 && x < 10 {
				c = cEye
			}
			b.pixel(x, 10, c)
		}
	} else if m == Happy {
		b.pixel(2, 7, cHi)
		b.pixel(11, 7, cHi)
		if frac(t*1.2) < .65 {
			b.sparkle(-1, 4, cGold, !compact)
		}
		if frac(t*1.2+.4) < .65 {
			b.sparkle(14, 1, cBlush, !compact)
		}
	} else if m == Idle && p.paw > 1 {
		for _, xy := range [][2]int{{2, 4}, {3, 4}, {2, 5}, {3, 5}} {
			h.pixel(xy[0], xy[1], cHi)
		}
	}
	switch m {
	case Error:
		for y := 0; y < 3; y++ {
			b.pixel(14, y, cCoral)
		}
		b.pixel(14, 4, cCoral)
	case Wait:
		for i := 0; i < 3; i++ {
			c := cGold
			if i == int(t*2)%3 {
				c = cHi
			}
			b.pixel(10+i*2, -1, c)
		}
	}
}
