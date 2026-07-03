package sprite

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fillRect는 테스트용 사각형을 채웁니다.
func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, r, g, b uint8) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, 255
		}
	}
}

func TestInspectFramesClean(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 4; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 30, 30, 100, 110, 40, 80, 200) // 파란 캐릭터 블록
		frames = append(frames, f)
	}
	res := InspectFrames(frames, key, nil)
	if !res.Ok() {
		t.Fatalf("정상 프레임에서 오류 발생: %v", res.Errors)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("정상 프레임에서 경고 발생: %v", res.Warnings)
	}
}

func TestInspectFramesKeyResidue(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f, 30, 30, 100, 110, 40, 80, 200)
	fillRect(f, 40, 40, 60, 60, 250, 30, 250) // 마젠타 잔여물 400px
	res := InspectFrames([]*image.NRGBA{f}, key, nil)
	if res.Ok() {
		t.Fatal("마젠타 잔여물을 감지하지 못함")
	}
	joined := strings.Join(res.RetryHints, " ")
	if !strings.Contains(joined, "magenta") {
		t.Fatalf("마젠타 보정 힌트 누락: %v", res.RetryHints)
	}
}

func TestInspectFramesEdgeAndEmpty(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	// 가장자리에 닿은 프레임
	edge := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(edge, 0, 30, 70, 110, 40, 80, 200) // x=0부터 시작 → 잘림
	// 빈 프레임
	empty := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(empty, 60, 60, 65, 65, 40, 80, 200) // 25px뿐

	res := InspectFrames([]*image.NRGBA{edge, empty}, key, nil)
	if res.Ok() {
		t.Fatal("빈 프레임을 오류로 감지하지 못함")
	}
	foundEdge := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "가장자리") {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("가장자리 잘림 경고 누락: %v", res.Warnings)
	}
}

func TestInspectFramesSizeOutlier(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 3; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 20, 20, 110, 110, 40, 80, 200) // 8100px
		frames = append(frames, f)
	}
	tiny := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(tiny, 50, 50, 80, 80, 40, 80, 200) // 900px ≈ 0.11×
	frames = append(frames, tiny)

	res := InspectFrames(frames, key, nil)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "작습니다") {
			found = true
		}
	}
	if !found {
		t.Fatalf("크기 이상치 경고 누락: %v", res.Warnings)
	}
}

func TestInspectFramesAspectOutlierIsError(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	var frames []*image.NRGBA
	for i := 0; i < 5; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
		fillRect(f, 35, 20, 95, 110, 40, 80, 200)
		frames = append(frames, f)
	}
	sliver := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(sliver, 58, 20, 70, 110, 40, 80, 200)
	frames = append(frames, sliver)

	res := InspectFrames(frames, key, nil)
	if res.Ok() {
		t.Fatal("세로 슬라이버 프레임을 오류로 감지하지 못함")
	}
	joined := strings.Join(res.Errors, " ")
	if !strings.Contains(joined, "비정상적으로 좁습니다") && !strings.Contains(joined, "가로세로비") {
		t.Fatalf("bbox/aspect 오류 메시지 누락: %v", res.Errors)
	}
}

func TestInspectFramesWithFacingConsistentView(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	frames := []*image.NRGBA{
		makeSideViewFrame(0),
		makeSideViewFrame(4),
		makeSideViewFrame(-3),
		makeSideViewFrame(2),
		makeSideViewFrame(-2),
		makeSideViewFrame(3),
	}

	res := InspectFramesWithFacing(frames, key, nil, "east")
	for _, err := range res.Errors {
		if strings.Contains(err, "시점/방향") {
			t.Fatalf("consistent facing was flagged as view drift: errors=%v warnings=%v", res.Errors, res.Warnings)
		}
	}
}

