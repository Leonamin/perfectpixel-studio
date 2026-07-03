package sprite

import (
	"fmt"
	"image"
	"sort"

	xdraw "golang.org/x/image/draw"
)

const alphaThreshold = 10 // 이 값 이하의 알파는 빈(투명) 픽셀로 취급

// frameContent는 스트립 좌표계에서 추출한 한 포즈의 콘텐츠입니다.
type frameContent struct {
	img    *image.NRGBA // bbox로 자른 콘텐츠
	minX   int
	cx     float64 // 알파 가중 질량 중심 X (스트립 좌표)
	bottom int     // 베이스라인(콘텐츠 최하단 행, 스트립 좌표)
}

// extractContent는 컬럼 구간 span 안의 불투명 픽셀을 bbox로 잘라냅니다.
// 소유권 추적(연결요소) 없이 구간 내 모든 콘텐츠를 모으므로, 팔다리가 분리되어도
// 한 포즈로 안전하게 합쳐집니다.
func extractContent(strip *image.NRGBA, span colSpan, h int) frameContent {
	minX, minY, maxX, maxY := span.end, h, span.start-1, -1
	var sumWX, sumW float64
	for x := span.start; x < span.end; x++ {
		for y := 0; y < h; y++ {
			a := strip.Pix[strip.PixOffset(x, y)+3]
			if a <= alphaThreshold {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			sumWX += float64(x) * float64(a)
			sumW += float64(a)
		}
	}
	if maxX < minX || maxY < minY {
		return frameContent{}
	}
	gw, gh := maxX-minX+1, maxY-minY+1
	dst := image.NewNRGBA(image.Rect(0, 0, gw, gh))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			si := strip.PixOffset(x, y)
			if strip.Pix[si+3] <= alphaThreshold {
				continue
			}
			di := dst.PixOffset(x-minX, y-minY)
			copy(dst.Pix[di:di+4], strip.Pix[si:si+4])
		}
	}
	cx := float64(minX+maxX+1) / 2
	if sumW > 0 {
		cx = sumWX / sumW
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: maxY}
}

func extractContents(strip *image.NRGBA, segs []colSpan, h int) []frameContent {
	fcs := make([]frameContent, 0, len(segs))
	for _, s := range segs {
		fc := extractContent(strip, s, h)
		if fc.img != nil {
			fcs = append(fcs, fc)
		}
	}
	return fcs
}

func extractComponentContent(strip *image.NRGBA, comp alphaComponent, normalizeBottom bool) frameContent {
	minX, minY, maxX, maxY := comp.minX, comp.minY, comp.maxX, comp.maxY
	gw, gh := maxX-minX+1, maxY-minY+1
	if gw <= 0 || gh <= 0 {
		return frameContent{}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, gw, gh))
	var sumWX, sumW float64
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			si := strip.PixOffset(x, y)
			a := strip.Pix[si+3]
			if a <= alphaThreshold {
				continue
			}
			di := dst.PixOffset(x-minX, y-minY)
			copy(dst.Pix[di:di+4], strip.Pix[si:si+4])
			sumWX += float64(x) * float64(a)
			sumW += float64(a)
		}
	}
	cx := float64(minX+maxX+1) / 2
	if sumW > 0 {
		cx = sumWX / sumW
	}
	bottom := maxY
	if normalizeBottom {
		bottom = 0
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: bottom}
}

func gridFrameContents(strip *image.NRGBA, expected int) ([]frameContent, int, int, int, bool) {
	if expected < 4 {
		return nil, 0, 0, 0, false
	}
	comps := majorAlphaComponents(strip, 0.20)
	if len(comps) < expected {
		return nil, 0, 0, 0, false
	}
	var best []alphaComponent
	bestRows, bestCols := 0, 0
	bestScore := 1e18
	total := len(comps)
	for rows := 2; rows <= total; rows++ {
		if total%rows != 0 {
			continue
		}
		cols := total / rows
		ordered, score, ok := fitGridComponents(comps, rows, cols)
		if !ok {
			continue
		}
		if score < bestScore {
			best, bestRows, bestCols, bestScore = ordered, rows, cols, score
		}
	}
	if len(best) < expected {
		return nil, 0, 0, 0, false
	}
	fcs := make([]frameContent, 0, expected)
	for _, comp := range best[:expected] {
		fc := extractComponentContent(strip, comp, true)
		if fc.img == nil {
			return nil, 0, 0, 0, false
		}
		fcs = append(fcs, fc)
	}
	return fcs, bestRows, bestCols, total, true
}

