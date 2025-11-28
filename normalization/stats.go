package normalization

import (
	"sync"
	"time"
)

// PerformanceMetrics содержит метрики производительности
type PerformanceMetrics struct {
	// AI метрики
	TotalAIRequests     int64         `json:"total_ai_requests"`
	SuccessfulAIRequest int64         `json:"successful_ai_requests"`
	FailedAIRequests    int64         `json:"failed_ai_requests"`
	AverageAILatency    time.Duration `json:"average_ai_latency_ms"`
	TotalAILatency      time.Duration `json:"total_ai_latency"`

	// Кеш метрики
	CacheHits        int64   `json:"cache_hits"`
	CacheMisses      int64   `json:"cache_misses"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
	CacheSize        int     `json:"cache_size"`
	CacheMemoryUsage int64   `json:"cache_memory_usage_bytes"`

	// Батч метрики
	TotalBatches         int64   `json:"total_batches"`
	TotalBatchedItems    int64   `json:"total_batched_items"`
	AverageItemsPerBatch float64 `json:"average_items_per_batch"`

	// Качество нормализации
	TotalNormalized      int64   `json:"total_normalized"`
	BasicNormalized      int64   `json:"basic_normalized"`
	AIEnhanced           int64   `json:"ai_enhanced"`
	BenchmarkQuality     int64   `json:"benchmark_quality"`
	AverageQualityScore  float64 `json:"average_quality_score"`
	TotalQualityScore    float64 `json:"total_quality_score"`

	// Временные метрики
	StartTime           time.Time     `json:"start_time"`
	TotalProcessingTime time.Duration `json:"total_processing_time_ms"`
	AverageItemTime     time.Duration `json:"average_item_time_ms"`

	// Ошибки
	TotalErrors      int64            `json:"total_errors"`
	ErrorsByType     map[string]int64 `json:"errors_by_type"`
	LastError        string           `json:"last_error"`
	LastErrorTime    time.Time        `json:"last_error_time"`
}

// StatsCollector собирает и хранит статистику
type StatsCollector struct {
	metrics PerformanceMetrics
	mu      sync.RWMutex
}

// NewStatsCollector создает новый сборщик статистики
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		metrics: PerformanceMetrics{
			StartTime:    time.Now(),
			ErrorsByType: make(map[string]int64),
		},
	}
}

// RecordAIRequest записывает метрики AI запроса
func (sc *StatsCollector) RecordAIRequest(duration time.Duration, success bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.TotalAIRequests++
	if success {
		sc.metrics.SuccessfulAIRequest++
	} else {
		sc.metrics.FailedAIRequests++
	}

	sc.metrics.TotalAILatency += duration
	if sc.metrics.TotalAIRequests > 0 {
		sc.metrics.AverageAILatency = sc.metrics.TotalAILatency / time.Duration(sc.metrics.TotalAIRequests)
	}
}

// RecordCacheAccess записывает обращение к кешу
func (sc *StatsCollector) RecordCacheAccess(hit bool, cacheSize int, memoryUsage int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if hit {
		sc.metrics.CacheHits++
	} else {
		sc.metrics.CacheMisses++
	}

	total := sc.metrics.CacheHits + sc.metrics.CacheMisses
	if total > 0 {
		sc.metrics.CacheHitRate = float64(sc.metrics.CacheHits) / float64(total)
	}

	sc.metrics.CacheSize = cacheSize
	sc.metrics.CacheMemoryUsage = memoryUsage
}

// RecordBatch записывает метрики батча
func (sc *StatsCollector) RecordBatch(itemsCount int) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.TotalBatches++
	sc.metrics.TotalBatchedItems += int64(itemsCount)

	if sc.metrics.TotalBatches > 0 {
		sc.metrics.AverageItemsPerBatch = float64(sc.metrics.TotalBatchedItems) / float64(sc.metrics.TotalBatches)
	}
}

// RecordNormalization записывает результат нормализации
func (sc *StatsCollector) RecordNormalization(processingLevel string, qualityScore float64, duration time.Duration) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.TotalNormalized++
	sc.metrics.TotalQualityScore += qualityScore
	sc.metrics.TotalProcessingTime += duration

	switch processingLevel {
	case "basic":
		sc.metrics.BasicNormalized++
	case "ai_enhanced":
		sc.metrics.AIEnhanced++
	case "benchmark":
		sc.metrics.BenchmarkQuality++
	}

	if sc.metrics.TotalNormalized > 0 {
		sc.metrics.AverageQualityScore = sc.metrics.TotalQualityScore / float64(sc.metrics.TotalNormalized)
		sc.metrics.AverageItemTime = sc.metrics.TotalProcessingTime / time.Duration(sc.metrics.TotalNormalized)
	}
}

// RecordError записывает информацию об ошибке
func (sc *StatsCollector) RecordError(errorType string, errorMsg string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.TotalErrors++
	sc.metrics.ErrorsByType[errorType]++
	sc.metrics.LastError = errorMsg
	sc.metrics.LastErrorTime = time.Now()
}

// GetMetrics возвращает текущие метрики
func (sc *StatsCollector) GetMetrics() PerformanceMetrics {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Создаем копию для безопасного возврата
	metrics := sc.metrics
	metrics.ErrorsByType = make(map[string]int64)
	for k, v := range sc.metrics.ErrorsByType {
		metrics.ErrorsByType[k] = v
	}

	return metrics
}

// Reset сбрасывает все метрики
func (sc *StatsCollector) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics = PerformanceMetrics{
		StartTime:    time.Now(),
		ErrorsByType: make(map[string]int64),
	}
}

// GetSummary возвращает краткую сводку метрик
func (sc *StatsCollector) GetSummary() map[string]interface{} {
	metrics := sc.GetMetrics()

	uptime := time.Since(metrics.StartTime)
	throughput := 0.0
	if uptime.Seconds() > 0 {
		throughput = float64(metrics.TotalNormalized) / uptime.Seconds()
	}

	summary := map[string]interface{}{
		"uptime_seconds": uptime.Seconds(),
		"throughput_items_per_second": throughput,

		"ai": map[string]interface{}{
			"total_requests":     metrics.TotalAIRequests,
			"successful":         metrics.SuccessfulAIRequest,
			"failed":             metrics.FailedAIRequests,
			"success_rate":       float64(metrics.SuccessfulAIRequest) / float64(metrics.TotalAIRequests),
			"average_latency_ms": metrics.AverageAILatency.Milliseconds(),
		},

		"cache": map[string]interface{}{
			"hits":              metrics.CacheHits,
			"misses":            metrics.CacheMisses,
			"hit_rate":          metrics.CacheHitRate,
			"size":              metrics.CacheSize,
			"memory_usage_kb":   metrics.CacheMemoryUsage / 1024,
		},

		"batch": map[string]interface{}{
			"total_batches":          metrics.TotalBatches,
			"total_items":            metrics.TotalBatchedItems,
			"average_items_per_batch": metrics.AverageItemsPerBatch,
		},

		"quality": map[string]interface{}{
			"total_normalized":      metrics.TotalNormalized,
			"basic":                 metrics.BasicNormalized,
			"ai_enhanced":           metrics.AIEnhanced,
			"benchmark":             metrics.BenchmarkQuality,
			"average_quality_score": metrics.AverageQualityScore,
			"average_item_time_ms":  metrics.AverageItemTime.Milliseconds(),
		},

		"errors": map[string]interface{}{
			"total":        metrics.TotalErrors,
			"by_type":      metrics.ErrorsByType,
			"last_error":   metrics.LastError,
			"last_error_at": metrics.LastErrorTime,
		},
	}

	return summary
}

// GetDetailedReport возвращает детальный отчет
func (sc *StatsCollector) GetDetailedReport() map[string]interface{} {
	metrics := sc.GetMetrics()

	report := map[string]interface{}{
		"overview": map[string]interface{}{
			"start_time": metrics.StartTime,
			"uptime":     time.Since(metrics.StartTime).String(),
			"status":     "running",
		},

		"performance": map[string]interface{}{
			"total_items_processed":  metrics.TotalNormalized,
			"total_processing_time":  metrics.TotalProcessingTime.String(),
			"average_time_per_item":  metrics.AverageItemTime.String(),
			"items_per_second":       float64(metrics.TotalNormalized) / time.Since(metrics.StartTime).Seconds(),
		},

		"ai_stats": map[string]interface{}{
			"total_requests":      metrics.TotalAIRequests,
			"successful_requests": metrics.SuccessfulAIRequest,
			"failed_requests":     metrics.FailedAIRequests,
			"success_rate_pct":    (float64(metrics.SuccessfulAIRequest) / float64(metrics.TotalAIRequests)) * 100,
			"average_latency":     metrics.AverageAILatency.String(),
			"total_latency":       metrics.TotalAILatency.String(),
		},

		"cache_stats": map[string]interface{}{
			"hits":               metrics.CacheHits,
			"misses":             metrics.CacheMisses,
			"hit_rate_pct":       metrics.CacheHitRate * 100,
			"entries":            metrics.CacheSize,
			"memory_usage_mb":    float64(metrics.CacheMemoryUsage) / (1024 * 1024),
			"memory_usage_bytes": metrics.CacheMemoryUsage,
		},

		"batch_stats": map[string]interface{}{
			"total_batches":           metrics.TotalBatches,
			"total_items_batched":     metrics.TotalBatchedItems,
			"average_items_per_batch": metrics.AverageItemsPerBatch,
			"batch_efficiency_pct":    (metrics.AverageItemsPerBatch / 10.0) * 100, // Assuming max batch size is 10
		},

		"quality_distribution": map[string]interface{}{
			"total":                 metrics.TotalNormalized,
			"basic_normalized":      metrics.BasicNormalized,
			"ai_enhanced":           metrics.AIEnhanced,
			"benchmark_quality":     metrics.BenchmarkQuality,
			"basic_pct":             (float64(metrics.BasicNormalized) / float64(metrics.TotalNormalized)) * 100,
			"ai_enhanced_pct":       (float64(metrics.AIEnhanced) / float64(metrics.TotalNormalized)) * 100,
			"benchmark_pct":         (float64(metrics.BenchmarkQuality) / float64(metrics.TotalNormalized)) * 100,
			"average_quality_score": metrics.AverageQualityScore,
		},

		"error_stats": map[string]interface{}{
			"total_errors":    metrics.TotalErrors,
			"errors_by_type":  metrics.ErrorsByType,
			"last_error":      metrics.LastError,
			"last_error_time": metrics.LastErrorTime,
			"error_rate_pct":  (float64(metrics.TotalErrors) / float64(metrics.TotalNormalized)) * 100,
		},
	}

	return report
}

// TimeSeries представляет временной ряд метрик
type TimeSeries struct {
	Timestamp time.Time              `json:"timestamp"`
	Values    map[string]interface{} `json:"values"`
}

// TimeSeriesCollector собирает метрики в виде временных рядов
type TimeSeriesCollector struct {
	series   []TimeSeries
	maxSize  int
	interval time.Duration
	mu       sync.RWMutex
	stats    *StatsCollector
}

// NewTimeSeriesCollector создает новый сборщик временных рядов
func NewTimeSeriesCollector(stats *StatsCollector, interval time.Duration, maxSize int) *TimeSeriesCollector {
	tsc := &TimeSeriesCollector{
		series:   make([]TimeSeries, 0, maxSize),
		maxSize:  maxSize,
		interval: interval,
		stats:    stats,
	}

	go tsc.collect()

	return tsc
}

// collect периодически собирает метрики
func (tsc *TimeSeriesCollector) collect() {
	ticker := time.NewTicker(tsc.interval)
	defer ticker.Stop()

	for range ticker.C {
		tsc.snapshot()
	}
}

// snapshot создает снимок текущих метрик
func (tsc *TimeSeriesCollector) snapshot() {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	metrics := tsc.stats.GetMetrics()

	ts := TimeSeries{
		Timestamp: time.Now(),
		Values: map[string]interface{}{
			"total_normalized":      metrics.TotalNormalized,
			"ai_requests":           metrics.TotalAIRequests,
			"cache_hit_rate":        metrics.CacheHitRate,
			"average_quality_score": metrics.AverageQualityScore,
			"average_ai_latency_ms": metrics.AverageAILatency.Milliseconds(),
		},
	}

	tsc.series = append(tsc.series, ts)

	// Удаляем старые записи, если превышен лимит
	if len(tsc.series) > tsc.maxSize {
		tsc.series = tsc.series[len(tsc.series)-tsc.maxSize:]
	}
}

// GetSeries возвращает временные ряды
func (tsc *TimeSeriesCollector) GetSeries() []TimeSeries {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	// Возвращаем копию
	series := make([]TimeSeries, len(tsc.series))
	copy(series, tsc.series)

	return series
}
