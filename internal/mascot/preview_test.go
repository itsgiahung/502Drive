package mascot

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/muesli/termenv"
)

var previewMoods = [...]Mood{Idle, Busy, Happy, Error, Wait, Sleep}

type crop struct{ x0, y0, w, h int }

func miniCrop() crop { return crop{int(cx) - MiniCols/2, miniY0, MiniCols, MiniRows * 2} }

func previewBG(dark bool) color.RGBA {
	c := (Options{Dark: dark}).background()
	return color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 255}
}

// PNG/GIF previews use the same alpha compositing and terminal colour conversion.
func previewDraw(dst draw.Image, f *Frame, cr crop, ox, oy, scale int, o Options) {
	previewTerminalDraw(dst, f, cr, ox, oy, scale, scale, o)
}

func previewTerminalDraw(dst draw.Image, f *Frame, cr crop, ox, oy, pw, ph int, o Options) {
	bg := o.background()
	for y := 0; y < cr.h; y++ {
		for x := 0; x < cr.w; x++ {
			c, ok := final(f.cv[cr.y0+y][cr.x0+x], bg)
			col := previewBG(o.Dark)
			if ok {
				col = color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 255}
				if o.Profile == termenv.ANSI || o.Profile == termenv.ANSI256 {
					n := 256
					if o.Profile == termenv.ANSI {
						n = 16
					}
					c = terminalPalette[nearestTerminalColor(c, n)]
					col = color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 255}
				}

			}
			draw.Draw(dst, image.Rect(ox+x*pw, oy+y*ph, ox+(x+1)*pw, oy+(y+1)*ph), &image.Uniform{C: col}, image.Point{}, draw.Src)
		}
	}
}

// A small ASCII bitmap font keeps preview generation inside the standard library.
var previewFont = map[rune]string{
	'A': "010101111101101", 'B': "110101110101110", 'C': "011100100100011", 'D': "110101101101110",
	'E': "111100110100111", 'F': "111100110100100", 'G': "011100101101011", 'H': "101101111101101",
	'I': "111010010010111", 'J': "001001001101010", 'K': "101101110101101", 'L': "100100100100111",
	'M': "101111111101101", 'N': "101111111111101", 'O': "010101101101010", 'P': "110101110100100",
	'Q': "010101101111011", 'R': "110101110101101", 'S': "011100010001110", 'T': "111010010010010",
	'U': "101101101101111", 'V': "101101101101010", 'W': "101101111111101", 'X': "101101010101101",
	'Y': "101101010010010", 'Z': "111001010100111", '0': "111101101101111", '1': "010110010010111",
	'2': "110001010100111", '3': "110001010001110", '4': "101101111001001", '5': "111100110001110",
	'6': "011100111101111", '7': "111001010010010", '8': "111101111101111", '9': "111101111001110",
	'-': "000000111000000", '/': "001001010100100", '.': "000000000000010",
}

func previewLabel(dst draw.Image, text string, x, y int, dark bool) {
	col := color.RGBA{72, 76, 110, 255}
	if dark {
		col = color.RGBA{198, 204, 230, 255}
	}
	for _, ch := range text {
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		for i, bit := range previewFont[ch] {
			if bit == '1' {
				r := image.Rect(x+(i%3)*2, y+(i/3)*2, x+(i%3+1)*2, y+(i/3+1)*2)
				draw.Draw(dst, r, &image.Uniform{C: col}, image.Point{}, draw.Src)
			}
		}
		x += 8
	}
}

