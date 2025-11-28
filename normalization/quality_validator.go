package normalization

import (
	"strings"
	"unicode"
)

// QualityScore представляет оценку качества нормализации
type QualityScore struct {
	Overall              float64 `json:"overall"`              // Общая оценка (0-1)
	CategoryConfidence   float64 `json:"category_confidence"`  // Уверенность в категории
	NameClarity          float64 `json:"name_clarity"`         // Ясность имени
	Consistency          float64 `json:"consistency"`          // Согласованность
	Completeness         float64 `json:"completeness"`         // Полнота данных
	Standardization      float64 `json:"standardization"`      // Стандартизация
	AIConfidenceBonus    float64 `json:"ai_confidence_bonus"`  // Бонус от AI уверенности
	IsBenchmarkQuality   bool    `json:"is_benchmark_quality"` // Достигнут ли уровень эталона (≥0.9)

	// Расширенные метрики DQAS
	KpvedAccuracy        float64 `json:"kpved_accuracy"`       // Точность КПВЭД кода (формат + заполненность)
	DuplicateScore       float64 `json:"duplicate_score"`      // Оценка уникальности (1.0 = уникален)
	DataEnrichment       float64 `json:"data_enrichment"`      // Обогащение данных (AI reasoning, KPVED и т.д.)
}

// QualityValidator валидатор качества нормализации
type QualityValidator struct {
	categorizer    *Categorizer
	nameNormalizer *NameNormalizer
}

// NewQualityValidator создает новый валидатор качества
func NewQualityValidator() *QualityValidator {
	return &QualityValidator{
		categorizer:    NewCategorizer(),
		nameNormalizer: NewNameNormalizer(),
	}
}

// ValidateQuality оценивает качество нормализованной записи
func (qv *QualityValidator) ValidateQuality(
	sourceName string,
	normalizedName string,
	category string,
	aiConfidence float64,
	processingLevel string,
) *QualityScore {
	score := &QualityScore{}

	// 1. Оценка уверенности в категории (20% веса)
	score.CategoryConfidence = qv.evaluateCategoryConfidence(category, sourceName)

	// 2. Оценка ясности имени (25% веса)
	score.NameClarity = qv.evaluateNameClarity(normalizedName)

	// 3. Оценка согласованности (20% веса)
	score.Consistency = qv.evaluateConsistency(sourceName, normalizedName, category)

	// 4. Оценка полноты данных (15% веса)
	score.Completeness = qv.evaluateCompleteness(normalizedName, category)

	// 5. Оценка стандартизации (20% веса)
	score.Standardization = qv.evaluateStandardization(normalizedName)

	// 6. Бонус от AI (если использовался AI)
	if processingLevel == "ai_enhanced" && aiConfidence > 0 {
		score.AIConfidenceBonus = aiConfidence * 0.1 // До 10% бонуса
	}

	// Рассчитываем общую оценку с весами
	score.Overall = (score.CategoryConfidence * 0.20) +
		(score.NameClarity * 0.25) +
		(score.Consistency * 0.20) +
		(score.Completeness * 0.15) +
		(score.Standardization * 0.20) +
		score.AIConfidenceBonus

	// Ограничиваем максимум единицей
	if score.Overall > 1.0 {
		score.Overall = 1.0
	}

	// Проверяем, достигнут ли уровень эталона
	score.IsBenchmarkQuality = score.Overall >= 0.9

	return score
}

// evaluateCategoryConfidence оценивает уверенность в категории
func (qv *QualityValidator) evaluateCategoryConfidence(category, sourceName string) float64 {
	// Если категория "другое" - низкая уверенность
	if category == "другое" {
		return 0.3
	}

	// Проверяем, соответствует ли категория ключевым словам в имени
	nameLower := strings.ToLower(sourceName)
	categoryLower := strings.ToLower(category)

	// Список ключевых слов для каждой категории
	categoryKeywords := map[string][]string{
		"инструмент":              {"молот", "отвер", "ключ", "пила", "дрель", "шуруп"},
		"медикаменты":             {"лекарств", "препарат", "таблет", "мазь", "сироп"},
		"стройматериалы":          {"цемент", "кирпич", "блок", "панель", "плита"},
		"электроника":             {"компьютер", "телефон", "планшет", "монитор", "принтер"},
		"оборудование":            {"станок", "агрегат", "установ", "машин"},
		"канцелярия":              {"ручк", "карандаш", "тетрад", "папк", "скреп"},
		"автоаксессуары":          {"автомобиль", "авто", "машин", "колес", "шин"},
		"средства очистки":        {"мыло", "моющ", "чист", "порош"},
		"продукты":                {"хлеб", "молок", "мяс", "рыб", "овощ"},
		"сельское хозяйство":      {"семен", "удобр", "корм", "сельхоз"},
		"связь":                   {"телефон", "роутер", "модем", "антенн"},
		"сантехника":              {"кран", "труб", "смесител", "унитаз"},
		"мебель":                  {"стол", "стул", "шкаф", "диван", "кресл"},
		"инструменты измерительные": {"измер", "метр", "линейк", "уровень"},
	}

	keywords, exists := categoryKeywords[categoryLower]
	if !exists {
		return 0.7 // Средняя уверенность для неизвестных категорий
	}

	// Проверяем наличие ключевых слов
	for _, keyword := range keywords {
		if strings.Contains(nameLower, keyword) {
			return 0.95 // Высокая уверенность
		}
	}

	return 0.6 // Категория не подтверждена ключевыми словами
}

