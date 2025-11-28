package normalization

import (
	"fmt"
	"regexp"
	"strings"

	"httpserver/database"
)

// NameNormalizer нормализует наименования товаров
type NameNormalizer struct {
	technicalCodeRegex     *regexp.Regexp
	dimensionRegex         *regexp.Regexp
	numbersWithUnitsRegex  *regexp.Regexp
	numbersWithUnitsNoSpaceRegex *regexp.Regexp // Числа с единицами без пробела (например, "120mm")
	standaloneNumbersRegex *regexp.Regexp
	articleCodeRegex       *regexp.Regexp // Артикулы/коды в начале строки (например, "wbc00z0002")
	trailingSpecialCharsRegex *regexp.Regexp // Специальные символы в конце строки
}

// NewNameNormalizer создает новый нормализатор имен
func NewNameNormalizer() *NameNormalizer {
	return &NameNormalizer{
		// Технические коды вида "ER-00013004"
		technicalCodeRegex: regexp.MustCompile(`\b[A-Z]{2}-\d+\b`),
		// Размеры вида 100x100 или 100х100 (с русской х)
		dimensionRegex: regexp.MustCompile(`\d+[xх]\d+`),
		// Числа с единицами измерения (с пробелом) - русские и английские единицы
		numbersWithUnitsRegex: regexp.MustCompile(`\d+\.?\d*\s*(см|мм|м|л|кг|%|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек|mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec)`),
		// Числа с единицами измерения без пробела (например, "120mm", "50kg")
		numbersWithUnitsNoSpaceRegex: regexp.MustCompile(`\d+\.?\d*(?:mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`),
		// Отдельно стоящие числа
		standaloneNumbersRegex: regexp.MustCompile(`\b\d+\b`),
		// Артикулы/коды в начале строки (например, "wbc00z0002", "wb500z0002")
		// Паттерн: буквы + цифры + буквы + цифры (смешанный формат артикулов)
		articleCodeRegex: regexp.MustCompile(`^[a-zа-я]{2,}\d+[a-zа-я]*\d+\s*`),
		// Специальные символы в конце строки (*, -, ., и т.д.)
		trailingSpecialCharsRegex: regexp.MustCompile(`[^\w\sа-яА-Я]+$`),
	}
}

// NormalizeName нормализует наименование товара
func (n *NameNormalizer) NormalizeName(name string) string {
	if name == "" {
		return ""
	}

	// 1. Приводим к нижнему регистру
	normalized := strings.ToLower(name)

	// 2. Удаляем артикулы/коды в начале строки (например, "wbc00z0002")
	normalized = n.articleCodeRegex.ReplaceAllString(normalized, "")

	// 3. Удаляем технические коды (коды вида "ER-00013004")
	normalized = n.technicalCodeRegex.ReplaceAllString(normalized, "")

	// 4. Удаляем размеры вида 100x100 или 100х100
	normalized = n.dimensionRegex.ReplaceAllString(normalized, "")

	// 5. Удаляем числа с единицами измерения без пробела (например, "120mm", "50kg")
	normalized = n.numbersWithUnitsNoSpaceRegex.ReplaceAllString(normalized, "")

	// 6. Удаляем числа с единицами измерения (с пробелом)
	normalized = n.numbersWithUnitsRegex.ReplaceAllString(normalized, "")

	// 7. Удаляем отдельно стоящие числа
	normalized = n.standaloneNumbersRegex.ReplaceAllString(normalized, "")

	// 8. Удаляем лишние пробелы и знаки препинания
	normalized = strings.Join(strings.Fields(normalized), " ")

	// 9. Удаляем специальные символы в конце строки (*, -, ., и т.д.)
	normalized = n.trailingSpecialCharsRegex.ReplaceAllString(normalized, "")

	// 10. Удаляем лишние знаки препинания в начале и конце
	normalized = strings.Trim(normalized, " ,.-+")

	return normalized
}

