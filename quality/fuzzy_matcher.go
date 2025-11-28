package quality

import (
	"fmt"
	"log"
	"strings"
	"time"

	"httpserver/database"
)

// DuplicateGroup группа потенциальных дубликатов
type DuplicateGroup struct {
	Items      []DuplicateItem
	Similarity float64
}

// DuplicateItem элемент в группе дубликатов
type DuplicateItem struct {
	Reference string
	Name      string
}

// FuzzyMatcher нечеткий сопоставитель для поиска дубликатов
type FuzzyMatcher struct {
	db                *database.DB
	threshold         float64
	maxComparisons    int // Максимальное количество сравнений на элемент
	batchSize         int // Размер батча для обработки
	prefixLength      int // Длина префикса для предварительной фильтрации
}

// NewFuzzyMatcher создает новый нечеткий сопоставитель
func NewFuzzyMatcher(db *database.DB, threshold float64) *FuzzyMatcher {
	if threshold <= 0 {
		threshold = 0.85 // Порог по умолчанию
	}
	return &FuzzyMatcher{
		db:             db,
		threshold:      threshold,
		maxComparisons: 1000, // Максимум 1000 сравнений на элемент
		batchSize:      500,  // Обрабатываем по 500 элементов за раз
		prefixLength:   3,    // Используем первые 3 символа для фильтрации
	}
}

// FindDuplicateNames находит дубликаты по наименованию для номенклатуры
// Использует оптимизированный алгоритм с батчингом и предварительной фильтрацией
func (fm *FuzzyMatcher) FindDuplicateNames(uploadID int, databaseID int) ([]DuplicateGroup, error) {
	log.Printf("Starting fuzzy duplicate search for upload %d", uploadID)
	startTime := time.Now()

	// Получаем все наименования номенклатуры
	query := `
		SELECT DISTINCT nomenclature_reference, nomenclature_name 
		FROM nomenclature_items 
		WHERE upload_id = ? AND nomenclature_name IS NOT NULL AND nomenclature_name != ''
		ORDER BY nomenclature_name
	`

	rows, err := fm.db.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query nomenclature items: %w", err)
	}
	defer rows.Close()

	items := make([]DuplicateItem, 0)
	for rows.Next() {
		var item DuplicateItem
		if err := rows.Scan(&item.Reference, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}

	totalItems := len(items)
	log.Printf("Found %d items to analyze for duplicates", totalItems)

	if totalItems == 0 {
		return []DuplicateGroup{}, nil
	}

	// Используем оптимизированный поиск с батчингом
	groups := fm.findDuplicatesOptimized(items)

	elapsed := time.Since(startTime)
	log.Printf("Fuzzy duplicate search completed: found %d groups in %v (%.2f items/sec)",
		len(groups), elapsed, float64(totalItems)/elapsed.Seconds())

	// Сохранение найденных проблем
	for _, group := range groups {
		if len(group.Items) > 1 {
			description := fmt.Sprintf("Найдено %d потенциальных дубликатов с схожестью %.1f%%",
				len(group.Items), group.Similarity*100)

			// Сохраняем проблему для первого элемента группы
			issue := database.DataQualityIssue{
				UploadID:        uploadID,
				DatabaseID:      databaseID,
				EntityType:      "nomenclature",
				EntityReference: group.Items[0].Reference,
				IssueType:       "fuzzy_duplicate",
				IssueSeverity:   "MEDIUM",
				FieldName:       "name",
				ActualValue:     group.Items[0].Name,
				Description:     description,
				DetectedAt:      time.Now(),
				Status:          "OPEN",
			}
			if err := fm.db.SaveQualityIssue(&issue); err != nil {
				log.Printf("Error saving fuzzy duplicate issue: %v", err)
			}
		}
	}

	return groups, nil
}

// findDuplicates находит дубликаты в списке элементов (старый метод O(n²))
func (fm *FuzzyMatcher) findDuplicates(items []DuplicateItem) []DuplicateGroup {
	groups := []DuplicateGroup{}
	processed := make(map[string]bool)

	for i, item1 := range items {
		if processed[item1.Reference] {
			continue
		}

		group := DuplicateGroup{
			Items:      []DuplicateItem{item1},
			Similarity: 1.0,
		}

		for j, item2 := range items {
			if i == j || processed[item2.Reference] {
				continue
			}

			similarity := fm.calculateSimilarity(item1.Name, item2.Name)
			if similarity >= fm.threshold {
				group.Items = append(group.Items, item2)
				if similarity < group.Similarity {
					group.Similarity = similarity
				}
				processed[item2.Reference] = true
			}
		}

		if len(group.Items) > 1 {
			groups = append(groups, group)
		}

		processed[item1.Reference] = true
	}

	return groups
}

