package sprite

import (
	"image"
	"math"
	"strings"
	"testing"
)

// fillBox는 NRGBA 이미지에 불투명 사각형을 채웁니다 (양끝 포함).
func fillBox(img *image.NRGBA, x0, y0, x1, y1 int, r, g, b uint8) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, 255
		}
	}
}

// torsoCenterX는 프레임에서 세로 픽셀 수가 가장 많은 열 묶음(토르소)의 중심을 구합니다.
func torsoCenterX(f *image.NRGBA, minCount int) float64 {
	w, h := f.Rect.Dx(), f.Rect.Dy()
	first, last := -1, -1
	for x := 0; x < w; x++ {
		cnt := 0
		for y := 0; y < h; y++ {
			if f.Pix[f.PixOffset(x, y)+3] > alphaThreshold {
				cnt++
			}
		}
		if cnt >= minCount {
			if first < 0 {
				first = x
			}
			last = x
		}
	}
	return float64(first+last) / 2
}

// TestExtractCentroidAnchor는 동일 포즈들이 모두 셀 중앙에 정렬되고,
// 팔이 한쪽으로 뻗은 프레임도 bbox 중심 배치보다 토르소가 덜 흔들리는지 검증합니다.
func TestExtractCentroidAnchor(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 480, 100))
	// 프레임 1·3·4: 20px 폭 토르소만
	fillBox(strip, 30, 20, 49, 79, 200, 100, 50)
	fillBox(strip, 270, 20, 289, 79, 200, 100, 50)
	fillBox(strip, 390, 20, 409, 79, 200, 100, 50)
	// 프레임 2: 같은 토르소 + 오른쪽으로 길게 뻗은 얇은 팔 (bbox를 40px 확장)
	fillBox(strip, 150, 20, 169, 79, 200, 100, 50)
	fillBox(strip, 170, 30, 209, 33, 200, 100, 50)

	res := ExtractFrames(strip, 4, 100, 100, 8)
	if res.Found != 4 {
		t.Fatalf("프레임 수 오류: %d (%v)", res.Found, res.Warnings)
	}

	centers := make([]float64, 4)
	for i, f := range res.Frames {
		centers[i] = torsoCenterX(f, 40) // 토르소 열은 60px, 팔 열은 4px
	}
	minC, maxC := centers[0], centers[0]
	for _, c := range centers[1:] {
		minC = math.Min(minC, c)
		maxC = math.Max(maxC, c)
	}
	spread := maxC - minC
	// bbox 중심 배치라면 프레임 2의 토르소가 ~20px 왼쪽으로 밀림(spread≈20).
	// 질량 중심 앵커에서는 팔 질량 비율만큼만 밀려 spread가 한 자릿수여야 함.
	if spread >= 6 {
		t.Fatalf("토르소 흔들림 과다: spread=%.1f centers=%v", spread, centers)
	}
	// 팔 없는 프레임들은 셀 중앙(50)에 정확히 정렬되어야 함
	for _, i := range []int{0, 2, 3} {
		if math.Abs(centers[i]-50) > 2 {
			t.Fatalf("프레임 %d 중앙 정렬 실패: %.1f", i+1, centers[i])
		}
	}
}

// TestExtractSlotGuard는 포즈에서 멀리 떨어진 잔여 blob이 프레임에 병합되지 않는지 검증합니다.
func TestExtractSlotGuard(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 600, 100))
	fillBox(strip, 20, 20, 79, 79, 200, 100, 50)   // 포즈 1 (60×60)
	fillBox(strip, 120, 20, 179, 79, 200, 100, 50) // 포즈 2 (60×60)
	fillBox(strip, 560, 10, 579, 29, 90, 200, 90)  // 원거리 잔여물 (20×20)

	res := ExtractFrames(strip, 2, 256, 256, 16)
	if res.Found != 2 {
		t.Fatalf("프레임 수 오류: %d", res.Found)
	}
	for i, f := range res.Frames {
		opaque := 0
		for p := 3; p < len(f.Pix); p += 4 {
			if f.Pix[p] > alphaThreshold {
				opaque++
			}
		}
		// 각 프레임은 60×60 본체만 포함해야 함 (잔여물 400px 병합 시 4000)
		if opaque != 3600 {
			t.Fatalf("프레임 %d에 잔여물 병합 의심: opaque=%d", i+1, opaque)
		}
	}
}

func TestExtractRepairsNarrowSideFrameSplit(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 600, 120))
	// 모든 포즈를 얇은 연결부로 이어 하나의 긴 run처럼 만들고, 4번째 슬롯 안에는
	// 앞/뒤 다리가 별도 peak처럼 보이는 측면 걷기 형태를 만든다. 순수 peak 분할은
	// 4번째 포즈를 세로 슬라이버로 쪼갤 수 있고, 슬롯 기반 복구는 한 슬롯으로 묶어야 한다.
	fillBox(strip, 45, 72, 555, 74, 200, 100, 50) // 낮은 알파 질량 연결부
	fillBox(strip, 25, 25, 64, 94, 200, 100, 50)
	fillBox(strip, 125, 25, 164, 94, 200, 100, 50)
	fillBox(strip, 225, 25, 264, 94, 200, 100, 50)
	fillBox(strip, 316, 25, 326, 94, 200, 100, 50) // 잘못 분리되기 쉬운 앞다리 peak
	fillBox(strip, 350, 25, 389, 94, 200, 100, 50)
	fillBox(strip, 425, 25, 464, 94, 200, 100, 50)
	fillBox(strip, 505, 25, 544, 94, 200, 100, 50)

	res := ExtractFrames(strip, 6, 128, 128, 8)
	if res.Found != 6 {
		t.Fatalf("프레임 수 오류: %d (%v)", res.Found, res.Warnings)
	}
	repaired := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "슬롯 기준") {
			repaired = true
			break
		}
	}
	if !repaired {
		t.Fatalf("슬롯 기반 복구 경로가 실행되지 않음: warnings=%v", res.Warnings)
	}
	widths := make([]int, len(res.Frames))
	for i, f := range res.Frames {
		minX, maxX := f.Rect.Dx(), -1
		for y := 0; y < f.Rect.Dy(); y++ {
			for x := 0; x < f.Rect.Dx(); x++ {
				if f.Pix[f.PixOffset(x, y)+3] <= alphaThreshold {
					continue
				}
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
			}
		}
		if maxX >= minX {
			widths[i] = maxX - minX + 1
		}
	}
	if widths[3] < 35 {
		t.Fatalf("4번째 프레임이 세로 슬라이버로 추출됨: widths=%v warnings=%v", widths, res.Warnings)
	}
}
