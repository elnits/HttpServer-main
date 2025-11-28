package normalization

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"
	"sync"

	"httpserver/database"
)

// HeapItem представляет элемент в heap (для топ-N)
type HeapItem struct {
	Key      string      // Уникальный ключ элемента
	Count    int         // Количество/частота
	Score    float64     // Оценка/вес
	Metadata interface{} // Дополнительные данные
}

// MinHeap реализует min-heap для эффективного хранения топ-N элементов
// Аналог heapq из Python
type MinHeap struct {
	items   []HeapItem
	maxSize int
	mu      sync.RWMutex
}

// Реализация heap.Interface
func (h *MinHeap) Len() int { return len(h.items) }

func (h *MinHeap) Less(i, j int) bool {
	// Сравниваем сначала по Count, затем по Score
	if h.items[i].Count == h.items[j].Count {
		return h.items[i].Score < h.items[j].Score
	}
	return h.items[i].Count < h.items[j].Count
}

func (h *MinHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *MinHeap) Push(x interface{}) {
	h.items = append(h.items, x.(HeapItem))
}

func (h *MinHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[0 : n-1]
	return item
}

// NewMinHeap создает новый MinHeap с заданным максимальным размером
func NewMinHeap(maxSize int) *MinHeap {
	h := &MinHeap{
		items:   make([]HeapItem, 0, maxSize),
		maxSize: maxSize,
	}
	heap.Init(h)
	return h
}

// Add добавляет элемент в heap
// Если размер превышает maxSize, удаляет минимальный элемент
func (h *MinHeap) Add(key string, count int, score float64, metadata interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	item := HeapItem{
		Key:      key,
		Count:    count,
		Score:    score,
		Metadata: metadata,
	}

	if len(h.items) < h.maxSize {
		heap.Push(h, item)
	} else {
		// Если новый элемент больше минимального, заменяем
		if h.items[0].Count < item.Count || (h.items[0].Count == item.Count && h.items[0].Score < item.Score) {
			heap.Pop(h)
			heap.Push(h, item)
		}
	}
}

// GetTopN возвращает топ-N элементов, отсортированных по убыванию
func (h *MinHeap) GetTopN() []HeapItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Копируем и сортируем по убыванию
	sorted := make([]HeapItem, len(h.items))
	copy(sorted, h.items)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count == sorted[j].Count {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].Count > sorted[j].Count
	})

	return sorted
}

// Clear очищает heap
func (h *MinHeap) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = h.items[:0]
	heap.Init(h)
}

// CategoryAggregate статистика по категории
type CategoryAggregate struct {
	Category        string
	TotalItems      int
	AvgConfidence   float64
	TotalConfidence float64
	MinConfidence   float64
	MaxConfidence   float64
	PatternCounts   map[PatternType]int
	TopAttributes   []AttributeFrequency
}

// AttributeFrequency частота встречаемости атрибута
type AttributeFrequency struct {
	AttributeName  string
	AttributeValue string
	Count          int
	Percentage     float64
}

// PatternStatistics статистика по паттернам
type PatternStatistics struct {
	// Распределение по длине токенов (аналог token_stats из Python)
	TokenLengthDistribution map[int]int

	// Топ-N паттернов
	TopPatterns *MinHeap

	// Количество по типам паттернов
	PatternTypeCount map[PatternType]int

	// Статистика по категориям
	CategoryStats map[string]*CategoryAggregate

	// Общая статистика
	TotalItems         int
	TotalPatterns      int
	AvgPatternsPerItem float64
	AvgTokenLength     float64
}

// PatternAnalyzer анализатор паттернов с накоплением статистики
type PatternAnalyzer struct {
	stats *PatternStatistics
	mu    sync.RWMutex
}

// NewPatternAnalyzer создает новый анализатор паттернов
func NewPatternAnalyzer(topN int) *PatternAnalyzer {
	return &PatternAnalyzer{
		stats: &PatternStatistics{
			TokenLengthDistribution: make(map[int]int),
			TopPatterns:             NewMinHeap(topN),
			PatternTypeCount:        make(map[PatternType]int),
			CategoryStats:           make(map[string]*CategoryAggregate),
		},
	}
}