// PatternMatchInfo информация о найденном паттерне с позицией
type PatternMatchInfo struct {
	PatternType string
	MatchedText string
	Position    int
	EndPosition int
}

// ExtractAttributes извлекает атрибуты из названия товара
// Возвращает нормализованное имя и список извлеченных атрибутов
// Выявляет паттерны и связанные реквизиты, которые идут после найденных паттернов
func (n *NameNormalizer) ExtractAttributes(name string) (string, []*database.ItemAttribute) {
	if name == "" {
		return "", nil
	}

	var attributes []*database.ItemAttribute
	normalized := strings.ToLower(name)
	originalName := normalized
	var patternMatches []PatternMatchInfo

	// 1. Извлекаем артикулы/коды в начале строки
	articleMatches := n.articleCodeRegex.FindStringSubmatchIndex(originalName)
	if len(articleMatches) >= 2 {
		start, end := articleMatches[0], articleMatches[1]
		articleCode := strings.TrimSpace(originalName[start:end])
		attributes = append(attributes, &database.ItemAttribute{
			AttributeType:  "article_code",
			AttributeName:  "article",
			AttributeValue: articleCode,
			OriginalText:   articleCode,
			Confidence:     1.0,
		})
		patternMatches = append(patternMatches, PatternMatchInfo{
			PatternType: "article_code",
			MatchedText: articleCode,
			Position:    start,
			EndPosition: end,
		})
		normalized = n.articleCodeRegex.ReplaceAllString(normalized, "")
	}

	// 2. Извлекаем технические коды
	technicalMatches := n.technicalCodeRegex.FindAllStringSubmatchIndex(originalName, -1)
	for _, match := range technicalMatches {
		if len(match) >= 2 {
			start, end := match[0], match[1]
			code := originalName[start:end]
			attributes = append(attributes, &database.ItemAttribute{
				AttributeType:  "technical_code",
				AttributeName:  "code",
				AttributeValue: code,
				OriginalText:   code,
				Confidence:     1.0,
			})
			patternMatches = append(patternMatches, PatternMatchInfo{
				PatternType: "technical_code",
				MatchedText: code,
				Position:    start,
				EndPosition: end,
			})
		}
	}
	normalized = n.technicalCodeRegex.ReplaceAllString(normalized, "")

	// 3. Извлекаем размеры вида 100x100 или 100х100
	dimensionMatches := n.dimensionRegex.FindAllStringSubmatchIndex(originalName, -1)
	for _, match := range dimensionMatches {
		if len(match) >= 2 {
			start, end := match[0], match[1]
			dimensionText := originalName[start:end]
			parts := regexp.MustCompile(`[xх]`).Split(dimensionText, 2)
			if len(parts) == 2 {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  "width",
					AttributeValue: parts[0],
					OriginalText:   dimensionText,
					Confidence:     1.0,
				})
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  "height",
					AttributeValue: parts[1],
					OriginalText:   dimensionText,
					Confidence:     1.0,
				})
				patternMatches = append(patternMatches, PatternMatchInfo{
					PatternType: "dimension",
					MatchedText: dimensionText,
					Position:    start,
					EndPosition: end,
				})
			}
		}
	}
	normalized = n.dimensionRegex.ReplaceAllString(normalized, "")

	// 4. Извлекаем числа с единицами измерения без пробела (например, "120mm")
	unitNoSpaceMatches := n.numbersWithUnitsNoSpaceRegex.FindAllStringSubmatchIndex(originalName, -1)
	for _, match := range unitNoSpaceMatches {
		if len(match) >= 2 {
			start, end := match[0], match[1]
			matchText := originalName[start:end]
			re := regexp.MustCompile(`(\d+\.?\d*)(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
			submatches := re.FindStringSubmatch(matchText)
			if len(submatches) >= 3 {
				value := submatches[1]
				unit := submatches[2]
				
				attrName := n.getAttributeNameByUnit(unit)
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "numeric_value",
					AttributeName:  attrName,
					AttributeValue: value,
					Unit:           unit,
					OriginalText:   matchText,
					Confidence:     1.0,
				})
			}
		}
	}
	normalized = n.numbersWithUnitsNoSpaceRegex.ReplaceAllString(normalized, "")

	// 5. Извлекаем числа с единицами измерения (с пробелом)
	unitMatches := n.numbersWithUnitsRegex.FindAllStringSubmatchIndex(originalName, -1)
	for _, match := range unitMatches {
		if len(match) >= 2 {
			start, end := match[0], match[1]
			matchText := originalName[start:end]
			re := regexp.MustCompile(`(\d+\.?\d*)\s*(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
			submatches := re.FindStringSubmatch(matchText)
			if len(submatches) >= 3 {
				value := submatches[1]
				unit := submatches[2]
				
				attrName := n.getAttributeNameByUnit(unit)
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "numeric_value",
					AttributeName:  attrName,
					AttributeValue: value,
					Unit:           unit,
					OriginalText:   matchText,
					Confidence:     1.0,
				})
			}
		}
	}
	normalized = n.numbersWithUnitsRegex.ReplaceAllString(normalized, "")

	// 6. Для каждого найденного паттерна ищем связанные реквизиты
	// Используем map для отслеживания уже извлеченных атрибутов, чтобы избежать дублирования
	extractedKeys := make(map[string]bool)
	for _, attr := range attributes {
		key := attr.AttributeType + ":" + attr.AttributeName + ":" + attr.AttributeValue
		if attr.Unit != "" {
			key += ":" + attr.Unit
		}
		extractedKeys[key] = true
	}
	
	for _, patternMatch := range patternMatches {
		relatedAttrs := n.extractRelatedAttributes(originalName, patternMatch.EndPosition, patternMatch.PatternType)
		for _, attr := range relatedAttrs {
			key := attr.AttributeType + ":" + attr.AttributeName + ":" + attr.AttributeValue
			if attr.Unit != "" {
				key += ":" + attr.Unit
			}
			// Добавляем только если еще не извлечен
			if !extractedKeys[key] {
				attributes = append(attributes, attr)
				extractedKeys[key] = true
			}
		}
	}

	// 7. Удаляем отдельно стоящие числа (не извлекаем, так как они не имеют контекста)
	normalized = n.standaloneNumbersRegex.ReplaceAllString(normalized, "")

	// 8. Удаляем лишние пробелы и знаки препинания
	normalized = strings.Join(strings.Fields(normalized), " ")

	// 9. Удаляем специальные символы в конце строки
	normalized = n.trailingSpecialCharsRegex.ReplaceAllString(normalized, "")

	// 10. Удаляем лишние знаки препинания в начале и конце
	normalized = strings.Trim(normalized, " ,.-+")

	return normalized, attributes
}

