package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Menu glyph keys. Glyphs are drawn in code (no assets to ship) as shapes in a
// unit square, rendered at 4x and averaged down so the edges stay smooth.
const (
	glyphCopy     = "copy"
	glyphWindow   = "window"
	glyphList     = "list"
	glyphFolder   = "folder"
	glyphPause    = "pause"
	glyphPlay     = "play"
	glyphSettings = "settings"
	glyphExit     = "exit"
	glyphStop     = "stop"
)

// glyphColor is a mid grey that reads on both dark and light menus.
var glyphColor = color.NRGBA{R: 0x9a, G: 0xa1, B: 0xab, A: 0xff}

func box(x, y, x0, y0, x1, y1 float64) bool { return x >= x0 && x <= x1 && y >= y0 && y <= y1 }

func outline(x, y, x0, y0, x1, y1, t float64) bool {
	return box(x, y, x0, y0, x1, y1) && !box(x, y, x0+t, y0+t, x1-t, y1-t)
}

func disc(x, y, cx, cy, r float64) bool { return math.Hypot(x-cx, y-cy) <= r }

// glyphs maps a key to a shape test over the unit square.
var glyphs = map[string]func(x, y float64) bool{
	glyphCopy: func(x, y float64) bool {
		return outline(x, y, .12, .12, .64, .64, .09) || box(x, y, .36, .36, .88, .88)
	},
	glyphWindow: func(x, y float64) bool {
		return outline(x, y, .1, .16, .9, .84, .08) || box(x, y, .1, .16, .9, .36)
	},
	glyphList: func(x, y float64) bool {
		for _, cy := range []float64{.26, .5, .74} {
			if box(x, y, .3, cy-.05, .9, cy+.05) || disc(x, y, .16, cy, .06) {
				return true
			}
		}
		return false
	},
	glyphFolder: func(x, y float64) bool {
		return box(x, y, .08, .34, .92, .8) || box(x, y, .08, .2, .42, .38)
	},
	glyphPause: func(x, y float64) bool {
		return box(x, y, .24, .18, .42, .82) || box(x, y, .58, .18, .76, .82)
	},
	glyphPlay: func(x, y float64) bool {
		return x >= .28 && x <= .8 && math.Abs(y-.5) <= (.8-x)/(.8-.28)*.32
	},
	glyphSettings: func(x, y float64) bool {
		for i, cy := range []float64{.26, .5, .74} {
			kx := []float64{.66, .34, .7}[i]
			if box(x, y, .1, cy-.035, .9, cy+.035) || disc(x, y, kx, cy, .11) {
				return true
			}
		}
		return false
	},
	glyphExit: func(x, y float64) bool {
		return box(x, y, .2, .2, .8, .8) && (math.Abs(x-y) <= .07 || math.Abs(x+y-1) <= .07)
	},
	glyphStop: func(x, y float64) bool { return box(x, y, .22, .22, .78, .78) },
}

// glyphImage renders one glyph at size x size, or nil for an unknown key.
func glyphImage(key string, size int) *image.NRGBA {
	shape, ok := glyphs[key]
	if !ok {
		return nil
	}
	const ss = 4
	big := image.NewNRGBA(image.Rect(0, 0, size*ss, size*ss))
	n := float64(size * ss)
	for y := range size * ss {
		for x := range size * ss {
			if shape((float64(x)+.5)/n, (float64(y)+.5)/n) {
				big.SetNRGBA(x, y, glyphColor)
			}
		}
	}
	return resize(big, size)
}

// menuGlyphPNG is the PNG a menu row shows, or nil when the key has no glyph.
func menuGlyphPNG(key string) []byte {
	img := glyphImage(key, 32)
	if img == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// opaqueEdges snaps every pixel to fully opaque or fully transparent. The
// Linux StatusNotifier layer of the systray library keeps only the low byte of
// the premultiplied 16-bit channels, which scrambles the colours of any
// semi-transparent pixel; a hard edge avoids that at the cost of a slightly
// coarser corner.
func opaqueEdges(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	for i := 0; i+3 < len(src.Pix); i += 4 {
		if src.Pix[i+3] >= 128 {
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = src.Pix[i], src.Pix[i+1], src.Pix[i+2], 0xff
		}
	}
	return dst
}

// trayPNG is the PNG handed to the Linux tray: the icon with hard edges.
func trayPNG(st State, frame, size int) ([]byte, error) {
	img, err := drawIcon(st, frame, size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, opaqueEdges(img)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