func fitGridComponents(comps []alphaComponent, rows, cols int) ([]alphaComponent, float64, bool) {
	if rows < 2 || cols < 1 || len(comps) != rows*cols {
		return nil, 0, false
	}
	widths := make([]int, 0, len(comps))
	heights := make([]int, 0, len(comps))
	for _, c := range comps {
		widths = append(widths, c.width())
		heights = append(heights, c.height())
	}
	medW := medianInt(widths)
	medH := medianInt(heights)
	if medW <= 0 || medH <= 0 {
		return nil, 0, false
	}

	sorted := append([]alphaComponent(nil), comps...)
	sortAlphaComponentsY(sorted)
	ordered := make([]alphaComponent, 0, len(comps))
	rowCenters := make([]float64, 0, rows)
	rowMaxY := make([]int, 0, rows)
	score := float64(rows) * 0.01
	for r := 0; r < rows; r++ {
		group := append([]alphaComponent(nil), sorted[r*cols:(r+1)*cols]...)
		sortAlphaComponentsX(group)
		minCY, maxCY, sumCY := group[0].cy(), group[0].cy(), 0.0
		minY, maxY := group[0].minY, group[0].maxY
		for _, c := range group {
			cy := c.cy()
			if cy < minCY {
				minCY = cy
			}
			if cy > maxCY {
				maxCY = cy
			}
			if c.minY < minY {
				minY = c.minY
			}
			if c.maxY > maxY {
				maxY = c.maxY
			}
			sumCY += cy
		}
		if cols > 1 && maxCY-minCY > float64(medH)*0.55 {
			return nil, 0, false
		}
		for c := 1; c < cols; c++ {
			if group[c].cx()-group[c-1].cx() < float64(medW)*0.55 {
				return nil, 0, false
			}
		}
		rowCenter := sumCY / float64(cols)
		if r > 0 {
			centerGap := rowCenter - rowCenters[r-1]
			edgeGap := minY - rowMaxY[r-1]
			if centerGap < float64(medH)*0.50 || edgeGap < 4 {
				return nil, 0, false
			}
		}
		score += (maxCY - minCY) / float64(medH)
		rowCenters = append(rowCenters, rowCenter)
		rowMaxY = append(rowMaxY, maxY)
		ordered = append(ordered, group...)
	}

	if cols > 1 && rows > 1 {
		for c := 0; c < cols; c++ {
			centers := make([]int, 0, rows)
			for r := 0; r < rows; r++ {
				centers = append(centers, int(ordered[r*cols+c].cx()+0.5))
			}
			medCX := medianInt(centers)
			for _, cx := range centers {
				score += relDev(float64(cx), float64(medCX)) * 0.2
				if absInt(cx-medCX) > medW {
					return nil, 0, false
				}
			}
		}
	}
	return ordered, score, true
}