// extractRelatedAttributes извлекает связанные реквизиты после найденного паттерна
// Анализирует текст после позиции паттерна и ищет типичные реквизиты
func (n *NameNormalizer) extractRelatedAttributes(text string, afterPosition int, patternType string) []*database.ItemAttribute {
	var attributes []*database.ItemAttribute
	
	if afterPosition >= len(text) {
		return attributes
	}
	
	// Берем текст после паттерна (максимум 100 символов для анализа)
	remainingText := text[afterPosition:]
	if len(remainingText) > 100 {
		remainingText = remainingText[:100]
	}
	
	// В зависимости от типа паттерна ищем разные связанные реквизиты
	switch patternType {
	case "dimension":
		// После размера обычно идут: толщина, материал, цвет, тип, покрытие
		// Ищем толщину (число с единицами длины) - первое число с единицами длины после размера
		thicknessRegex := regexp.MustCompile(`(\d+\.?\d*)\s*(mm|см|мм|cm|m|м)\b`)
		thicknessMatches := thicknessRegex.FindAllStringSubmatch(remainingText, 1)
		for _, match := range thicknessMatches {
			if len(match) >= 3 {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "numeric_value",
					AttributeName:  "thickness",
					AttributeValue: match[1],
					Unit:           match[2],
					OriginalText:   match[0],
					Confidence:     0.9,
				})
				break // Берем только первое совпадение
			}
		}
		
		// Ищем материал (обычно после размера)
		materialKeywords := []string{"сталь", "металл", "пластик", "дерево", "стекло", "алюминий", "бетон", "кирпич", "steel", "metal", "plastic", "wood", "glass", "aluminum", "concrete", "brick"}
		for _, keyword := range materialKeywords {
			if strings.Contains(remainingText, keyword) {
				// Извлекаем слово материала
				materialRegex := regexp.MustCompile(`\b(` + keyword + `[а-яa-z]*)\b`)
				materialMatch := materialRegex.FindString(remainingText)
				if materialMatch != "" {
					attributes = append(attributes, &database.ItemAttribute{
						AttributeType:  "text_value",
						AttributeName:  "material",
						AttributeValue: materialMatch,
						OriginalText:   materialMatch,
						Confidence:     0.8,
					})
					break
				}
			}
		}
		
		// Ищем цвет после размера
		colorKeywords := []string{"белый", "черный", "серый", "красный", "синий", "зеленый", "желтый", "коричневый", "бежевый", "white", "black", "gray", "grey", "red", "blue", "green", "yellow", "brown", "beige"}
		for _, keyword := range colorKeywords {
			if strings.Contains(remainingText, keyword) {
				colorRegex := regexp.MustCompile(`\b(` + keyword + `[а-яa-z]*)\b`)
				colorMatch := colorRegex.FindString(remainingText)
				if colorMatch != "" {
					attributes = append(attributes, &database.ItemAttribute{
						AttributeType:  "text_value",
						AttributeName:  "color",
						AttributeValue: colorMatch,
						OriginalText:   colorMatch,
						Confidence:     0.75,
					})
					break
				}
			}
		}
		
		// Ищем тип покрытия/обработки
		coatingKeywords := []string{"оцинкован", "покрашен", "полирован", "матовый", "глянцевый", "galvanized", "painted", "polished", "matte", "glossy"}
		for _, keyword := range coatingKeywords {
			if strings.Contains(remainingText, keyword) {
				coatingRegex := regexp.MustCompile(`\b(` + keyword + `[а-яa-z]*)\b`)
				coatingMatch := coatingRegex.FindString(remainingText)
				if coatingMatch != "" {
					attributes = append(attributes, &database.ItemAttribute{
						AttributeType:  "text_value",
						AttributeName:  "coating",
						AttributeValue: coatingMatch,
						OriginalText:   coatingMatch,
						Confidence:     0.75,
					})
					break
				}
			}
		}
		
	case "article_code":
		// После артикула обычно идут: размеры, характеристики, тип, толщина
		// Ищем размеры после артикула
		dimAfterArticle := n.dimensionRegex.FindString(remainingText)
		if dimAfterArticle != "" {
			parts := regexp.MustCompile(`[xх]`).Split(dimAfterArticle, 2)
			if len(parts) == 2 {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  "width",
					AttributeValue: parts[0],
					OriginalText:   dimAfterArticle,
					Confidence:     0.85,
				})
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  "height",
					AttributeValue: parts[1],
					OriginalText:   dimAfterArticle,
					Confidence:     0.85,
				})
			}
		}
		
		// Ищем единицы измерения после артикула (толщина, вес и т.д.)
		unitAfterArticle := n.numbersWithUnitsNoSpaceRegex.FindString(remainingText)
		if unitAfterArticle != "" {
			re := regexp.MustCompile(`(\d+\.?\d*)(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
			submatches := re.FindStringSubmatch(unitAfterArticle)
			if len(submatches) >= 3 {
				attrName := n.getAttributeNameByUnit(submatches[2])
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "numeric_value",
					AttributeName:  attrName,
					AttributeValue: submatches[1],
					Unit:           submatches[2],
					OriginalText:   unitAfterArticle,
					Confidence:     0.85,
				})
			}
		}
		
		// Ищем тип товара после артикула
		typeKeywords := []string{"панель", "лист", "профиль", "труба", "уголок", "швеллер", "panel", "sheet", "profile", "pipe", "angle", "channel"}
		for _, keyword := range typeKeywords {
			if strings.Contains(remainingText, keyword) {
				typeRegex := regexp.MustCompile(`\b(` + keyword + `[а-яa-z]*)\b`)
				typeMatch := typeRegex.FindString(remainingText)
				if typeMatch != "" {
					attributes = append(attributes, &database.ItemAttribute{
						AttributeType:  "text_value",
						AttributeName:  "type",
						AttributeValue: typeMatch,
						OriginalText:   typeMatch,
						Confidence:     0.8,
					})
					break
				}
			}
		}
		
	case "technical_code":
		// После технического кода обычно идут: характеристики, параметры
		// Ищем параметры (числа с единицами)
		paramsAfterCode := n.numbersWithUnitsNoSpaceRegex.FindAllString(remainingText, 3)
		for _, param := range paramsAfterCode {
			re := regexp.MustCompile(`(\d+\.?\d*)(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
			submatches := re.FindStringSubmatch(param)
			if len(submatches) >= 3 {
				attrName := n.getAttributeNameByUnit(submatches[2])
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "numeric_value",
					AttributeName:  attrName,
					AttributeValue: submatches[1],
					Unit:           submatches[2],
					OriginalText:   param,
					Confidence:     0.8,
				})
			}
		}
	}
	
	return attributes
}