// evaluateNameClarity оценивает ясность нормализованного имени
func (qv *QualityValidator) evaluateNameClarity(normalizedName string) float64 {
	if normalizedName == "" {
		return 0.0
	}

	score := 1.0

	// Проверка длины (оптимально 10-50 символов)
	nameLen := len([]rune(normalizedName))
	if nameLen < 3 {
		score -= 0.5 // Слишком короткое
	} else if nameLen < 10 {
		score -= 0.2
	} else if nameLen > 100 {
		score -= 0.3 // Слишком длинное
	} else if nameLen > 50 {
		score -= 0.1
	}

	// Проверка на наличие спецсимволов (должно быть минимум)
	specialCount := 0
	digitCount := 0
	letterCount := 0
	for _, r := range normalizedName {
		if unicode.IsLetter(r) {
			letterCount++
		} else if unicode.IsDigit(r) {
			digitCount++
		} else if !unicode.IsSpace(r) {
			specialCount++
		}
	}

	// Слишком много цифр (>40%)
	if float64(digitCount)/float64(nameLen) > 0.4 {
		score -= 0.2
	}

	// Слишком много спецсимволов (>5%)
	if float64(specialCount)/float64(nameLen) > 0.05 {
		score -= 0.3
	}

	// Должны быть буквы
	if letterCount == 0 {
		score -= 0.5
	}

	// Проверка регистра (нормализованное имя должно быть в нижнем регистре)
	if normalizedName != strings.ToLower(normalizedName) {
		score -= 0.1
	}

	if score < 0 {
		score = 0
	}
	return score
}

// evaluateConsistency оценивает согласованность между исходным и нормализованным именем
func (qv *QualityValidator) evaluateConsistency(sourceName, normalizedName, category string) float64 {
	if sourceName == "" || normalizedName == "" {
		return 0.0
	}

	score := 1.0

	// Проверяем, что основные слова сохранены
	sourceWords := extractKeyWords(sourceName)
	normalizedWords := extractKeyWords(normalizedName)

	if len(sourceWords) == 0 {
		return 0.5
	}

	// Считаем процент сохраненных ключевых слов
	preservedCount := 0
	for _, sourceWord := range sourceWords {
		for _, normWord := range normalizedWords {
			if strings.Contains(normWord, sourceWord) || strings.Contains(sourceWord, normWord) {
				preservedCount++
				break
			}
		}
	}

	preservationRate := float64(preservedCount) / float64(len(sourceWords))
	if preservationRate < 0.3 {
		score -= 0.5 // Слишком много потеряно
	} else if preservationRate < 0.5 {
		score -= 0.3
	} else if preservationRate < 0.7 {
		score -= 0.1
	}

	return score
}

// evaluateCompleteness оценивает полноту данных
func (qv *QualityValidator) evaluateCompleteness(normalizedName, category string) float64 {
	score := 1.0

	// Проверяем наличие имени
	if normalizedName == "" {
		score -= 0.5
	}

	// Проверяем наличие категории
	if category == "" || category == "другое" {
		score -= 0.3
	}

	// Проверяем, что имя содержит смысловые слова (не только артикулы/коды)
	words := extractKeyWords(normalizedName)
	if len(words) == 0 {
		score -= 0.2
	}

	if score < 0 {
		score = 0
	}
	return score
}

// evaluateStandardization оценивает стандартизацию
func (qv *QualityValidator) evaluateStandardization(normalizedName string) float64 {
	score := 1.0

	// Проверка формата (должно быть в нижнем регистре, без лишних пробелов)
	if normalizedName != strings.TrimSpace(normalizedName) {
		score -= 0.2
	}

	// Проверка на дублирующие пробелы
	if strings.Contains(normalizedName, "  ") {
		score -= 0.1
	}

	// Проверка на стандартные префиксы/суффиксы
	nameLower := strings.ToLower(normalizedName)

	// Не должно начинаться с артикула
	if strings.HasPrefix(nameLower, "арт") || strings.HasPrefix(nameLower, "код") {
		score -= 0.3
	}

	// Не должно содержать лишние символы в начале/конце
	if strings.HasPrefix(normalizedName, "-") || strings.HasSuffix(normalizedName, "-") {
		score -= 0.1
	}

	if score < 0 {
		score = 0
	}
	return score
}

