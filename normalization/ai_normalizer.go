package normalization

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"httpserver/nomenclature"
)

// AIResult представляет результат нормализации от AI
type AIResult struct {
	NormalizedName string  `json:"normalized_name"`
	Category       string  `json:"category"`
	Confidence     float64 `json:"confidence"`
	Reasoning      string  `json:"reasoning"`
}

// AIStats статистика работы AI нормализатора
type AIStats struct {
	TotalCalls  int64
	CacheHits   int64
	CacheMisses int64
	Errors      int64
	totalLatency int64 // в наносекундах
	mutex       sync.RWMutex
}

// AINormalizer использует Arliai API для нормализации
type AINormalizer struct {
	aiClient       *nomenclature.AIClient
	cache          *AICache
	statsCollector *StatsCollector
	systemPrompt   string
	stats          *AIStats // старая статистика для совместимости
	batchProcessor *BatchProcessor // Батчевый процессор для группировки AI запросов
	batchEnabled   bool // Флаг включения батчевой обработки
}

// NewAINormalizer создает новый AI нормализатор
// Если model пустая, используется значение из переменной окружения ARLIAI_MODEL или дефолт "GLM-4.5-Air"
func NewAINormalizer(apiKey string, model ...string) *AINormalizer {
	modelName := "GLM-4.5-Air" // Дефолтная модель
	if len(model) > 0 && model[0] != "" {
		modelName = model[0]
	} else {
		// Пытаемся получить из переменной окружения
		if envModel := os.Getenv("ARLIAI_MODEL"); envModel != "" {
			modelName = envModel
		}
	}
	client := nomenclature.NewAIClient(apiKey, modelName)

	// Создаем кеш с TTL 1 час и макс. 10000 записей
	cache := NewAICache(1*time.Hour, 10000)

	// Создаем сборщик статистики
	statsCollector := NewStatsCollector()

	systemPrompt := `Ты - эксперт по нормализации наименований товаров и их категоризации.

ТВОЯ ЗАДАЧА:
1. НОРМАЛИЗОВАТЬ наименование товара:
   - Исправить опечатки и грамматические ошибки
   - Привести к стандартной форме
   - Удалить технические коды, артикулы, размеры (но сохранить смысл)
   - Унифицировать синонимы (например: "молоток" вместо "молотак", "отвертка" вместо "отвертка крестовая №2")
   - Использовать единообразную терминологию

2. ОПРЕДЕЛИТЬ КАТЕГОРИЮ товара из списка:
   - инструмент
   - медикаменты
   - стройматериалы
   - электроника
   - оборудование
   - расходники
   - автоаксессуары
   - канцелярия
   - средства очистки
   - продукты
   - сельское хозяйство
   - связь
   - сантехника
   - мебель
   - инструменты измерительные
   - программное обеспечение
   - упаковка
   - другое

ВАЖНЫЕ ПРАВИЛА:
- Нормализованное имя должно быть лаконичным и понятным (2-100 символов)
- Сохраняй ключевые характеристики товара (материал, назначение)
- Категория должна точно соответствовать товару
- Если не уверен в категории - выбирай "другое"
- Уверенность (confidence) от 0.0 до 1.0 (0.9+ только если полностью уверен)

ФОРМАТ ОТВЕТА - СТРОГО JSON:
{
    "normalized_name": "нормализованное наименование",
    "category": "категория из списка",
    "confidence": 0.95,
    "reasoning": "краткое объяснение нормализации и выбора категории"
}

ПРИМЕРЫ:

Вход: "МОЛОТАК СТРОИТЕЛЬНЫЙ 500гр ER-00013004"
Ответ:
{
    "normalized_name": "молоток строительный",
    "category": "инструмент",
    "confidence": 0.98,
    "reasoning": "Исправлена опечатка 'молотак', удален артикул и вес"
}

Вход: "Кабель медный ВВГнг 3х2.5 100м"
Ответ:
{
    "normalized_name": "кабель ввгнг",
    "category": "стройматериалы",
    "confidence": 0.95,
    "reasoning": "Удалены технические характеристики, сохранен тип кабеля"
}

Отвечай ТОЛЬКО JSON, без дополнительных пояснений.`

	return &AINormalizer{
		aiClient:       client,
		cache:          cache,
		statsCollector: statsCollector,
		systemPrompt:   systemPrompt,
		stats:          &AIStats{},
		batchProcessor: nil, // Инициализируется через EnableBatchProcessing()
		batchEnabled:   false,
	}
}

// EnableBatchProcessing включает батчевую обработку AI запросов
// batchSize - количество элементов в одном батче
// flushInterval - интервал автоматической обработки накопленных запросов
func (a *AINormalizer) EnableBatchProcessing(batchSize int, flushInterval time.Duration) {
	if a.batchProcessor != nil {
		// Закрываем существующий процессор
		a.batchProcessor.Close()
	}

	// Создаем новый батч-процессор
	a.batchProcessor = NewBatchProcessor(a.aiClient, batchSize, flushInterval)
	a.batchEnabled = true
	log.Printf("✓ Батчевая обработка AI включена: размер батча=%d, интервал=%v", batchSize, flushInterval)
}