// getAttributeNameByUnit определяет имя атрибута по единице измерения
func (n *NameNormalizer) getAttributeNameByUnit(unit string) string {
	unit = strings.ToLower(unit)
	
	// Длина/размер
	if unit == "mm" || unit == "мм" || unit == "cm" || unit == "см" || unit == "m" || unit == "м" {
		return "thickness"
	}
	
	// Вес
	if unit == "kg" || unit == "кг" || unit == "g" || unit == "г" || unit == "мг" {
		return "weight"
	}
	
	// Объем
	if unit == "l" || unit == "л" || unit == "ml" || unit == "мл" {
		return "volume"
	}
	
	// Мощность
	if unit == "w" || unit == "в" || unit == "watt" || unit == "kw" || unit == "квт" || unit == "вт" {
		return "power"
	}
	
	// Напряжение/ток
	if unit == "v" || unit == "в" || unit == "a" || unit == "а" {
		return "electrical"
	}
	
	// Время
	if unit == "h" || unit == "ч" || unit == "min" || unit == "мин" || unit == "sec" || unit == "сек" {
		return "duration"
	}
	
	// Количество
	if unit == "шт" {
		return "quantity"
	}
	
	// Процент
	if unit == "%" {
		return "percentage"
	}
	
	return "value"
}

