package sprite

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const hamsterWalkEastCleanFixture = "/home/leonamin/podong-assets/hamster-ew-baseseed/debug/walk-east/attempt-01-clean.png"

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
