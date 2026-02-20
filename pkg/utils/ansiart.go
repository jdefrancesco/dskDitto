package utils

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EXAMPLE
//  [go:embed] assets/help-icon.png
// var helpIconPNG []byte

// func printHelpIcon() {
//     img, _, err := image.Decode(bytes.NewReader(helpIconPNG))
//     if err != nil {
//         log.Printf("help icon decode failed: %v", err)
//         return
//     }
//     utils.RenderImageANSI(img, utils.Options{Width: 60, PaletteBits: 4})
//     fmt.Println()
// }

// Options controls rendering behaviour.
type Options struct {
	Width        int // target width in terminal cells
	EnableShadow bool
	PaletteBits  int // bits per channel (set <8 to enable ordered dithering)
}

// resizeNearest does a basic nearest-neighbour resize (no deps).
func resizeNearest(src image.Image, newW, newH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	b := src.Bounds()

	if newW == b.Dx() && newH == b.Dy() {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}

	scaleX := float64(b.Dx()) / float64(newW)
	scaleY := float64(b.Dy()) / float64(newH)

	sample := func(x, y int) color.NRGBA {
		x = clamp(x, b.Min.X, b.Max.X-1)
		y = clamp(y, b.Min.Y, b.Max.Y-1)
		return color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
	}

	for y := 0; y < newH; y++ {
		srcY := float64(b.Min.Y) + (float64(y)+0.5)*scaleY - 0.5
		y0 := int(math.Floor(srcY))
		y1 := y0 + 1
		fy := srcY - float64(y0)

		for x := 0; x < newW; x++ {
			srcX := float64(b.Min.X) + (float64(x)+0.5)*scaleX - 0.5
			x0 := int(math.Floor(srcX))
			x1 := x0 + 1
			fx := srcX - float64(x0)

			c00 := sample(x0, y0)
			c10 := sample(x1, y0)
			c01 := sample(x0, y1)
			c11 := sample(x1, y1)

			r := bilerp(c00.R, c10.R, c01.R, c11.R, fx, fy)
			g := bilerp(c00.G, c10.G, c01.G, c11.G, fx, fy)
			bCol := bilerp(c00.B, c10.B, c01.B, c11.B, fx, fy)
			a := bilerp(c00.A, c10.A, c01.A, c11.A, fx, fy)

			dst.Set(x, y, color.NRGBA{uint8(r), uint8(g), uint8(bCol), uint8(a)})
		}
	}
	return dst
}

