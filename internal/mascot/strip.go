package mascot

import "math"

var tinyFolder = []string{
	"oo....",
	"oHHHHo",
	"offffo",
	"oooooo",
}

func (cv *canvas) tinyFolder(x, y int, hi, fill rgb, a float64) {
	for r, row := range tinyFolder {
		for c, ch := range row {
			switch ch {
			case 'o':
				cv.blend(x+c, y+r, mix(cOutline, fill, .2), a)
			case 'H':
				cv.blend(x+c, y+r, hi, a)
			case 'f':
				cv.blend(x+c, y+r, fill, a)
			}
		}
	}
}

// Strip draws the file transfer in four text rows: a tiny source folder, a tiny
// destination folder and files that hop between them. It is shown only while
// something is copying or syncing.
func Strip(t float64) *Frame {
	cv := &canvas{}
	const left, right = 1, StripCols - 7
	for i, ph := range flow(t * 1.3) {
		kind := int(math.Floor(t*1.3*flowSpeed+float64(i)/3)) + i
		x := float64(left+4) + (float64(right-left-4))*ph
		y := 4.2 - 4.4*math.Sin(ph*math.Pi) // hop: starts low, peaks near the top
		base := paperTints[kind%len(paperTints)]
		for r := 0; r < 3; r++ {
			for c := 0; c < 2; c++ {
				col := base
				if r == 1 {
					col = cGold
				}
				cv.blend(int(x)+c, int(y)+r, col, 1)
			}
		}
	}
	bump := 0
	for _, ph := range flow(t * 1.3) {
		if ph > .94 {
			bump = 1
		}
	}
	cv.tinyFolder(left, 4, hx(170, 178, 255), hx(120, 130, 240), 1)
	cv.tinyFolder(right, 4-bump, hx(255, 228, 140), cGold, 1)
	return &Frame{cv}
}
