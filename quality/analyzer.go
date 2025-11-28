package quality

import (
	"fmt"
	"log"

	"httpserver/database"
)

// QualityAnalyzer основной анализатор качества данных
type QualityAnalyzer struct {
	db *database.DB
}

// NewQualityAnalyzer создает новый анализатор качества
func NewQualityAnalyzer(db *database.DB) *QualityAnalyzer {
	return &QualityAnalyzer{db: db}
}

// AnalyzeUpload запускает полный анализ качества для выгрузки
func (qa *QualityAnalyzer) AnalyzeUpload(uploadID int, databaseID int) error {
	log.Printf("Starting quality analysis for upload %d, database %d", uploadID, databaseID)

	// Анализ номенклатуры
	if err := qa.analyzeNomenclature(uploadID, databaseID); err != nil {
		log.Printf("Error analyzing nomenclature: %v", err)
		// Продолжаем анализ других сущностей
	}

	// Анализ контрагентов
	if err := qa.analyzeCounterparties(uploadID, databaseID); err != nil {
		log.Printf("Error analyzing counterparties: %v", err)
		// Продолжаем анализ
	}

	// Поиск нечетких дубликатов по наименованию
	if err := qa.findFuzzyDuplicates(uploadID, databaseID); err != nil {
		log.Printf("Error finding fuzzy duplicates: %v", err)
	}

	// Расчет общего скора
	if err := qa.calculateOverallScore(uploadID, databaseID); err != nil {
		log.Printf("Error calculating overall score: %v", err)
	}

	// Обновление трендов
	if err := qa.db.UpdateQualityTrends(databaseID); err != nil {
		log.Printf("Error updating quality trends: %v", err)
	}

	log.Printf("Quality analysis completed for upload %d", uploadID)
	return nil
}

// findFuzzyDuplicates находит нечеткие дубликаты по наименованию
func (qa *QualityAnalyzer) findFuzzyDuplicates(uploadID int, databaseID int) error {
	fuzzyMatcher := NewFuzzyMatcher(qa.db, 0.85)
	groups, err := fuzzyMatcher.FindDuplicateNames(uploadID, databaseID)
	if err != nil {
		return fmt.Errorf("failed to find fuzzy duplicates: %w", err)
	}

	if len(groups) > 0 {
		log.Printf("Found %d groups of fuzzy duplicates", len(groups))
	}

	return nil
}

// calculateOverallScore рассчитывает общий балл качества для выгрузки
func (qa *QualityAnalyzer) calculateOverallScore(uploadID int, databaseID int) error {
	// Получаем все метрики для выгрузки
	metrics, err := qa.db.GetQualityMetrics(uploadID)
	if err != nil {
		return fmt.Errorf("failed to get quality metrics: %w", err)
	}

	if len(metrics) == 0 {
		return nil
	}

	// Рассчитываем средний балл по всем метрикам
	var totalScore float64
	var count int

	for _, metric := range metrics {
		totalScore += metric.MetricValue
		count++
	}

	if count == 0 {
		return nil
	}

	overallScore := totalScore / float64(count)

	// Обновляем балл в таблице uploads
	if err := qa.db.UpdateUploadQualityScore(uploadID, overallScore); err != nil {
		return fmt.Errorf("failed to update upload quality score: %w", err)
	}

	log.Printf("Overall quality score for upload %d: %.2f", uploadID, overallScore)
	return nil
}

