package normalization

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// DuplicateType тип дубликата
type DuplicateType string

const (
	DuplicateTypeExact     DuplicateType = "exact"      // Точное совпадение
	DuplicateTypeSemantic  DuplicateType = "semantic"   // Семантическое сходство
	DuplicateTypePhonetic  DuplicateType = "phonetic"   // Фонетическое сходство
	DuplicateTypeWordBased DuplicateType = "word_based" // Группировка по общим словам
	DuplicateTypeMixed     DuplicateType = "mixed"      // Смешанный тип
)

// DuplicateGroup группа дубликатов
type DuplicateGroup struct {
	GroupID         string           `json:"group_id"`         // Уникальный ID группы
	Type            DuplicateType    `json:"type"`             // Тип дубликата
	SimilarityScore float64          `json:"similarity_score"` // Оценка сходства (0-1)
	ItemIDs         []int            `json:"item_ids"`         // ID записей в группе
	Items           []DuplicateItem  `json:"items"`            // Полные данные записей
	SuggestedMaster int              `json:"suggested_master"` // Предлагаемый master record ID
	Confidence      float64          `json:"confidence"`       // Уверенность в дубликате
	Reason          string           `json:"reason"`           // Причина определения как дубликат
}

// DuplicateItem запись в группе дубликатов
type DuplicateItem struct {
	ID              int     `json:"id"`
	Code            string  `json:"code"`
	NormalizedName  string  `json:"normalized_name"`
	Category        string  `json:"category"`
	QualityScore    float64 `json:"quality_score"`
	MergedCount     int     `json:"merged_count"`
	ProcessingLevel string  `json:"processing_level"`
}

// DuplicateAnalyzer анализатор дубликатов
type DuplicateAnalyzer struct {
	exactThreshold          float64 // Порог для exact matching
	semanticThreshold       float64 // Порог для semantic matching
	phoneticThreshold       float64 // Порог для phonetic matching
	wordBasedMinCommonWords int     // Минимальное количество общих слов для группировки
	wordBasedUseStopWords   bool    // Использовать ли стоп-слова при группировке
}

// NewDuplicateAnalyzer создает новый анализатор дубликатов
func NewDuplicateAnalyzer() *DuplicateAnalyzer {
	return &DuplicateAnalyzer{
		exactThreshold:          1.0,  // 100% совпадение
		semanticThreshold:       0.85, // 85% similarity
		phoneticThreshold:       0.90, // 90% phonetic similarity
		wordBasedMinCommonWords: 1,    // Минимум 1 общее слово
		wordBasedUseStopWords:   false, // Не использовать стоп-слова по умолчанию
	}
}

// AnalyzeDuplicates находит все дубликаты в списке записей
// Использует несколько методов: exact matching, semantic similarity, phonetic matching, и word-based grouping
// Пример использования:
//   analyzer := normalization.NewDuplicateAnalyzer()
//   groups := analyzer.AnalyzeDuplicates(items)
func (da *DuplicateAnalyzer) AnalyzeDuplicates(items []DuplicateItem) []DuplicateGroup {
	var allGroups []DuplicateGroup

	// 1. Exact duplicates по коду
	codeGroups := da.findExactDuplicatesByCode(items)
	allGroups = append(allGroups, codeGroups...)

	// 2. Exact duplicates по нормализованному имени
	nameGroups := da.findExactDuplicatesByName(items)
	allGroups = append(allGroups, nameGroups...)

	// 3. Semantic duplicates (косинусная близость)
	semanticGroups := da.findSemanticDuplicates(items)
	allGroups = append(allGroups, semanticGroups...)

	// 4. Phonetic duplicates (для опечаток)
	phoneticGroups := da.findPhoneticDuplicates(items)
	allGroups = append(allGroups, phoneticGroups...)

	// 5. Word-based duplicates (по общим словам)
	wordGroups := da.findWordBasedDuplicates(items)
	allGroups = append(allGroups, wordGroups...)

	// 6. Объединяем пересекающиеся группы
	mergedGroups := da.mergeOverlappingGroups(allGroups)

	// 7. Для каждой группы выбираем master record
	for i := range mergedGroups {
		mergedGroups[i].SuggestedMaster = da.selectMasterRecord(mergedGroups[i].Items)
	}

	return mergedGroups
}

