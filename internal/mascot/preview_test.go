package mascot

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestWritePreview writes a contact sheet of the small cat and the strip when
// MASCOT_PREVIEW names a file. It is how the pixel art is checked by eye.
func TestWritePreview(t *testing.T) {
	path := os.Getenv("MASCOT_PREVIEW")
	if path == "" {
		t.Skip("set MASCOT_PREVIEW=/path/sheet.png to write a preview")
	}
	const sc = 12
	type crop struct{ x0, y0, w, h int }
	mini := crop{int(cx) - MiniCols/2, miniY0, MiniCols, MiniRows * 2}
	strip := crop{0, 0, StripCols, StripRows * 2}
	frames := []struct {
		f *Frame
		c crop
	}{
		{StaticMini(Idle, .3), mini}, {StaticMini(Idle, 3.25), mini}, {StaticMini(Busy, 1), mini}, {StaticMini(Happy, .42), mini},
		{StaticMini(Error, .2), mini}, {StaticMini(Wait, 1), mini}, {StaticMini(Sleep, 1), mini},
		{Strip(0.2), strip}, {Strip(0.9), strip}, {Strip(1.6), strip},
	}
	img := image.NewRGBA(image.Rect(0, 0, StripCols*sc, (18+3*9)*sc+60))
	bg := color.RGBA{30, 30, 46, 255}
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, bg)
		}
	}
	draw := func(f *Frame, c crop, ox, oy int) {
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				if col, ok := final(f.cv[c.y0+y][c.x0+x], rgb{30, 30, 46}); ok {
					for dy := 0; dy < sc; dy++ {
						for dx := 0; dx < sc; dx++ {
							img.Set(ox+x*sc+dx, oy+y*sc+dy, color.RGBA{uint8(col.r), uint8(col.g), uint8(col.b), 255})
						}
					}
				}
			}
		}
	}
	for i := 0; i < 7; i++ {
		draw(frames[i].f, frames[i].c, i*MiniCols*sc, 0)
	}
	for i := 7; i < 10; i++ {
		draw(frames[i].f, frames[i].c, 0, 18*sc+20+(i-7)*9*sc)
	}
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if err = png.Encode(fh, img); err != nil {
		t.Fatal(err)
	}
}
