package session

import (
	"context"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

const sessionIDFallback = "default"

// SessionIDFromContext extracts the MCP session ID from ctx.
// Falls back to "default" when no session is present, ensuring single-session
// and stdio deployments still benefit from the preview→confirm cache.
func SessionIDFromContext(ctx context.Context) string {
	if s := mcpserver.ClientSessionFromContext(ctx); s != nil {
		if id := s.SessionID(); id != "" {
			return id
		}
	}
	return sessionIDFallback
}

// CacheOrFetch returns the cached value for key if present and of type T.
// On a cache miss it calls fetch, stores the result, and returns it.
// This is the canonical pattern for preview→confirm caching: the preview
// handler calls CacheOrFetch (which fetches and stores), and the confirm
// handler calls it again (which returns the cached value, skipping the
// redundant BC API round-trip).
func CacheOrFetch[T any](cache *Cache, key string, fetch func() (T, error)) (T, error) {
	if raw, ok := cache.Get(key); ok {
		if v, ok := raw.(T); ok {
			return v, nil
		}
	}
	v, err := fetch()
	if err != nil {
		var zero T
		return zero, err
	}
	cache.Set(key, v)
	return v, nil
}

type entry struct {
	value     any
	expiresAt time.Time
}

func (e entry) expired() bool {
	return time.Now().After(e.expiresAt)
}

const defaultMaxEntries = 1000

// Cache provides session-scoped, TTL-based caching for data shared between
// sequential tool calls (e.g., product lists fetched in a search that are
// reused by a subsequent bulk update). Each MCP session gets its own Cache
// to avoid cross-session data leaks. Entries are bounded by maxEntries to
// prevent memory exhaustion.
type Cache struct {
	mu         sync.RWMutex
	items      map[string]entry
	ttl        time.Duration
	maxEntries int
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		items:      make(map[string]entry),
		ttl:        ttl,
		maxEntries: defaultMaxEntries,
	}
}

func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked()
	if len(c.items) >= c.maxEntries {
		c.evictOldestLocked()
	}
	c.items[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || e.expired() {
		return nil, false
	}
	return e.value, true
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]entry)
}

// Evict removes all expired entries.
func (c *Cache) Evict() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked()
}

func (c *Cache) evictExpiredLocked() {
	for k, e := range c.items {
		if e.expired() {
			delete(c.items, k)
		}
	}
}

func (c *Cache) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, e := range c.items {
		if first || e.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.expiresAt
			first = false
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}

const defaultMaxSessions = 100

// sessionEntry pairs a cache with the time it was created, used to evict
// the oldest session when the session cap is reached.
type sessionEntry struct {
	cache     *Cache
	createdAt time.Time
}

// Store manages per-session caches, keyed by MCP session ID. The total
// number of sessions is bounded to prevent resource exhaustion. When the
// cap is reached the oldest session (by creation time) is evicted rather
// than a random one, so active sessions are less likely to lose their
// preview cache between the preview and confirm steps.
type Store struct {
	mu          sync.RWMutex
	sessions    map[string]sessionEntry
	ttl         time.Duration
	maxSessions int
}

func NewStore(defaultTTL time.Duration) *Store {
	return &Store{
		sessions:    make(map[string]sessionEntry),
		ttl:         defaultTTL,
		maxSessions: defaultMaxSessions,
	}
}

func (s *Store) ForSession(sessionID string) *Cache {
	s.mu.RLock()
	if e, ok := s.sessions[sessionID]; ok {
		s.mu.RUnlock()
		return e.cache
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.sessions[sessionID]; ok {
		return e.cache
	}
	if len(s.sessions) >= s.maxSessions {
		s.evictOldestSessionLocked()
	}
	c := NewCache(s.ttl)
	s.sessions[sessionID] = sessionEntry{cache: c, createdAt: time.Now()}
	return c
}

// ForContext is a convenience wrapper around ForSession that extracts the
// session ID from ctx via SessionIDFromContext. Prefer this over ForSession
// in tool handlers so the session-ID logic stays in one place.
func (s *Store) ForContext(ctx context.Context) *Cache {
	return s.ForSession(SessionIDFromContext(ctx))
}

// evictOldestSessionLocked removes the session with the earliest createdAt
// timestamp. Must be called with s.mu held for writing.
func (s *Store) evictOldestSessionLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, e := range s.sessions {
		if first || e.createdAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.createdAt
			first = false
		}
	}
	if !first {
		delete(s.sessions, oldestKey)
	}
}

func (s *Store) RemoveSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}