// ExtractAttributesContextual извлекает атрибуты с учетом контекста (использует ContextualTokenizer)
// Этот метод более продвинутый, чем ExtractAttributes - он учитывает вложенность скобок и правильно обрабатывает сложные названия
//
// Пример:
//   input: "Кабель (медный (многожильный) 2x1.5мм) 100м"
//   Правильно извлечет: медный, многожильный, 2x1.5мм, 100м
func (n *NameNormalizer) ExtractAttributesContextual(name string) (string, []*database.ItemAttribute) {
	if name == "" {
		return "", nil
	}

	var attributes []*database.ItemAttribute
	tokenizer := NewContextualTokenizer()

	// 1. Извлекаем основной текст (depth=0) и вложенные атрибуты
	mainText, bracketedAttrs := tokenizer.SplitSmartly(name)

	// 2. Извлекаем ключ-значение пары из скобок
	keyValuePairs := tokenizer.ExtractKeyValuePairs(name)
	for key, value := range keyValuePairs {
		// Определяем тип атрибута
		attrType := "text_value"
		attrName := key

		// Проверяем, является ли значение числом с единицами
		re := regexp.MustCompile(`(\d+\.?\d*)\s*(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
		if matches := re.FindStringSubmatch(value); len(matches) >= 3 {
			attrType = "numeric_value"
			attrName = n.getAttributeNameByUnit(matches[2])

			attributes = append(attributes, &database.ItemAttribute{
				AttributeType:  attrType,
				AttributeName:  attrName,
				AttributeValue: matches[1],
				Unit:           matches[2],
				OriginalText:   value,
				Confidence:     0.9,
			})
			continue
		}

		// Проверяем, является ли это размером
		if n.dimensionRegex.MatchString(value) {
			parts := regexp.MustCompile(`[xх]`).Split(value, -1)
			for i, part := range parts {
				dimName := "dimension"
				if i == 0 {
					dimName = "width"
				} else if i == 1 {
					dimName = "height"
				} else if i == 2 {
					dimName = "depth"
				}

				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  dimName,
					AttributeValue: strings.TrimSpace(part),
					OriginalText:   value,
					Confidence:     0.9,
				})
			}
			continue
		}

		// Обычный текстовый атрибут
		attributes = append(attributes, &database.ItemAttribute{
			AttributeType:  attrType,
			AttributeName:  attrName,
			AttributeValue: value,
			OriginalText:   value,
			Confidence:     0.85,
		})
	}

	// 3. Анализируем атрибуты в скобках (не ключ-значение)
	for _, attr := range bracketedAttrs {
		// Пропускаем уже обработанные ключ-значение пары
		alreadyProcessed := false
		for key := range keyValuePairs {
			if strings.Contains(attr, key+":") || strings.Contains(attr, key+"=") {
				alreadyProcessed = true
				break
			}
		}
		if alreadyProcessed {
			continue
		}

		// Извлекаем размеры
		if n.dimensionRegex.MatchString(attr) {
			dimMatch := n.dimensionRegex.FindString(attr)
			parts := regexp.MustCompile(`[xх]`).Split(dimMatch, -1)
			for i, part := range parts {
				dimName := "dimension"
				if i == 0 {
					dimName = "width"
				} else if i == 1 {
					dimName = "height"
				} else if i == 2 {
					dimName = "depth"
				}

				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "dimension",
					AttributeName:  dimName,
					AttributeValue: strings.TrimSpace(part),
					OriginalText:   dimMatch,
					Confidence:     0.9,
				})
			}
		}

		// Извлекаем числа с единицами
		if n.numbersWithUnitsNoSpaceRegex.MatchString(attr) || n.numbersWithUnitsRegex.MatchString(attr) {
			re := regexp.MustCompile(`(\d+\.?\d*)\s*(mm|cm|m|kg|g|l|ml|w|a|v|watt|kw|h|min|sec|см|мм|м|л|кг|г|мг|шт|мл|в|а|вт|квт|ч|мин|сек)`)
			matches := re.FindAllStringSubmatch(attr, -1)
			for _, match := range matches {
				if len(match) >= 3 {
					attrName := n.getAttributeNameByUnit(match[2])
					attributes = append(attributes, &database.ItemAttribute{
						AttributeType:  "numeric_value",
						AttributeName:  attrName,
						AttributeValue: match[1],
						Unit:           match[2],
						OriginalText:   attr,
						Confidence:     0.9,
					})
				}
			}
		}

		// Извлекаем материалы
		materialKeywords := []string{"медный", "медн", "стальной", "сталь", "алюминиев", "пластиков", "деревян", "стеклян"}
		for _, keyword := range materialKeywords {
			if strings.Contains(strings.ToLower(attr), keyword) {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "text_value",
					AttributeName:  "material",
					AttributeValue: attr,
					OriginalText:   attr,
					Confidence:     0.8,
				})
				break
			}
		}

		// Извлекаем цвета
		colorKeywords := []string{"белый", "черн", "сер", "красн", "син", "зелен", "желт", "коричнев"}
		for _, keyword := range colorKeywords {
			if strings.Contains(strings.ToLower(attr), keyword) {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "text_value",
					AttributeName:  "color",
					AttributeValue: attr,
					OriginalText:   attr,
					Confidence:     0.8,
				})
				break
			}
		}

		// Извлекаем типы (многожильный, одножильный и т.д.)
		typeKeywords := []string{"многожильн", "одножильн", "двухжильн", "трехжильн"}
		for _, keyword := range typeKeywords {
			if strings.Contains(strings.ToLower(attr), keyword) {
				attributes = append(attributes, &database.ItemAttribute{
					AttributeType:  "text_value",
					AttributeName:  "type",
					AttributeValue: attr,
					OriginalText:   attr,
					Confidence:     0.85,
				})
				break
			}
		}
	}

	// 4. Нормализуем основной текст (используем стандартный метод)
	normalized := n.NormalizeName(mainText)

	return normalized, attributes
}

// ExtractAttributesWithPositional извлекает атрибуты используя позиционную схему
// Полезно для стандартизированных данных, где позиция токена определяет его смысл
func (n *NameNormalizer) ExtractAttributesWithPositional(name string, schemaName string) (string, []*database.ItemAttribute, error) {
	if name == "" {
		return "", nil, nil
	}

	// Получаем схему из реестра
	registry := NewSchemaRegistry()
	schema, err := registry.Get(schemaName)
	if err != nil {
		return "", nil, err
	}

	// Используем контекстный токенизатор для разделения по запятым
	tokenizer := NewContextualTokenizer()
	tokens := tokenizer.SplitByDelimiter(name, ',')

	// Извлекаем атрибуты по позициям
	attributes, errors := schema.ExtractByPosition(tokens)

	// Если есть критические ошибки, возвращаем их
	if len(errors) > 0 {
		return name, attributes, fmt.Errorf("extraction errors: %v", errors)
	}

	// Нормализуем название (удаляем извлеченные атрибуты)
	normalized := n.NormalizeName(name)

	return normalized, attributes, nil
}

// CompareExtractionMethods сравнивает результаты различных методов извлечения
// Полезно для оценки качества извлечения и выбора лучшего метода
func (n *NameNormalizer) CompareExtractionMethods(name string) map[string]interface{} {
	result := make(map[string]interface{})

	// Метод 1: Стандартный ExtractAttributes
	norm1, attrs1 := n.ExtractAttributes(name)
	result["standard"] = map[string]interface{}{
		"normalized":     norm1,
		"attributes":     attrs1,
		"count":          len(attrs1),
		"method":         "regex-based",
	}

	// Метод 2: Контекстный ExtractAttributesContextual
	norm2, attrs2 := n.ExtractAttributesContextual(name)
	result["contextual"] = map[string]interface{}{
		"normalized":     norm2,
		"attributes":     attrs2,
		"count":          len(attrs2),
		"method":         "contextual-tokenization",
	}

	// Сравнение
	result["comparison"] = map[string]interface{}{
		"standard_count":   len(attrs1),
		"contextual_count": len(attrs2),
		"difference":       len(attrs2) - len(attrs1),
		"recommendation":   n.recommendMethod(len(attrs1), len(attrs2), name),
	}

	return result
}

// recommendMethod рекомендует метод извлечения на основе результатов
func (n *NameNormalizer) recommendMethod(standardCount, contextualCount int, name string) string {
	// Если в названии есть скобки, контекстный метод лучше
	if strings.Contains(name, "(") && strings.Contains(name, ")") {
		return "contextual"
	}

	// Если контекстный метод нашел значительно больше атрибутов
	if contextualCount > standardCount+2 {
		return "contextual"
	}

	// Если результаты похожи, используем стандартный (быстрее)
	if abs(contextualCount-standardCount) <= 1 {
		return "standard"
	}

	// В остальных случаях - контекстный (более точный)
	return "contextual"
}

// abs возвращает абсолютное значение разности
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

