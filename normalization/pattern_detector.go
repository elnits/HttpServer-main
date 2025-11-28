package normalization

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"httpserver/database"
)

// PatternType тип обнаруженного паттерна
type PatternType string

const (
	PatternTypo              PatternType = "typo"                // Опечатка
	PatternExtraSpaces       PatternType = "extra_spaces"        // Лишние пробелы
	PatternTechnicalCode     PatternType = "technical_code"     // Технический код
	PatternArticul           PatternType = "articul"            // Артикул
	PatternDimension         PatternType = "dimension"          // Размеры
	PatternMixedCase         PatternType = "mixed_case"          // Смешанный регистр
	PatternSpecialChars       PatternType = "special_chars"      // Специальные символы
	PatternAbbreviation      PatternType = "abbreviation"       // Аббревиатура
	PatternIncompleteWord    PatternType = "incomplete_word"    // Незавершенное слово
	PatternDuplicateWords    PatternType = "duplicate_words"    // Дублирующиеся слова
	PatternInconsistentFormat PatternType = "inconsistent_format" // Несогласованный формат
	PatternNumbersInName      PatternType = "numbers_in_name"   // Числа в названии
	PatternUnitsOfMeasure     PatternType = "units_of_measure"   // Единицы измерения
	PatternPrefixSuffix       PatternType = "prefix_suffix"      // Префиксы/суффиксы
	PatternBrand             PatternType = "brand"              // Бренд
	PatternModel             PatternType = "model"              // Модель товара
)

// PatternMatch найденный паттерн в названии
type PatternMatch struct {
	Type           PatternType `json:"type"`            // Тип паттерна
	Position       int         `json:"position"`       // Позиция в строке
	Length         int         `json:"length"`          // Длина совпадения
	MatchedText    string      `json:"matched_text"`   // Найденный текст
	SuggestedFix   string      `json:"suggested_fix"`  // Предлагаемое исправление
	Confidence     float64     `json:"confidence"`     // Уверенность (0-1)
	Description    string      `json:"description"`    // Описание проблемы
	Severity       string      `json:"severity"`       // Серьезность: low, medium, high, critical
	AutoFixable    bool        `json:"auto_fixable"`   // Можно ли исправить автоматически
}

// PatternDetector детектор паттернов в названиях
type PatternDetector struct {
	patterns []PatternRule
	parser   *StatefulParser   // Stateful парсер для контекстной детекции
	analyzer *PatternAnalyzer  // Анализатор статистики паттернов
}

// PatternRule правило для обнаружения паттерна
type PatternRule struct {
	Type        PatternType
	Regex       *regexp.Regexp
	Description string
	Severity    string
	AutoFixable bool
	FixFunc     func(string, *regexp.Regexp) string // Функция исправления
	Confidence  float64
}

// NewPatternDetector создает новый детектор паттернов
func NewPatternDetector() *PatternDetector {
	detector := &PatternDetector{
		patterns: make([]PatternRule, 0),
		parser:   NewStatefulParser(),
		analyzer: NewPatternAnalyzer(100), // Топ-100 паттернов
	}
	detector.registerDefaultPatterns()
	return detector
}

