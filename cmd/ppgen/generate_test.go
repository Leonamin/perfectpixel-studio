package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPNG(t *testing.T, path string, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLoadBaseKeepsTransparentPNGBytes(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 3; y < 5; y++ {
		for x := 3; x < 5; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 80, B: 200, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "base.png")
	raw := writeTestPNG(t, path, img)

	base, got, cleaned, err := loadBase(path, "pixel")
	if err != nil {
		t.Fatalf("loadBase 실패: %v", err)
	}
	if cleaned {
		t.Fatal("투명 base는 재정리하지 않아야 합니다")
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("투명 base PNG 바이트는 그대로 재사용해야 합니다")
	}
	if base.NRGBAAt(3, 3).A != 255 || base.NRGBAAt(0, 0).A != 0 {
		t.Fatal("base alpha가 보존되지 않았습니다")
	}
}

func TestLoadBaseCleansOpaquePNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
		}
	}
	for y := 6; y < 10; y++ {
		for x := 6; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 30, G: 120, B: 220, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "base.png")
	writeTestPNG(t, path, img)

	base, got, cleaned, err := loadBase(path, "pixel")
	if err != nil {
		t.Fatalf("loadBase 실패: %v", err)
	}
	if !cleaned {
		t.Fatal("불투명 base는 배경 제거되어야 합니다")
	}
	if len(got) == 0 {
		t.Fatal("정리된 PNG 바이트가 비었습니다")
	}
	if base.NRGBAAt(0, 0).A != 0 {
		t.Fatal("불투명 배경이 제거되지 않았습니다")
	}
}
