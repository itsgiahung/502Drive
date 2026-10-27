// Package mascot draws the 502Drive cat as pixel art with half-block
// characters and animates it (idle, copying, done, error, sleep). It only
// produces text; callers decide whether colour and motion are allowed.
package mascot

import "math"

const (
	W      = 60
	H      = 26
	ground = 24
	cx     = 30.0
	S      = 0.58
	// MiniScale preserves a readable face plus a small body in the Home panel.
	MiniScale = 0.57
)

func (cv *canvas) blend(x, y int, c rgb, a float64) {
	if x < 0 || y < 0 || x >= W || y >= H || a <= 0 {
		return
	}
	p := &cv[y][x]
	na := a + p.a*(1-a)
	if na <= 0 {
		return
	}
	p.c = rgb{(c.r*a + p.c.r*p.a*(1-a)) / na, (c.g*a + p.c.g*p.a*(1-a)) / na, (c.b*a + p.c.b*p.a*(1-a)) / na}
	p.a = na
}

// pose is the continuous motion state; the sprite renderer snaps it to pixels.
type pose struct {
	sx, sy, lift, shake, ear, droop     float64
	tilt, head, tail, paw, curl, fright float64
	// yarn and fly are idle toys (0 = away, 1 = fully in play). They follow the
	// fidget timeline directly; the toy motion itself is drawn from the clock.
	yarn, fly float64
}

func (p pose) X(lx float64) float64 { return cx + p.shake + lx*p.sx }
func (p pose) Y(ly float64) float64 { return ground - p.lift + ly*p.sy }

func (cv *canvas) star(x, y int, ph float64, c rgb) {
	s := math.Sin(ph * math.Pi)
	if s < .15 {
		return
	}
	cv.blend(x, y, c, 1)
	if s > .6 {
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			cv.blend(x+d[0], y+d[1], c, .85)
		}
	}
	if s > .9 {
		cv.blend(x+2, y, c, .5)
		cv.blend(x-2, y, c, .5)
		cv.blend(x, y-2, c, .5)
		cv.blend(x, y+2, c, .5)
	}
}

const (
	srcX, dstX = 2, 49
	folderRows = 7
	flowSpeed  = .5
)

var folderBits = []string{
	".ooo.....",
	"oHHHooooo",
	"oHHHHHHHo",
	"offfffffo",
	"offfffffo",
	"offfffffo",
	"ooooooooo",
}

func (cv *canvas) folder(x, y int, hi, fill rgb, a float64) {
	for r, row := range folderBits {
		for c, ch := range row {
			switch ch {
			case 'o':
				cv.blend(x+c, y+r, mix(cOutline, fill, .15), a)
			case 'H':
				cv.blend(x+c, y+r, hi, a)
			case 'f':
				cv.blend(x+c, y+r, fill, a)
			}
		}
	}
}

var paperTints = []rgb{{246, 246, 255}, {170, 205, 255}, {255, 190, 215}}

func (cv *canvas) paper(x, y int, kind int, a float64, clip func(x, y int) bool) {
	base := paperTints[kind%len(paperTints)]
	for r := 0; r < 4; r++ {
		for c := 0; c < 3; c++ {
			if clip != nil && clip(x+c, y+r) {
				continue
			}
			col := base
			switch kind % 3 {
			case 0: // document, two ink rules
				if (r == 1 || r == 2) && c < 2 {
					col = cMid
				}
			case 1: // photograph, a sunset over a green hill
				if r == 1 && c == 1 {
					col = cGold
				}
				if r > 1 {
					col = hx(111, 204, 177)
				}
			case 2: // archive, a gold zipper down the middle
				if c == 1 {
					col = cGold
				}
				if c == 1 && r%2 == 1 {
					col = cOutline
				}
			}
			if r == 0 && c == 2 {
				col = mix(base, cOutline, .3)
			}
			cv.blend(x+c, y+r, col, a)
		}
	}
}

// packetPos is the flight path: it leaves the source folder, rises over the
// mascot's ears (apex clears the ear tips) and drops into the destination.
func packetPos(ph float64) (x, y float64) {
	const x0, x1 = srcX + 3, dstX + 3
	const y0, y1 = ground - 6.0, ground - 6.0
	const apex = 17.0
	x = x0 + (x1-x0)*ph
	y = y0 + (y1-y0)*ph - 4*apex*ph*(1-ph)
	return
}

// flow returns each packet's phase 0..1 from source folder to destination.
func flow(t float64) [3]float64 {
	var o [3]float64
	for i := range o {
		o[i] = frac(t*flowSpeed + float64(i)/3)
	}
	return o
}

func (cv *canvas) packets(t float64) {
	for i, ph := range flow(t) {
		kind := int(math.Floor(t*flowSpeed+float64(i)/3)) + i
		clip := func(x, y int) bool {
			if y < ground-5 {
				return false
			}
			return (ph < .3 && x >= srcX && x < srcX+9) || (ph > .7 && x >= dstX && x < dstX+9)
		}
		for k, a := range []float64{.16, .3} {
			tx, ty := packetPos(math.Max(0, ph-.045*float64(2-k)))
			cv.blend(int(tx)+1, int(ty)+1, cGold, a)
		}
		x, y := packetPos(ph)
		cv.paper(int(x), int(y), kind, 1, clip)
		if ph > .9 {
			bx, by := packetPos(1)
			a := (ph - .9) / .1
			for _, d := range [][2]int{{-3, -1}, {4, -2}, {0, -3}} {
				cv.blend(int(bx)+1+d[0], int(by)+d[1], cGold, 1-a*.6)
			}
		}
	}
}

func scene(cv *canvas, mood Mood, t float64, p pose, f face, props bool, accent rgb) {
	*cv = canvas{}
	size := 1
	if props {
		size = 2
		sa, da := .35, .35
		if mood == Busy {
			sa, da = 1, 1
		}
		if mood == Happy {
			sa, da = .5, 1
		}
		cv.folder(srcX, ground-folderRows+1, hx(170, 178, 255), hx(120, 130, 240), sa)
		cv.folder(dstX, ground-folderRows+1, hx(255, 228, 140), cGold, da)
	}
	cv.cat(mood, t, p, f, accent, size, false)
	if mood == Busy && props {
		cv.packets(t * 1.3)
	}
}