// registerDefaultPatterns регистрирует стандартные паттерны
func (pd *PatternDetector) registerDefaultPatterns() {
	// Технические коды (ER-00013004, ABC-12345)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternTechnicalCode,
		Regex:       regexp.MustCompile(`\b[A-ZА-Я]{2,5}-\d{4,10}\b`),
		Description: "Технический код в названии",
		Severity:    "medium",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, "") },
		Confidence:  0.95,
	})

	// Артикулы (арт.123, арт:456, артикул 789)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternArticul,
		Regex:       regexp.MustCompile(`(?i)\b(арт\.?|артикул|art\.?)\s*:?\s*\d+[-\w]*\b`),
		Description: "Артикул в названии",
		Severity:    "medium",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, "") },
		Confidence:  0.9,
	})

	// Размеры (100x100, 50х50, 10x20x30)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternDimension,
		Regex:       regexp.MustCompile(`\b\d+[xхXХ]\d+([xхXХ]\d+)?\b`),
		Description: "Размеры в названии",
		Severity:    "low",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, "") },
		Confidence:  0.85,
	})

	// Единицы измерения с числами (100м, 50кг, 2.5л)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternUnitsOfMeasure,
		Regex:       regexp.MustCompile(`\b\d+\.?\d*\s*(см|мм|м|л|кг|%|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек|м²|м³|мм²|см²)\b`),
		Description: "Единицы измерения в названии",
		Severity:    "low",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, "") },
		Confidence:  0.8,
	})

	// Лишние пробелы (более 2 подряд)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternExtraSpaces,
		Regex:       regexp.MustCompile(`\s{3,}`),
		Description: "Множественные пробелы",
		Severity:    "low",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, " ") },
		Confidence:  0.95,
	})

	// Смешанный регистр (СоСтАвЛеНнЫй, ВЕРХНИЙ+нижний)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternMixedCase,
		Regex:       regexp.MustCompile(`\b([А-ЯЁ]{2,}[а-яё]+|[а-яё]+[А-ЯЁ]{2,})|[A-Z]{2,}[a-z]+|[a-z]+[A-Z]{2,}\b`),
		Description: "Смешанный регистр",
		Severity:    "medium",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string {
			// Приводим к нижнему регистру, но сохраняем первую букву заглавной для каждого слова
			words := strings.Fields(s)
			for i, word := range words {
				if len(word) > 0 {
					runes := []rune(word)
					runes[0] = unicode.ToUpper(runes[0])
					for j := 1; j < len(runes); j++ {
						runes[j] = unicode.ToLower(runes[j])
					}
					words[i] = string(runes)
				}
			}
			return strings.Join(words, " ")
		},
		Confidence: 0.7,
	})

	// Специальные символы в неподходящих местах (!@#$%^&*)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternSpecialChars,
		Regex:       regexp.MustCompile(`[!@#$%^&*_=+<>?/\\|~` + "`" + `]`),
		Description: "Специальные символы",
		Severity:    "medium",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return r.ReplaceAllString(s, " ") },
		Confidence:  0.8,
	})

	// Дублирующиеся слова (молоток молоток, кабель кабель)
	// Используем более простой паттерн, так как Go regexp не поддерживает backreferences
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternDuplicateWords,
		Regex:       regexp.MustCompile(`\b(\w+)\s+\w+\b`),
		Description: "Дублирующиеся слова",
		Severity:    "high",
		AutoFixable: true,
		FixFunc: func(s string, r *regexp.Regexp) string {
			words := strings.Fields(s)
			if len(words) < 2 {
				return s
			}
			var result []string
			for i, word := range words {
				if i == 0 || !strings.EqualFold(word, words[i-1]) {
					result = append(result, word)
				}
			}
			return strings.Join(result, " ")
		},
		Confidence: 0.9,
	})

	// Числа в начале или конце названия (123Товар, Товар456)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternNumbersInName,
		Regex:       regexp.MustCompile(`(^\d+[-\w]*\s+|\s+\d+[-\w]*$)`),
		Description: "Числа в начале или конце названия",
		Severity:    "low",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return strings.TrimSpace(r.ReplaceAllString(s, " ")) },
		Confidence:  0.75,
	})

	// Префиксы/суффиксы (№123, #456, -TEST)
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternPrefixSuffix,
		Regex:       regexp.MustCompile(`(^[№#]\s*\d+|-\s*[A-ZА-Я]+$|^\d+\s*-)`),
		Description: "Префиксы или суффиксы",
		Severity:    "low",
		AutoFixable: true,
		FixFunc:     func(s string, r *regexp.Regexp) string { return strings.TrimSpace(r.ReplaceAllString(s, "")) },
		Confidence:  0.7,
	})

	// Незавершенные слова (товар..., кабе...)
	// Go regexp не поддерживает lookahead, используем упрощенный паттерн
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternIncompleteWord,
		Regex:       regexp.MustCompile(`\b\w{1,2}\.\.\.|\b\w{1,2}\s|^\w{1,2}$`),
		Description: "Незавершенное слово",
		Severity:    "medium",
		AutoFixable: false, // Требует контекста
		FixFunc:     nil,
		Confidence:  0.6,
	})

	// Бренды (Samsung, Apple, LG, Bosch, Sony, Philips, Siemens, Panasonic, etc.)
	// Паттерн для известных мировых брендов электроники и бытовой техники
	pd.patterns = append(pd.patterns, PatternRule{
		Type: PatternBrand,
		Regex: regexp.MustCompile(`(?i)\b(Samsung|Apple|LG|Bosch|Sony|Philips|Siemens|Panasonic|Xiaomi|Huawei|Lenovo|` +
			`HP|Dell|Asus|Acer|MSI|Gigabyte|Intel|AMD|Nvidia|Canon|Nikon|Olympus|Fujifilm|` +
			`Whirlpool|Electrolux|Indesit|Ariston|Gorenje|Candy|Zanussi|Beko|Hotpoint|Miele|` +
			`Liebherr|AEG|Neff|Kuppersberg|Haier|Hisense|TCL|Sharp|Toshiba|Hitachi|JVC|Pioneer|` +
			`Grundig|Vestel|Midea|Artel|Shivaki|Atlant|Pozis|Норд|Бирюса|Саратов|Свияга|` +
			`Microsoft|Google|Motorola|Nokia|OnePlus|Oppo|Vivo|Realme|Honor|Redmi|Poco|` +
			`Tesla|Makita|Bosch|DeWalt|Hitachi|Metabo|Stanley|Black\+Decker|Ryobi|Einhell)\b`),
		Description: "Бренд товара",
		Severity:    "info",
		AutoFixable: false, // Бренд нужно сохранять, а не удалять
		FixFunc:     nil,
		Confidence:  0.95,
	})

	// Модели товаров (обычно комбинации букв и цифр: SM-A515F, iPhone 13 Pro, GX-100, etc.)
	// Паттерн для обнаружения моделей в названиях товаров
	pd.patterns = append(pd.patterns, PatternRule{
		Type:        PatternModel,
		Regex:       regexp.MustCompile(`\b([A-ZА-Я]{1,4}[-\s]?\d{2,5}[A-ZА-Я]?|[A-ZА-Я]{2,}\s*\d{1,3}\s*(Pro|Max|Plus|Ultra|Mini|Lite|SE)?)\b`),
		Description: "Модель товара",
		Severity:    "info",
		AutoFixable: false, // Модель нужно сохранять
		FixFunc:     nil,
		Confidence:  0.85,
	})
}

