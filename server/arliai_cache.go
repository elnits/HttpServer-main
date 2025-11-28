package server

import (
	"sync"
	"time"
)

// CacheEntry запись в кеше
type CacheEntry struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// ArliaiCache кеш для результатов Arliai API
type ArliaiCache struct {
	mu              sync.RWMutex
	statusCache     *CacheEntry
	modelsCache     *CacheEntry
	statusTTL       time.Duration
	modelsTTL       time.Duration
}

// NewArliaiCache создает новый кеш
func NewArliaiCache() *ArliaiCache {
	return &ArliaiCache{
		statusTTL: 60 * time.Second,  // Кеш статуса на 60 секунд
		modelsTTL: 300 * time.Second, // Кеш моделей на 5 минут
	}
}

// GetStatus получает статус из кеша
func (c *ArliaiCache) GetStatus() (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.statusCache == nil {
		return nil, false
	}

	if time.Since(c.statusCache.Timestamp) > c.statusCache.TTL {
		return nil, false
	}

	return c.statusCache.Data, true
}

// SetStatus устанавливает статус в кеш
func (c *ArliaiCache) SetStatus(data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.statusCache = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       c.statusTTL,
	}
}

// GetModels получает модели из кеша
func (c *ArliaiCache) GetModels() (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.modelsCache == nil {
		return nil, false
	}

	if time.Since(c.modelsCache.Timestamp) > c.modelsCache.TTL {
		return nil, false
	}

	return c.modelsCache.Data, true
}

// SetModels устанавливает модели в кеш
func (c *ArliaiCache) SetModels(data interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.modelsCache = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       c.modelsTTL,
	}
}

// Clear очищает кеш
func (c *ArliaiCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.statusCache = nil
	c.modelsCache = nil
}

// GetStatusAge возвращает возраст кеша статуса
func (c *ArliaiCache) GetStatusAge() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.statusCache == nil {
		return 0
	}

	return time.Since(c.statusCache.Timestamp)
}

// GetModelsAge возвращает возраст кеша моделей
func (c *ArliaiCache) GetModelsAge() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.modelsCache == nil {
		return 0
	}

	return time.Since(c.modelsCache.Timestamp)
}