// NormalizeWithAI нормализует название товара с помощью AI
func (a *AINormalizer) NormalizeWithAI(name string) (*AIResult, error) {
	startTime := time.Now()

	// Проверяем кэш (case-insensitive)
	sourceName := strings.ToLower(strings.TrimSpace(name))

	if cached, exists := a.cache.Get(sourceName); exists {
		// Кеш hit
		atomic.AddInt64(&a.stats.CacheHits, 1)
		cacheStats := a.cache.GetStats()
		a.statsCollector.RecordCacheAccess(true, cacheStats.Entries, cacheStats.MemoryUsageB)

		return &AIResult{
			NormalizedName: cached.NormalizedName,
			Category:       cached.Category,
			Confidence:     cached.Confidence,
			Reasoning:      cached.Reasoning,
		}, nil
	}

	// Кеш miss
	atomic.AddInt64(&a.stats.CacheMisses, 1)
	atomic.AddInt64(&a.stats.TotalCalls, 1)
	cacheStats := a.cache.GetStats()
	a.statsCollector.RecordCacheAccess(false, cacheStats.Entries, cacheStats.MemoryUsageB)

	// Используем батчевую обработку если включена
	if a.batchEnabled && a.batchProcessor != nil {
		result := a.batchProcessor.Add(name)

		duration := time.Since(startTime)

		if result.Error != nil {
			atomic.AddInt64(&a.stats.Errors, 1)
			a.statsCollector.RecordAIRequest(duration, false)
			a.statsCollector.RecordError("batch_ai_request", result.Error.Error())
			return nil, fmt.Errorf("batch AI request failed: %v", result.Error)
		}

		// Успешный результат - сохраняем в кеш
		atomic.AddInt64(&a.stats.totalLatency, int64(duration))
		a.statsCollector.RecordAIRequest(duration, true)

		aiResult := &AIResult{
			NormalizedName: result.NormalizedName,
			Category:       result.Category,
			Confidence:     result.Confidence,
			Reasoning:      result.Reasoning,
		}

		a.cache.Set(sourceName, aiResult.NormalizedName, aiResult.Category, aiResult.Confidence, aiResult.Reasoning)

		return aiResult, nil
	}

	// Отправляем запрос к AI
	userPrompt := fmt.Sprintf("НАИМЕНОВАНИЕ ТОВАРА ДЛЯ ОБРАБОТКИ: \"%s\"", name)
	response, err := a.aiClient.GetCompletion(a.systemPrompt, userPrompt)

	duration := time.Since(startTime)

	if err != nil {
		atomic.AddInt64(&a.stats.Errors, 1)
		a.statsCollector.RecordAIRequest(duration, false)
		a.statsCollector.RecordError("ai_request", err.Error())
		return nil, fmt.Errorf("AI request failed: %v", err)
	}

	// Парсим JSON ответ
	var result AIResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		atomic.AddInt64(&a.stats.Errors, 1)
		a.statsCollector.RecordAIRequest(duration, false)
		a.statsCollector.RecordError("json_parse", err.Error())
		return nil, fmt.Errorf("failed to parse AI response: %v, response: %s", err, response)
	}

	// Валидация результата
	if result.NormalizedName == "" {
		atomic.AddInt64(&a.stats.Errors, 1)
		a.statsCollector.RecordAIRequest(duration, false)
		a.statsCollector.RecordError("validation", "empty normalized_name")
		return nil, fmt.Errorf("AI returned empty normalized_name")
	}

	if result.Category == "" {
		result.Category = "другое"
	}

	if result.Confidence == 0 {
		result.Confidence = 0.5 // default low confidence
	}

	// Сохраняем в кэш
	a.cache.Set(sourceName, result.NormalizedName, result.Category, result.Confidence, result.Reasoning)

	// Обновляем статистику
	latency := time.Since(startTime).Nanoseconds()
	atomic.AddInt64(&a.stats.totalLatency, latency)
	a.statsCollector.RecordAIRequest(duration, true)

	return &result, nil
}

// RequiresAI определяет, требует ли товар AI обработки
func (a *AINormalizer) RequiresAI(name, category string) bool {
	// Если категория "другое" - точно нужен AI
	if category == "другое" {
		return true
	}

	// Если название очень длинное (> 50 символов)
	if len(name) > 50 {
		return true
	}

	// Если много слов (> 5)
	wordCount := len(strings.Fields(name))
	if wordCount > 5 {
		return true
	}

	// Если содержит нестандартные символы (много цифр, спецсимволы)
	if containsComplexPatterns(name) {
		return true
	}

	// Если содержит технические спецификации
	if containsTechnicalSpecs(name) {
		return true
	}

	return false
}

