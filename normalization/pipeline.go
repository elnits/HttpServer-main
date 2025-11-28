package normalization

import (
	"fmt"
	"log"
	"time"

	"httpserver/database"
)

// ProcessingLevel уровень обработки
type ProcessingLevel string

const (
	LevelBasic      ProcessingLevel = "basic"
	LevelAIEnhanced ProcessingLevel = "ai_enhanced"
	LevelBenchmark  ProcessingLevel = "benchmark"
)

// PipelineStats статистика работы пайплайна
type PipelineStats struct {
	TotalProcessed    int            `json:"total_processed"`
	BasicCount        int            `json:"basic_count"`
	AIEnhancedCount   int            `json:"ai_enhanced_count"`
	BenchmarkCount    int            `json:"benchmark_count"`
	AverageQuality    float64        `json:"average_quality"`
	QualityByLevel    map[string]float64 `json:"quality_by_level"`
	ProcessingTime    time.Duration  `json:"processing_time"`
}

// NormalizationPipeline управляет многоуровневой обработкой
type NormalizationPipeline struct {
	db               *database.DB
	normalizer       *Normalizer
	qualityValidator *QualityValidator
	events           chan<- string
	stats            *PipelineStats
}

// NewNormalizationPipeline создает новый пайплайн
func NewNormalizationPipeline(
	db *database.DB,
	normalizer *Normalizer,
	events chan<- string,
) *NormalizationPipeline {
	return &NormalizationPipeline{
		db:               db,
		normalizer:       normalizer,
		qualityValidator: NewQualityValidator(),
		events:           events,
		stats: &PipelineStats{
			QualityByLevel: make(map[string]float64),
		},
	}
}

// ProcessWithQuality выполняет нормализацию с оценкой качества
func (p *NormalizationPipeline) ProcessWithQuality() error {
	startTime := time.Now()
	p.sendEvent("🔄 Запуск многоуровневой нормализации с контролем качества...")
	log.Println("Запуск многоуровневой нормализации с контролем качества...")

	// Сначала выполняем обычную нормализацию
	if err := p.normalizer.ProcessNormalization(); err != nil {
		return fmt.Errorf("normalization failed: %w", err)
	}

	// Теперь оцениваем качество и обновляем уровни
	p.sendEvent("📊 Оценка качества нормализованных данных...")
	log.Println("Оценка качества нормализованных данных...")

	if err := p.evaluateAndUpdateQuality(); err != nil {
		return fmt.Errorf("quality evaluation failed: %w", err)
	}

	p.stats.ProcessingTime = time.Since(startTime)

	// Отправляем финальную статистику
	p.sendFinalStats()

	return nil
}

// evaluateAndUpdateQuality оценивает качество и обновляет уровни обработки
func (p *NormalizationPipeline) evaluateAndUpdateQuality() error {
	// Получаем все нормализованные записи
	items, err := p.db.GetNormalizedItems(0, 0) // Все записи
	if err != nil {
		return fmt.Errorf("failed to get normalized items: %w", err)
	}

	p.sendEvent(fmt.Sprintf("Оценка качества для %d записей...", len(items)))

	totalQuality := 0.0
	levelQuality := make(map[string]float64)
	levelCounts := make(map[string]int)

	batchSize := 1000
	processedCount := 0

	for _, item := range items {
		// Оценка качества
		quality := p.qualityValidator.ValidateQuality(
			item.SourceName,
			item.NormalizedName,
			item.Category,
			item.AIConfidence,
			item.ProcessingLevel,
		)

		// Обновляем уровень, если достигнут benchmark качества
		newLevel := item.ProcessingLevel
		if quality.IsBenchmarkQuality && item.ProcessingLevel != "benchmark" {
			newLevel = "benchmark"

			// Обновляем в БД
			if err := p.db.UpdateProcessingLevel(item.ID, "benchmark", quality.Overall); err != nil {
				log.Printf("Failed to update processing level for item %d: %v", item.ID, err)
			}
		}

		// Собираем статистику
		totalQuality += quality.Overall
		levelQuality[newLevel] += quality.Overall
		levelCounts[newLevel]++

		// Обновляем счетчики по уровням
		switch ProcessingLevel(newLevel) {
		case LevelBasic:
			p.stats.BasicCount++
		case LevelAIEnhanced:
			p.stats.AIEnhancedCount++
		case LevelBenchmark:
			p.stats.BenchmarkCount++
		}

		processedCount++

		// Отправляем событие каждые 1000 записей
		if processedCount%batchSize == 0 {
			progress := float64(processedCount) / float64(len(items)) * 100
			p.sendEvent(fmt.Sprintf("Оценено качества: %d из %d (%.1f%%)", processedCount, len(items), progress))
		}
	}

	p.stats.TotalProcessed = len(items)

	if len(items) > 0 {
		p.stats.AverageQuality = totalQuality / float64(len(items))
	}

	// Рассчитываем среднее качество по уровням
	for level, count := range levelCounts {
		if count > 0 {
			p.stats.QualityByLevel[level] = levelQuality[level] / float64(count)
		}
	}

	return nil
}

// sendEvent отправляет событие в канал
func (p *NormalizationPipeline) sendEvent(message string) {
	if p.events != nil {
		select {
		case p.events <- message:
		default:
			// Канал полон, пропускаем
		}
	}
}

// sendFinalStats отправляет финальную статистику
func (p *NormalizationPipeline) sendFinalStats() {
	stats := fmt.Sprintf(
		"✅ Многоуровневая нормализация завершена за %v\n"+
			"📊 Статистика:\n"+
			"  • Всего обработано: %d записей\n"+
			"  • Базовый уровень: %d (%.1f%%)\n"+
			"  • AI улучшено: %d (%.1f%%)\n"+
			"  • Эталонное качество: %d (%.1f%%)\n"+
			"  • Средняя оценка качества: %.2f",
		p.stats.ProcessingTime,
		p.stats.TotalProcessed,
		p.stats.BasicCount,
		float64(p.stats.BasicCount)/float64(p.stats.TotalProcessed)*100,
		p.stats.AIEnhancedCount,
		float64(p.stats.AIEnhancedCount)/float64(p.stats.TotalProcessed)*100,
		p.stats.BenchmarkCount,
		float64(p.stats.BenchmarkCount)/float64(p.stats.TotalProcessed)*100,
		p.stats.AverageQuality,
	)

	p.sendEvent(stats)
	log.Println(stats)
}

// GetStats возвращает статистику пайплайна
func (p *NormalizationPipeline) GetStats() *PipelineStats {
	return p.stats
}