// ExtractFrames는 투명 배경 스트립에서 포즈를 투영 분할로 검출해 셀 크기 프레임으로
// 만듭니다. 모든 프레임에 공통 스케일을 적용하고, 질량 중심으로 수평 정렬하며,
// 공통 베이스라인 기준으로 수직 오프셋(점프 호 등)을 보존합니다.
func ExtractFrames(strip *image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	res := ExtractResult{Expected: expected}
	h := strip.Rect.Dy()

	var fcs []frameContent
	found := 0
	if gridFcs, rows, cols, detected, ok := gridFrameContents(strip, expected); ok {
		fcs = gridFcs
		found = expected
		if detected > expected {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%d행 x %d열 그리드에서 %d개 포즈를 감지해 앞의 %d개를 재사용했습니다.", rows, cols, detected, expected))
		} else {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%d행 x %d열 그리드 레이아웃을 감지해 각 셀의 포즈를 재사용했습니다.", rows, cols))
		}
	} else {
		segs, natural := segmentStrip(strip, expected)
		if len(segs) == 0 {
			res.Warnings = append(res.Warnings, "이미지에서 캐릭터를 찾지 못했습니다. 다시 생성해 주세요.")
			return res
		}

		fcs = extractContents(strip, segs, h)
		found = natural
		if expected > 1 && len(fcs) == expected && hasContentShapeOutlier(fcs) {
			slotSegs := slotGuidedSegments(strip, expected)
			slotFcs := extractContents(strip, slotSegs, h)
			if len(slotFcs) == expected && contentShapeScore(slotFcs)+0.15 < contentShapeScore(fcs) {
				fcs = slotFcs
				found = expected
				res.Warnings = append(res.Warnings,
					"프레임 폭 이상치를 감지해 균등 슬롯 기준으로 스트립 분할을 보정했습니다.")
			}
		}
	}
	if len(fcs) == 0 {
		res.Warnings = append(res.Warnings, "유효한 포즈를 찾지 못했습니다. 다시 생성해 주세요.")
		return res
	}

	// 공통 베이스라인 + 공유 스케일
	baseline := 0
	for _, g := range fcs {
		if g.bottom > baseline {
			baseline = g.bottom
		}
	}
	availW := cellW - margin*2
	availH := cellH - margin*2
	if availW < 8 || availH < 8 {
		availW, availH = cellW, cellH
	}
	maxBodyW, maxBodyH := 1, 1
	for _, g := range fcs {
		bw, bh := bodyExtent(g.img)
		if bw > maxBodyW {
			maxBodyW = bw
		}
		if bh > maxBodyH {
			maxBodyH = bh
		}
	}
	scale := minf(float64(availW)/float64(maxBodyW), float64(availH)/float64(maxBodyH))
	if scale > 1 {
		scale = 1
	}

	for _, g := range fcs {
		// scale은 body extent 기준으로 계산했으므로, 그 외 바운딩 박스 빈 공간은
		// 여유 공간에 맞춰 조정한다.
		boxScale := minf(scale, minf(float64(availW)/float64(g.img.Rect.Dx()), float64(availH)/float64(g.img.Rect.Dy())))
		if boxScale > 1 {
			boxScale = 1
		}
		// 세로 정렬에서는 실제 발/하체 부분(bottom)이 아니라 콘텐츠 하단 기준 사용
		sw := int(float64(g.img.Rect.Dx())*boxScale + 0.5)
		sh := int(float64(g.img.Rect.Dy())*boxScale + 0.5)
		if sw < 1 {
			sw = 1
		}
		if sh < 1 {
			sh = 1
		}
		scaled := g.img
		if sw != g.img.Rect.Dx() || sh != g.img.Rect.Dy() {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, g.img, g.img.Rect, xdraw.Over, nil)
		}
		// 공통 baseline 보정을 위해 strip 내 콘텐츠 하단 대비 offset을 scale
		contentBaseline := int(float64(baseline-g.bottom)*boxScale + 0.5)

		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		// 질량 중심이 셀 중앙에 오도록 수평 배치 (팔다리가 한쪽으로 뻗어도
		// 면적이 큰 몸통이 지배해 프레임 간 흔들림이 적음).
		left := int(float64(cellW)/2 - (g.cx-float64(g.minX))*boxScale + 0.5)
		if left < 0 {
			left = 0
		}
		if left+sw > cellW {
			left = cellW - sw
		}
		top := cellH - margin - contentBaseline - sh
		if top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		cleanupAlpha(cell)
		removeSmallAlphaComponents(cell)
		res.Frames = append(res.Frames, cell)
	}

	res.Found = found
	if found != expected {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("기대한 %d개와 다른 %d개의 포즈가 감지되었습니다. 포즈가 겹쳤거나 누락됐을 수 있어 재생성을 권장합니다.", expected, found))
	}
	return res
}

