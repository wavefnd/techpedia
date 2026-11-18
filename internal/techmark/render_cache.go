package techmark

import (
	"crypto/sha256"
	"sync"
)

type renderCache struct {
	mu            sync.Mutex
	buildMu       sync.Mutex
	entries       map[[sha256.Size]byte]renderCacheEntry
	keys          [][sha256.Size]byte
	head          int
	bytes         int
	limit         int
	maxBytes      int
	maxEntryBytes int
	generation    uint64
}

type renderCacheEntry struct {
	value string
	size  int
}

func newRenderCache(limit int) *renderCache {
	return &renderCache{
		entries:       make(map[[sha256.Size]byte]renderCacheEntry, limit),
		limit:         limit,
		maxBytes:      16 << 20,
		maxEntryBytes: 4 << 20,
	}
}

func (p *Parser) renderCached(source string, linkPrefix string) string {
	if p.cache == nil || p.cache.limit <= 0 {
		return p.render(source, linkPrefix)
	}
	p.cache.mu.Lock()
	generation := p.cache.generation
	p.cache.mu.Unlock()
	hash := sha256.New()
	// A large string-to-byte conversion would allocate the whole article even
	// on a warm hit. Reuse one small hashing buffer instead.
	var chunk [4096]byte
	writeString := func(value string) {
		for len(value) > 0 {
			n := copy(chunk[:], value)
			hash.Write(chunk[:n])
			value = value[n:]
		}
	}
	writeString(linkPrefix)
	chunk[0] = 0
	hash.Write(chunk[:1])
	writeString(source)
	var key [sha256.Size]byte
	hash.Sum(key[:0])
	if rendered, ok := p.cache.get(key); ok {
		return rendered
	}
	// Cache hits never wait for parsing. Cold renders are coalesced and
	// serialized per parser to bound transient allocations on small VPSes.
	p.cache.buildMu.Lock()
	defer p.cache.buildMu.Unlock()
	if rendered, ok := p.cache.get(key); ok {
		return rendered
	}
	rendered := p.render(source, linkPrefix)
	p.cache.putGeneration(key, rendered, generation)
	return rendered
}

func (c *renderCache) get(key [sha256.Size]byte) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	return entry.value, ok
}

func (c *renderCache) put(key [sha256.Size]byte, value string) {
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()
	c.putGeneration(key, value, generation)
}

func (c *renderCache) putGeneration(key [sha256.Size]byte, value string, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return
	}
	if _, exists := c.entries[key]; exists {
		return
	}
	size := len(value)
	if size > c.maxEntryBytes || size > c.maxBytes {
		return
	}
	for (len(c.entries) >= c.limit || c.bytes+size > c.maxBytes) && c.head < len(c.keys) {
		oldKey := c.keys[c.head]
		c.head++
		if old, exists := c.entries[oldKey]; exists {
			delete(c.entries, oldKey)
			c.bytes -= old.size
		}
	}
	if c.head > c.limit && c.head*2 >= len(c.keys) {
		c.keys = append([][sha256.Size]byte(nil), c.keys[c.head:]...)
		c.head = 0
	}
	c.keys = append(c.keys, key)
	c.entries[key] = renderCacheEntry{value: value, size: size}
	c.bytes += size
}

func (c *renderCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[[sha256.Size]byte]renderCacheEntry, c.limit)
	c.keys = nil
	c.head = 0
	c.bytes = 0
	c.generation++
}