// AnalyzeItem анализирует один элемент и обновляет статистику
func (pa *PatternAnalyzer) AnalyzeItem(name string, patterns []PatternMatch, category string, confidence float64) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.stats.TotalItems++

	// Токенизация для подсчета длины
	tokenizer := NewContextualTokenizer()
	structInfo := tokenizer.AnalyzeStructure(name)
	tokenCount := structInfo.TextTokens + structInfo.NumberTokens

	// Обновляем распределение по длине токенов
	pa.stats.TokenLengthDistribution[tokenCount]++

	// Обновляем счетчики паттернов
	pa.stats.TotalPatterns += len(patterns)

	for _, pattern := range patterns {
		pa.stats.PatternTypeCount[pattern.Type]++

		// Добавляем в топ-N
		key := fmt.Sprintf("%s:%s", pattern.Type, pattern.MatchedText)
		pa.stats.TopPatterns.Add(key, 1, pattern.Confidence, pattern)
	}

	// Обновляем статистику по категориям
	pa.updateCategoryStats(category, confidence, patterns)
}

// updateCategoryStats обновляет статистику по категории (аналог defaultdict из Python)
func (pa *PatternAnalyzer) updateCategoryStats(category string, confidence float64, patterns []PatternMatch) {
	// Автоматически создаем, если не существует (defaultdict pattern)
	if pa.stats.CategoryStats[category] == nil {
		pa.stats.CategoryStats[category] = &CategoryAggregate{
			Category:      category,
			PatternCounts: make(map[PatternType]int),
			MinConfidence: confidence,
			MaxConfidence: confidence,
		}
	}

	agg := pa.stats.CategoryStats[category]
	agg.TotalItems++
	agg.TotalConfidence += confidence
	agg.AvgConfidence = agg.TotalConfidence / float64(agg.TotalItems)

	// Обновляем min/max confidence
	if confidence < agg.MinConfidence {
		agg.MinConfidence = confidence
	}
	if confidence > agg.MaxConfidence {
		agg.MaxConfidence = confidence
	}

	// Подсчет паттернов по типам
	for _, pattern := range patterns {
		agg.PatternCounts[pattern.Type]++
	}
}

// GetPercentageDistribution возвращает процентное распределение типов паттернов
func (pa *PatternAnalyzer) GetPercentageDistribution() map[PatternType]float64 {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	total := 0
	for _, count := range pa.stats.PatternTypeCount {
		total += count
	}

	if total == 0 {
		return make(map[PatternType]float64)
	}

	percentages := make(map[PatternType]float64)
	for patternType, count := range pa.stats.PatternTypeCount {
		percentages[patternType] = float64(count) / float64(total) * 100.0
	}

	return percentages
}

// GetTopNPatterns возвращает топ-N паттернов
func (pa *PatternAnalyzer) GetTopNPatterns(n int) []HeapItem {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	allTop := pa.stats.TopPatterns.GetTopN()
	if n > len(allTop) {
		n = len(allTop)
	}
	return allTop[:n]
}

// GetStatistics возвращает полную статистику
func (pa *PatternAnalyzer) GetStatistics() *PatternStatistics {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	// Вычисляем средние значения
	if pa.stats.TotalItems > 0 {
		pa.stats.AvgPatternsPerItem = float64(pa.stats.TotalPatterns) / float64(pa.stats.TotalItems)

		totalTokens := 0
		for length, count := range pa.stats.TokenLengthDistribution {
			totalTokens += length * count
		}
		pa.stats.AvgTokenLength = float64(totalTokens) / float64(pa.stats.TotalItems)
	}

	return pa.stats
}

// Reset сбрасывает всю статистику
func (pa *PatternAnalyzer) Reset() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.stats = &PatternStatistics{
		TokenLengthDistribution: make(map[int]int),
		TopPatterns:             NewMinHeap(pa.stats.TopPatterns.maxSize),
		PatternTypeCount:        make(map[PatternType]int),
		CategoryStats:           make(map[string]*CategoryAggregate),
	}
}

// AnalyzeBatch анализирует батч элементов
func (pa *PatternAnalyzer) AnalyzeBatch(items []*database.CatalogItem, detector *PatternDetector) {
	for _, item := range items {
		patterns := detector.DetectPatterns(item.Name)
		// CatalogItem не имеет поля Category, используем пустую строку или CatalogName
		pa.AnalyzeItem(item.Name, patterns, item.CatalogName, 1.0)
	}
}

// GetCategoryReport генерирует отчет по категории
func (pa *PatternAnalyzer) GetCategoryReport(category string) *CategoryAggregate {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	if agg, exists := pa.stats.CategoryStats[category]; exists {
		return agg
	}
	return nil
}

// GetAllCategoriesReport генерирует отчет по всем категориям
func (pa *PatternAnalyzer) GetAllCategoriesReport() []*CategoryAggregate {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	reports := make([]*CategoryAggregate, 0, len(pa.stats.CategoryStats))
	for _, agg := range pa.stats.CategoryStats {
		reports = append(reports, agg)
	}

	// Сортируем по количеству элементов
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].TotalItems > reports[j].TotalItems
	})

	return reports
}

