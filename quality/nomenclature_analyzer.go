package quality

import (
	"fmt"
	"log"
	"math"
	"time"

	"httpserver/database"
)

// analyzeNomenclature анализирует качество данных номенклатуры
func (qa *QualityAnalyzer) analyzeNomenclature(uploadID int, databaseID int) error {
	log.Printf("Analyzing nomenclature quality for upload %d", uploadID)

	// 1. Анализ полноты данных
	completenessMetric, err := qa.analyzeNomenclatureCompleteness(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing completeness: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&completenessMetric); err != nil {
			log.Printf("Error saving completeness metric: %v", err)
		}
	}

	// 2. Анализ уникальности
	uniquenessMetric, err := qa.analyzeNomenclatureUniqueness(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing uniqueness: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&uniquenessMetric); err != nil {
			log.Printf("Error saving uniqueness metric: %v", err)
		}
	}

	// 3. Анализ валидности
	validityMetric, err := qa.analyzeNomenclatureValidity(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing validity: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&validityMetric); err != nil {
			log.Printf("Error saving validity metric: %v", err)
		}
	}

	// 4. Анализ консистентности
	consistencyMetric, err := qa.analyzeNomenclatureConsistency(uploadID, databaseID)
	if err != nil {
		log.Printf("Error analyzing consistency: %v", err)
	} else {
		if err := qa.db.SaveQualityMetric(&consistencyMetric); err != nil {
			log.Printf("Error saving consistency metric: %v", err)
		}
	}

	return nil
}