// DetectPatterns обнаруживает все паттерны в названии
func (pd *PatternDetector) DetectPatterns(name string) []PatternMatch {
	var matches []PatternMatch

	if name == "" {
		return matches
	}

	for _, rule := range pd.patterns {
		ruleMatches := rule.Regex.FindAllStringSubmatchIndex(name, -1)
		for _, match := range ruleMatches {
			if len(match) >= 2 {
				start := match[0]
				end := match[1]
				matchedText := name[start:end]

				var suggestedFix string
				if rule.FixFunc != nil {
					suggestedFix = rule.FixFunc(matchedText, rule.Regex)
				} else {
					suggestedFix = matchedText // Без изменений, если нет функции исправления
				}

				matches = append(matches, PatternMatch{
					Type:         rule.Type,
					Position:     start,
					Length:       end - start,
					MatchedText:  matchedText,
					SuggestedFix: suggestedFix,
					Confidence:   rule.Confidence,
					Description:  rule.Description,
					Severity:     rule.Severity,
					AutoFixable:  rule.AutoFixable,
				})
			}
		}
	}

	return matches
}

// ApplyFixes применяет все автоприменяемые исправления
func (pd *PatternDetector) ApplyFixes(name string, matches []PatternMatch) string {
	fixed := name

	// Применяем исправления для каждого типа паттерна только один раз
	appliedTypes := make(map[PatternType]bool)

	for _, match := range matches {
		if match.AutoFixable && !appliedTypes[match.Type] {
			rule := pd.findRuleByType(match.Type)
			if rule != nil && rule.FixFunc != nil {
				// Применяем исправление ко всей строке
				fixed = rule.FixFunc(fixed, rule.Regex)
				appliedTypes[match.Type] = true
			}
		}
	}

	// Очищаем лишние пробелы
	fixed = strings.Join(strings.Fields(fixed), " ")
	fixed = strings.TrimSpace(fixed)

	return fixed
}

// findRuleByType находит правило по типу
func (pd *PatternDetector) findRuleByType(patternType PatternType) *PatternRule {
	for i := range pd.patterns {
		if pd.patterns[i].Type == patternType {
			return &pd.patterns[i]
		}
	}
	return nil
}

// GetPatternSummary возвращает сводку по найденным паттернам
func (pd *PatternDetector) GetPatternSummary(matches []PatternMatch) map[string]interface{} {
	summary := make(map[string]interface{})
	summary["total"] = len(matches)

	byType := make(map[PatternType]int)
	bySeverity := make(map[string]int)
	autoFixableCount := 0

	for _, match := range matches {
		byType[match.Type]++
		bySeverity[match.Severity]++
		if match.AutoFixable {
			autoFixableCount++
		}
	}

	summary["by_type"] = byType
	summary["by_severity"] = bySeverity
	summary["auto_fixable"] = autoFixableCount

	return summary
}