// slotGuidedSegments는 생성 프롬프트의 "균등한 가로 슬롯" 계약을 prior로 사용해
// 각 슬롯 경계 주변에서 가장 알파 질량이 낮은 컷을 고릅니다. 투영 peak가 한 포즈의
// 앞/뒤 다리를 별도 포즈로 오인해 세로 슬라이버를 만들 때 복구 경로로만 사용합니다.
func slotGuidedSegments(strip *image.NRGBA, expected int) []colSpan {
	w := strip.Rect.Dx()
	if w == 0 || expected < 1 {
		return nil
	}
	if expected == 1 {
		return []colSpan{{0, w}}
	}
	raw := projectAlpha(strip)
	win := w / 220
	if win < 3 {
		win = 3
	}
	p := smoothProfile(raw, win)
	mx := maxOf(p)
	ideal := float64(w) / float64(expected)
	minW := int(ideal * 0.35)
	if minW < 8 {
		minW = 8
	}
	if minW*expected > w {
		minW = w / expected
		if minW < 1 {
			minW = 1
		}
	}

	cuts := []int{0}
	last := 0
	for i := 1; i < expected; i++ {
		center := int(ideal*float64(i) + 0.5)
		radius := int(ideal * 0.33)
		if radius < 4 {
			radius = 4
		}
		lo, hi := center-radius, center+radius
		if lo < last+minW {
			lo = last + minW
		}
		maxHi := w - (expected-i)*minW
		if hi > maxHi {
			hi = maxHi
		}
		if lo > hi {
			lo, hi = last+minW, maxHi
		}
		best := lo
		bestCost := 1e18
		for x := lo; x <= hi && x < len(p); x++ {
			if !valleyCutOK(p, 0, w, x) {
				continue
			}
			distPenalty := 0.0
			if radius > 0 && mx > 0 {
				distPenalty = mx * 0.08 * float64(absInt(x-center)) / float64(radius)
			}
			cost := p[x] + distPenalty
			if cost < bestCost {
				bestCost = cost
				best = x
			}
		}
		if bestCost >= 1e17 {
			return nil
		}
		if best <= last {
			best = last + minW
		}
		cuts = append(cuts, best)
		last = best
	}
	cuts = append(cuts, w)

	segs := make([]colSpan, 0, expected)
	for i := 0; i < expected; i++ {
		segs = append(segs, colSpan{cuts[i], cuts[i+1]})
	}
	return segs
}

func hasContentShapeOutlier(fcs []frameContent) bool {
	return contentShapeScore(fcs) >= 0.65
}

func contentShapeScore(fcs []frameContent) float64 {
	if len(fcs) < 3 {
		return 0
	}
	widths := make([]int, 0, len(fcs))
	heights := make([]int, 0, len(fcs))
	aspects := make([]int, 0, len(fcs))
	for _, fc := range fcs {
		w, h := fc.img.Rect.Dx(), fc.img.Rect.Dy()
		if w <= 0 || h <= 0 {
			continue
		}
		widths = append(widths, w)
		heights = append(heights, h)
		aspects = append(aspects, int((float64(w)/float64(h))*1000+0.5))
	}
	if len(widths) < 3 {
		return 0
	}
	medW := medianInt(widths)
	medH := medianInt(heights)
	medA := medianInt(aspects)
	if medW <= 0 || medH <= 0 || medA <= 0 {
		return 0
	}
	score := 0.0
	for i := range widths {
		wDev := relDev(float64(widths[i]), float64(medW))
		hDev := relDev(float64(heights[i]), float64(medH))
		aDev := relDev(float64(aspects[i]), float64(medA))
		if s := wDev + 0.35*hDev + aDev; s > score {
			score = s
		}
	}
	return score
}

func medianInt(vals []int) int {
	cp := append([]int(nil), vals...)
	sort.Ints(cp)
	return cp[len(cp)/2]
}

func relDev(v, base float64) float64 {
	if base <= 0 {
		return 0
	}
	if v > base {
		return v/base - 1
	}
	return base/v - 1
}

// bodyExtent는 알파 질량 80%를 포함하는 최소 크기를 "실제 바디" extent로 반환합니다.
// 길게 뻗은 팔다리 outlier가 스케일을 과대 산정하는 것을 막습니다.
func bodyExtent(img *image.NRGBA) (int, int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return 1, 1
	}
	alphaX := make([]float64, w)
	alphaY := make([]float64, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := float64(img.Pix[img.PixOffset(x, y)+3])
			alphaX[x] += a
			alphaY[y] += a
		}
	}
	cutX := cumulativeExtent(alphaX, 0.80)
	cutY := cumulativeExtent(alphaY, 0.80)
	if cutX < 1 {
		cutX = 1
	}
	if cutY < 1 {
		cutY = 1
	}
	return cutX, cutY
}

// cumulativeExtent는 질량 누적 비율 massFrac를 커버하는 가장 좁은 연속 구간의 길이를 반환합니다.
func cumulativeExtent(mass []float64, massFrac float64) int {
	total := 0.0
	for _, v := range mass {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := total * massFrac
	n := len(mass)
	best := n
	left := 0
	cur := 0.0
	for right := 0; right < n; right++ {
		cur += mass[right]
		for cur >= target {
			if span := right - left + 1; span < best {
				best = span
			}
			cur -= mass[left]
			left++
		}
	}
	return best
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
