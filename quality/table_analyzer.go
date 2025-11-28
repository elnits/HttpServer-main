package quality

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"httpserver/database"
	"httpserver/normalization"
)

// TableAnalyzer анализатор качества для любой таблицы
type TableAnalyzer struct {
	db *database.DB
}

// NewTableAnalyzer создает новый анализатор таблиц
func NewTableAnalyzer(db *database.DB) *TableAnalyzer {
	return &TableAnalyzer{db: db}
}

// TableItem представляет запись из любой таблицы для анализа
type TableItem struct {
	ID             int
	Code           string
	Name           string
	Category       string
	KpvedCode      string
	KpvedConfidence float64
	ProcessingLevel string
	AIConfidence    float64
	AIReasoning     string
	MergedCount     int
}

// AnalyzeTableForDuplicates анализирует таблицу на дубликаты используя упрощенный word-based подход
func (ta *TableAnalyzer) AnalyzeTableForDuplicates(
	tableName, codeColumn, nameColumn string,
	batchSize int,
	progressCallback func(processed, total int),
) (int, error) {
	log.Printf("Starting simple duplicate analysis for table %s", tableName)

	// Получаем общее количество записей
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s IS NOT NULL AND %s != ''", 
		tableName, nameColumn, nameColumn)
	if err := ta.db.QueryRow(countQuery).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count records: %w", err)
	}

	if total == 0 {
		return 0, nil
	}

	// Используем упрощенный алгоритм поиска дубликатов
	return ta.findDuplicatesSimple(tableName, codeColumn, nameColumn, total, batchSize, progressCallback)
}

// findDuplicatesSimple упрощенный алгоритм поиска дубликатов по словам
func (ta *TableAnalyzer) findDuplicatesSimple(
	tableName, codeColumn, nameColumn string,
	total, batchSize int,
	progressCallback func(processed, total int),
) (int, error) {
	// Читаем все записи порциями и строим индекс слов
	wordToItems := make(map[string]map[int]bool) // слово -> множество ID записей
	itemWords := make(map[int]map[string]bool)   // ID -> множество слов
	items := make(map[int]normalization.DuplicateItem) // ID -> запись

	processed := 0
	offset := 0

	// Читаем все записи и строим индекс
	for offset < total {
		query := fmt.Sprintf(`
			SELECT id, %s as code, %s as name, 
				COALESCE(category, '') as category,
				COALESCE(kpved_code, '') as kpved_code,
				COALESCE(processing_level, 'basic') as processing_level,
				COALESCE(merged_count, 0) as merged_count
			FROM %s
			WHERE %s IS NOT NULL AND %s != ''
			ORDER BY id
			LIMIT ? OFFSET ?
		`, codeColumn, nameColumn, tableName, nameColumn, nameColumn)

		rows, err := ta.db.Query(query, batchSize, offset)
		if err != nil {
			return 0, fmt.Errorf("failed to query records: %w", err)
		}

		for rows.Next() {
			var item normalization.DuplicateItem
			var code, name, category, kpvedCode, processingLevel sql.NullString
			var mergedCount int

			if err := rows.Scan(&item.ID, &code, &name, &category, &kpvedCode, &processingLevel, &mergedCount); err != nil {
				rows.Close()
				return 0, fmt.Errorf("failed to scan record: %w", err)
			}

			item.Code = getStringValue(code)
			item.NormalizedName = getStringValue(name)
			item.Category = getStringValue(category)
			item.ProcessingLevel = getStringValue(processingLevel)
			item.MergedCount = mergedCount
			item.QualityScore = 0.0

			// Разделяем наименование на слова
			words := ta.tokenizeName(item.NormalizedName)
			if len(words) > 0 {
				wordSet := make(map[string]bool)
				for _, word := range words {
					wordSet[word] = true
					// Добавляем в обратный индекс
					if wordToItems[word] == nil {
						wordToItems[word] = make(map[int]bool)
					}
					wordToItems[word][item.ID] = true
				}
				itemWords[item.ID] = wordSet
				items[item.ID] = item
			}

			processed++
		}
		rows.Close()

		offset += batchSize

		// Обновляем прогресс
		if progressCallback != nil {
			progressCallback(processed, total)
		}
	}

	log.Printf("Indexed %d items with words. Starting duplicate detection...", len(items))

	// Находим дубликаты: группируем записи с общими словами
	groups := ta.findDuplicateGroupsByWords(items, itemWords, wordToItems)

	// Сохраняем группы
	savedCount := 0
	for _, group := range groups {
		if err := ta.saveDuplicateGroup(group); err != nil {
			log.Printf("Error saving duplicate group: %v", err)
		} else {
			savedCount++
		}
	}

	log.Printf("Duplicate analysis completed. Found %d groups, saved %d", len(groups), savedCount)
	return savedCount, nil
}

