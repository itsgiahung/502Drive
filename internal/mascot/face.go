package mascot

import "math"

type face struct {
	gx, gy, pupil, blink, joy, open, smile, blush, think float64
}

func (s sprite) face(f face) {
	look := max(-1, min(1, int(math.Round(f.gx))))
	for _, x := range []int{4, 9} {
		switch {
		case f.joy > .6:
			s.pixel(x, 3, cEye)
			s.pixel(x-1, 4, cEye)
			s.pixel(x+1, 4, cEye)
		case f.blink > .55:
			s.pixel(x, 4, cEye)
			if x == 4 {
				s.pixel(x-1, 4, cEye)
			} else {
				s.pixel(x+1, 4, cEye)
			}
		default:
			// One column, two pixels tall. Wide (2x2) eyes read as a big dark
			// square at this size, so excitement shows elsewhere (ears, mouth).
			s.pixel(x+look, 3, cEye)
			s.pixel(x+look, 4, cEye)
		}
	}
	s.pixel(6, 5, cEye)
	s.pixel(7, 5, cEye)
	if f.open > .35 {
		s.pixel(6, 6, cEye)
		s.pixel(7, 6, cEye)
		if f.open > .7 {
			s.pixel(6, 6, cBlush)
			s.pixel(7, 6, cBlush)
		}
	}
	if f.blush > .75 {
		s.pixel(2, 5, cBlush)
		s.pixel(11, 5, cBlush)
	}
}