// findDuplicatesOptimized находит дубликаты с оптимизацией через предварительную фильтрацию
func (fm *FuzzyMatcher) findDuplicatesOptimized(items []DuplicateItem) []DuplicateGroup {
	groups := []DuplicateGroup{}
	processed := make(map[string]bool)

	// Группируем элементы по префиксам для предварительной фильтрации
	prefixMap := make(map[string][]DuplicateItem)
	for _, item := range items {
		prefix := fm.getPrefix(item.Name)
		prefixMap[prefix] = append(prefixMap[prefix], item)
	}

	log.Printf("Grouped %d items into %d prefix groups", len(items), len(prefixMap))

	// Обрабатываем элементы батчами
	totalProcessed := 0
	for _, item1 := range items {
		if processed[item1.Reference] {
			continue
		}

		// Получаем кандидатов для сравнения на основе префикса
		candidates := fm.getCandidates(item1, prefixMap, processed)

		// Ограничиваем количество сравнений
		if len(candidates) > fm.maxComparisons {
			// Берем первые maxComparisons кандидатов
			candidates = candidates[:fm.maxComparisons]
		}

		group := DuplicateGroup{
			Items:      []DuplicateItem{item1},
			Similarity: 1.0,
		}

		comparisons := 0
		for _, candidate := range candidates {
			if processed[candidate.Reference] {
				continue
			}

			similarity := fm.calculateSimilarity(item1.Name, candidate.Name)
			comparisons++

			if similarity >= fm.threshold {
				group.Items = append(group.Items, candidate)
				if similarity < group.Similarity {
					group.Similarity = similarity
				}
				processed[candidate.Reference] = true
			}
		}

		if len(group.Items) > 1 {
			groups = append(groups, group)
		}

		processed[item1.Reference] = true
		totalProcessed++

		// Логируем прогресс каждые 1000 элементов
		if totalProcessed%1000 == 0 {
			log.Printf("Processed %d/%d items, found %d duplicate groups", totalProcessed, len(items), len(groups))
		}
	}

	return groups
}

// getPrefix возвращает префикс строки для предварительной фильтрации
func (fm *FuzzyMatcher) getPrefix(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if len(normalized) < fm.prefixLength {
		return normalized
	}
	return normalized[:fm.prefixLength]
}

// getCandidates возвращает список кандидатов для сравнения на основе префикса
func (fm *FuzzyMatcher) getCandidates(item DuplicateItem, prefixMap map[string][]DuplicateItem, processed map[string]bool) []DuplicateItem {
	prefix := fm.getPrefix(item.Name)
	candidates := make([]DuplicateItem, 0)

	// Добавляем элементы с тем же префиксом
	if items, exists := prefixMap[prefix]; exists {
		for _, candidate := range items {
			if candidate.Reference != item.Reference && !processed[candidate.Reference] {
				candidates = append(candidates, candidate)
			}
		}
	}

	// Также добавляем элементы с похожими префиксами (для учета опечаток в начале)
	// Проверяем префиксы, отличающиеся на 1 символ
	for p, items := range prefixMap {
		if p != prefix && fm.prefixSimilarity(prefix, p) >= 0.5 {
			for _, candidate := range items {
				if candidate.Reference != item.Reference && !processed[candidate.Reference] {
					candidates = append(candidates, candidate)
				}
			}
		}
	}

	return candidates
}

// prefixSimilarity проверяет схожесть префиксов
func (fm *FuzzyMatcher) prefixSimilarity(p1, p2 string) float64 {
	if len(p1) != len(p2) {
		return 0.0
	}
	matches := 0
	for i := 0; i < len(p1) && i < len(p2); i++ {
		if p1[i] == p2[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(p1))
}

// calculateSimilarity рассчитывает схожесть двух строк используя алгоритм Левенштейна
func (fm *FuzzyMatcher) calculateSimilarity(s1, s2 string) float64 {
	// Нормализация строк
	norm1 := strings.ToLower(strings.TrimSpace(s1))
	norm2 := strings.ToLower(strings.TrimSpace(s2))

	if norm1 == norm2 {
		return 1.0
	}

	// Расчет расстояния Левенштейна
	distance := levenshteinDistance(norm1, norm2)
	maxLen := max(len(norm1), len(norm2))

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance рассчитывает расстояние Левенштейна между двумя строками
func levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	column := make([]int, len(r1)+1)

	for y := 1; y <= len(r1); y++ {
		column[y] = y
	}

	for x := 1; x <= len(r2); x++ {
		column[0] = x
		lastDiag := x - 1
		for y := 1; y <= len(r1); y++ {
			oldDiag := column[y]
			cost := 0
			if r1[y-1] != r2[x-1] {
				cost = 1
			}
			column[y] = min3(column[y]+1, column[y-1]+1, lastDiag+cost)
			lastDiag = oldDiag
		}
	}

	return column[len(r1)]
}

// min3 возвращает минимальное из трех чисел
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// max возвращает максимальное из двух чисел
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