func previewFile(t *testing.T, path string, encode func(*os.File) error) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = encode(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

// MASCOT_PREVIEW writes dark/light, zoom/native raster, compact, low-colour,
// full-scene and strip previews. MASCOT_GIF writes a four-second animated sheet;
// MASCOT_GIF_DIR additionally exports individual moods and a transition loop.
func TestWritePreview(t *testing.T) {
	path, gifPath, gifDir := os.Getenv("MASCOT_PREVIEW"), os.Getenv("MASCOT_GIF"), os.Getenv("MASCOT_GIF_DIR")
	if path == "" && gifPath == "" && gifDir == "" {
		t.Skip("set MASCOT_PREVIEW or MASCOT_GIF")
	}
	const cell = 144
	if path != "" {
		img := image.NewRGBA(image.Rect(0, 0, cell*6, 1160))
		draw.Draw(img, img.Bounds(), &image.Uniform{C: previewBG(true)}, image.Point{}, draw.Src)
		for row, dark := range []bool{true, false} {
			y := row * 215
			draw.Draw(img, image.Rect(0, y, cell*6, y+215), &image.Uniform{C: previewBG(dark)}, image.Point{}, draw.Src)
			for i, m := range previewMoods {
				x := i*cell + 12
				previewLabel(img, string(m), x, y+10, dark)
				f := StaticMini(m, .6)
				previewDraw(img, f, miniCrop(), x, y+30, 6, Options{Dark: dark})
				previewLabel(img, "1X", x, y+178, dark)
				previewDraw(img, f, miniCrop(), x+30, y+170, 1, Options{Dark: dark})
			}
		}
		for row, profile := range []termenv.Profile{termenv.ANSI256, termenv.ANSI} {
			y := 440 + row*122
			previewLabel(img, profile.Name(), 12, y, true)
			for i, m := range previewMoods {
				previewDraw(img, StaticMini(m, 1), miniCrop(), i*cell+12, y+20, 4, Options{Dark: true, Profile: profile})
			}
		}
		previewLabel(img, fmt.Sprintf("COMPACT %dX%d", MiniCols, CompactRows), 12, 690, true)
		for i, m := range previewMoods {
			cv := StaticMini(m, .6).compactCanvas()
			previewDraw(img, &Frame{cv: &cv}, crop{int(cx) - MiniCols/2, H - CompactRows*2, MiniCols, CompactRows * 2}, i*cell+12, 710, 6, opts())
		}
		previewLabel(img, "FULL SCENE", 12, 810, true)
		previewDraw(img, Static(Busy, .7), crop{0, 0, W, H}, 12, 830, 5, opts())
		previewLabel(img, "TRANSFER STRIP", 340, 810, true)
		for i := 0; i < 3; i++ {
			previewDraw(img, Strip(float64(i)*.35), crop{0, 0, StripCols, StripRows * 2}, 340, 834+i*36, 4, opts())
		}
		previewLabel(img, "FIDGETS  YAWN  STRETCH  GROOM  GLANCE  SIT", 12, 980, true)
		for kind := 0; kind < 5; kind++ {
			for bucket := int64(0); ; bucket++ {
				if int(noise(motionSeed+4, bucket)*5) != kind {
					continue
				}
				ts := float64(bucket)*12 + 6.5 + noise(motionSeed+3, bucket) + 1.2
				previewDraw(img, StaticMini(Idle, ts), miniCrop(), kind*cell+12, 1010, 6, opts())
				break
			}
		}
		previewFile(t, path, func(f *os.File) error { return png.Encode(f, img) })
		// A separate 8x20-cell image avoids mistaking a zoomed atlas for a terminal.
		native := image.NewRGBA(image.Rect(0, 0, 480, 360))
		draw.Draw(native, native.Bounds(), &image.Uniform{C: previewBG(true)}, image.Point{}, draw.Src)
		for i, mood := range previewMoods {
			x, y := (i%3)*160+8, (i/3)*180
			previewLabel(native, string(mood), x, y+4, true)
			previewTerminalDraw(native, StaticMini(mood, .6), miniCrop(), x, y+20, 8, 10, opts())
		}
		nativePath := filepath.Join(filepath.Dir(path), "mascot-terminal.png")
		previewFile(t, nativePath, func(f *os.File) error { return png.Encode(f, native) })
	}
	if gifPath != "" {
		writePreviewGIF(t, gifPath, previewMoods[:], false)
	}
	if gifDir != "" {
		for _, mood := range previewMoods {
			writePreviewGIF(t, filepath.Join(gifDir, string(mood)+".gif"), []Mood{mood}, false)
		}
		writePreviewGIF(t, filepath.Join(gifDir, "transitions.gif"), []Mood{Idle}, true)
	}
}

func writePreviewGIF(t *testing.T, path string, moods []Mood, transitions bool) {
	t.Helper()
	const cell, rowHeight = 160, 180
	cols := min(3, len(moods))
	rows := (len(moods) + cols - 1) / cols
	hasStrip := transitions
	for _, mood := range moods {
		hasStrip = hasStrip || mood == Busy
	}
	width, height := cell*cols, rowHeight*rows
	if hasStrip {
		width = max(width, StripCols*8+20)
		height += StripRows*20 + 24
	}
	anims := make([]*Anim, len(moods))
	for i := range anims {
		anims[i] = NewMini()
	}
	g := &gif.GIF{LoopCount: 0}
	n := 60
	if transitions {
		n = 120
	}
	for tick := 0; tick < n; tick++ {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(img, img.Bounds(), &image.Uniform{C: previewBG(true)}, image.Point{}, draw.Src)
		showStrip := false
		for i, mood := range moods {
			if transitions {
				mood = []Mood{Idle, Busy, Happy, Idle, Error, Wait, Sleep, Idle}[tick/15]
			}
			if mood == Busy {
				showStrip = true
			}
			x, y := (width-cell*cols)/2+(i%cols)*cell+8, (i/cols)*rowHeight
			previewLabel(img, string(mood), x, y+4, true)
			f := anims[i].Step(mood, 1.0/15)
			previewTerminalDraw(img, f, miniCrop(), x, y+20, 8, 10, opts())
		}
		if showStrip {
			previewTerminalDraw(img, Strip(float64(tick)/15), crop{0, 0, StripCols, StripRows * 2}, (width-StripCols*8)/2, rowHeight*rows+12, 8, 10, opts())
		}
		// Preserve the exact flat colors instead of quantizing the sprite to Plan9.
		pal := image.NewPaletted(img.Bounds(), nil)
		indices := map[color.RGBA]uint8{}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := img.RGBAAt(x, y)
				idx, ok := indices[c]
				if !ok {
					if len(pal.Palette) == 256 {
						t.Fatal("preview exceeded GIF palette")
					}
					idx = uint8(len(pal.Palette))
					indices[c] = idx
					pal.Palette = append(pal.Palette, c)
				}
				pal.Pix[y*pal.Stride+x] = idx
			}
		}
		g.Image = append(g.Image, pal)
		// 7/6/7 centiseconds repeats to make fifteen frames exactly one second.
		g.Delay = append(g.Delay, []int{7, 6, 7}[tick%3])
	}
	previewFile(t, path, func(f *os.File) error { return gif.EncodeAll(f, g) })
}
