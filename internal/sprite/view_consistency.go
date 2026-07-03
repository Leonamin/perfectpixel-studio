package sprite

import (
	"fmt"
	"image"
	"math"
	"strings"
)

const (
	viewSignatureGrid     = 6
	viewConsistencyWarn   = 0.68
	viewConsistencyError  = 0.58
	viewConsistencyMinLen = 4
	backFaceDetailMin     = 0.0015
	backFaceDetailJump    = 2.8
)

type viewSignature struct {
	bins   [viewSignatureGrid * viewSignatureGrid]float64
	aspect float64
	valid  bool
}

// InspectFramesWithFacing extends the normal frame inspection with a conservative
// view-lock check for directed animation states. It is intentionally stricter only
// when a facing direction was requested, so ordinary non-directional states keep
// the existing behavior.
func InspectFramesWithFacing(frames []*image.NRGBA, key [3]uint8, base *image.NRGBA, facing string) InspectResult {
	res := InspectFrames(frames, key, base)
	inspectFacingConsistency(frames, strings.TrimSpace(facing), &res)
	return res
}

func inspectFacingConsistency(frames []*image.NRGBA, facing string, res *InspectResult) {
	if !shouldInspectFacing(facing) || len(frames) < viewConsistencyMinLen {
		return
	}
	inspectBackFacingDetailConsistency(frames, facing, res)
	sigs := make([]viewSignature, len(frames))
	valid := 0
	for i, f := range frames {
		sigs[i] = frameViewSignature(f)
		if sigs[i].valid {
			valid++
		}
	}
	if valid < viewConsistencyMinLen {
		return
	}

	ref := viewReferenceSignature(sigs)
	if ref < 0 {
		return
	}

	hintAdded := false
	for i, sig := range sigs {
		if i == ref || !sig.valid {
			continue
		}
		sim := viewSignatureSimilarity(sig, sigs[ref])
		if sim < viewConsistencyError {
			res.Errors = append(res.Errors,
				fmt.Sprintf("프레임 %d의 시점/방향 실루엣이 같은 방향의 다른 프레임과 크게 다릅니다 (유사도 %.0f%%)", i+1, sim*100))
			if !hintAdded {
				addRetryHintOnce(res, facingRetryHint(facing))
				hintAdded = true
			}
		} else if sim < viewConsistencyWarn {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("프레임 %d의 시점/방향 실루엣이 다른 프레임과 다소 다릅니다 (유사도 %.0f%%)", i+1, sim*100))
			if !hintAdded {
				addRetryHintOnce(res, facingRetryHint(facing))
				hintAdded = true
			}
		}
	}
}

func inspectBackFacingDetailConsistency(frames []*image.NRGBA, facing string, res *InspectResult) {
	if facing != "north" && facing != "north-east" {
		return
	}
	scores := make([]float64, len(frames))
	for i, f := range frames {
		scores[i] = upperSaturatedDetailRatio(f)
	}
	maxScore := 0.0
	for _, s := range scores {
		if s > maxScore {
			maxScore = s
		}
	}
	if maxScore < backFaceDetailMin {
		return
	}
	sorted := append([]float64(nil), scores...)
	sortFloat64s(sorted)
	baseline := sorted[(len(sorted)-1)/2]
	if baseline > 0 && maxScore/baseline < backFaceDetailJump {
		return
	}
	for i, s := range scores {
		if s < maxScore*0.60 {
			continue
		}
		res.Errors = append(res.Errors,
			fmt.Sprintf("프레임 %d의 뒷면 방향 시점에서 얼굴/눈으로 보이는 고채도 디테일이 다른 프레임보다 두드러집니다", i+1))
		addRetryHintOnce(res, facingRetryHint(facing))
		return
	}
}

func shouldInspectFacing(facing string) bool {
	switch facing {
	case "south", "north", "east", "south-east", "north-east":
		return true
	default:
		return false
	}
}