// tokenizeName разделяет наименование на слова
func (ta *TableAnalyzer) tokenizeName(name string) []string {
	if name == "" {
		return []string{}
	}

	// Приводим к lowercase
	name = strings.ToLower(name)

	// Убираем пунктуацию, оставляем только буквы, цифры и пробелы
	reg := regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	name = reg.ReplaceAllString(name, " ")

	// Разделяем по пробелам
	words := strings.Fields(name)

	// Фильтруем: убираем очень короткие слова (меньше 2 символов) и стоп-слова
	filtered := make([]string, 0, len(words))
	stopWords := map[string]bool{
		"и": true, "в": true, "на": true, "для": true, "с": true, "по": true,
		"от": true, "до": true, "из": true, "к": true, "о": true, "об": true,
		"а": true, "но": true, "или": true, "если": true, "что": true,
	}

	for _, word := range words {
		word = strings.TrimSpace(word)
		// Пропускаем очень короткие слова и стоп-слова
		if len(word) >= 2 && !stopWords[word] {
			// Проверяем, что слово содержит хотя бы одну букву
			hasLetter := false
			for _, r := range word {
				if unicode.IsLetter(r) {
					hasLetter = true
					break
				}
			}
			if hasLetter {
				filtered = append(filtered, word)
			}
		}
	}

	return filtered
}

