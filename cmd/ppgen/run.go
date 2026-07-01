package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"
)

// runGen은 전체 파이프라인을 실행합니다: 베이스 생성 → 상태별 생성 → 번들 내보내기 → 요약 출력.
func runGen(opt options) error {
	if opt.seed < -1 {
		return fmt.Errorf("-seed는 -1 또는 0 이상의 정수여야 합니다")
	}
	p, provider, model, err := resolveProvider(opt)
	if err != nil {
		return err
	}
	var presets []sprite.PresetInfo
	if !opt.baseOnly {
		presets, err = selectStates(opt)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(opt.out, 0o755); err != nil {
		return err
	}

	logf := func(format string, a ...any) {
		if !opt.quiet && !opt.jsonOut {
			fmt.Printf(format, a...)
		}
	}
	logf("프로바이더: %s · 모델: %s · 스타일: %s · 상태 %d개 · 출력: %s\n",
		provider, model, opt.style, len(presets), opt.out)
	warnSeedSupport(provider, opt)
	if opt.seed >= 0 && seedSupported(provider) {
		logf("seed: %d\n", opt.seed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opt.timeout)
	defer cancel()

	style := sprite.ResolveStyle(opt.style, "")

	// 1) 베이스 캐릭터
	t0 := time.Now()
	var baseClean *image.NRGBA
	var baseBytes []byte
	if strings.TrimSpace(opt.base) != "" {
		logf("베이스 캐릭터 로드 중... ")
		cleaned := false
		baseClean, baseBytes, cleaned, err = loadBase(opt.base, opt.style)
		if err != nil {
			return fmt.Errorf("베이스 로드 실패: %w", err)
		}
		if cleaned && !opt.quiet {
			fmt.Fprintf(os.Stderr, "경고: -base 이미지 배경이 투명하지 않아 배경 제거 후 사용합니다\n")
		}
	} else {
		logf("베이스 캐릭터 생성 중... ")
		baseClean, baseBytes, err = generateBase(ctx, p, opt.desc, opt.style, style, genOptions(opt))
		if err != nil {
			return fmt.Errorf("베이스 생성 실패: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(opt.out, "base.png"), baseBytes, 0o644); err != nil {
		return fmt.Errorf("base.png 저장 실패: %w", err)
	}
	logf("완료 (%.0fs)\n", time.Since(t0).Seconds())

	// baseonly: 상태/번들 생성을 건너뛰고 base.png만 남긴다.
	if opt.baseOnly {
		if opt.jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"provider": provider, "model": model, "style": opt.style,
				"base": filepath.Join(opt.out, "base.png"),
			})
		}
		logf("baseonly 완료 · %s\n", filepath.Join(opt.out, "base.png"))
		return nil
	}

	// 2) 상태별 생성
	var states []sprite.StateFrames
	var rows []resultRow
	for _, kw := range presets {
		spec := sprite.StateSpec{Name: kw.Name, Frames: kw.Frames, FPS: kw.FPS, Loop: kw.Loop, Action: kw.Action}
		ts := time.Now()
		logf("[%s] %s 생성 중... ", kw.Category, kw.Name)
		res := genState(ctx, p, opt, style, spec, [][]byte{baseBytes}, baseClean)
		logf("%d/%d 시도%d 점수%d (%.0fs)\n", res.Found, res.Expected, res.Attempts, res.Score, time.Since(ts).Seconds())
		rows = append(rows, res.row())
		if len(res.frames) > 0 {
			states = append(states, sprite.StateFrames{Spec: spec, Frames: res.frames})
		}
	}

	// 3) 8방향 세트 (선택)
	if strings.TrimSpace(opt.dirset) != "" {
		logf("=== 8방향 세트: %s ===\n", opt.dirset)
		dirStates, dirRows := genDirectionSet(ctx, p, opt, style, opt.dirset, baseBytes, baseClean, logf)
		states = append(states, dirStates...)
		rows = append(rows, dirRows...)
	}

	if len(states) == 0 {
		return fmt.Errorf("생성된 상태가 없어 내보낼 번들이 없습니다")
	}

	// 4) 게임 엔진용 번들 내보내기
	summary, err := exportBundle(opt.out, opt.desc, states, rows)
	if err != nil {
		return fmt.Errorf("내보내기 실패: %w", err)
	}
	summary.Provider = provider
	summary.Model = model
	summary.Style = opt.style

	if opt.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	logf("\n번들 완료 · 상태 %d개 · 시트 %dx%d · %s\n",
		summary.Animations, summary.SheetWidth, summary.SheetHeight, opt.out)
	logf("  - sprite-sheet.png / manifest.json / sprite-sheet.json (Aseprite)\n")
	logf("  - frames/<state>/frame-NN.png · gif/<state>.gif · apng/<state>.png\n")
	return nil
}

func genOptions(opt options) gen.GenOpts {
	if opt.seed < 0 {
		return gen.GenOpts{}
	}
	seed := opt.seed
	return gen.GenOpts{Seed: &seed}
}

func seedSupported(provider string) bool {
	switch provider {
	case gen.ProviderGemini, gen.ProviderOpenRouter, gen.ProviderFal:
		return true
	default:
		return false
	}
}

func warnSeedSupport(provider string, opt options) {
	if opt.seed < 0 || opt.quiet || seedSupported(provider) {
		return
	}
	fmt.Fprintf(os.Stderr, "경고: -seed는 현재 %s 프로바이더에서 지원되지 않아 무시됩니다\n", gen.ProviderLabel(provider))
}

// genDirectionSet은 5방향 AI 생성 + 3방향 미러링으로 8방향 세트를 만듭니다.
func genDirectionSet(ctx context.Context, p gen.Provider, opt options, style, key string,
	baseBytes []byte, baseClean *image.NRGBA, logf func(string, ...any)) ([]sprite.StateFrames, []resultRow) {

	pre, ok := sprite.PresetByName(key)
	if !ok {
		logf("8방향 세트: 알 수 없는 키워드 %q (건너뜀)\n", key)
		return nil, nil
	}
	var states []sprite.StateFrames
	var rows []resultRow
	frameByDir := map[string][]*image.NRGBA{}
	var southRef []byte
	requested, hasDirFilter := parseDirFilter(opt.dirs, logf)
	// 미러 방향: west<-east, south-west<-south-east, north-west<-north-east
	mirror := map[string]string{"west": "east", "south-west": "south-east", "north-west": "north-east"}
	wantsOutput := func(dir string) bool {
		return !hasDirFilter || requested[dir]
	}
	needsAISource := func(dir string) bool {
		if wantsOutput(dir) {
			return true
		}
		for dst, src := range mirror {
			if src == dir && requested[dst] {
				return true
			}
		}
		return false
	}

	aiDirs := sprite.GeneratedDirections
	for _, d := range aiDirs {
		if hasDirFilter && !needsAISource(d) {
			continue
		}
		spec := sprite.StateSpec{Name: key + "-" + d, Frames: pre.Frames, FPS: pre.FPS, Loop: pre.Loop, Action: pre.Action, Facing: d}
		refs := [][]byte{baseBytes}
		if d != "south" && southRef != nil {
			refs = append(refs, southRef)
		}
		var bN *image.NRGBA
		if !sprite.IsBackFacing(d) {
			bN = baseClean
		}
		logf("  [%s] 생성 중... ", d)
		res := genState(ctx, p, opt, style, spec, refs, bN)
		logf("%d/%d 점수%d\n", res.Found, res.Expected, res.Score)
		if len(res.frames) > 0 {
			frameByDir[d] = res.frames
			if d == "south" && res.rawClean != nil {
				southRef = pngBytes(res.rawClean)
			}
			if wantsOutput(d) {
				states = append(states, sprite.StateFrames{Spec: spec, Frames: res.frames})
			}
		}
		if wantsOutput(d) {
			rows = append(rows, res.row())
		}
	}

	for dst, src := range mirror {
		if !wantsOutput(dst) {
			continue
		}
		srcFrames := frameByDir[src]
		if len(srcFrames) == 0 {
			continue
		}
		var mirrored []*image.NRGBA
		for _, f := range srcFrames {
			mirrored = append(mirrored, sprite.MirrorNRGBA(f))
		}
		spec := sprite.StateSpec{Name: key + "-" + dst, Frames: pre.Frames, FPS: pre.FPS, Loop: pre.Loop, Action: pre.Action, Facing: dst}
		states = append(states, sprite.StateFrames{Spec: spec, Frames: mirrored})
		rows = append(rows, resultRow{Name: spec.Name, Expected: pre.Frames, Found: len(mirrored), Status: "mirrored"})
		logf("  [%s] 미러링(%s) %d프레임\n", dst, src, len(mirrored))
	}
	return states, rows
}

func parseDirFilter(raw string, logf func(string, ...any)) (map[string]bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	valid := map[string]bool{}
	for _, d := range sprite.Directions {
		valid[d.Key] = true
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if !valid[key] {
			logf("  방향 필터: 알 수 없는 방향 %q 건너뜀\n", key)
			continue
		}
		out[key] = true
	}
	if len(out) == 0 {
		logf("  방향 필터: 유효한 방향이 없습니다\n")
	}
	return out, true
}