// analyzeNomenclatureCompleteness анализирует полноту данных номенклатуры
func (qa *QualityAnalyzer) analyzeNomenclatureCompleteness(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "completeness",
		MetricName:     "nomenclature_completeness",
		MeasuredAt:     time.Now(),
	}

	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN nomenclature_code IS NOT NULL AND nomenclature_code != '' THEN 1 END) as with_code,
			COUNT(CASE WHEN nomenclature_name IS NOT NULL AND nomenclature_name != '' THEN 1 END) as with_name,
			COUNT(CASE WHEN characteristic_name IS NOT NULL AND characteristic_name != '' THEN 1 END) as with_characteristic
		FROM nomenclature_items 
		WHERE upload_id = ?
	`

	var total, withCode, withName, withChar int
	row := qa.db.QueryRow(query, uploadID)
	err := row.Scan(&total, &withCode, &withName, &withChar)
	if err != nil {
		return metric, fmt.Errorf("failed to query completeness: %w", err)
	}

	if total == 0 {
		metric.MetricValue = 0
		metric.Status = "FAIL"
		return metric, nil
	}

	codeCompleteness := float64(withCode) / float64(total) * 100
	nameCompleteness := float64(withName) / float64(total) * 100
	charCompleteness := float64(withChar) / float64(total) * 100

	// Валидация значений
	codeCompleteness = validateMetricValue(codeCompleteness)
	nameCompleteness = validateMetricValue(nameCompleteness)
	charCompleteness = validateMetricValue(charCompleteness)

	metric.MetricValue = (codeCompleteness + nameCompleteness) / 2
	metric.MetricValue = validateMetricValue(metric.MetricValue)

	metric.Details = map[string]interface{}{
		"total_records":          total,
		"code_completeness":      codeCompleteness,
		"name_completeness":      nameCompleteness,
		"characteristic_completeness": charCompleteness,
	}

	log.Printf("Completeness analysis: total=%d, code=%.2f%%, name=%.2f%%, char=%.2f%%, overall=%.2f%%",
		total, codeCompleteness, nameCompleteness, charCompleteness, metric.MetricValue)

	// Определяем статус
	if metric.MetricValue >= 95 {
		metric.Status = "PASS"
	} else if metric.MetricValue >= 80 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	// Выявление проблем
	if codeCompleteness < 95 {
		issue := database.DataQualityIssue{
			UploadID:       uploadID,
			DatabaseID:     databaseID,
			EntityType:     "nomenclature",
			IssueType:      "missing_code",
			IssueSeverity:  "HIGH",
			FieldName:      "code",
			Description:    fmt.Sprintf("Не заполнены коды у %.1f%% записей", 100-codeCompleteness),
			DetectedAt:     time.Now(),
			Status:         "OPEN",
		}
		if err := qa.db.SaveQualityIssue(&issue); err != nil {
			log.Printf("Error saving completeness issue: %v", err)
		}
	}

	return metric, nil
}

// analyzeNomenclatureUniqueness анализирует уникальность номенклатуры
func (qa *QualityAnalyzer) analyzeNomenclatureUniqueness(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "uniqueness",
		MetricName:     "nomenclature_uniqueness",
		MeasuredAt:     time.Now(),
	}

	// Поиск дубликатов по коду
	duplicateQuery := `
		SELECT nomenclature_code, COUNT(*) as duplicate_count
		FROM nomenclature_items 
		WHERE upload_id = ? AND nomenclature_code IS NOT NULL AND nomenclature_code != ''
		GROUP BY nomenclature_code 
		HAVING COUNT(*) > 1
	`

	rows, err := qa.db.Query(duplicateQuery, uploadID)
	if err != nil {
		return metric, fmt.Errorf("failed to query duplicates: %w", err)
	}
	defer rows.Close()

	var totalDuplicates int
	duplicateCodes := []string{}

	for rows.Next() {
		var code string
		var count int
		if err := rows.Scan(&code, &count); err != nil {
			continue
		}
		totalDuplicates += count
		duplicateCodes = append(duplicateCodes, code)
	}

	// Общее количество записей
	var totalRecords int
	row := qa.db.QueryRow("SELECT COUNT(*) FROM nomenclature_items WHERE upload_id = ?", uploadID)
	err = row.Scan(&totalRecords)
	if err != nil {
		return metric, fmt.Errorf("failed to get total records: %w", err)
	}

	uniquenessScore := 100.0
	if totalRecords > 0 {
		uniquenessScore = float64(totalRecords-totalDuplicates) / float64(totalRecords) * 100
	}

	uniquenessScore = validateMetricValue(uniquenessScore)
	metric.MetricValue = uniquenessScore
	metric.Details = map[string]interface{}{
		"total_records":    totalRecords,
		"duplicate_records": totalDuplicates,
		"duplicate_codes":  duplicateCodes,
	}

	log.Printf("Uniqueness analysis: total=%d, duplicates=%d, uniqueness=%.2f%%",
		totalRecords, totalDuplicates, uniquenessScore)

	// Определяем статус
	if uniquenessScore >= 99 {
		metric.Status = "PASS"
	} else if uniquenessScore >= 95 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	// Сохранение проблем дубликатов
	for _, code := range duplicateCodes {
		issue := database.DataQualityIssue{
			UploadID:       uploadID,
			DatabaseID:     databaseID,
			EntityType:     "nomenclature",
			IssueType:      "duplicate_code",
			IssueSeverity:  "MEDIUM",
			FieldName:      "code",
			ActualValue:    code,
			Description:    fmt.Sprintf("Дубликат кода: %s", code),
			DetectedAt:     time.Now(),
			Status:         "OPEN",
		}
		if err := qa.db.SaveQualityIssue(&issue); err != nil {
			log.Printf("Error saving duplicate issue: %v", err)
		}
	}

	return metric, nil
}

// analyzeNomenclatureValidity анализирует валидность данных номенклатуры
func (qa *QualityAnalyzer) analyzeNomenclatureValidity(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "validity",
		MetricName:     "nomenclature_validity",
		MeasuredAt:     time.Now(),
	}

	// Проверяем наличие пустых кодов и наименований
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN (nomenclature_code IS NULL OR nomenclature_code = '') THEN 1 END) as invalid_code,
			COUNT(CASE WHEN (nomenclature_name IS NULL OR nomenclature_name = '') THEN 1 END) as invalid_name
		FROM nomenclature_items
		WHERE upload_id = ?
	`

	var total, invalidCode, invalidName int
	row := qa.db.QueryRow(query, uploadID)
	err := row.Scan(&total, &invalidCode, &invalidName)
	if err != nil {
		return metric, fmt.Errorf("failed to query validity: %w", err)
	}

	if total == 0 {
		metric.MetricValue = 0
		metric.Status = "FAIL"
		return metric, nil
	}

	validityScore := float64(total-invalidCode-invalidName) / float64(total) * 100
	validityScore = validateMetricValue(validityScore)

	metric.MetricValue = validityScore
	metric.Details = map[string]interface{}{
		"total_records": total,
		"invalid_codes": invalidCode,
		"invalid_names": invalidName,
	}

	log.Printf("Validity analysis: total=%d, invalid_codes=%d, invalid_names=%d, validity=%.2f%%",
		total, invalidCode, invalidName, validityScore)

	// Определяем статус
	if validityScore >= 95 {
		metric.Status = "PASS"
	} else if validityScore >= 80 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	return metric, nil
}

