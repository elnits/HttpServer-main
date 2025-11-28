package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// CacheEntry представляет запись в кеше
type CacheEntry struct {
	NormalizedName string
	Category       string
	Confidence     float64
	Reasoning      string
	Timestamp      time.Time
	ExpiresAt      time.Time
}

// CacheStats содержит статистику работы кеша
type CacheStats struct {
	Hits         int64
	Misses       int64
	Entries      int
	HitRate      float64
	MemoryUsageB int64
}

// AICache управляет кешированием результатов AI нормализации
type AICache struct {
	cache      map[string]*CacheEntry
	mu         sync.RWMutex
	ttl        time.Duration
	hits       int64
	misses     int64
	maxEntries int
}

// NewAICache создает новый экземпляр кеша
func NewAICache(ttl time.Duration, maxEntries int) *AICache {
	cache := &AICache{
		cache:      make(map[string]*CacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}

	// Запускаем горутину для периодической очистки устаревших записей
	go cache.cleanupExpired()

	return cache
}

// generateKey создает уникальный ключ для исходного наименования
func (c *AICache) generateKey(sourceName string) string {
	hash := sha256.Sum256([]byte(sourceName))
	return hex.EncodeToString(hash[:])
}

// Get получает результат из кеша
func (c *AICache) Get(sourceName string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.generateKey(sourceName)
	entry, exists := c.cache[key]

	if !exists {
		c.mu.RUnlock()
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}

	// Проверяем, не истек ли срок действия
	if time.Now().After(entry.ExpiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.cache, key)
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}

	c.mu.RUnlock()
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	c.mu.RLock()

	return entry, true
}

// Set добавляет результат в кеш
func (c *AICache) Set(sourceName, normalizedName, category string, confidence float64, reasoning string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Если достигнут лимит записей, удаляем самую старую
	if len(c.cache) >= c.maxEntries {
		c.evictOldest()
	}

	key := c.generateKey(sourceName)
	now := time.Now()

	c.cache[key] = &CacheEntry{
		NormalizedName: normalizedName,
		Category:       category,
		Confidence:     confidence,
		Reasoning:      reasoning,
		Timestamp:      now,
		ExpiresAt:      now.Add(c.ttl),
	}
}

// evictOldest удаляет самую старую запись из кеша
func (c *AICache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	first := true
	for key, entry := range c.cache {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// cleanupExpired периодически удаляет устаревшие записи
func (c *AICache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		expiredKeys := make([]string, 0)

		for key, entry := range c.cache {
			if now.After(entry.ExpiresAt) {
				expiredKeys = append(expiredKeys, key)
			}
		}

		for _, key := range expiredKeys {
			delete(c.cache, key)
		}
		c.mu.Unlock()
	}
}

// GetStats возвращает статистику работы кеша
func (c *AICache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	// Примерный расчет использования памяти
	memoryUsage := int64(len(c.cache) * 256) // ~256 байт на запись

	return CacheStats{
		Hits:         c.hits,
		Misses:       c.misses,
		Entries:      len(c.cache),
		HitRate:      hitRate,
		MemoryUsageB: memoryUsage,
	}
}

// Clear очищает весь кеш
func (c *AICache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
	c.hits = 0
	c.misses = 0
}

// Size возвращает количество записей в кеше
func (c *AICache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// SetTTL обновляет время жизни записей в кеше
func (c *AICache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}