// findDuplicateGroupsByWords находит группы дубликатов по общим словам
func (ta *TableAnalyzer) findDuplicateGroupsByWords(
	items map[int]normalization.DuplicateItem,
	itemWords map[int]map[string]bool,
	wordToItems map[string]map[int]bool,
) []normalization.DuplicateGroup {
	minCommonWords := 2 // Минимум 2 общих слова для считания дубликатами
	var groups []normalization.DuplicateGroup
	processed := make(map[int]bool)
	groupCounter := 0

	// Проходим по всем словам, которые встречаются в нескольких записях
	for _, itemSet := range wordToItems {
		// Пропускаем слова, которые встречаются только в одной записи
		if len(itemSet) < 2 {
			continue
		}

		// Получаем список записей с этим словом
		candidateIDs := make([]int, 0, len(itemSet))
		for id := range itemSet {
			if !processed[id] {
				candidateIDs = append(candidateIDs, id)
			}
		}

		if len(candidateIDs) < 2 {
			continue
		}

		// Группируем записи с достаточным количеством общих слов
		for i := 0; i < len(candidateIDs); i++ {
			if processed[candidateIDs[i]] {
				continue
			}

			words1 := itemWords[candidateIDs[i]]
			if words1 == nil {
				continue
			}

			var groupItemIDs []int
			var groupItems []normalization.DuplicateItem
			groupItemIDs = append(groupItemIDs, candidateIDs[i])
			groupItems = append(groupItems, items[candidateIDs[i]])

			// Ищем другие записи с достаточным количеством общих слов
			for j := i + 1; j < len(candidateIDs); j++ {
				if processed[candidateIDs[j]] {
					continue
				}

				words2 := itemWords[candidateIDs[j]]
				if words2 == nil {
					continue
				}

				// Подсчитываем общие слова
				commonWords := 0
				commonWordsList := make([]string, 0)
				for w := range words1 {
					if words2[w] {
						commonWords++
						commonWordsList = append(commonWordsList, w)
					}
				}

				// Если достаточно общих слов, добавляем в группу
				if commonWords >= minCommonWords {
					groupItemIDs = append(groupItemIDs, candidateIDs[j])
					groupItems = append(groupItems, items[candidateIDs[j]])
				}
			}

			// Если группа содержит минимум 2 элемента, создаем DuplicateGroup
			if len(groupItemIDs) >= 2 {
				// Вычисляем similarity score
				// Берем среднее значение общих слов / максимальное количество слов
				var totalSimilarity float64
				for k := 0; k < len(groupItems); k++ {
					for l := k + 1; l < len(groupItems); l++ {
						wordsK := itemWords[groupItems[k].ID]
						wordsL := itemWords[groupItems[l].ID]
						if wordsK == nil || wordsL == nil {
							continue
						}

						common := 0
						maxWords := len(wordsK)
						if len(wordsL) > maxWords {
							maxWords = len(wordsL)
						}

						for w := range wordsK {
							if wordsL[w] {
								common++
							}
						}

						if maxWords > 0 {
							similarity := float64(common) / float64(maxWords)
							totalSimilarity += similarity
						}
					}
				}

				pairs := len(groupItems) * (len(groupItems) - 1) / 2
				avgSimilarity := float64(0)
				if pairs > 0 {
					avgSimilarity = totalSimilarity / float64(pairs)
				}

				// Формируем список общих слов для reason
				words1 := itemWords[groupItems[0].ID]
				commonWordsList := make([]string, 0)
				for w := range words1 {
					allHave := true
					for m := 1; m < len(groupItems); m++ {
						wordsM := itemWords[groupItems[m].ID]
						if !wordsM[w] {
							allHave = false
							break
						}
					}
					if allHave {
						commonWordsList = append(commonWordsList, w)
					}
				}

				reason := fmt.Sprintf("Общие слова (%d): %s", len(commonWordsList), strings.Join(commonWordsList, ", "))
				if len(commonWordsList) == 0 {
					reason = "Обнаружено сходство по словам"
				}

				// Выбираем master record (запись с наибольшим количеством слов или лучшим качеством)
				masterID := ta.selectMasterRecord(groupItems, itemWords)

				groups = append(groups, normalization.DuplicateGroup{
					GroupID:         fmt.Sprintf("word_%d", groupCounter),
					Type:            normalization.DuplicateTypeWordBased,
					SimilarityScore: avgSimilarity,
					ItemIDs:         groupItemIDs,
					Items:           groupItems,
					SuggestedMaster: masterID,
					Confidence:      avgSimilarity,
					Reason:          reason,
				})

				// Помечаем элементы как обработанные
				for _, id := range groupItemIDs {
					processed[id] = true
				}
				groupCounter++
			}
		}
	}

	return groups
}

// selectMasterRecord выбирает master record из группы дубликатов
func (ta *TableAnalyzer) selectMasterRecord(
	items []normalization.DuplicateItem,
	itemWords map[int]map[string]bool,
) int {
	if len(items) == 0 {
		return 0
	}

	// Выбираем запись с наибольшим количеством слов (наиболее полное наименование)
	bestID := items[0].ID
	bestScore := 0.0

	for _, item := range items {
		words := itemWords[item.ID]
		wordCount := len(words)
		
		// Бонус за наличие кода
		score := float64(wordCount)
		if item.Code != "" {
			score += 0.5
		}
		
		// Бонус за лучшее качество
		score += item.QualityScore

		if score > bestScore {
			bestScore = score
			bestID = item.ID
		}
	}

	return bestID
}