// AnalyzeWordBasedDuplicates выполняет только word-based анализ дубликатов
// Полезно для отдельного вызова без других методов анализа
// Группирует элементы по общим словам в нормализованных названиях
// Пример использования:
//   analyzer := normalization.NewDuplicateAnalyzer()
//   analyzer.wordBasedMinCommonWords = 2  // Минимум 2 общих слова
//   analyzer.wordBasedUseStopWords = false  // Исключить стоп-слова
//   groups := analyzer.AnalyzeWordBasedDuplicates(items)
func (da *DuplicateAnalyzer) AnalyzeWordBasedDuplicates(items []DuplicateItem) []DuplicateGroup {
	// Находим word-based дубликаты
	groups := da.findWordBasedDuplicates(items)

	// Для каждой группы выбираем master record
	for i := range groups {
		groups[i].SuggestedMaster = da.selectMasterRecord(groups[i].Items)
	}

	return groups
}

// findExactDuplicatesByCode находит точные дубликаты по коду
func (da *DuplicateAnalyzer) findExactDuplicatesByCode(items []DuplicateItem) []DuplicateGroup {
	codeMap := make(map[string][]DuplicateItem)

	// Группируем по коду
	for _, item := range items {
		if item.Code == "" {
			continue
		}
		code := strings.TrimSpace(strings.ToLower(item.Code))
		codeMap[code] = append(codeMap[code], item)
	}

	// Создаем группы для дубликатов
	var groups []DuplicateGroup
	groupCounter := 0
	for code, duplicates := range codeMap {
		if len(duplicates) < 2 {
			continue // Не дубликат
		}

		itemIDs := make([]int, len(duplicates))
		for i, item := range duplicates {
			itemIDs[i] = item.ID
		}

		groups = append(groups, DuplicateGroup{
			GroupID:         formatGroupID("code", groupCounter),
			Type:            DuplicateTypeExact,
			SimilarityScore: 1.0,
			ItemIDs:         itemIDs,
			Items:           duplicates,
			Confidence:      1.0,
			Reason:          "Exact match by code: " + code,
		})
		groupCounter++
	}

	return groups
}

// findExactDuplicatesByName находит точные дубликаты по имени
func (da *DuplicateAnalyzer) findExactDuplicatesByName(items []DuplicateItem) []DuplicateGroup {
	nameMap := make(map[string][]DuplicateItem)

	// Группируем по нормализованному имени
	for _, item := range items {
		if item.NormalizedName == "" {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(item.NormalizedName))
		nameMap[name] = append(nameMap[name], item)
	}

	// Создаем группы для дубликатов
	var groups []DuplicateGroup
	groupCounter := 0
	for name, duplicates := range nameMap {
		if len(duplicates) < 2 {
			continue
		}

		itemIDs := make([]int, len(duplicates))
		for i, item := range duplicates {
			itemIDs[i] = item.ID
		}

		groups = append(groups, DuplicateGroup{
			GroupID:         formatGroupID("name", groupCounter),
			Type:            DuplicateTypeExact,
			SimilarityScore: 1.0,
			ItemIDs:         itemIDs,
			Items:           duplicates,
			Confidence:      1.0,
			Reason:          "Exact match by name: " + name,
		})
		groupCounter++
	}

	return groups
}

