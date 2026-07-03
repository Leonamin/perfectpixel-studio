package sprite

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hamsterWalkEastCleanFixture = "/home/leonamin/podong-assets/hamster-ew-baseseed/debug/walk-east/attempt-01-clean.png"
const catWalkEastGridFixture = "/home/leonamin/dev/podong-podong/scratchpad/cat-full/debug/walk-east/attempt-01-clean.png"

func loadNRGBAFixture(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("fixture not found: %s", path)
		}
		t.Fatalf("fixture open failed: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("fixture decode failed: %v", err)
	}
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x-b.Min.X, y-b.Min.Y, img.At(x, y))
		}
	}
	return out
}

func frameContentBoxes(frames []*image.NRGBA) []FrameRect {
	boxes := make([]FrameRect, len(frames))
	for i, f := range frames {
		boxes[i] = contentBBox(f)
	}
	return boxes
}

func firstOpaqueRGB(t *testing.T, f *image.NRGBA) [3]uint8 {
	t.Helper()
	for y := 0; y < f.Rect.Dy(); y++ {
		for x := 0; x < f.Rect.Dx(); x++ {
			i := f.PixOffset(x, y)
			if f.Pix[i+3] > alphaThreshold {
				return [3]uint8{f.Pix[i], f.Pix[i+1], f.Pix[i+2]}
			}
		}
	}
	t.Fatal("frame has no opaque pixels")
	return [3]uint8{}
}

func hasWarningContaining(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestExtractGridLayout2x3(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 360, 220))
	colors := [][3]uint8{
		{210, 30, 30},
		{30, 180, 30},
		{40, 80, 210},
		{220, 170, 30},
		{30, 180, 190},
		{180, 80, 210},
	}
	i := 0
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			x0 := 22 + col*120
			y0 := 24 + row*100
			c := colors[i]
			fillBox(strip, x0, y0, x0+59, y0+49, c[0], c[1], c[2])
			i++
		}
	}

	res := ExtractFrames(strip, 6, 80, 80, 8)
	if res.Found != 6 || len(res.Frames) != 6 {
		t.Fatalf("grid extraction failed: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	if !hasWarningContaining(res.Warnings, "2행 x 3열") {
		t.Fatalf("grid warning missing: %v", res.Warnings)
	}
	for i, want := range colors {
		got := firstOpaqueRGB(t, res.Frames[i])
		if got != want {
			t.Fatalf("frame order/color mismatch at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestExtractGridLayout4x1(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 120, 420))
	colors := [][3]uint8{
		{210, 30, 30},
		{30, 180, 30},
		{40, 80, 210},
		{220, 170, 30},
	}
	for i, c := range colors {
		y0 := 18 + i*100
		fillBox(strip, 24, y0, 83, y0+49, c[0], c[1], c[2])
	}

	res := ExtractFrames(strip, 4, 80, 80, 8)
	if res.Found != 4 || len(res.Frames) != 4 {
		t.Fatalf("vertical grid extraction failed: found=%d frames=%d warnings=%v", res.Found, len(res.Frames), res.Warnings)
	}
	if !hasWarningContaining(res.Warnings, "4행 x 1열") {
		t.Fatalf("vertical grid warning missing: %v", res.Warnings)
	}
	for i, want := range colors {
		got := firstOpaqueRGB(t, res.Frames[i])
		if got != want {
			t.Fatalf("frame order/color mismatch at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestExtractHamsterWalkEastCleanFixtureReportsMissingPose(t *testing.T) {
	fixture := hamsterWalkEastCleanFixture
	if override := os.Getenv("PPGEN_HAMSTER_WALK_EAST_CLEAN"); override != "" {
		fixture = override
	}
	fixture, _ = filepath.Abs(fixture)
	strip := loadNRGBAFixture(t, fixture)

	res := ExtractFrames(strip, 6, 256, 256, 24)
	if res.Found != 5 {
		t.Fatalf("walk-east fixture should report the visible 5 poses: found=%d warnings=%v", res.Found, res.Warnings)
	}
	if len(res.Frames) != 5 {
		t.Fatalf("walk-east fixture emitted %d frames", len(res.Frames))
	}
	if len(res.Warnings) == 0 {
		t.Fatal("walk-east fixture should emit a frame-count mismatch warning")
	}

	boxes := frameContentBoxes(res.Frames)
	for i, b := range boxes {
		if b.W < 80 {
			t.Fatalf("walk-east frame %02d is too narrow: boxes=%+v warnings=%v", i+1, boxes, res.Warnings)
		}
		if b.H < 80 {
			t.Fatalf("walk-east frame %02d is too short: boxes=%+v warnings=%v", i+1, boxes, res.Warnings)
		}
	}
}

func TestExtractCatWalkEastGridFixture(t *testing.T) {
	fixture := catWalkEastGridFixture
	if override := os.Getenv("PPGEN_CAT_WALK_EAST_GRID"); override != "" {
		fixture = override
	}
	fixture, _ = filepath.Abs(fixture)
	strip := loadNRGBAFixture(t, fixture)

	res := ExtractFrames(strip, 6, 256, 256, 24)
	if res.Found != 6 {
		t.Fatalf("cat grid fixture should be reusable as 6 poses: found=%d warnings=%v", res.Found, res.Warnings)
	}
	if len(res.Frames) != 6 {
		t.Fatalf("cat grid fixture emitted %d frames", len(res.Frames))
	}
	if !hasWarningContaining(res.Warnings, "2행 x 3열") {
		t.Fatalf("cat grid fixture did not use grid extraction: warnings=%v", res.Warnings)
	}
	for i, f := range res.Frames {
		if comps := majorAlphaComponents(f, 0.20); len(comps) != 1 {
			t.Fatalf("frame %d should contain one major component, got %d", i+1, len(comps))
		}
	}
}