// analyzeNomenclatureConsistency анализирует консистентность данных номенклатуры
func (qa *QualityAnalyzer) analyzeNomenclatureConsistency(uploadID int, databaseID int) (database.DataQualityMetric, error) {
	metric := database.DataQualityMetric{
		UploadID:       uploadID,
		DatabaseID:     databaseID,
		MetricCategory: "consistency",
		MetricName:     "nomenclature_consistency",
		MeasuredAt:     time.Now(),
	}

	// Проверяем согласованность данных (например, наличие характеристики при наличии ссылки на характеристику)
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN (characteristic_reference IS NOT NULL AND characteristic_reference != '') 
				AND (characteristic_name IS NULL OR characteristic_name = '') THEN 1 END) as inconsistent
		FROM nomenclature_items
		WHERE upload_id = ?
	`

	var total, inconsistent int
	row := qa.db.QueryRow(query, uploadID)
	err := row.Scan(&total, &inconsistent)
	if err != nil {
		return metric, fmt.Errorf("failed to query consistency: %w", err)
	}

	if total == 0 {
		metric.MetricValue = 100
		metric.Status = "PASS"
		return metric, nil
	}

	consistencyScore := float64(total-inconsistent) / float64(total) * 100
	consistencyScore = validateMetricValue(consistencyScore)

	metric.MetricValue = consistencyScore
	metric.Details = map[string]interface{}{
		"total_records": total,
		"inconsistent_records": inconsistent,
	}

	log.Printf("Consistency analysis: total=%d, inconsistent=%d, consistency=%.2f%%",
		total, inconsistent, consistencyScore)

	// Определяем статус
	if consistencyScore >= 95 {
		metric.Status = "PASS"
	} else if consistencyScore >= 80 {
		metric.Status = "WARNING"
	} else {
		metric.Status = "FAIL"
	}

	return metric, nil
}

// validateMetricValue проверяет и исправляет значение метрики
func validateMetricValue(value float64) float64 {
	// Проверка на NaN
	if math.IsNaN(value) {
		log.Printf("Warning: NaN value detected in metric, setting to 0.0")
		return 0.0
	}

	// Проверка на Infinity
	if math.IsInf(value, 0) {
		log.Printf("Warning: Infinity value detected in metric, setting to 0.0")
		return 0.0
	}

	// Ограничиваем значение диапазоном 0-100
	if value < 0.0 {
		log.Printf("Warning: Negative metric value %.2f, setting to 0.0", value)
		return 0.0
	}
	if value > 100.0 {
		log.Printf("Warning: Metric value %.2f exceeds 100, setting to 100.0", value)
		return 100.0
	}

	return value
}