// GetStats возвращает статистику работы AI
func (a *AINormalizer) GetStats() *AIStats {
	a.stats.mutex.RLock()
	defer a.stats.mutex.RUnlock()

	totalCalls := atomic.LoadInt64(&a.stats.TotalCalls)
	cacheHits := atomic.LoadInt64(&a.stats.CacheHits)
	cacheMisses := atomic.LoadInt64(&a.stats.CacheMisses)
	errors := atomic.LoadInt64(&a.stats.Errors)
	totalLatency := atomic.LoadInt64(&a.stats.totalLatency)

	return &AIStats{
		TotalCalls:   totalCalls,
		CacheHits:    cacheHits,
		CacheMisses:  cacheMisses,
		Errors:       errors,
		totalLatency: totalLatency,
	}
}

// AvgLatency возвращает среднюю латентность AI вызовов
func (s *AIStats) AvgLatency() time.Duration {
	if s.TotalCalls == 0 {
		return 0
	}
	avgNanos := s.totalLatency / s.TotalCalls
	return time.Duration(avgNanos)
}

// CacheHitRate возвращает процент попаданий в кэш
func (s *AIStats) CacheHitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total) * 100
}

// ClearCache очищает кэш нормализации
func (a *AINormalizer) ClearCache() {
	a.cache.Clear()
	log.Println("AI normalizer cache cleared")
}

// GetCacheSize возвращает размер кэша
func (a *AINormalizer) GetCacheSize() int {
	return a.cache.Size()
}

// GetStatsCollector возвращает сборщик статистики
func (a *AINormalizer) GetStatsCollector() *StatsCollector {
	return a.statsCollector
}

// GetCacheStats возвращает статистику кэша
func (a *AINormalizer) GetCacheStats() CacheStats {
	if a.cache == nil {
		return CacheStats{}
	}
	return a.cache.GetStats()
}

// GetCircuitBreakerState возвращает состояние Circuit Breaker
func (a *AINormalizer) GetCircuitBreakerState() map[string]interface{} {
	if a.aiClient == nil {
		return map[string]interface{}{
			"enabled":        false,
			"state":          "unknown",
			"can_proceed":    false,
			"failure_count":  0,
			"success_count":  0,
			"last_failure_time": nil,
		}
	}
	return a.aiClient.GetCircuitBreakerState()
}

// GetBatchProcessorStats возвращает статистику батчевой обработки
func (a *AINormalizer) GetBatchProcessorStats() map[string]interface{} {
	if !a.batchEnabled || a.batchProcessor == nil {
		return map[string]interface{}{
			"enabled":             false,
			"queue_size":          0,
			"total_batches":       0,
			"avg_items_per_batch": 0.0,
			"api_calls_saved":     0,
		}
	}

	stats := a.batchProcessor.GetStats()
	queueSize := a.batchProcessor.QueueSize()

	// Рассчитываем количество сэкономленных API вызовов
	// Если бы не было батчей, каждый элемент требовал бы отдельный вызов
	// С батчами: total_items / avg_items_per_batch вызовов
	apiCallsSaved := int64(0)
	if stats.TotalItems > 0 && stats.AverageItemsPerBatch > 0 {
		actualCalls := stats.TotalBatches
		withoutBatching := stats.TotalItems
		apiCallsSaved = withoutBatching - actualCalls
		if apiCallsSaved < 0 {
			apiCallsSaved = 0
		}
	}

	var lastBatchTime *string
	if !stats.LastBatchTime.IsZero() {
		timeStr := stats.LastBatchTime.Format(time.RFC3339)
		lastBatchTime = &timeStr
	}

	return map[string]interface{}{
		"enabled":             true,
		"queue_size":          queueSize,
		"total_batches":       stats.TotalBatches,
		"avg_items_per_batch": stats.AverageItemsPerBatch,
		"api_calls_saved":     apiCallsSaved,
		"last_batch_time":     lastBatchTime,
	}
}

// containsComplexPatterns проверяет наличие сложных паттернов
func containsComplexPatterns(name string) bool {
	digitCount := 0
	specialCount := 0

	for _, r := range name {
		if unicode.IsDigit(r) {
			digitCount++
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) {
			specialCount++
		}
	}

	// Если более 30% цифр или более 10% спецсимволов
	nameLen := len([]rune(name))
	if nameLen == 0 {
		return false
	}

	digitRatio := float64(digitCount) / float64(nameLen)
	specialRatio := float64(specialCount) / float64(nameLen)

	return digitRatio > 0.3 || specialRatio > 0.1
}

// containsTechnicalSpecs проверяет наличие технических спецификаций
func containsTechnicalSpecs(name string) bool {
	nameLower := strings.ToLower(name)

	// Единицы измерения
	units := []string{"мм", "см", "м", "л", "кг", "г", "вт", "в", "а", "мл", "км", "т"}
	for _, unit := range units {
		if strings.Contains(nameLower, unit) {
			return true
		}
	}

	// Паттерны размеров (100x50, 3х2.5)
	if strings.Contains(nameLower, "x") || strings.Contains(nameLower, "х") {
		return true
	}

	// Артикулы и коды (ER-000, АРТ.)
	if strings.Contains(nameLower, "арт") || strings.Contains(nameLower, "код") {
		return true
	}

	return false
}