// SuggestCorrection предлагает исправление на основе найденных паттернов
func (pd *PatternDetector) SuggestCorrection(originalName string, matches []PatternMatch) string {
	if len(matches) == 0 {
		return originalName
	}

	// Применяем все автоприменяемые исправления
	corrected := pd.ApplyFixes(originalName, matches)

	// Если есть неавтоприменяемые паттерны, оставляем пометку
	hasNonAutoFixable := false
	for _, match := range matches {
		if !match.AutoFixable {
			hasNonAutoFixable = true
			break
		}
	}

	if hasNonAutoFixable {
		// Можно добавить пометку, что требуется ручная проверка
		// Но пока просто возвращаем исправленную версию
	}

	return corrected
}

// FormatPatternReport форматирует отчет о найденных паттернах
func (pd *PatternDetector) FormatPatternReport(name string, matches []PatternMatch) string {
	if len(matches) == 0 {
		return fmt.Sprintf("✓ Название '%s' не содержит проблемных паттернов", name)
	}

	var report strings.Builder
	report.WriteString(fmt.Sprintf("Найдено %d паттернов в '%s':\n", len(matches), name))

	for i, match := range matches {
		report.WriteString(fmt.Sprintf("%d. [%s] %s: '%s' (позиция %d, длина %d)\n",
			i+1, match.Severity, match.Description, match.MatchedText, match.Position, match.Length))
		if match.AutoFixable {
			report.WriteString(fmt.Sprintf("   → Автоисправление: '%s'\n", match.SuggestedFix))
		} else {
			report.WriteString("   → Требуется ручная проверка\n")
		}
	}

	return report.String()
}

// ExtractedAttribute представляет извлеченный атрибут (бренд, модель, etc.)
type ExtractedAttribute struct {
	Type       string  `json:"type"`       // Тип атрибута (brand, model, dimension, etc.)
	Value      string  `json:"value"`      // Значение атрибута
	Confidence float64 `json:"confidence"` // Уверенность извлечения
	Position   int     `json:"position"`   // Позиция в тексте
}

// ExtractBrands извлекает все бренды из названия товара
// Возвращает список найденных брендов с их позициями и уверенностью
func (pd *PatternDetector) ExtractBrands(name string) []ExtractedAttribute {
	var brands []ExtractedAttribute

	// Паттерн для брендов (тот же, что и в registerDefaultPatterns)
	brandPattern := regexp.MustCompile(`(?i)\b(Samsung|Apple|LG|Bosch|Sony|Philips|Siemens|Panasonic|Xiaomi|Huawei|Lenovo|` +
		`HP|Dell|Asus|Acer|MSI|Gigabyte|Intel|AMD|Nvidia|Canon|Nikon|Olympus|Fujifilm|` +
		`Whirlpool|Electrolux|Indesit|Ariston|Gorenje|Candy|Zanussi|Beko|Hotpoint|Miele|` +
		`Liebherr|AEG|Neff|Kuppersberg|Haier|Hisense|TCL|Sharp|Toshiba|Hitachi|JVC|Pioneer|` +
		`Grundig|Vestel|Midea|Artel|Shivaki|Atlant|Pozis|Норд|Бирюса|Саратов|Свияга|` +
		`Microsoft|Google|Motorola|Nokia|OnePlus|Oppo|Vivo|Realme|Honor|Redmi|Poco|` +
		`Tesla|Makita|Bosch|DeWalt|Hitachi|Metabo|Stanley|Black\+Decker|Ryobi|Einhell)\b`)

	matches := brandPattern.FindAllStringSubmatchIndex(name, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			start := match[0]
			end := match[1]
			brandName := name[start:end]

			brands = append(brands, ExtractedAttribute{
				Type:       "brand",
				Value:      brandName,
				Confidence: 0.95,
				Position:   start,
			})
		}
	}

	return brands
}

