package tray

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is the tray appearance. The project logo is embedded and the state is
// drawn over it in code, so nothing is loaded from the network or disk.
type State string

const (
	StateIdle         State = "idle"
	StateBusy         State = "busy"
	StateNeeds        State = "needs"
	StateError        State = "error"
	StateOffline      State = "offline"
	StatePaused       State = "paused"
	StateReconnecting State = "reconnecting"
)

// AllStates lists every icon state in the order the menu documents them.
var AllStates = []State{StateBusy, StateIdle, StatePaused, StateOffline, StateReconnecting, StateNeeds, StateError}

const (
	// iconSize is the PNG size used for the tray (scaled by the desktop).
	iconSize = 64
	// minIcon and maxIcon bound the sizes PNG will render.
	minIcon = 16
	maxIcon = 512
	// busyFrames and busyFrameMS cap the busy animation at 4 fps.
	busyFrames  = 4
	busyFrameMS = 250
)

//go:embed assets/logo-512.png
var logoPNG []byte

var (
	logoOnce sync.Once
	logoImg  *image.NRGBA
	logoErr  error
)

// logo decodes the embedded 512x512 project logo once.
func logo() (*image.NRGBA, error) {
	logoOnce.Do(func() {
		src, err := png.Decode(bytes.NewReader(logoPNG))
		if err != nil {
			logoErr = err
			return
		}
		b := src.Bounds()
		if b.Dx() != 512 || b.Dy() != 512 {
			logoErr = fmt.Errorf("embedded logo is %dx%d, want 512x512", b.Dx(), b.Dy())
			return
		}
		dst := image.NewNRGBA(b)
		draw.Draw(dst, b, src, b.Min, draw.Src)
		logoImg = trimMargin(dst)
	})
	return logoImg, logoErr
}

// trimMargin crops the transparent border around the artwork and returns a
// square, so the logo fills the tray slot as large as its neighbours do.
func trimMargin(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if src.NRGBAAt(x, y).A > 8 {
				minX, maxX = min(minX, x), max(maxX, x+1)
				minY, maxY = min(minY, y), max(maxY, y+1)
			}
		}
	}
	if maxX <= minX || maxY <= minY {
		return src
	}
	side := max(maxX-minX, maxY-minY)
	x0 := min(max(minX-(side-(maxX-minX))/2, b.Min.X), b.Max.X-side)
	y0 := min(max(minY-(side-(maxY-minY))/2, b.Min.Y), b.Max.Y-side)
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(x0, y0), draw.Src)
	return dst
}

// resize shrinks src to size x size by area averaging in premultiplied space,
// which keeps edges clean at 16-64 px where nearest-neighbour looks broken.
func resize(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(src.Bounds().Dx()) / float64(size)
	for y := 0; y < size; y++ {
		y0, y1 := float64(y)*f, float64(y+1)*f
		for x := 0; x < size; x++ {
			x0, x1 := float64(x)*f, float64(x+1)*f
			var r, g, b, a, wsum float64
			for sy := int(y0); sy < int(math.Ceil(y1)) && sy < src.Bounds().Dy(); sy++ {
				wy := math.Min(y1, float64(sy+1)) - math.Max(y0, float64(sy))
				for sx := int(x0); sx < int(math.Ceil(x1)) && sx < src.Bounds().Dx(); sx++ {
					w := wy * (math.Min(x1, float64(sx+1)) - math.Max(x0, float64(sx)))
					c := src.NRGBAAt(sx, sy)
					al := float64(c.A) / 255
					r += float64(c.R) * al * w
					g += float64(c.G) * al * w
					b += float64(c.B) * al * w
					a += al * w
					wsum += w
				}
			}
			if a > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r/a + 0.5), G: uint8(g/a + 0.5), B: uint8(b/a + 0.5), A: uint8(a/wsum*255 + 0.5)})
			}
		}
	}
	return dst
}

// dot paints an anti-aliased filled circle with a dark rim, used for the
// needs-action and error badges.
func dot(img *image.NRGBA, cx, cy, r float64, fill color.NRGBA) {
	rim := color.NRGBA{R: 0x0a, G: 0x10, B: 0x1c, A: 0xff}
	for y := int(cy - r - 2); y <= int(cy+r+2); y++ {
		for x := int(cx - r - 2); x <= int(cx+r+2); x++ {
			if !(image.Point{x, y}.In(img.Bounds())) {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			switch {
			case d <= r-1:
				img.SetNRGBA(x, y, fill)
			case d <= r+1:
				img.SetNRGBA(x, y, rim)
			}
		}
	}
}

// ring paints one quarter of a thin bright ring around the icon; the quarter
// moves with the frame, which is the busy animation.
func ring(img *image.NRGBA, size, frame int) {
	c := float64(size) / 2
	radius := c * 0.94
	width := math.Max(1.5, float64(size)/28)
	start := float64(frame%busyFrames) * math.Pi / 2
	glow := color.NRGBA{R: 0x5c, G: 0xc0, B: 0xff, A: 0xff}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			if math.Abs(math.Hypot(dx, dy)-radius) > width/2 {
				continue
			}
			ang := math.Mod(math.Atan2(dy, dx)-start+4*math.Pi, 2*math.Pi)
			if ang < math.Pi/2 {
				img.SetNRGBA(x, y, glow)
			}
		}
	}
}