// FormatReport генерирует текстовый отчет
func (pa *PatternAnalyzer) FormatReport() string {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	var report strings.Builder

	report.WriteString("=== СТАТИСТИКА ПАТТЕРНОВ ===\n\n")

	report.WriteString(fmt.Sprintf("Всего элементов: %d\n", pa.stats.TotalItems))
	report.WriteString(fmt.Sprintf("Всего паттернов: %d\n", pa.stats.TotalPatterns))
	report.WriteString(fmt.Sprintf("Среднее паттернов на элемент: %.2f\n", pa.stats.AvgPatternsPerItem))
	report.WriteString(fmt.Sprintf("Средняя длина токенов: %.2f\n\n", pa.stats.AvgTokenLength))

	// Распределение по длине токенов
	report.WriteString("Распределение по длине токенов:\n")
	var tokenLengths []int
	for length := range pa.stats.TokenLengthDistribution {
		tokenLengths = append(tokenLengths, length)
	}
	sort.Ints(tokenLengths)
	for _, length := range tokenLengths {
		count := pa.stats.TokenLengthDistribution[length]
		percentage := float64(count) / float64(pa.stats.TotalItems) * 100.0
		report.WriteString(fmt.Sprintf("  %d токенов: %d элементов (%.1f%%)\n", length, count, percentage))
	}
	report.WriteString("\n")

	// Процентное распределение типов паттернов
	report.WriteString("Распределение типов паттернов:\n")
	percentages := pa.GetPercentageDistribution()

	// Сортируем по проценту
	type typePerc struct {
		Type       PatternType
		Percentage float64
	}
	var typePercs []typePerc
	for pt, perc := range percentages {
		typePercs = append(typePercs, typePerc{pt, perc})
	}
	sort.Slice(typePercs, func(i, j int) bool {
		return typePercs[i].Percentage > typePercs[j].Percentage
	})

	for _, tp := range typePercs {
		count := pa.stats.PatternTypeCount[tp.Type]
		report.WriteString(fmt.Sprintf("  %s: %d (%.1f%%)\n", tp.Type, count, tp.Percentage))
	}
	report.WriteString("\n")

	// Топ-10 паттернов
	report.WriteString("Топ-10 паттернов:\n")
	topPatterns := pa.GetTopNPatterns(10)
	for i, item := range topPatterns {
		report.WriteString(fmt.Sprintf("  %d. %s (count: %d, score: %.2f)\n",
			i+1, item.Key, item.Count, item.Score))
	}
	report.WriteString("\n")

	// Статистика по категориям
	report.WriteString("Статистика по категориям:\n")
	categoryReports := pa.GetAllCategoriesReport()
	for i, catRep := range categoryReports {
		if i >= 10 { // Показываем только топ-10 категорий
			break
		}
		report.WriteString(fmt.Sprintf("  %d. %s: %d элементов (avg confidence: %.2f)\n",
			i+1, catRep.Category, catRep.TotalItems, catRep.AvgConfidence))
	}

	return report.String()
}

// Counter простой счетчик (аналог collections.Counter из Python)
type Counter struct {
	counts map[string]int
	mu     sync.RWMutex
}

// NewCounter создает новый счетчик
func NewCounter() *Counter {
	return &Counter{
		counts: make(map[string]int),
	}
}

// Increment увеличивает счетчик для ключа
func (c *Counter) Increment(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[key]++
}

// Add добавляет значение к счетчику
func (c *Counter) Add(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[key] += value
}

// Get возвращает значение счетчика
func (c *Counter) Get(key string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.counts[key]
}

// MostCommon возвращает N самых частых элементов
func (c *Counter) MostCommon(n int) []struct {
	Key   string
	Count int
} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type kv struct {
		Key   string
		Count int
	}

	var items []kv
	for k, v := range c.counts {
		items = append(items, kv{k, v})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})

	if n > len(items) {
		n = len(items)
	}

	result := make([]struct {
		Key   string
		Count int
	}, n)

	for i := 0; i < n; i++ {
		result[i] = struct {
			Key   string
			Count int
		}{items[i].Key, items[i].Count}
	}

	return result
}

// Total возвращает общую сумму всех значений
func (c *Counter) Total() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := 0
	for _, count := range c.counts {
		total += count
	}
	return total
}

// Clear очищает счетчик
func (c *Counter) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts = make(map[string]int)
}
