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

// sleepingCat is curled into a ring: the head rests on the left, the back is
// a low dome and the tail wraps around the front, its tip by the nose.
var sleepingCat = []string{
	".b...b.bbbbbbb..",
	".bbbbbbbbbbbbbb.",
	"bbbbbbbbbbbbbbbb",
	"bbbbbbbbbbbbbbbv",
	"vbbbbbbbbbbbbbbv",
	".vvvvvvvvvvvvvv.",
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

// letter puts one text character at sprite pixel (x, y); it fills a whole
// terminal cell whatever the sprite scale.
func (s sprite) letter(x, y int, ch rune, c rgb) {
	cx, cy := s.x+x*s.scale, s.y+y*s.scale
	if cx < 0 || cy < 0 || cx >= W || cy >= H {
		return
	}
	s.cv[cy][cx] = px{c: c, a: 1, ch: ch}
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
		cv.sleeping(b, t, accent)
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
	if h.y < b.y {
		// The head rose a row (a deep breath, a stretch): the chest fills the
		// gap, so the cat grows taller instead of showing a thin neck.
		for x := 3; x <= 10; x++ {
			b.pixel(x, 6, cFur)
		}
	}
	h.stamp(catHead, 0, accent)
	h.face(f)
	if p.yarn > .5 {
		b.yarn(t)
	}
	if p.fly > .5 {
		b.butterfly(t)
	}

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
		// Washing: a fur-coloured paw rubs the cheek from outside the face's
		// outline (so it reads as a paw, not a hole), with a flick of tongue.
		lick := math.Sin(t*9) > 0
		py := 5
		if lick {
			py = 4
		}
		for _, xy := range [][2]int{{0, py}, {1, py}, {0, py + 1}, {1, py + 1}} {
			h.pixel(xy[0], xy[1], cFur)
		}
		h.pixel(0, py+1, cFurShade)
		if lick {
			h.pixel(6, 6, cBlush)
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

// sleeping draws the curled cat: closed eyes, a back that rises and falls
// with each breath, and two small "z"s that appear in turn and drift up.
func (cv *canvas) sleeping(b sprite, t float64, accent rgb) {
	bed := b
	bed.y += 4 * b.scale
	bed.stamp(sleepingCat, 0, accent)
	if math.Sin(t*1.1) > .2 { // inhale: the round back rises above the head
		for x := 8; x <= 10; x++ {
			bed.pixel(x, -1, cFur)
		}
	}
	for _, x := range []int{1, 2, 4, 5} { // closed eyes
		bed.pixel(x, 2, cEye)
	}
	bed.pixel(3, 3, cBlush)
	bed.pixel(0, 3, cFurShade) // the tail's tip curls up under the chin
	// "z Z": a small z rises from the head, then a capital Z higher up and to
	// the right; both are real text characters, so they stay small and crisp.
	ph := frac(t / 3.2)
	if ph < .85 {
		bed.letter(6, -2, 'z', accent)
	}
	if ph > .35 && ph < .85 {
		bed.letter(10, -4, 'Z', accent)
	}
}

// yarn is a ball of yarn on the ground left of the cat. It rolls out after
// each bat of the left paw; its loose thread turns as it rolls. The mini cat
// uses solid pixels only, so toys appear and leave rather than fade.
func (s sprite) yarn(t float64) {
	x := int(math.Round(yarnX(t)))
	for dy := 7; dy <= 9; dy++ {
		for dx := 0; dx < 3; dx++ {
			s.pixel(x+dx, dy, cBlush)
		}
	}
	// A gold thread wound across the ball; it flips as the ball rolls.
	thread := [][2]int{{0, 7}, {1, 8}, {2, 9}}
	if int(math.Abs(yarnX(t))*2)%2 == 1 {
		thread = [][2]int{{2, 7}, {1, 8}, {0, 9}}
	}
	for _, xy := range thread {
		s.pixel(x+xy[0], xy[1], cGold)
	}
	if math.Cos(t*2.6) > .5 { // the paw reaches out at the start of each roll
		s.pixel(2, 8, cFur)
		s.pixel(1, 8, cFur)
	}
}

// butterfly flutters beside the right ear: a dark body and two wings that
// open and fold.
func (s sprite) butterfly(t float64) {
	bx, by := butterfly(t)
	x, y := int(math.Round(bx)), int(math.Round(by))
	s.pixel(x, y, cHi)   // a pale body stays visible on a dark terminal
	if int(t*8)%2 == 0 { // wings open
		s.pixel(x-1, y, cGold)
		s.pixel(x+1, y, cBlush)
	} else { // wings folded up over the body
		s.pixel(x, y-1, cGold)
		s.pixel(x+1, y-1, cBlush)
	}
}