// drawIcon renders the logo for a state at any supported size.
func drawIcon(st State, frame, size int) (*image.NRGBA, error) {
	base, err := logo()
	if err != nil {
		return nil, err
	}
	img := resize(base, size)
	s := float64(size)
	switch st {
	case StateBusy:
		ring(img, size, frame)
	case StateNeeds:
		dot(img, s*0.78, s*0.78, s*0.17, color.NRGBA{R: 0xff, G: 0xb5, B: 0x34, A: 0xff})
	case StateError:
		dot(img, s*0.78, s*0.78, s*0.17, color.NRGBA{R: 0xff, G: 0x4d, B: 0x4d, A: 0xff})
	case StateOffline:
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] /= 2
		}
	case StatePaused:
		// Two bars in the corner: they stay readable at 16px, unlike a thin symbol.
		bar := color.NRGBA{R: 0xf2, G: 0xf2, B: 0xf2, A: 0xff}
		rect(img, s*0.64, s*0.62, s*0.75, s*0.92, bar)
		rect(img, s*0.82, s*0.62, s*0.93, s*0.92, bar)
	case StateReconnecting:
		dot(img, s*0.78, s*0.78, s*0.17, color.NRGBA{R: 0x4d, G: 0xa6, B: 0xff, A: 0xff})
	}
	return img, nil
}

// PNG encodes one icon frame.
func PNG(st State, frame, size int) ([]byte, error) {
	if size < minIcon || size > maxIcon {
		return nil, fmt.Errorf("icon size %d must be between %d and %d", size, minIcon, maxIcon)
	}
	img, err := drawIcon(st, frame, size)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FrameAt returns the busy animation frame at now: four frames per second, so
// the tray never animates faster than 4 fps.
func FrameAt(now time.Time) int {
	return int(now.UnixMilli()/busyFrameMS) % busyFrames
}

// ICO wraps PNG images in an ICO container, which Windows accepts for tray and
// shell icons. Each directory entry points at a PNG payload.
func ICO(st State, sizes []int) ([]byte, error) { return icoFrame(st, 0, sizes) }

// icoFrame is ICO for one frame of the busy animation.
func icoFrame(st State, frame int, sizes []int) ([]byte, error) {
	if len(sizes) == 0 {
		return nil, errors.New("an ICO needs at least one size")
	}
	type image struct {
		size int
		data []byte
	}
	images := make([]image, 0, len(sizes))
	for _, size := range sizes {
		data, err := PNG(st, frame, size)
		if err != nil {
			return nil, err
		}
		images = append(images, image{size: size, data: data})
	}
	var buf bytes.Buffer
	buf.Write([]byte{0, 0})                            // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(images)))
	offset := 6 + 16*len(images)
	for _, img := range images {
		dim := byte(img.size)
		if img.size >= 256 {
			dim = 0 // 256 is encoded as 0
		}
		buf.WriteByte(dim)                                  // width
		buf.WriteByte(dim)                                  // height
		buf.WriteByte(0)                                    // palette
		buf.WriteByte(0)                                    // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&buf, binary.LittleEndian, uint32(len(img.data)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(img.data)
	}
	for _, img := range images {
		buf.Write(img.data)
	}
	return buf.Bytes(), nil
}

// IconFiles are the assets written by `502drive tray --write-icons`.
const (
	IconPNG = "502drive.png"
	IconICO = "502drive.ico"
)

// WriteIcons writes the desktop icon set into dir (created if needed).
func WriteIcons(dir string) error {
	if dir == "" {
		return errors.New("an icon directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	big, err := PNG(StateIdle, 0, 256)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, IconPNG), big, 0o644); err != nil {
		return err
	}
	ico, err := ICO(StateIdle, []int{16, 32, 48, 64, 256})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, IconICO), ico, 0o644)
}

// decodeIcon is used by tests to check the produced image.
func decodeIcon(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

// rect fills an axis-aligned box in icon pixels, clipped to the image.
func rect(img *image.NRGBA, x0, y0, x1, y1 float64, fill color.NRGBA) {
	b := img.Bounds()
	for y := max(b.Min.Y, int(y0)); y < min(b.Max.Y, int(y1)+1); y++ {
		for x := max(b.Min.X, int(x0)); x < min(b.Max.X, int(x1)+1); x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
}