// findSemanticDuplicates находит семантически похожие дубликаты
func (da *DuplicateAnalyzer) findSemanticDuplicates(items []DuplicateItem) []DuplicateGroup {
	var groups []DuplicateGroup

	// Строим TF-IDF векторы для каждого item
	corpus := make([]string, len(items))
	for i, item := range items {
		corpus[i] = item.NormalizedName
	}
	tfidfVectors := buildTFIDFVectors(corpus)

	// Сравниваем каждую пару
	processed := make(map[int]bool)
	groupCounter := 0

	for i := 0; i < len(items); i++ {
		if processed[i] {
			continue
		}

		var duplicates []DuplicateItem
		var itemIDs []int
		duplicates = append(duplicates, items[i])
		itemIDs = append(itemIDs, items[i].ID)

		for j := i + 1; j < len(items); j++ {
			if processed[j] {
				continue
			}

			// Вычисляем косинусную близость
			similarity := cosineSimilarity(tfidfVectors[i], tfidfVectors[j])

			if similarity >= da.semanticThreshold {
				duplicates = append(duplicates, items[j])
				itemIDs = append(itemIDs, items[j].ID)
				processed[j] = true
			}
		}

		// Если нашли хотя бы один дубликат
		if len(duplicates) >= 2 {
			processed[i] = true

			avgSimilarity := 0.0
			for k := 0; k < len(duplicates)-1; k++ {
				for l := k + 1; l < len(duplicates); l++ {
					avgSimilarity += cosineSimilarity(
						tfidfVectors[getItemIndex(items, duplicates[k].ID)],
						tfidfVectors[getItemIndex(items, duplicates[l].ID)],
					)
				}
			}
			pairCount := float64((len(duplicates) * (len(duplicates) - 1)) / 2)
			if pairCount > 0 {
				avgSimilarity /= pairCount
			}

			groups = append(groups, DuplicateGroup{
				GroupID:         formatGroupID("semantic", groupCounter),
				Type:            DuplicateTypeSemantic,
				SimilarityScore: avgSimilarity,
				ItemIDs:         itemIDs,
				Items:           duplicates,
				Confidence:      avgSimilarity,
				Reason:          "Semantic similarity detected",
			})
			groupCounter++
		}
	}

	return groups
}

// findPhoneticDuplicates находит фонетически похожие дубликаты (для опечаток)
func (da *DuplicateAnalyzer) findPhoneticDuplicates(items []DuplicateItem) []DuplicateGroup {
	var groups []DuplicateGroup

	processed := make(map[int]bool)
	groupCounter := 0

	for i := 0; i < len(items); i++ {
		if processed[i] {
			continue
		}

		var duplicates []DuplicateItem
		var itemIDs []int
		duplicates = append(duplicates, items[i])
		itemIDs = append(itemIDs, items[i].ID)

		phonetic1 := phoneticHash(items[i].NormalizedName)

		for j := i + 1; j < len(items); j++ {
			if processed[j] {
				continue
			}

			phonetic2 := phoneticHash(items[j].NormalizedName)

			// Levenshtein distance для фонетических хэшей
			similarity := 1.0 - float64(levenshteinDistance(phonetic1, phonetic2))/float64(max(len(phonetic1), len(phonetic2)))

			if similarity >= da.phoneticThreshold {
				duplicates = append(duplicates, items[j])
				itemIDs = append(itemIDs, items[j].ID)
				processed[j] = true
			}
		}

		if len(duplicates) >= 2 {
			processed[i] = true

			groups = append(groups, DuplicateGroup{
				GroupID:         formatGroupID("phonetic", groupCounter),
				Type:            DuplicateTypePhonetic,
				SimilarityScore: da.phoneticThreshold,
				ItemIDs:         itemIDs,
				Items:           duplicates,
				Confidence:      0.8, // Меньше уверенности для фонетических
				Reason:          "Phonetic similarity (possible typo)",
			})
			groupCounter++
		}
	}

	return groups
}

