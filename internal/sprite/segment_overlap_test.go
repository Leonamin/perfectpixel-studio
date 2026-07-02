package sprite

import (
	"image"
	"testing"
)

// TestSegmentRejectsShallowForcedOverlap는 깊은 valley가 없는 붙은 덩어리를
// expected 수량에 맞추려고 몸통 내부에서 억지로 자르지 않는지 검증합니다.
func TestSegmentRejectsShallowForcedOverlap(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	fillBox(strip, 40, 20, 140, 79, 200, 100, 50)
	fillBox(strip, 120, 20, 220, 79, 200, 100, 50)
	segs, natural := segmentStrip(strip, 2)
	if natural != 1 {
		t.Fatalf("natural=%d segs=%v", natural, segs)
	}
}

func TestSegmentDoesNotForceMissingPoseAcrossRuns(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 600, 100))
	fillBox(strip, 20, 20, 79, 79, 200, 100, 50)
	fillBox(strip, 140, 20, 199, 79, 200, 100, 50)
	fillBox(strip, 260, 20, 319, 79, 200, 100, 50)
	fillBox(strip, 380, 20, 439, 79, 200, 100, 50)
	fillBox(strip, 500, 20, 559, 79, 200, 100, 50)

	segs, natural := segmentStrip(strip, 6)
	if natural != 5 || len(segs) != 5 {
		t.Fatalf("missing pose should stay a mismatch: natural=%d segs=%v", natural, segs)
	}
}
