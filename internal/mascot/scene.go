// Package mascot draws the 502Drive cat as pixel art with half-block
// characters and animates it (idle, copying, done, error, sleep). It only
// produces text; callers decide whether colour and motion are allowed.
package mascot

import "math"

const (
	W      = 60
	H      = 26
	ground = 23
	cx     = 30.0
	S      = 0.58
)

type rgb struct{ r, g, b float64 }
type px struct {
	c rgb
	a float64
}
type canvas [H][W]px

func hx(r, g, b float64) rgb { return rgb{r, g, b} }
func mix(a, b rgb, t float64) rgb {
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

var (
	cTop     = hx(160, 232, 255)
	cMid     = hx(98, 172, 255)
	cBot     = hx(112, 108, 238)
	cOutline = hx(40, 42, 104)
	cHi      = hx(232, 250, 255)
	cBlush   = hx(255, 140, 172)
	cEye     = hx(28, 26, 60)
	cMouth   = hx(70, 30, 80)
	cTongue  = hx(255, 118, 140)
	cGold    = hx(255, 210, 90)
	cCoral   = hx(255, 130, 130)
	cWhite   = hx(255, 255, 255)
	cFoot    = hx(70, 74, 170)
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

func (cv *canvas) ellipse(ex, ey, rx, ry float64, c rgb, a float64) {
	for y := int(ey - ry - 1); y <= int(ey+ry+1); y++ {
		for x := int(ex - rx - 1); x <= int(ex+rx+1); x++ {
			dx, dy := (float64(x)+.5-ex)/rx, (float64(y)+.5-ey)/ry
			if dx*dx+dy*dy <= 1 {
				cv.blend(x, y, c, a)
			}
		}
	}
}

// pose is the body transform; local coordinates have y<0 upward from the feet.
type pose struct{ sx, sy, lift, shake, ear, droop float64 }

func (p pose) X(lx float64) float64 { return cx + p.shake + lx*p.sx }
func (p pose) Y(ly float64) float64 { return ground - p.lift + ly*p.sy }

func (cv *canvas) plot(p pose, lx, ly float64, c rgb, a float64) {
	cv.blend(int(math.Floor(p.X(lx))), int(math.Floor(p.Y(ly))), c, a)
}
func (cv *canvas) line(p pose, x0, y0, x1, y1 float64, c rgb) {
	for i := 0.0; i <= 1; i += .05 {
		cv.plot(p, x0+(x1-x0)*i, y0+(y1-y0)*i, c, 1)
	}
}

var blobs = [][3]float64{{-9, -8, 7.8}, {9, -8, 7.8}, {-5, -12, 7.6}, {5, -12, 7.6}, {0, -11, 9.4}, {0, -4.5, 8.4}}

type pt [2]float64

func inTri(x, y float64, a, b, c pt) bool {
	d := func(p, q, r pt) float64 { return (p[0]-r[0])*(q[1]-r[1]) - (q[0]-r[0])*(p[1]-r[1]) }
	p := pt{x, y}
	d1, d2, d3 := d(p, a, b), d(p, b, c), d(p, c, a)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

// ear returns the cat-ear triangle on side s (-1 left, +1 right) in local
// coordinates; ear splays the tip outward, droop folds it down.
func ear(s float64, p pose) (a, b, c pt) {
	return pt{s * 11.4, -13.5}, pt{s * 3, -20.5}, pt{s*(8.6+p.ear) + 0, -27.5 + p.droop}
}

func earInner(s float64, p pose) (a, b, c pt) {
	a, b, c = ear(s, p)
	g := pt{(a[0] + b[0] + c[0]) / 3, (a[1] + b[1] + c[1]) / 3}
	k := func(v pt) pt { return pt{g[0] + (v[0]-g[0])*.5, g[1] + (v[1]-g[1])*.5 + 1.2} }
	return k(a), k(b), k(c)
}

func bodyMask(p pose) (m [H][W]bool) {
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			lx, ly := (float64(x)+.5-p.X(0))/p.sx, (float64(y)+.5-p.Y(0))/p.sy
			f := 0.0
			for _, b := range blobs {
				dx, dy := lx-b[0], ly-b[1]
				f += b[2] * b[2] / (dx*dx + dy*dy + 1e-6)
			}
			m[y][x] = f >= 2.6
			for _, s := range []float64{-1, 1} {
				a, b, c := ear(s, p)
				if inTri(lx, ly, a, b, c) {
					m[y][x] = true
				}
			}
		}
	}
	return
}

func (cv *canvas) body(p pose, tintColor rgb, tint float64) (m [H][W]bool) {
	m = bodyMask(p)
	minY, maxY := H, 0
	for y := range m {
		for x := range m[y] {
			if m[y][x] {
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	at := func(x, y int) bool { return x >= 0 && y >= 0 && x < W && y < H && m[y][x] }
	hlx, hly := p.X(-6.5), float64(minY)+2.6
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !m[y][x] {
				continue
			}
			t := float64(y-minY) / float64(max(1, maxY-minY))
			var c rgb
			if t < .5 {
				c = mix(cTop, cMid, t*2)
			} else {
				c = mix(cMid, cBot, (t-.5)*2)
			}
			if tint > 0 {
				c = mix(c, tintColor, tint)
			}
			if !at(x+1, y) || !at(x, y+1) {
				c = mix(c, cBot, .35)
			}
			dx, dy := (float64(x)+.5-hlx)/2.6, (float64(y)+.5-hly)/1.5
			if dx*dx+dy*dy <= 1 {
				c = mix(c, cHi, .65)
			}
			lx, ly := (float64(x)+.5-p.X(0))/p.sx, (float64(y)+.5-p.Y(0))/p.sy
			for _, sd := range []float64{-1, 1} {
				ta, tb, tc := earInner(sd, p)
				if inTri(lx, ly, ta, tb, tc) {
					c = mix(c, cBlush, .8)
				}
			}
			if !at(x-1, y) || !at(x+1, y) || !at(x, y-1) || !at(x, y+1) {
				c = cOutline
			}
			cv.blend(x, y, c, 1)
		}
	}
	return
}

type face struct {
	eyes   string // open, blink, happy, closed
	gx, gy float64
	mouth  string // smile, open, flat, sad, tiny
	blush  float64
	brows  bool
}

func (cv *canvas) face(p pose, f face) {
	for _, s := range []float64{-1, 1} {
		ex, ey := s*5.6+f.gx, -11.5+f.gy
		switch f.eyes {
		case "open":
			cv.ellipse(p.X(ex), p.Y(ey), 2.5*p.sx, 3.4*p.sy, cEye, 1)
			cv.blend(int(p.X(ex-.7)), int(p.Y(ey-1.5)), cWhite, 1)
		case "blink", "closed":
			for dx := -2.6; dx <= 2.6; dx += .4 {
				cv.plot(p, ex+dx, ey+.9-.9*(1-dx*dx/7), cEye, 1)
				cv.plot(p, ex+dx, ey+1.9-.9*(1-dx*dx/7), cEye, 1)
			}
		case "happy":
			for dx := -2.7; dx <= 2.7; dx += .3 {
				y := ey + 1 - 2.2*(1-dx*dx/7.3)
				cv.plot(p, ex+dx, y, cEye, 1)
				cv.plot(p, ex+dx, y+1, cEye, 1)
			}
		}
		if f.blush > 0 {
			cv.ellipse(p.X(s*10.2), p.Y(-6.8), 2.4*p.sx, 1.4*p.sy, cBlush, .55*f.blush)
		}
		if f.brows {
			cv.line(p, s*5.6-s*2.6, -17.3+0, s*5.6+s*2.4, -15.3, cEye) // inner ends lower→upper below
		}
	}
	my := -6.2
	switch f.mouth {
	case "smile":
		for dx := -2.2; dx <= 2.2; dx += .3 {
			cv.plot(p, dx, my+1.4*(1-dx*dx/4.9), cMouth, 1)
		}
	case "open":
		cv.ellipse(p.X(0), p.Y(my+.8), 2.4*p.sx, 2*p.sy, cMouth, 1)
		cv.ellipse(p.X(0), p.Y(my+1.7), 1.4*p.sx, .9*p.sy, cTongue, 1)
	case "flat":
		for dx := -1.4; dx <= 1.4; dx += .3 {
			cv.plot(p, dx, my+.8, cMouth, 1)
		}
	case "sad":
		for dx := -2.2; dx <= 2.2; dx += .3 {
			cv.plot(p, dx, my+1.8-1.4*(1-dx*dx/4.9), cMouth, 1)
		}
	case "tiny":
		cv.ellipse(p.X(0), p.Y(my+.9), 1.1, .9, cMouth, 1)
	}
}

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

var glyphZ = []string{"###", "..#", ".#.", "###"}

func (cv *canvas) zed(x, y int, a float64) {
	for r, row := range glyphZ {
		for c, ch := range row {
			if ch == '#' {
				cv.blend(x+c, y+r, cWhite, a)
			}
		}
	}
}

// trackPacket points the eyes at the packet closest to the mascot.
func trackPacket(t float64) (gx, gy float64) {
	best, bd := 0.0, 9.0
	for _, ph := range flow(t) {
		if d := math.Abs(ph - .5); d < bd {
			best, bd = ph, d
		}
	}
	x, y := packetPos(best)
	gx = math.Max(-2, math.Min(2, (x-cx)/6))
	if y < ground-14 {
		gy = -1.2
	} else {
		gy = .4
	}
	return
}

func frac(x float64) float64 { return x - math.Floor(x) }

// target returns the pose and face a mood wants at local time t.
func target(mood string, t float64) (pose, face) {
	p := pose{sx: S, sy: S}
	f := face{eyes: "open", mouth: "smile", blush: .35}
	switch mood {
	case "idle":
		s := math.Sin(2.2 * t)
		p.sy, p.sx = S*(1+.035*s), S*(1-.02*s)
		switch m := math.Mod(t, 6); {
		case m > 2 && m < 3.2:
			f.gx = -1
		case m > 4 && m < 5.2:
			f.gx = 1
		}
		if math.Mod(t, 3.2) < .14 {
			f.eyes = "blink"
		}
		if m := math.Mod(t, 4.1); m < .35 {
			p.ear = -1.6 * math.Sin(m/.35*math.Pi)
		}
	case "busy":
		g := gulp(t)
		p.lift = .4*math.Abs(math.Sin(6*t)) + 1.2*g
		p.sy = S * (1 + .03*math.Sin(12*t) + .07*g)
		p.sx = S * (1 - .03*g)
		p.ear = 1.4 * g
		f.gx, f.gy = trackPacket(t)
		f.mouth, f.blush = "flat", 0
		if g > .5 {
			f.mouth = "tiny"
		}
		if math.Mod(t, 2.4) > 2.3 {
			f.eyes = "blink"
		}
	case "happy":
		ph := frac(t / .85)
		p.lift = 3 * 4 * ph * (1 - ph)
		if ph < .1 || ph > .9 {
			p.sx, p.sy = S*1.12, S*.84
		} else {
			p.sx, p.sy = S*.95, S*1.08
		}
		p.ear = .9 * math.Sin(t*14)
		f = face{eyes: "happy", mouth: "open", blush: .95}
	case "error":
		p.shake = math.Sin(t*38) * math.Exp(-t*1.6) * 1.0
		p.sx, p.sy = S*1.03, S*.96
		p.droop = 5
		f = face{eyes: "open", mouth: "sad", brows: true}
	case "wait":
		s := math.Sin(1.6 * t)
		p.sy, p.sx = S*(1+.02*s), S*(1-.01*s)
		p.droop = 2
		f = face{eyes: "open", gx: -1.2, gy: -.8, mouth: "flat"}
	case "sleep":
		s := math.Sin(1.2 * t)
		p.sy, p.sx = S*(.94+.05*s), S*(1-.03*s)
		p.droop = 3
		f = face{eyes: "closed", mouth: "tiny", blush: .5}
	}
	return p, f
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
			if r == 1 {
				col = cGold
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

// gulp is 1 when a packet passes behind the mascot, so it can swallow it.
func gulp(t float64) float64 {
	g := 0.0
	for _, ph := range flow(t) {
		d := (ph - .5) / .12
		g = math.Max(g, math.Exp(-d*d))
	}
	return g
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

func scene(mood string, t float64, p pose, f face) *canvas {
	cv := &canvas{}
	h := p.lift
	cv.ellipse(cx+p.shake, ground+1.1, 8.5*(1-h/12), 1, rgb{0, 0, 0}, .35*(1-h/6))
	srcA, dstA := .35, .35
	dstBump := 0.0
	switch mood {
	case "busy":
		srcA, dstA = 1, 1
		for _, ph := range flow(t) {
			if ph > .93 {
				dstBump = 1
			}
		}
	case "happy":
		srcA, dstA = .5, 1
		dstBump = math.Max(0, math.Sin(t*7.4)) * .99
	}
	cv.folder(srcX, ground-folderRows+1, hx(170, 178, 255), hx(120, 130, 240), srcA)
	cv.folder(dstX, ground-folderRows+1-int(dstBump+.5), hx(255, 228, 140), cGold, dstA)
	for _, s := range []float64{-1, 1} {
		cv.ellipse(p.X(s*5.2), p.Y(.9), 3.1*p.sx, 1.0, cFoot, 1)
	}
	tintColor, tint := cCoral, 0.0
	switch mood {
	case "error":
		tint = .28
	case "wait":
		tintColor, tint = rgb{255, 200, 90}, .42
	}
	cv.body(p, tintColor, tint)
	cv.face(p, f)
	if mood == "busy" {
		cv.packets(t)
	}
	switch mood {
	case "busy":
		d := frac(t * .8)
		cv.blend(int(p.X(12)), int(p.Y(-17+d*5)), hx(150, 215, 255), 1-d)
		cv.blend(int(p.X(12)), int(p.Y(-18+d*5)), hx(200, 235, 255), 1-d)
	case "happy":
		pos := [][3]float64{{20, 4, 0}, {40, 5, .33}, {12, 9, .66}, {49, 6, .15}, {8, 3, .5}}
		for _, s := range pos {
			cv.star(int(s[0]), int(s[1]), frac(t*1.4+s[2]), cGold)
		}
	case "error":
		x, y := int(p.X(14.5)), 2
		for k := 0; k < 3; k++ {
			cv.blend(x, y+k, cCoral, 1)
		}
		cv.blend(x, y+4, cCoral, 1)
	case "wait":
		for i := 0; i < 3; i++ {
			if int(t*2)%4 > i {
				cv.blend(int(p.X(11))+i*2, int(p.Y(-24)), cWhite, 1)
			}
		}
	case "sleep":
		for i := 0.0; i < 2; i++ {
			ph := frac(t*.3 + i/2)
			cv.zed(int(p.X(11)+ph*4), int(p.Y(-14)-ph*5), math.Sin(ph*math.Pi))
		}
	}
	return cv
}