// applyShadow composites a dark, slightly blurred offset copy behind src.
func applyShadow(src *image.RGBA) *image.RGBA {
	w := src.Bounds().Dx()
	h := src.Bounds().Dy()

	shadow := image.NewRGBA(src.Bounds())

	// Create dark offset copy.
	const (
		offsetX = 2
		offsetY = 1
		// Value 0 to 1 controls how dark.
		intensity = 0.4
	)

	for y := range h {
		for x := range w {
			r, g, b, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			sx := x + offsetX
			sy := y + offsetY
			if sx >= w || sy >= h {
				continue
			}
			dr := uint8(float64(r>>8) * intensity)
			dg := uint8(float64(g>>8) * intensity)
			db := uint8(float64(b>>8) * intensity)
			shadow.Set(sx, sy, color.RGBA{dr, dg, db, 255})
		}
	}

	// Very small box blur to soften the shadow edges.
	blur := func(src *image.RGBA) *image.RGBA {
		dst := image.NewRGBA(src.Bounds())
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var rs, gs, bs, as, count uint32
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx := x + dx
						ny := y + dy
						if nx < 0 || ny < 0 || nx >= w || ny >= h {
							continue
						}
						r, g, b, a := src.At(nx, ny).RGBA()
						rs += r
						gs += g
						bs += b
						as += a
						count++
					}
				}
				if count == 0 {
					continue
				}
				avgR := rs / count
				avgG := gs / count
				avgB := bs / count
				avgA := as / count
				dst.Set(x, y, color.RGBA{
					R: fromRGBA16(avgR),
					G: fromRGBA16(avgG),
					B: fromRGBA16(avgB),
					A: fromRGBA16(avgA),
				})
			}
		}
		return dst
	}
	shadow = blur(shadow)

	//  Composite shadow behind src (simple "over" with opaque fg).
	out := image.NewRGBA(src.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fr, fg, fb, fa := src.At(x, y).RGBA()
			sr, sg, sb, sa := shadow.At(x, y).RGBA()

			if fa>>8 > 0 { // foreground pixel wins
				out.Set(x, y, color.RGBA{
					R: fromRGBA16(fr),
					G: fromRGBA16(fg),
					B: fromRGBA16(fb),
					A: 255,
				})
			} else if sa>>8 > 0 {
				out.Set(x, y, color.RGBA{
					R: fromRGBA16(sr),
					G: fromRGBA16(sg),
					B: fromRGBA16(sb),
					A: 255,
				})
			} else {
				out.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	return out
}

// 4x4 Bayer matrix for ordered dithering (0..15).
var bayer4 = [4][4]float64{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// quantizeWithDither crushes a color channel with an ordered dither.
// bitsPerChan: 1–4 (higher = more colors, less retro).
func quantizeWithDither(v uint8, bitsPerChan int, x, y int) uint8 {
	if bitsPerChan <= 0 {
		return v
	}
	if bitsPerChan > 8 {
		bitsPerChan = 8
	}
	levels := 1 << bitsPerChan       // e.g. 8 for 3 bits
	step := 256.0 / float64(levels)  // size of each bucket
	b := bayer4[y&3][x&3]/16.0 - 0.5 // -0.5..+0.5
	val := float64(v) + b*step*0.75  // tweak factor for strength
	if val < 0 {
		val = 0
	}
	if val > 255 {
		val = 255
	}
	q := int(val / step)
	if q >= levels {
		q = levels - 1
	}
	return uint8(float64(q) * step)
}

// RenderImageANSI prints an image as 8-bit-style ANSI art using foreground /
// background colors and the ▀ character (top half block). It applies optional
// shadow + ordered dithering.
func RenderImageANSI(img image.Image, opts Options) {
	if opts.PaletteBits <= 0 {
		opts.PaletteBits = 8 // default to full color unless caller opts in to dithering
	}
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()

	outW := opts.Width
	if outW <= 0 || outW > w {
		outW = w
	}

	// Height in terminal rows (2 image pixels per cell vertically).
	scale := float64(outW) / float64(w)
	outH := int(math.Round(float64(h) * scale))
	if outH%2 != 0 {
		outH++
	}

	resized := resizeNearest(img, outW, outH)
	if opts.EnableShadow {
		resized = applyShadow(resized)
	}
	resized = cropOpaqueRegion(resized)
	outW = resized.Bounds().Dx()
	outH = resized.Bounds().Dy()

	useQuant := opts.PaletteBits > 0 && opts.PaletteBits < 8

	for y := 0; y < outH; y += 2 {
		for x := 0; x < outW; x++ {
			top := color.NRGBAModel.Convert(resized.At(x, y)).(color.NRGBA)
			bottom := color.NRGBA{A: 0}
			if y+1 < outH {
				bottom = color.NRGBAModel.Convert(resized.At(x, y+1)).(color.NRGBA)
			}

			ta := top.A
			ba := bottom.A

			// Quantise visible pixels only so transparent regions stay untouched.
			var tr, tg, tb, br, bg, bb uint8
			if ta > 0 {
				if useQuant {
					tr = quantizeWithDither(top.R, opts.PaletteBits, x, y)
					tg = quantizeWithDither(top.G, opts.PaletteBits, x, y)
					tb = quantizeWithDither(top.B, opts.PaletteBits, x, y)
				} else {
					tr, tg, tb = top.R, top.G, top.B
				}
			}
			if ba > 0 {
				if useQuant {
					br = quantizeWithDither(bottom.R, opts.PaletteBits, x, y+1)
					bg = quantizeWithDither(bottom.G, opts.PaletteBits, x, y+1)
					bb = quantizeWithDither(bottom.B, opts.PaletteBits, x, y+1)
				} else {
					br, bg, bb = bottom.R, bottom.G, bottom.B
				}
			}

			switch {
			case ta == 0 && ba == 0:
				fmt.Print("\x1b[0m ")
			case ta == 0:
				// Only bottom pixel is visible → draw lower half block.
				fmt.Printf("\x1b[0m\x1b[38;2;%d;%d;%dm▄", br, bg, bb)
			case ba == 0:
				// Only top pixel visible → draw upper half block and reset bg.
				fmt.Printf("\x1b[38;2;%d;%d;%dm\x1b[49m▀", tr, tg, tb)
			default:
				// Both visible → regular top-half block with background colour.
				fmt.Printf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
					tr, tg, tb,
					br, bg, bb,
				)
			}
		}
		fmt.Print("\x1b[0m\n")
	}
	fmt.Print("\x1b[0m") // final reset
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: image.(png|jpg) [width]")
		os.Exit(2)
	}

	inputPath := filepath.Clean(os.Args[1])
	if inputPath == "" || inputPath == "." || inputPath == ".." {
		fmt.Fprintln(os.Stderr, "invalid input path")
		os.Exit(2)
	}
	if filepath.IsAbs(inputPath) {
		fmt.Fprintln(os.Stderr, "absolute input paths are not allowed")
		os.Exit(2)
	}
	if inputPath == ".." || strings.HasPrefix(inputPath, ".."+string(os.PathSeparator)) {
		fmt.Fprintln(os.Stderr, "input path escapes working directory")
		os.Exit(2)
	}

	// #nosec G703 -- CLI helper: user supplies a local path to read.
	f, err := os.Open(inputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	opts := Options{
		Width:        64, // default
		EnableShadow: true,
		PaletteBits:  3, // 3 bits/channel ≈ 512 colors, nice retro
	}
	if len(os.Args) >= 3 {
		if w, err := strconv.Atoi(os.Args[2]); err == nil && w > 0 {
			opts.Width = w
		}
	}

	RenderImageANSI(img, opts)
}

func bilerp(c00, c10, c01, c11 uint8, fx, fy float64) float64 {
	c00f := float64(c00)
	c10f := float64(c10)
	c01f := float64(c01)
	c11f := float64(c11)

	return c00f*(1-fx)*(1-fy) + c10f*fx*(1-fy) + c01f*(1-fx)*fy + c11f*fx*fy
}

// cropOpaqueRegion trims fully transparent borders so the rendered art focuses on
// visible pixels.
func cropOpaqueRegion(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	found := false

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a>>8 > 0 {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
				found = true
			}
		}
	}

	if !found {
		return img
	}

	rect := image.Rect(minX, minY, maxX+1, maxY+1)
	if rect == b {
		return img
	}

	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	return dst
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func fromRGBA16(v uint32) uint8 {
	// Values produced by color.RGBA() occupy the 0-65535 range; clamp before narrowing to a byte.
	val := v >> 8
	if val > 0xFF {
		return 0xFF
	}
	return byte(val)
}