// AnalyzeTableForViolations анализирует таблицу на нарушения правил качества
func (ta *TableAnalyzer) AnalyzeTableForViolations(
	tableName, codeColumn, nameColumn string,
	batchSize int,
	progressCallback func(processed, total int),
) (int, error) {
	log.Printf("Starting violations analysis for table %s", tableName)

	// Получаем общее количество записей
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	if err := ta.db.QueryRow(countQuery).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count records: %w", err)
	}

	if total == 0 {
		return 0, nil
	}

	// Создаем движок правил
	rulesEngine := normalization.NewQualityRulesEngine()

	// Обрабатываем порциями
	processed := 0
	offset := 0
	totalViolations := 0

	for offset < total {
		// Читаем порцию данных
		query := fmt.Sprintf(`
			SELECT id, 
				COALESCE(%s, '') as code, 
				COALESCE(%s, '') as name,
				COALESCE(category, '') as category,
				COALESCE(kpved_code, '') as kpved_code,
				COALESCE(kpved_confidence, 0.0) as kpved_confidence,
				COALESCE(processing_level, 'basic') as processing_level,
				COALESCE(ai_confidence, 0.0) as ai_confidence,
				COALESCE(ai_reasoning, '') as ai_reasoning,
				COALESCE(merged_count, 0) as merged_count
			FROM %s
			ORDER BY id
			LIMIT ? OFFSET ?
		`, codeColumn, nameColumn, tableName)

		rows, err := ta.db.Query(query, batchSize, offset)
		if err != nil {
			return 0, fmt.Errorf("failed to query records: %w", err)
		}

		for rows.Next() {
			var item normalization.ItemData
			var code, name, category, kpvedCode, processingLevel, aiReasoning sql.NullString
			var kpvedConfidence, aiConfidence sql.NullFloat64

			if err := rows.Scan(&item.ID, &code, &name, &category, &kpvedCode, 
				&kpvedConfidence, &processingLevel, &aiConfidence, &aiReasoning, &item.MergedCount); err != nil {
				rows.Close()
				return 0, fmt.Errorf("failed to scan record: %w", err)
			}

			item.Code = getStringValue(code)
			item.NormalizedName = getStringValue(name)
			item.Category = getStringValue(category)
			item.KpvedCode = getStringValue(kpvedCode)
			item.KpvedConfidence = getFloat64Value(kpvedConfidence)
			item.ProcessingLevel = getStringValue(processingLevel)
			item.AIConfidence = getFloat64Value(aiConfidence)
			item.AIReasoning = getStringValue(aiReasoning)

			// Проверяем правила
			violations := rulesEngine.CheckAll(item)

			// Сохраняем нарушения
			for _, violation := range violations {
				if err := ta.saveViolation(item.ID, violation); err != nil {
					log.Printf("Error saving violation: %v", err)
				} else {
					totalViolations++
				}
			}

			processed++
		}
		rows.Close()

		offset += batchSize

		// Обновляем прогресс
		if progressCallback != nil {
			progressCallback(processed, total)
		}
	}

	log.Printf("Violations analysis completed. Found %d violations", totalViolations)
	return totalViolations, nil
}

// AnalyzeTableForSuggestions анализирует таблицу и генерирует предложения
func (ta *TableAnalyzer) AnalyzeTableForSuggestions(
	tableName, codeColumn, nameColumn string,
	batchSize int,
	progressCallback func(processed, total int),
) (int, error) {
	log.Printf("Starting suggestions analysis for table %s", tableName)

	// Получаем общее количество записей
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	if err := ta.db.QueryRow(countQuery).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count records: %w", err)
	}

	if total == 0 {
		return 0, nil
	}

	// Создаем движки
	rulesEngine := normalization.NewQualityRulesEngine()
	suggestionEngine := normalization.NewSuggestionEngine()

	// Обрабатываем порциями
	processed := 0
	offset := 0
	totalSuggestions := 0

	for offset < total {
		// Читаем порцию данных
		query := fmt.Sprintf(`
			SELECT id, 
				COALESCE(%s, '') as code, 
				COALESCE(%s, '') as name,
				COALESCE(category, '') as category,
				COALESCE(kpved_code, '') as kpved_code,
				COALESCE(kpved_confidence, 0.0) as kpved_confidence,
				COALESCE(processing_level, 'basic') as processing_level,
				COALESCE(ai_confidence, 0.0) as ai_confidence,
				COALESCE(ai_reasoning, '') as ai_reasoning,
				COALESCE(merged_count, 0) as merged_count
			FROM %s
			ORDER BY id
			LIMIT ? OFFSET ?
		`, codeColumn, nameColumn, tableName)

		rows, err := ta.db.Query(query, batchSize, offset)
		if err != nil {
			return 0, fmt.Errorf("failed to query records: %w", err)
		}

		for rows.Next() {
			var item normalization.ItemData
			var code, name, category, kpvedCode, processingLevel, aiReasoning sql.NullString
			var kpvedConfidence, aiConfidence sql.NullFloat64

			if err := rows.Scan(&item.ID, &code, &name, &category, &kpvedCode, 
				&kpvedConfidence, &processingLevel, &aiConfidence, &aiReasoning, &item.MergedCount); err != nil {
				rows.Close()
				return 0, fmt.Errorf("failed to scan record: %w", err)
			}

			item.Code = getStringValue(code)
			item.NormalizedName = getStringValue(name)
			item.Category = getStringValue(category)
			item.KpvedCode = getStringValue(kpvedCode)
			item.KpvedConfidence = getFloat64Value(kpvedConfidence)
			item.ProcessingLevel = getStringValue(processingLevel)
			item.AIConfidence = getFloat64Value(aiConfidence)
			item.AIReasoning = getStringValue(aiReasoning)

			// Проверяем правила для получения нарушений
			violations := rulesEngine.CheckAll(item)

			// Генерируем предложения
			suggestions := suggestionEngine.GenerateSuggestions(item, violations)

			// Сохраняем предложения
			for _, suggestion := range suggestions {
				if err := ta.saveSuggestion(item.ID, suggestion); err != nil {
					log.Printf("Error saving suggestion: %v", err)
				} else {
					totalSuggestions++
				}
			}

			processed++
		}
		rows.Close()

		offset += batchSize

		// Обновляем прогресс
		if progressCallback != nil {
			progressCallback(processed, total)
		}
	}

	log.Printf("Suggestions analysis completed. Found %d suggestions", totalSuggestions)
	return totalSuggestions, nil
}

