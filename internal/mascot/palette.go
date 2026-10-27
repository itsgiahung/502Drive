package mascot

import "github.com/muesli/termenv"

type rgb struct{ r, g, b float64 }
type px struct {
	c rgb
	a float64
	// ch, when set, is printed in colour c instead of a half block: small
	// text (the sleeping "z Z") stays crisp where pixels cannot draw a letter.
	ch rune
}
type canvas [H][W]px

func hx(r, g, b float64) rgb { return rgb{r, g, b} }
func mix(a, b rgb, t float64) rgb {
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

var (
	cFur      = hx(139, 210, 235)
	cFurShade = hx(104, 132, 196)
	cMid      = hx(98, 172, 255)
	cOutline  = hx(40, 42, 104)
	cHi       = hx(232, 250, 255)
	cBlush    = hx(255, 140, 172)
	cEye      = hx(28, 26, 60)
	cGold     = hx(255, 210, 90)
	cCoral    = hx(255, 130, 130)
	cWhite    = hx(255, 255, 255)
)

// Terminal palettes are immutable. A nearest RGB lookup avoids the expensive
// perceptual conversion (and its allocations) on every animation frame.
var terminalPalette = func() (out [256]rgb) {
	for i := range out {
		r, g, b := termenv.ConvertToRGB(termenv.ANSI256Color(i)).RGB255()
		out[i] = rgb{float64(r), float64(g), float64(b)}
	}
	return
}()

func nearestTerminalColor(c rgb, count int) int {
	best, distance := 0, 1e9
	for i, p := range terminalPalette[:count] {
		dr, dg, db := c.r-p.r, c.g-p.g, c.b-p.b
		d := dr*dr + dg*dg + db*db
		if d < distance {
			best, distance = i, d
		}
	}
	return best
}
