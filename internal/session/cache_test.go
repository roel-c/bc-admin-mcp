package session_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/stretchr/testify/suite"
)

type CacheSuite struct {
	suite.Suite
	cache *session.Cache
}

func TestCacheSuite(t *testing.T) {
	suite.Run(t, new(CacheSuite))
}

func (s *CacheSuite) SetupTest() {
	s.cache = session.NewCache(1 * time.Second)
}

func (s *CacheSuite) TestSetAndGet() {
	s.cache.Set("key1", "value1")
	val, ok := s.cache.Get("key1")
	s.True(ok)
	s.Equal("value1", val)
}

func (s *CacheSuite) TestGetMissingKeyReturnsFalse() {
	_, ok := s.cache.Get("nonexistent")
	s.False(ok)
}

func (s *CacheSuite) TestTTLExpiration() {
	shortCache := session.NewCache(50 * time.Millisecond)
	shortCache.Set("expires", "soon")

	val, ok := shortCache.Get("expires")
	s.True(ok)
	s.Equal("soon", val)

	time.Sleep(60 * time.Millisecond)

	_, ok = shortCache.Get("expires")
	s.False(ok)
}

func (s *CacheSuite) TestDeleteRemovesEntry() {
	s.cache.Set("to-delete", "value")
	s.cache.Delete("to-delete")
	_, ok := s.cache.Get("to-delete")
	s.False(ok)
}

func (s *CacheSuite) TestClearRemovesAll() {
	s.cache.Set("a", 1)
	s.cache.Set("b", 2)
	s.cache.Set("c", 3)
	s.cache.Clear()

	_, okA := s.cache.Get("a")
	_, okB := s.cache.Get("b")
	_, okC := s.cache.Get("c")
	s.False(okA)
	s.False(okB)
	s.False(okC)
}

func (s *CacheSuite) TestEvictRemovesExpired() {
	shortCache := session.NewCache(50 * time.Millisecond)
	shortCache.Set("expires", "soon")
	time.Sleep(60 * time.Millisecond)

	shortCache.Evict()
	_, ok := shortCache.Get("expires")
	s.False(ok)
}

func (s *CacheSuite) TestConcurrentAccess() {
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			s.cache.Set(key, n)
			s.cache.Get(key)
		}(i)
	}
	wg.Wait()
}

func (s *CacheSuite) TestCacheOrFetchReturnsCachedValue() {
	s.cache.Set("key", "cached")
	calls := 0
	val, err := session.CacheOrFetch(s.cache, "key", func() (string, error) {
		calls++
		return "fetched", nil
	})
	s.NoError(err)
	s.Equal("cached", val)
	s.Equal(0, calls, "fetch should not be called on a cache hit")
}

func (s *CacheSuite) TestCacheOrFetchCallsFetchOnMiss() {
	calls := 0
	val, err := session.CacheOrFetch(s.cache, "missing", func() (string, error) {
		calls++
		return "fetched", nil
	})
	s.NoError(err)
	s.Equal("fetched", val)
	s.Equal(1, calls)
}

func (s *CacheSuite) TestCacheOrFetchStoresFetchedValue() {
	_, _ = session.CacheOrFetch(s.cache, "key", func() (string, error) {
		return "stored", nil
	})
	raw, ok := s.cache.Get("key")
	s.True(ok)
	s.Equal("stored", raw)
}

func (s *CacheSuite) TestCacheOrFetchPropagatesFetchError() {
	_, err := session.CacheOrFetch(s.cache, "key", func() (string, error) {
		return "", fmt.Errorf("fetch failed")
	})
	s.EqualError(err, "fetch failed")
	_, ok := s.cache.Get("key")
	s.False(ok, "failed fetch must not populate the cache")
}

func (s *CacheSuite) TestCacheOrFetchDoesNotCacheOnError() {
	calls := 0
	for range 2 {
		_, _ = session.CacheOrFetch(s.cache, "key", func() (string, error) {
			calls++
			return "", fmt.Errorf("fetch failed")
		})
	}
	s.Equal(2, calls, "fetch should be retried when a previous call errored")
}

type StoreSuite struct {
	suite.Suite
	store *session.Store
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, new(StoreSuite))
}

func (s *StoreSuite) SetupTest() {
	s.store = session.NewStore(1 * time.Second)
}

func (s *StoreSuite) TestForSessionCreatesCache() {
	cache := s.store.ForSession("session-1")
	s.NotNil(cache)
}

func (s *StoreSuite) TestForSessionReturnsSameInstance() {
	cache1 := s.store.ForSession("session-1")
	cache2 := s.store.ForSession("session-1")
	s.Equal(cache1, cache2)
}

func (s *StoreSuite) TestForSessionIsolatesSessions() {
	c1 := s.store.ForSession("s1")
	c2 := s.store.ForSession("s2")

	c1.Set("key", "from-s1")
	c2.Set("key", "from-s2")

	val1, _ := c1.Get("key")
	val2, _ := c2.Get("key")
	s.Equal("from-s1", val1)
	s.Equal("from-s2", val2)
}

func (s *StoreSuite) TestForContextFallsBackToDefault() {
	cache := s.store.ForContext(context.Background())
	s.NotNil(cache)
	defaultCache := s.store.ForSession("default")
	s.Equal(defaultCache, cache, "ForContext with no MCP session should use the 'default' bucket")
}

func (s *StoreSuite) TestSessionIDFromContextReturnsFallback() {
	id := session.SessionIDFromContext(context.Background())
	s.Equal("default", id)
}

func (s *StoreSuite) TestRemoveSessionCleansUp() {
	cache := s.store.ForSession("to-remove")
	cache.Set("key", "value")

	s.store.RemoveSession("to-remove")

	newCache := s.store.ForSession("to-remove")
	_, ok := newCache.Get("key")
	s.False(ok)
}