// findWordBasedDuplicates находит дубликаты на основе общих слов в названиях
// Алгоритм:
//   1. Извлекает слова из нормализованных названий (с учетом стоп-слов)
//   2. Строит обратный индекс: слово -> список элементов
//   3. Группирует элементы с минимум wordBasedMinCommonWords общими словами
//   4. Вычисляет similarity score на основе количества общих слов
func (da *DuplicateAnalyzer) findWordBasedDuplicates(items []DuplicateItem) []DuplicateGroup {
	var groups []DuplicateGroup

	// 1. Извлекаем слова для каждого элемента
	itemWords := make(map[int]map[string]bool) // itemID -> множество слов
	wordToItems := make(map[string]map[int]bool) // слово -> множество itemID

	for _, item := range items {
		if item.NormalizedName == "" {
			continue
		}

		// Токенизируем с учетом настройки стоп-слов
		words := tokenizeWithOptions(item.NormalizedName, da.wordBasedUseStopWords)
		if len(words) == 0 {
			continue
		}

		// Создаем множество слов для элемента
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
	}

	// 2. Группируем элементы по общим словам
	processed := make(map[int]bool)
	groupCounter := 0

	// Проходим по всем словам, которые встречаются в нескольких элементах
	for _, itemSet := range wordToItems {
		// Пропускаем слова, которые встречаются только в одном элементе
		if len(itemSet) < 2 {
			continue
		}

		// Создаем список элементов для этого слова
		var candidateItems []DuplicateItem
		for itemID := range itemSet {
			// Находим элемент по ID
			for i := range items {
				if items[i].ID == itemID {
					candidateItems = append(candidateItems, items[i])
					break
				}
			}
		}

		// Если элементов меньше 2, пропускаем
		if len(candidateItems) < 2 {
			continue
		}

		// 3. Проверяем, сколько общих слов у элементов в группе
		// Группируем элементы, которые имеют минимум wordBasedMinCommonWords общих слов
		for i := 0; i < len(candidateItems); i++ {
			if processed[candidateItems[i].ID] {
				continue
			}

			var groupItems []DuplicateItem
			var groupItemIDs []int
			groupItems = append(groupItems, candidateItems[i])
			groupItemIDs = append(groupItemIDs, candidateItems[i].ID)

			words1 := itemWords[candidateItems[i].ID]
			if words1 == nil {
				continue
			}

			// Ищем другие элементы с достаточным количеством общих слов
			for j := i + 1; j < len(candidateItems); j++ {
				if processed[candidateItems[j].ID] {
					continue
				}

				words2 := itemWords[candidateItems[j].ID]
				if words2 == nil {
					continue
				}

				// Подсчитываем общие слова
				commonWords := 0
				for w := range words1 {
					if words2[w] {
						commonWords++
					}
				}

				// Если достаточно общих слов, добавляем в группу
				if commonWords >= da.wordBasedMinCommonWords {
					groupItems = append(groupItems, candidateItems[j])
					groupItemIDs = append(groupItemIDs, candidateItems[j].ID)
				}
			}

			// Если группа содержит минимум 2 элемента, создаем DuplicateGroup
			if len(groupItems) >= 2 {
				// Вычисляем similarity score на основе общих слов
				// Используем среднее значение для всех пар в группе
				totalSimilarity := 0.0
				pairCount := 0

				for k := 0; k < len(groupItems); k++ {
					for l := k + 1; l < len(groupItems); l++ {
						wordsK := itemWords[groupItems[k].ID]
						wordsL := itemWords[groupItems[l].ID]

						// Подсчитываем общие слова
						commonCount := 0
						totalUnique := 0
						uniqueWords := make(map[string]bool)

						for w := range wordsK {
							uniqueWords[w] = true
							if wordsL[w] {
								commonCount++
							}
						}
						for w := range wordsL {
							uniqueWords[w] = true
						}
						totalUnique = len(uniqueWords)

						// Similarity = общие слова / максимальное количество уникальных слов
						if totalUnique > 0 {
							similarity := float64(commonCount) / float64(totalUnique)
							totalSimilarity += similarity
							pairCount++
						}
					}
				}

				avgSimilarity := 0.0
				if pairCount > 0 {
					avgSimilarity = totalSimilarity / float64(pairCount)
				}

				// Формируем список общих слов для reason
				if len(groupItems) >= 2 {
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

					reason := fmt.Sprintf("Common words (%d): %s", len(commonWordsList), strings.Join(commonWordsList, ", "))
					if len(commonWordsList) == 0 {
						reason = "Word-based similarity detected"
					}

					groups = append(groups, DuplicateGroup{
						GroupID:         formatGroupID("word", groupCounter),
						Type:            DuplicateTypeWordBased,
						SimilarityScore: avgSimilarity,
						ItemIDs:         groupItemIDs,
						Items:           groupItems,
						Confidence:      avgSimilarity,
						Reason:          reason,
					})

					// Помечаем элементы как обработанные
					for _, item := range groupItems {
						processed[item.ID] = true
					}
					groupCounter++
				}
			}
		}
	}

	return groups
}

