package server

import (
	"testing"
	"time"
)

func TestArliaiCache_Status(t *testing.T) {
	cache := NewArliaiCache()

	// Тест пустого кеша
	_, ok := cache.GetStatus()
	if ok {
		t.Error("Expected empty cache to return false")
	}

	// Тест установки и получения
	testData := map[string]interface{}{
		"connected": true,
		"model":     "test-model",
	}

	cache.SetStatus(testData)
	cached, ok := cache.GetStatus()
	if !ok {
		t.Fatal("Expected cached status to be available")
	}

	cachedMap := cached.(map[string]interface{})
	if cachedMap["connected"] != true {
		t.Errorf("Expected connected=true, got: %v", cachedMap["connected"])
	}

	// Тест возраста кеша
	age := cache.GetStatusAge()
	if age < 0 {
		t.Error("Expected cache age to be non-negative")
	}
}

func TestArliaiCache_Models(t *testing.T) {
	cache := NewArliaiCache()

	// Тест пустого кеша
	_, ok := cache.GetModels()
	if ok {
		t.Error("Expected empty cache to return false")
	}

	// Тест установки и получения
	testData := map[string]interface{}{
		"models": []interface{}{
			map[string]interface{}{"name": "model1"},
			map[string]interface{}{"name": "model2"},
		},
		"total": 2,
	}

	cache.SetModels(testData)
	cached, ok := cache.GetModels()
	if !ok {
		t.Fatal("Expected cached models to be available")
	}

	cachedMap := cached.(map[string]interface{})
	if cachedMap["total"] != 2 {
		t.Errorf("Expected total=2, got: %v", cachedMap["total"])
	}

	// Тест возраста кеша
	age := cache.GetModelsAge()
	if age < 0 {
		t.Error("Expected cache age to be non-negative")
	}
}

func TestArliaiCache_Clear(t *testing.T) {
	cache := NewArliaiCache()

	// Устанавливаем данные
	cache.SetStatus(map[string]interface{}{"test": "data"})
	cache.SetModels(map[string]interface{}{"test": "data"})

	// Очищаем
	cache.Clear()

	// Проверяем, что кеш пуст
	_, ok := cache.GetStatus()
	if ok {
		t.Error("Expected status cache to be cleared")
	}

	_, ok = cache.GetModels()
	if ok {
		t.Error("Expected models cache to be cleared")
	}
}

func TestArliaiCache_TTL(t *testing.T) {
	cache := NewArliaiCache()
	
	// Устанавливаем короткий TTL для теста
	cache.statusTTL = 50 * time.Millisecond
	cache.modelsTTL = 50 * time.Millisecond

	// Устанавливаем данные
	cache.SetStatus(map[string]interface{}{"test": "data"})
	cache.SetModels(map[string]interface{}{"test": "data"})

	// Проверяем, что данные доступны сразу
	_, ok := cache.GetStatus()
	if !ok {
		t.Error("Expected status to be available immediately")
	}

	// Ждем истечения TTL
	time.Sleep(100 * time.Millisecond)

	// Проверяем, что данные истекли
	_, ok = cache.GetStatus()
	if ok {
		t.Error("Expected status cache to be expired")
	}

	_, ok = cache.GetModels()
	if ok {
		t.Error("Expected models cache to be expired")
	}
}