// ExtractModels извлекает все модели из названия товара
// Возвращает список найденных моделей с их позициями и уверенностью
func (pd *PatternDetector) ExtractModels(name string) []ExtractedAttribute {
	var models []ExtractedAttribute

	// Паттерн для моделей (комбинации букв и цифр)
	modelPattern := regexp.MustCompile(`\b([A-ZА-Я]{1,4}[-\s]?\d{2,5}[A-ZА-Я]?|[A-ZА-Я]{2,}\s*\d{1,3}\s*(Pro|Max|Plus|Ultra|Mini|Lite|SE)?)\b`)

	matches := modelPattern.FindAllStringSubmatchIndex(name, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			start := match[0]
			end := match[1]
			modelName := name[start:end]

			models = append(models, ExtractedAttribute{
				Type:       "model",
				Value:      modelName,
				Confidence: 0.85,
				Position:   start,
			})
		}
	}

	return models
}

// ExtractDimensions извлекает размеры из названия товара (100x100, 50х50мм)
func (pd *PatternDetector) ExtractDimensions(name string) []ExtractedAttribute {
	var dimensions []ExtractedAttribute

	// Паттерн для размеров
	dimensionPattern := regexp.MustCompile(`\b\d+[xхXХ]\d+([xхXХ]\d+)?\s*(мм|см|м|mm|cm|m)?\b`)

	matches := dimensionPattern.FindAllStringSubmatchIndex(name, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			start := match[0]
			end := match[1]
			dimensionValue := name[start:end]

			dimensions = append(dimensions, ExtractedAttribute{
				Type:       "dimension",
				Value:      dimensionValue,
				Confidence: 0.9,
				Position:   start,
			})
		}
	}

	return dimensions
}

// ExtractArticles извлекает артикулы из названия товара (арт.123, артикул 456)
func (pd *PatternDetector) ExtractArticles(name string) []ExtractedAttribute {
	var articles []ExtractedAttribute

	// Паттерн для артикулов
	articlePattern := regexp.MustCompile(`(?i)\b(арт\.?|артикул|art\.?)\s*:?\s*(\d+[-\w]*)\b`)

	matches := articlePattern.FindAllStringSubmatchIndex(name, -1)
	for _, match := range matches {
		// match[4], match[5] - это начало и конец второй группы захвата (сам артикул без префикса)
		if len(match) >= 6 {
			start := match[4]
			end := match[5]
			articleValue := name[start:end]

			articles = append(articles, ExtractedAttribute{
				Type:       "article",
				Value:      articleValue,
				Confidence: 0.92,
				Position:   start,
			})
		}
	}

	return articles
}

// ExtractAllAttributes извлекает все атрибуты из названия товара
// Возвращает объединенный список всех найденных атрибутов (бренды, модели, размеры, артикулы)
func (pd *PatternDetector) ExtractAllAttributes(name string) []ExtractedAttribute {
	var allAttributes []ExtractedAttribute

	// Извлекаем все типы атрибутов
	allAttributes = append(allAttributes, pd.ExtractBrands(name)...)
	allAttributes = append(allAttributes, pd.ExtractModels(name)...)
	allAttributes = append(allAttributes, pd.ExtractDimensions(name)...)
	allAttributes = append(allAttributes, pd.ExtractArticles(name)...)

	return allAttributes
}

// DetectPatternsStateful выполняет детекцию паттернов с учетом состояния (stateful)
// Использует StatefulParser для анализа структуры с учетом вложенности скобок и кавычек
// Применяет паттерны только к токенам на указанной глубине (по умолчанию depth=0)
func (pd *PatternDetector) DetectPatternsStateful(name string, targetDepth int) []PatternMatch {
	var matches []PatternMatch

	if name == "" {
		return matches
	}

	// Парсим строку с учетом состояния
	tokens := pd.parser.ParseCharByChar(name)

	// Применяем паттерны только к токенам на целевой глубине
	for _, token := range tokens {
		// Пропускаем токены не на целевой глубине
		if token.Depth != targetDepth {
			continue
		}

		// Пропускаем скобки, разделители и прочие не текстовые токены
		if token.Type != TokenText && token.Type != TokenNumber {
			continue
		}

		// Применяем все правила к токену
		for _, rule := range pd.patterns {
			ruleMatches := rule.Regex.FindAllStringSubmatchIndex(token.Value, -1)
			for _, match := range ruleMatches {
				if len(match) >= 2 {
					start := match[0]
					end := match[1]
					matchedText := token.Value[start:end]

					var suggestedFix string
					if rule.FixFunc != nil {
						suggestedFix = rule.FixFunc(matchedText, rule.Regex)
					} else {
						suggestedFix = matchedText
					}

					// Позиция в оригинальной строке = позиция токена + позиция внутри токена
					absolutePosition := token.Position + start

					matches = append(matches, PatternMatch{
						Type:         rule.Type,
						Position:     absolutePosition,
						Length:       end - start,
						MatchedText:  matchedText,
						SuggestedFix: suggestedFix,
						Confidence:   rule.Confidence,
						Description:  fmt.Sprintf("%s (depth=%d)", rule.Description, token.Depth),
						Severity:     rule.Severity,
						AutoFixable:  rule.AutoFixable,
					})
				}
			}
		}
	}

	return matches
}

