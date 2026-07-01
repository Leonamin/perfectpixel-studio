package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFalSeedRequestBody(t *testing.T) {
	seed := 42
	var got falRequest
	c := NewFal("test-key", "fal-ai/nano-banana")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("요청 JSON 디코드 실패: %v", err)
		}
		body := `{"images":[{"url":"data:image/png;base64,` + base64.StdEncoding.EncodeToString(fakePNG) + `"}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := c.GenerateImage(context.Background(), "prompt", nil, "1:1", GenOpts{Seed: &seed}); err != nil {
		t.Fatalf("생성 실패: %v", err)
	}
	if got.Seed == nil || *got.Seed != seed {
		t.Fatalf("seed 누락: %+v", got.Seed)
	}
}

func TestGeminiSeedRequestBody(t *testing.T) {
	seed := 7
	var got genRequest
	c := NewClient("test-key", "")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("요청 JSON 디코드 실패: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(imageResponse())),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := c.GenerateImage(context.Background(), "prompt", nil, "1:1", GenOpts{Seed: &seed}); err != nil {
		t.Fatalf("생성 실패: %v", err)
	}
	if got.GenerationConfig == nil || got.GenerationConfig.Seed == nil || *got.GenerationConfig.Seed != seed {
		t.Fatalf("seed 누락: %+v", got.GenerationConfig)
	}
}

func TestOpenRouterSeedRequestBody(t *testing.T) {
	seed := 99
	var got orRequest
	c := NewOpenRouter("test-key", "")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("요청 읽기 실패: %v", err)
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("요청 JSON 디코드 실패: %v", err)
		}
		body := `{"choices":[{"message":{"images":[{"image_url":{"url":"data:image/png;base64,` + base64.StdEncoding.EncodeToString(fakePNG) + `"}}]}}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := c.GenerateImage(context.Background(), "prompt", nil, "1:1", GenOpts{Seed: &seed}); err != nil {
		t.Fatalf("생성 실패: %v", err)
	}
	if got.Seed == nil || *got.Seed != seed {
		t.Fatalf("seed 누락: %+v", got.Seed)
	}
}
