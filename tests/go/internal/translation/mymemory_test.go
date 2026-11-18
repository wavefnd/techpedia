package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func translationResponse(t *testing.T, text string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"responseStatus": http.StatusOK,
		"responseData":   map[string]string{"translatedText": text},
	})
	if err != nil {
		t.Fatalf("encode translation response: %v", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestMyMemoryTranslatesInterfaceWithBoundedSingleMessageRequests(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query().Get("q")
		if len(query) > maxMyMemoryQueryBytes {
			t.Errorf("query has %d bytes, limit is %d", len(query), maxMyMemoryQueryBytes)
		}
		if strings.ContainsAny(query, "\r\n") {
			t.Errorf("multiple messages were combined in one query: %q", query)
		}
		if got := r.URL.Query().Get("langpair"); got != "en|fr" {
			t.Errorf("langpair = %q, want en|fr", got)
		}
		requests.Add(1)
		return translationResponse(t, "fr "+query), nil
	})}

	result, err := NewMyMemory("https://translate.example/get", "operator@example.com", client).TranslateInterface(context.Background(), "fr")
	if err != nil {
		t.Fatalf("translate interface: %v", err)
	}
	if len(result) != len(InterfaceMessages()) {
		t.Fatalf("translated %d messages, want %d", len(result), len(InterfaceMessages()))
	}
	if result["history"] != "fr Revision history" {
		t.Fatalf("unexpected history translation %q", result["history"])
	}
	if got := requests.Load(); got != int32(len(InterfaceMessages())) {
		t.Fatalf("request count = %d, want %d individual messages", got, len(InterfaceMessages()))
	}
}

func TestMyMemoryRejectsChangedMessageStructure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return translationResponse(t, "unexpected\nsecond line"), nil
	})}

	_, err := NewMyMemory("https://translate.example/get", "", client).TranslateInterface(context.Background(), "fr")
	if !errors.Is(err, ErrTranslationUnavailable) {
		t.Fatalf("error = %v, want ErrTranslationUnavailable", err)
	}
}

func TestMyMemoryRejectsWrongWritingSystem(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return translationResponse(t, "translated "+r.URL.Query().Get("q")), nil
	})}

	_, err := NewMyMemory("https://translate.example/get", "", client).TranslateInterface(context.Background(), "ko")
	if !errors.Is(err, ErrTranslationUnavailable) {
		t.Fatalf("error = %v, want ErrTranslationUnavailable", err)
	}
}

func TestMyMemoryRejectsCompositeLocaleTags(t *testing.T) {
	translator := NewMyMemory("https://translate.example/get", "", &http.Client{})
	for _, tag := range []string{"zh-Hans", "pt-BR"} {
		if _, err := translator.TranslateInterface(context.Background(), tag); !errors.Is(err, ErrTranslationUnavailable) {
			t.Fatalf("TranslateInterface(%q) error = %v, want ErrTranslationUnavailable", tag, err)
		}
	}
}

func TestMyMemoryUsesEnglishSeedsWithoutHTTP(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected request")
	})}

	result, err := NewMyMemory("https://translate.example/get", "", client).TranslateInterface(context.Background(), "en")
	if err != nil {
		t.Fatalf("translate English interface: %v", err)
	}
	if requests != 0 || result["save"] != "Save document" {
		t.Fatalf("English translation used HTTP or wrong seed: requests=%d save=%q", requests, result["save"])
	}
}