// mergeOverlappingGroups объединяет пересекающиеся группы дубликатов
func (da *DuplicateAnalyzer) mergeOverlappingGroups(groups []DuplicateGroup) []DuplicateGroup {
	if len(groups) == 0 {
		return groups
	}

	merged := make([]DuplicateGroup, 0)
	used := make(map[int]bool)

	for i, group1 := range groups {
		if used[i] {
			continue
		}

		currentGroup := group1
		used[i] = true

		// Ищем пересечения с другими группами
		for j := i + 1; j < len(groups); j++ {
			if used[j] {
				continue
			}

			if hasOverlap(currentGroup.ItemIDs, groups[j].ItemIDs) {
				// Объединяем группы
				currentGroup = mergeGroups(currentGroup, groups[j])
				used[j] = true
			}
		}

		merged = append(merged, currentGroup)
	}

	return merged
}

// selectMasterRecord выбирает master record для группы дубликатов
func (da *DuplicateAnalyzer) selectMasterRecord(items []DuplicateItem) int {
	if len(items) == 0 {
		return 0
	}

	bestIndex := 0
	bestScore := calculateMasterScore(items[0])

	for i := 1; i < len(items); i++ {
		score := calculateMasterScore(items[i])
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}

	return items[bestIndex].ID
}

// calculateMasterScore вычисляет оценку для выбора master record
func calculateMasterScore(item DuplicateItem) float64 {
	score := 0.0

	// Предпочитаем записи с высоким качеством
	score += item.QualityScore * 40.0

	// Предпочитаем записи, которые уже объединяют другие (merged_count)
	score += float64(item.MergedCount) * 10.0

	// Предпочитаем AI-enhanced записи
	if item.ProcessingLevel == "ai_enhanced" {
		score += 20.0
	} else if item.ProcessingLevel == "benchmark" {
		score += 30.0
	}

	// Предпочитаем более длинные имена (больше информации)
	nameLen := float64(len([]rune(item.NormalizedName)))
	score += math.Min(nameLen/2.0, 10.0)

	return score
}

// --- Вспомогательные функции ---

// buildTFIDFVectors строит TF-IDF векторы для корпуса текстов
func buildTFIDFVectors(corpus []string) []map[string]float64 {
	// Подсчет частоты терминов в документах (IDF)
	docFreq := make(map[string]int)
	tokenizedDocs := make([][]string, len(corpus))

	for i, doc := range corpus {
		tokens := tokenize(doc)
		tokenizedDocs[i] = tokens

		uniqueTokens := make(map[string]bool)
		for _, token := range tokens {
			uniqueTokens[token] = true
		}

		for token := range uniqueTokens {
			docFreq[token]++
		}
	}

	// Вычисление TF-IDF
	vectors := make([]map[string]float64, len(corpus))
	numDocs := float64(len(corpus))

	for i, tokens := range tokenizedDocs {
		vector := make(map[string]float64)
		termFreq := make(map[string]int)

		// TF
		for _, token := range tokens {
			termFreq[token]++
		}

		// TF-IDF
		for term, freq := range termFreq {
			tf := float64(freq) / float64(len(tokens))
			idf := math.Log(numDocs / float64(docFreq[term]))
			vector[term] = tf * idf
		}

		vectors[i] = vector
	}

	return vectors
}

