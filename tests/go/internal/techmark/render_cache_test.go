package techmark

import (
	"crypto/sha256"
	"strings"
	"sync"
	"testing"
)

func TestLargeArticleUsesBoundedRenderCache(t *testing.T) {
	parser := NewParser()
	source := strings.Repeat("Paragraph text with [[Target]] and **bold**.\n\n", 6000)
	html := parser.RenderWithLocale(source, "ko")
	if len(html) <= 512<<10 || len(parser.cache.entries) != 1 {
		t.Fatalf("long article not admitted: bytes=%d entries=%d", len(html), len(parser.cache.entries))
	}
	if got := parser.RenderWithLocale(source, "ko"); got != html {
		t.Fatal("cached render changed HTML")
	}
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if got := parser.RenderWithLocale(source, "en"); !strings.Contains(got, "/en/wiki/") {
				t.Error("concurrent locale render mixed cached links")
			}
		}()
	}
	wait.Wait()
	if len(parser.cache.entries) != 2 || parser.cache.bytes > parser.cache.maxBytes {
		t.Fatal("coalesced cache exceeded its budget")
	}
}

func TestRenderCacheClearRejectsInFlightGeneration(t *testing.T) {
	cache := newRenderCache(10)
	key := sha256.Sum256([]byte("old personal value"))
	generation := cache.generation
	cache.clear()
	cache.putGeneration(key, "old personal HTML", generation)
	if _, ok := cache.get(key); ok {
		t.Fatal("invalidated render repopulated the cache")
	}
	cache.putGeneration(key, "safe HTML", cache.generation)
	if got, ok := cache.get(key); !ok || got != "safe HTML" {
		t.Fatal("new generation was not admitted")
	}
}

func TestRenderCacheEvictsByTotalBytes(t *testing.T) {
	cache := newRenderCache(20)
	cache.maxBytes, cache.maxEntryBytes = 100, 80
	for i := range 20 {
		cache.put(sha256.Sum256([]byte{byte(i)}), strings.Repeat("x", 60))
		if cache.bytes > 100 || len(cache.entries) != 1 {
			t.Fatal("cache byte budget exceeded")
		}
	}
	cache.put(sha256.Sum256([]byte("oversize")), strings.Repeat("x", 81))
	if cache.bytes != 60 || len(cache.entries) != 1 {
		t.Fatal("oversized render displaced admissible cache entry")
	}
}

func BenchmarkWarmArticleRender(b *testing.B) {
	for _, fixture := range []struct {
		name  string
		count int
	}{{"small", 200}, {"large", 6000}} {
		b.Run(fixture.name, func(b *testing.B) {
			source := strings.Repeat("Paragraph text with [[Target]] and **bold**.\n\n", fixture.count)
			parser := NewParser()
			parser.RenderWithLocale(source, "ko")
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				parser.RenderWithLocale(source, "ko")
			}
		})
	}
}
