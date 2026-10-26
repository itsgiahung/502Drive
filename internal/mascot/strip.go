package mascot

import "math"

// stripFolder splits the open lid/back and front lip so a packet really passes
// through the opening instead of being painted over the folder.
func (cv *canvas) stripFolder(x int, hi, fill rgb, open float64, front bool) {
	lift := int(math.Round(open))
	if !front {
		for y, row := range []string{"oo.....", "oHHoooo", "oHHHHHo", "ooooooo"} {
			for dx, ch := range row {
				c := cOutline
				if ch == 'H' {
					c = hi
				}
				if ch != '.' {
					cv.blend(x+dx, 3-lift+y, c, 1)
				}
			}
		}
		return
	}
	for y := 0; y < 3; y++ {
		for dx := 0; dx < 7; dx++ {
			c := fill
			if dx == 0 || dx == 6 || y == 2 {
				c = mix(cOutline, fill, .2)
			}
			if y == 0 && dx > 0 && dx < 6 {
				c = hi
			}
			cv.blend(x+dx, 5+y, c, 1)
		}
	}
	cv.blend(x+4, 6, mix(hi, cWhite, .4), .8)
}

func stripPos(ph float64) (x, y float64) {
	x = 5 + 44*smooth(ph)
	y = 3.6 - 3.1*math.Sin(ph*math.Pi)
	return
}

// Strip is a separate four-row transfer scene; callers show it only while busy.
func Strip(t float64) *Frame {
	t = sceneTime(t)
	cv := &canvas{}
	phases := flow(t * 1.3)
	srcOpen, dstOpen := 0.0, 0.0
	for _, ph := range phases {
		srcOpen = max(srcOpen, pulse(ph, 0, .25))
		dstOpen = max(dstOpen, pulse(ph, .75, .25))
	}
	srcHi, srcFill := hx(171, 192, 255), hx(116, 130, 229)
	dstHi, dstFill := hx(255, 230, 157), cGold
	cv.stripFolder(1, srcHi, srcFill, srcOpen, false)
	cv.stripFolder(48, dstHi, dstFill, dstOpen, false)
	for i, ph := range phases {
		kind := (int(math.Floor(t*1.3*flowSpeed+float64(i)/3)) + i) % 3
		for j := 1; j <= 3; j++ {
			x, y := stripPos(max(0, ph-float64(j)*.035))
			cv.blend(int(x), int(y)+1, []rgb{cGold, cBlush, cHi}[j-1], .55/float64(j))
		}
		x, y := stripPos(ph)
		cv.paper(int(x), int(y), kind, 1, nil)
		if ph > .88 {
			cv.star(48, 2, (ph-.88)/.12, cGold)
		}
	}
	cv.stripFolder(1, srcHi, srcFill, srcOpen, true)
	cv.stripFolder(48, dstHi, dstFill, dstOpen, true)
	return &Frame{cv: cv}
}