func frameViewSignature(f *image.NRGBA) viewSignature {
	var sig viewSignature
	if f == nil {
		return sig
	}
	box := contentBBox(f)
	if box.W <= 0 || box.H <= 0 {
		return sig
	}
	sig.aspect = float64(box.W) / float64(box.H)
	var total float64
	for y := box.Y; y < box.Y+box.H; y++ {
		gy := (y - box.Y) * viewSignatureGrid / box.H
		if gy >= viewSignatureGrid {
			gy = viewSignatureGrid - 1
		}
		for x := box.X; x < box.X+box.W; x++ {
			i := f.PixOffset(x, y)
			a := f.Pix[i+3]
			if a <= alphaThreshold {
				continue
			}
			gx := (x - box.X) * viewSignatureGrid / box.W
			if gx >= viewSignatureGrid {
				gx = viewSignatureGrid - 1
			}
			v := float64(a) / 255.0
			sig.bins[gy*viewSignatureGrid+gx] += v
			total += v
		}
	}
	if total <= 0 {
		return sig
	}
	for i := range sig.bins {
		sig.bins[i] /= total
	}
	sig.valid = true
	return sig
}

func upperSaturatedDetailRatio(f *image.NRGBA) float64 {
	if f == nil {
		return 0
	}
	box := contentBBox(f)
	if box.W <= 0 || box.H <= 0 {
		return 0
	}
	upperMaxY := box.Y + box.H*45/100
	if upperMaxY <= box.Y {
		upperMaxY = box.Y + 1
	}
	content := 0
	detail := 0
	for y := box.Y; y < box.Y+box.H; y++ {
		for x := box.X; x < box.X+box.W; x++ {
			i := f.PixOffset(x, y)
			if f.Pix[i+3] <= alphaThreshold {
				continue
			}
			content++
			if y >= upperMaxY {
				continue
			}
			if saturatedDetailPixel(f.Pix[i], f.Pix[i+1], f.Pix[i+2]) {
				detail++
			}
		}
	}
	if content == 0 {
		return 0
	}
	return float64(detail) / float64(content)
}

func saturatedDetailPixel(r, g, b uint8) bool {
	maxC := max3(r, g, b)
	minC := min3(r, g, b)
	return maxC >= 70 && int(maxC)-int(minC) >= 45
}

func viewReferenceSignature(sigs []viewSignature) int {
	bestIdx := -1
	bestScore := -1.0
	for i, sig := range sigs {
		if !sig.valid {
			continue
		}
		score := 0.0
		count := 0
		for j, other := range sigs {
			if i == j || !other.valid {
				continue
			}
			score += viewSignatureSimilarity(sig, other)
			count++
		}
		if count == 0 {
			continue
		}
		score /= float64(count)
		if score > bestScore {
			bestIdx = i
			bestScore = score
		}
	}
	return bestIdx
}

func viewSignatureSimilarity(a, b viewSignature) float64 {
	if !a.valid || !b.valid {
		return 1
	}
	shape := 0.0
	for i := range a.bins {
		shape += minf(a.bins[i], b.bins[i])
	}
	aspect := aspectSimilarity(a.aspect, b.aspect)
	return 0.82*shape + 0.18*aspect
}

func aspectSimilarity(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 1
	}
	d := math.Abs(math.Log(a / b))
	return 1 - minf(d/math.Log(2.4), 1)
}

func sortFloat64s(v []float64) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j-1] > v[j]; j-- {
			v[j-1], v[j] = v[j], v[j-1]
		}
	}
}

func max3(a, b, c uint8) uint8 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c uint8) uint8 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func addRetryHintOnce(res *InspectResult, hint string) {
	for _, existing := range res.RetryHints {
		if existing == hint {
			return
		}
	}
	res.RetryHints = append(res.RetryHints, hint)
}

func facingRetryHint(facing string) string {
	desc, ok := facingDescs[facing]
	if !ok {
		return "CRITICAL VIEW LOCK: the previous attempt mixed different camera or facing angles inside one animation. Redraw every pose from one fixed camera and one fixed facing direction; do not alternate between front, side, and back views."
	}
	return fmt.Sprintf(
		"CRITICAL VIEW LOCK: the previous attempt mixed different camera or facing angles inside one animation. Redraw every pose as %s (%s). Every frame must keep the same head, torso, limb and silhouette orientation; do not alternate between front, side, and back views.",
		desc.view, desc.body)
}