// saveDuplicateGroup сохраняет группу дубликатов в БД
func (ta *TableAnalyzer) saveDuplicateGroup(group normalization.DuplicateGroup) error {
	// Пропускаем группы с менее чем 2 элементами
	if len(group.ItemIDs) < 2 {
		return nil
	}

	// Создаем хэш группы
	itemIDsJSON, _ := json.Marshal(group.ItemIDs)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s-%s", group.Type, string(itemIDsJSON)))))

	// Проверяем, существует ли уже такая группа
	var exists bool
	if err := ta.db.QueryRow("SELECT EXISTS(SELECT 1 FROM duplicate_groups WHERE group_hash = ?)", hash).Scan(&exists); err == nil && exists {
		return nil // Группа уже существует
	}

	// Сохраняем группу
	dbGroup := &database.DuplicateGroup{
		GroupHash:         hash,
		DuplicateType:     string(group.Type),
		SimilarityScore:   group.SimilarityScore,
		ItemIDs:           group.ItemIDs,
		SuggestedMasterID: group.SuggestedMaster,
		Confidence:        group.Confidence,
		Reason:            group.Reason,
		Merged:            false,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	return ta.db.SaveDuplicateGroup(dbGroup)
}

// saveViolation сохраняет нарушение в БД
func (ta *TableAnalyzer) saveViolation(itemID int, violation normalization.Violation) error {
	dbViolation := &database.QualityViolation{
		NormalizedItemID: itemID,
		RuleName:         violation.RuleName,
		Category:         string(violation.Category),
		Severity:         string(violation.Severity),
		Description:      violation.Description,
		Field:            violation.Field,
		CurrentValue:     violation.CurrentValue,
		Recommendation:   violation.Recommendation,
		DetectedAt:       time.Now(),
	}

	return ta.db.SaveQualityViolation(dbViolation)
}

// saveSuggestion сохраняет предложение в БД
func (ta *TableAnalyzer) saveSuggestion(itemID int, suggestion normalization.Suggestion) error {
	dbSuggestion := &database.QualitySuggestion{
		NormalizedItemID: itemID,
		SuggestionType:   string(suggestion.Type),
		Priority:          string(suggestion.Priority),
		Field:             suggestion.Field,
		CurrentValue:      suggestion.CurrentValue,
		SuggestedValue:    suggestion.SuggestedValue,
		Confidence:        suggestion.Confidence,
		Reasoning:          suggestion.Reasoning,
		AutoApplyable:     suggestion.AutoApplyable,
		Applied:           false,
		CreatedAt:         time.Now(),
	}

	return ta.db.SaveQualitySuggestion(dbSuggestion)
}

// Helper functions

func getStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func getFloat64Value(nf sql.NullFloat64) float64 {
	if nf.Valid {
		return nf.Float64
	}
	return 0.0
}