// cosineSimilarity вычисляет косинусную близость между двумя векторами
func cosineSimilarity(vec1, vec2 map[string]float64) float64 {
	if len(vec1) == 0 || len(vec2) == 0 {
		return 0.0
	}

	dotProduct := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for term, val1 := range vec1 {
		if val2, exists := vec2[term]; exists {
			dotProduct += val1 * val2
		}
		norm1 += val1 * val1
	}

	for _, val2 := range vec2 {
		norm2 += val2 * val2
	}

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// tokenize разбивает текст на токены
func tokenize(text string) []string {
	return tokenizeWithOptions(text, false)
}

// tokenizeWithOptions разбивает текст на токены с опцией использования стоп-слов
func tokenizeWithOptions(text string, useStopWords bool) []string {
	// Приводим к нижнему регистру
	text = strings.ToLower(text)

	// Удаляем знаки пунктуации
	reg := regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	text = reg.ReplaceAllString(text, " ")

	// Разбиваем на слова
	words := strings.Fields(text)

	// Фильтруем стоп-слова только если useStopWords == false
	stopWords := map[string]bool{
		"и": true, "в": true, "на": true, "с": true, "для": true,
		"по": true, "из": true, "к": true, "от": true, "о": true,
		"а": true, "но": true, "или": true, "то": true, "что": true,
	}

	var tokens []string
	for _, word := range words {
		// Пропускаем короткие слова (меньше 2 символов)
		if len(word) < 2 {
			continue
		}
		// Пропускаем стоп-слова только если useStopWords == false
		if !useStopWords && stopWords[word] {
			continue
		}
		tokens = append(tokens, word)
	}

	return tokens
}

// phoneticHash создает фонетический хэш для русского текста
func phoneticHash(text string) string {
	text = strings.ToLower(text)

	// Замены для фонетической близости в русском языке
	replacements := map[string]string{
		"е": "и", "ё": "и", "и": "и", "й": "и",
		"о": "а", "а": "а",
		"б": "п", "п": "п",
		"в": "ф", "ф": "ф",
		"г": "к", "к": "к",
		"д": "т", "т": "т",
		"ж": "ш", "ш": "ш",
		"з": "с", "с": "с",
	}

	var result strings.Builder
	for _, r := range text {
		char := string(r)
		if replacement, ok := replacements[char]; ok {
			result.WriteString(replacement)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// levenshteinDistance вычисляет расстояние Левенштейна
func levenshteinDistance(s1, s2 string) int {
	r1 := []rune(s1)
	r2 := []rune(s2)
	len1 := len(r1)
	len2 := len(r2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}

	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len1][len2]
}

// hasOverlap проверяет наличие пересечений между двумя списками ID
func hasOverlap(ids1, ids2 []int) bool {
	set := make(map[int]bool)
	for _, id := range ids1 {
		set[id] = true
	}
	for _, id := range ids2 {
		if set[id] {
			return true
		}
	}
	return false
}

// mergeGroups объединяет две группы дубликатов
func mergeGroups(g1, g2 DuplicateGroup) DuplicateGroup {
	// Объединяем ID
	idSet := make(map[int]bool)
	for _, id := range g1.ItemIDs {
		idSet[id] = true
	}
	for _, id := range g2.ItemIDs {
		idSet[id] = true
	}

	var mergedIDs []int
	for id := range idSet {
		mergedIDs = append(mergedIDs, id)
	}

	// Объединяем items
	itemSet := make(map[int]DuplicateItem)
	for _, item := range g1.Items {
		itemSet[item.ID] = item
	}
	for _, item := range g2.Items {
		itemSet[item.ID] = item
	}

	var mergedItems []DuplicateItem
	for _, item := range itemSet {
		mergedItems = append(mergedItems, item)
	}

	// Выбираем тип
	dupType := DuplicateTypeMixed
	if g1.Type == g2.Type {
		dupType = g1.Type
	}

	// Средняя similarity
	avgSimilarity := (g1.SimilarityScore + g2.SimilarityScore) / 2.0

	return DuplicateGroup{
		GroupID:         g1.GroupID, // Сохраняем ID первой группы
		Type:            dupType,
		SimilarityScore: avgSimilarity,
		ItemIDs:         mergedIDs,
		Items:           mergedItems,
		Confidence:      (g1.Confidence + g2.Confidence) / 2.0,
		Reason:          "Merged: " + g1.Reason + " + " + g2.Reason,
	}
}

// formatGroupID форматирует ID группы
func formatGroupID(prefix string, counter int) string {
	return fmt.Sprintf("%s_%d", prefix, counter)
}

// getItemIndex находит индекс item по ID
func getItemIndex(items []DuplicateItem, id int) int {
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

// min возвращает минимум из трех чисел
func min(a, b, c int) int {
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

// max возвращает максимум из двух чисел
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
