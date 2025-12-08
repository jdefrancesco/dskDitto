package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

func TestResizeNearest(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(0, 0, color.RGBA{10, 0, 0, 255})
	src.Set(1, 0, color.RGBA{0, 20, 0, 255})
	src.Set(0, 1, color.RGBA{0, 0, 30, 255})
	src.Set(1, 1, color.RGBA{40, 50, 60, 255})

	dst := resizeNearest(src, 4, 4)

	if got, want := dst.Bounds().Dx(), 4; got != want {
		t.Fatalf("unexpected width: got %d want %d", got, want)
	}
	if got, want := dst.Bounds().Dy(), 4; got != want {
		t.Fatalf("unexpected height: got %d want %d", got, want)
	}

	assertColorClose(t, color.RGBAModel.Convert(dst.At(0, 0)).(color.RGBA), src.RGBAAt(0, 0))
	assertColorClose(t, color.RGBAModel.Convert(dst.At(3, 3)).(color.RGBA), src.RGBAAt(1, 1))
}

func TestApplyShadow(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 5))
	baseColor := color.RGBA{100, 150, 200, 255}
	src.Set(1, 1, baseColor)

	out := applyShadow(src)

	if got := color.RGBAModel.Convert(out.At(1, 1)).(color.RGBA); got != baseColor {
		t.Fatalf("original pixel altered: got %v want %v", got, baseColor)
	}

	shadowPixel := out.At(3, 2)
	r, g, b, a := shadowPixel.RGBA()
	if a == 0 {
		t.Fatalf("expected shadow pixel alpha to be >0, got 0")
	}
	if r == 0 && g == 0 && b == 0 {
		t.Fatalf("expected shadow pixel to carry color information, got %v", shadowPixel)
	}
}

func TestQuantizeWithDither(t *testing.T) {
	got := quantizeWithDither(128, 2, 0, 0)
	const want = uint8(64)
	if got != want {
		t.Fatalf("unexpected quantized value: got %d want %d", got, want)
	}
}

func TestRenderImageANSI(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(0, 1, color.RGBA{0, 0, 255, 255})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, r)
		copyDone <- err
	}()

	opts := Options{Width: 1, EnableShadow: false, PaletteBits: 8}
	RenderImageANSI(img, opts)

	w.Close()
	if err := <-copyDone; err != nil {
		t.Fatalf("failed to read render output: %v", err)
	}

	output := buf.String()
	var topR, topG, topB, bottomR, bottomG, bottomB int
	format := "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀\x1b[0m\n\x1b[0m"
	if _, err := fmt.Sscanf(output, format, &topR, &topG, &topB, &bottomR, &bottomG, &bottomB); err != nil {
		t.Fatalf("unexpected ANSI sequence: %q (parse error: %v)", output, err)
	}
	if topR < 240 || topG != 0 || topB != 0 {
		t.Fatalf("unexpected top color values: R=%d G=%d B=%d", topR, topG, topB)
	}
	if bottomR != 0 || bottomG != 0 || bottomB < 240 {
		t.Fatalf("unexpected bottom color values: R=%d G=%d B=%d", bottomR, bottomG, bottomB)
	}
}

func TestRenderImageANSIWithPNG(t *testing.T) {
	img := mustLoadFixtureImage(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()
	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, r)
		copyDone <- err
	}()

	opts := Options{Width: 80, EnableShadow: false, PaletteBits: 8}
	RenderImageANSI(img, opts)

	w.Close()
	if err := <-copyDone; err != nil {
		t.Fatalf("failed to read render output: %v", err)
	}

	output := buf.String()
	t.Logf("\n%s", output)
	if !strings.Contains(output, "▀") {
		t.Fatalf("expected output to contain block character, got %q", output)
	}

	expectedLines := expectedLineCount(img, opts)

	actualLines := strings.Count(output, "\n")
	if actualLines != expectedLines {
		t.Fatalf("unexpected line count: got %d want %d", actualLines, expectedLines)
	}

}

func TestRenderImageANSIToTerminal(t *testing.T) {
	if _, ok := os.LookupEnv("DSKDITTO_PRINT_ANSI_ART"); !ok {
		t.Skip("set DSKDITTO_PRINT_ANSI_ART=1 to view ANSI art output")
	}

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("unable to open /dev/tty: %v", err)
	}
	defer tty.Close()

	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()
	os.Stdout = tty

	img := mustLoadFixtureImage(t)
	opts := Options{Width: 80, EnableShadow: false, PaletteBits: 8}
	RenderImageANSI(img, opts)
	fmt.Fprintln(tty)
}

func expectedLineCount(img image.Image, opts Options) int {
	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()

	outW := opts.Width
	if outW <= 0 || outW > width {
		outW = width
	}

	scale := float64(outW) / float64(width)
	outH := int(math.Round(float64(height) * scale))
	if outH%2 != 0 {
		outH++
	}

	resized := resizeNearest(img, outW, outH)
	if opts.EnableShadow {
		resized = applyShadow(resized)
	}
	cropped := cropOpaqueRegion(resized)
	croppedH := cropped.Bounds().Dy()
	if croppedH%2 != 0 {
		croppedH++
	}
	return croppedH / 2
}

func assertColorClose(t *testing.T, got, want color.RGBA) {
	t.Helper()
	if abs(int(got.R)-int(want.R)) > 3 || abs(int(got.G)-int(want.G)) > 3 || abs(int(got.B)-int(want.B)) > 3 || abs(int(got.A)-int(want.A)) > 3 {
		t.Fatalf("color mismatch: got %v want %v", got, want)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func mustLoadFixtureImage(t testing.TB) image.Image {
	t.Helper()
	img, err := loadFixtureImage()
	if err != nil {
		t.Fatalf("failed to load png fixture: %v", err)
	}
	return img
}

func loadFixtureImage() (image.Image, error) {
	f, err := os.Open("testdata/image.png")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}