// extractKeyWords извлекает ключевые слова из текста
func extractKeyWords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	keyWords := make([]string, 0)

	// Фильтруем короткие слова и стоп-слова
	stopWords := map[string]bool{
		"и": true, "в": true, "на": true, "с": true, "для": true,
		"по": true, "из": true, "к": true, "от": true,
	}

	for _, word := range words {
		// Убираем пунктуацию
		word = strings.Trim(word, ".,!?;:-\"'")

		// Пропускаем короткие слова и стоп-слова
		if len(word) < 3 || stopWords[word] {
			continue
		}

		// Пропускаем числа
		if isNumeric(word) {
			continue
		}

		keyWords = append(keyWords, word)
	}

	return keyWords
}

// isNumeric проверяет, является ли строка числом
func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' && r != ',' {
			return false
		}
	}
	return len(s) > 0
}

// --- Расширенные методы для DQAS ---

// ValidateQualityExtended оценивает качество с расширенными метриками DQAS
func (qv *QualityValidator) ValidateQualityExtended(
	sourceName string,
	normalizedName string,
	category string,
	aiConfidence float64,
	processingLevel string,
	kpvedCode string,
	kpvedConfidence float64,
	aiReasoning string,
	isDuplicate bool,
) *QualityScore {
	// Базовая оценка качества
	score := qv.ValidateQuality(sourceName, normalizedName, category, aiConfidence, processingLevel)

	// Расширенные метрики DQAS
	score.KpvedAccuracy = qv.evaluateKpvedAccuracy(kpvedCode, kpvedConfidence)
	score.DuplicateScore = qv.evaluateDuplicateScore(isDuplicate)
	score.DataEnrichment = qv.evaluateDataEnrichment(aiReasoning, kpvedCode, processingLevel)

	// Пересчитываем общую оценку с учетом новых метрик
	score.Overall = (score.CategoryConfidence * 0.15) +
		(score.NameClarity * 0.20) +
		(score.Consistency * 0.15) +
		(score.Completeness * 0.10) +
		(score.Standardization * 0.15) +
		(score.KpvedAccuracy * 0.15) +
		(score.DuplicateScore * 0.05) +
		(score.DataEnrichment * 0.05) +
		score.AIConfidenceBonus

	// Ограничиваем максимум единицей
	if score.Overall > 1.0 {
		score.Overall = 1.0
	}

	// Проверяем, достигнут ли уровень эталона
	score.IsBenchmarkQuality = score.Overall >= 0.9

	return score
}

// evaluateKpvedAccuracy оценивает точность КПВЭД кода
func (qv *QualityValidator) evaluateKpvedAccuracy(kpvedCode string, kpvedConfidence float64) float64 {
	score := 0.0

	// 1. Проверяем заполненность
	if kpvedCode == "" {
		return 0.0 // Не заполнен - 0 баллов
	}
	score += 0.4 // +40% за наличие кода

	// 2. Проверяем формат КПВЭД (должен быть формат XX.XX.XX или подобный)
	if isValidKpvedFormat(kpvedCode) {
		score += 0.3 // +30% за корректный формат
	}

	// 3. Учитываем confidence классификации
	if kpvedConfidence > 0 {
		score += kpvedConfidence * 0.3 // До +30% от confidence
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// evaluateDuplicateScore оценивает уникальность (отсутствие дубликатов)
func (qv *QualityValidator) evaluateDuplicateScore(isDuplicate bool) float64 {
	if isDuplicate {
		return 0.0 // Дубликат - низкая оценка
	}
	return 1.0 // Уникальная запись - высокая оценка
}

// evaluateDataEnrichment оценивает обогащение данных
func (qv *QualityValidator) evaluateDataEnrichment(aiReasoning, kpvedCode, processingLevel string) float64 {
	score := 0.0

	// 1. Наличие AI reasoning
	if aiReasoning != "" && len(aiReasoning) > 10 {
		score += 0.4 // +40% за наличие детального AI обоснования
	}

	// 2. Наличие КПВЭД кода
	if kpvedCode != "" {
		score += 0.3 // +30% за наличие КПВЭД
	}

	// 3. Уровень обработки
	if processingLevel == "benchmark" {
		score += 0.3 // +30% за эталонный уровень
	} else if processingLevel == "ai_enhanced" {
		score += 0.2 // +20% за AI обработку
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// isValidKpvedFormat проверяет корректность формата КПВЭД кода
func isValidKpvedFormat(code string) bool {
	if code == "" {
		return false
	}

	// КПВЭД код обычно в формате XX.XX или XX.XX.XX или XX.XX.XX.XX
	// Проверяем базовые паттерны
	code = strings.TrimSpace(code)

	// Должен содержать только цифры и точки
	for _, r := range code {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}

	// Проверяем структуру с точками
	parts := strings.Split(code, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false // Неверное количество частей
	}

	// Каждая часть должна быть 2 цифры
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		// Проверяем что это числа
		for _, r := range part {
			if !unicode.IsDigit(r) {
				return false
			}
		}
	}

	return true
}