func TestInspectFramesWithFacingDetectsViewDrift(t *testing.T) {
	key := [3]uint8{255, 0, 255}
	frames := []*image.NRGBA{
		makeSideViewFrame(0),
		makeSideViewFrame(4),
		makeFrontViewFrame(),
		makeSideViewFrame(2),
		makeSideViewFrame(-2),
		makeSideViewFrame(3),
	}

	res := InspectFramesWithFacing(frames, key, nil, "north-east")
	found := false
	for _, err := range res.Errors {
		if strings.Contains(err, "프레임 3") && strings.Contains(err, "시점/방향") {
			found = true
		}
	}
	if !found {
		t.Fatalf("view drift was not detected: errors=%v warnings=%v", res.Errors, res.Warnings)
	}
	hintFound := false
	for _, hint := range res.RetryHints {
		if strings.Contains(hint, "VIEW LOCK") && strings.Contains(hint, "three-quarter back-right") {
			hintFound = true
		}
	}
	if !hintFound {
		t.Fatalf("view-lock retry hint missing: %v", res.RetryHints)
	}
}

func TestInspectCatNorthEastFacingFixtureReportsViewDrift(t *testing.T) {
	dir := "/home/leonamin/dev/podong-podong/scratchpad/cat-full/frames/walk-north-east"
	frames := make([]*image.NRGBA, 0, 6)
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, fmt.Sprintf("frame-%02d.png", i))
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				t.Skipf("fixture not found: %s", path)
			}
			t.Fatalf("fixture stat failed: %v", err)
		}
		frames = append(frames, loadNRGBAFixture(t, path))
	}

	res := InspectFramesWithFacing(frames, [3]uint8{255, 0, 255}, nil, "north-east")
	found := false
	for _, msg := range append(append([]string{}, res.Errors...), res.Warnings...) {
		if strings.Contains(msg, "시점/방향") || strings.Contains(msg, "얼굴/눈") {
			found = true
		}
	}
	if !found {
		ref := viewReferenceSignature(viewSignaturesForTest(frames))
		t.Fatalf("cat north-east fixture should report view drift: errors=%v warnings=%v detail=%v ref=%d sims=%v",
			res.Errors, res.Warnings, detailScoresForTest(frames), ref, viewSimsForTest(frames, ref))
	}
}

func makeSideViewFrame(step int) *image.NRGBA {
	f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f, 27, 58, 89, 84, 50, 90, 150)  // torso
	fillRect(f, 82, 42, 112, 68, 50, 90, 150) // head facing right
	fillRect(f, 17, 66, 29, 75, 50, 90, 150)  // tail base
	fillRect(f, 10, 70, 19, 79, 50, 90, 150)  // tail tip
	fillRect(f, 37+step, 82, 49+step, 111, 50, 90, 150)
	fillRect(f, 67-step, 82, 79-step, 111, 50, 90, 150)
	return f
}

func makeFrontViewFrame() *image.NRGBA {
	f := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	fillRect(f, 49, 50, 79, 92, 50, 90, 150)  // upright torso
	fillRect(f, 43, 25, 85, 58, 50, 90, 150)  // centered head
	fillRect(f, 35, 92, 51, 113, 50, 90, 150) // left leg
	fillRect(f, 77, 92, 93, 113, 50, 90, 150) // right leg
	return f
}

func detailScoresForTest(frames []*image.NRGBA) []float64 {
	out := make([]float64, len(frames))
	for i, f := range frames {
		out[i] = upperSaturatedDetailRatio(f)
	}
	return out
}

func viewSignaturesForTest(frames []*image.NRGBA) []viewSignature {
	out := make([]viewSignature, len(frames))
	for i, f := range frames {
		out[i] = frameViewSignature(f)
	}
	return out
}

func viewSimsForTest(frames []*image.NRGBA, ref int) []float64 {
	sigs := viewSignaturesForTest(frames)
	out := make([]float64, len(frames))
	if ref < 0 {
		return out
	}
	for i, sig := range sigs {
		out[i] = viewSignatureSimilarity(sig, sigs[ref])
	}
	return out
}