// DetectPatternsMultiDepth выполняет детекцию на всех уровнях вложенности
// Возвращает паттерны, сгруппированные по глубине
func (pd *PatternDetector) DetectPatternsMultiDepth(name string) map[int][]PatternMatch {
	result := make(map[int][]PatternMatch)

	if name == "" {
		return result
	}

	// Определяем максимальную глубину
	tokens := pd.parser.ParseCharByChar(name)
	maxDepth := 0
	for _, token := range tokens {
		if token.Depth > maxDepth {
			maxDepth = token.Depth
		}
	}

	// Детектируем паттерны на каждой глубине
	for depth := 0; depth <= maxDepth; depth++ {
		matches := pd.DetectPatternsStateful(name, depth)
		if len(matches) > 0 {
			result[depth] = matches
		}
	}

	return result
}

// AnalyzePatternDistribution выполняет статистический анализ паттернов
// Аналог analyze_performance_data из Python (token_stats, heapq, defaultdict)
func (pd *PatternDetector) AnalyzePatternDistribution(items []*database.CatalogItem) *PatternStatistics {
	// Сбрасываем предыдущую статистику
	pd.analyzer.Reset()

	// Анализируем каждый элемент
	for _, item := range items {
		patterns := pd.DetectPatterns(item.Name)

		// Используем CatalogName вместо Category (т.к. CatalogItem не имеет поля Category)
		category := item.CatalogName
		if category == "" {
			category = "unknown"
		}

		pd.analyzer.AnalyzeItem(item.Name, patterns, category, 1.0)
	}

	// Возвращаем полную статистику
	return pd.analyzer.GetStatistics()
}

// GetPatternStatistics возвращает текущую статистику паттернов
func (pd *PatternDetector) GetPatternStatistics() *PatternStatistics {
	return pd.analyzer.GetStatistics()
}

// GetTopPatterns возвращает топ-N наиболее частых паттернов
func (pd *PatternDetector) GetTopPatterns(n int) []HeapItem {
	return pd.analyzer.GetTopNPatterns(n)
}

// GetPatternPercentages возвращает процентное распределение типов паттернов
func (pd *PatternDetector) GetPatternPercentages() map[PatternType]float64 {
	return pd.analyzer.GetPercentageDistribution()
}

// FormatStatisticsReport генерирует текстовый отчет по статистике паттернов
func (pd *PatternDetector) FormatStatisticsReport() string {
	return pd.analyzer.FormatReport()
}

// AnalyzeStructureInfo анализирует структуру названия товара
// Возвращает информацию о токенах, скобках, глубине вложенности
type StructureAnalysis struct {
	TotalTokens      int
	TextTokens       int
	NumberTokens     int
	BracketPairs     int
	MaxDepth         int
	HasQuotes        bool
	DelimiterCount   int
	TokensByDepth    map[int][]string
	DepthDistribution map[int]int
}

func (pd *PatternDetector) AnalyzeStructure(name string) *StructureAnalysis {
	tokenizer := NewContextualTokenizer()
	structInfo := tokenizer.AnalyzeStructure(name)

	// Извлекаем токены по глубине
	tokens := pd.parser.ParseCharByChar(name)
	tokensByDepth := make(map[int][]string)

	for _, token := range tokens {
		if token.Type == TokenText || token.Type == TokenNumber {
			tokensByDepth[token.Depth] = append(tokensByDepth[token.Depth], token.Value)
		}
	}

	return &StructureAnalysis{
		TotalTokens:      structInfo.TotalTokens,
		TextTokens:       structInfo.TextTokens,
		NumberTokens:     structInfo.NumberTokens,
		BracketPairs:     structInfo.BracketPairs,
		MaxDepth:         structInfo.MaxDepth,
		HasQuotes:        structInfo.HasQuotes,
		DelimiterCount:   structInfo.DelimiterCount,
		TokensByDepth:    tokensByDepth,
		DepthDistribution: structInfo.DepthDistribution,
	}
}

