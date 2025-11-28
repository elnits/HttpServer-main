package normalization

import (
	"strings"
)

// ContextualTokenizer выполняет токенизацию с учетом контекста
// Основные возможности:
// - Разделение по разделителям только на depth=0
// - Извлечение вложенных структур по уровням
// - Игнорирование разделителей внутри кавычек/скобок
type ContextualTokenizer struct {
	parser *StatefulParser
}

// NewContextualTokenizer создает новый контекстный токенизатор
func NewContextualTokenizer() *ContextualTokenizer {
	return &ContextualTokenizer{
		parser: NewStatefulParser(),
	}
}

// SplitByDelimiter разделяет строку по разделителю только на depth=0
// Игнорирует разделители внутри скобок и кавычек
//
// Пример:
//   input: "Панель (белая, глянцевая), арт.123"
//   delimiter: ','
//   result: ["Панель (белая, глянцевая)", "арт.123"]
func (ct *ContextualTokenizer) SplitByDelimiter(input string, delimiter rune) []string {
	tokens := ct.parser.ParseCharByChar(input)

	var parts []string
	var currentPart strings.Builder
	delimiterStr := string(delimiter)

	for _, token := range tokens {
		// Разделяем только на depth=0 и если это нужный разделитель
		if token.Type == TokenDelimiter && token.Depth == 0 && token.Value == delimiterStr {
			if currentPart.Len() > 0 {
				parts = append(parts, strings.TrimSpace(currentPart.String()))
				currentPart.Reset()
			}
		} else {
			currentPart.WriteString(token.Value)
		}
	}

	// Добавляем последнюю часть
	if currentPart.Len() > 0 {
		parts = append(parts, strings.TrimSpace(currentPart.String()))
	}

	return parts
}

// SplitByMultipleDelimiters разделяет по нескольким разделителям
func (ct *ContextualTokenizer) SplitByMultipleDelimiters(input string, delimiters []rune) []string {
	tokens := ct.parser.ParseCharByChar(input)

	delimiterMap := make(map[string]bool)
	for _, d := range delimiters {
		delimiterMap[string(d)] = true
	}

	var parts []string
	var currentPart strings.Builder

	for _, token := range tokens {
		// Разделяем только на depth=0 и если это один из разделителей
		if token.Type == TokenDelimiter && token.Depth == 0 && delimiterMap[token.Value] {
			if currentPart.Len() > 0 {
				parts = append(parts, strings.TrimSpace(currentPart.String()))
				currentPart.Reset()
			}
		} else {
			currentPart.WriteString(token.Value)
		}
	}

	// Добавляем последнюю часть
	if currentPart.Len() > 0 {
		parts = append(parts, strings.TrimSpace(currentPart.String()))
	}

	return parts
}

// ExtractNestedAttributes извлекает атрибуты, сгруппированные по уровням вложенности
//
// Пример:
//   input: "Кабель (медный (многожильный) 2x1.5мм) 100м"
//   result: map[0]->["Кабель", "100м"], map[1]->["медный", "2x1.5мм"], map[2]->["многожильный"]
func (ct *ContextualTokenizer) ExtractNestedAttributes(input string) map[int][]string {
	tokens := ct.parser.ParseCharByChar(input)

	// Группируем по глубине
	byDepth := make(map[int][]string)
	var currentText strings.Builder
	var currentDepth int

	for i, token := range tokens {
		if token.Type == TokenBracketOpen {
			// Сохраняем текущий текст до открытия скобки
			if currentText.Len() > 0 {
				text := strings.TrimSpace(currentText.String())
				if text != "" {
					byDepth[currentDepth] = append(byDepth[currentDepth], text)
				}
				currentText.Reset()
			}

			// Увеличиваем глубину для следующих токенов
			if i+1 < len(tokens) {
				currentDepth = tokens[i+1].Depth
			}
		} else if token.Type == TokenBracketClose {
			// Сохраняем текущий текст внутри скобок
			if currentText.Len() > 0 {
				text := strings.TrimSpace(currentText.String())
				if text != "" {
					byDepth[currentDepth] = append(byDepth[currentDepth], text)
				}
				currentText.Reset()
			}

			// Возвращаемся на предыдущую глубину
			currentDepth = token.Depth
		} else if token.Type == TokenText || token.Type == TokenNumber {
			currentText.WriteString(token.Value)
		} else if token.Type == TokenWhitespace {
			if currentText.Len() > 0 {
				currentText.WriteString(token.Value)
			}
		} else if token.Type == TokenDelimiter {
			// Разделитель внутри скобок - сохраняем текст и начинаем новый
			if currentText.Len() > 0 {
				text := strings.TrimSpace(currentText.String())
				if text != "" {
					byDepth[currentDepth] = append(byDepth[currentDepth], text)
				}
				currentText.Reset()
			}
		}
	}

	// Сохраняем оставшийся текст
	if currentText.Len() > 0 {
		text := strings.TrimSpace(currentText.String())
		if text != "" {
			byDepth[currentDepth] = append(byDepth[currentDepth], text)
		}
	}

	return byDepth
}

// ExtractAttributesInBrackets извлекает все атрибуты, находящиеся внутри скобок
//
// Пример:
//   input: "Труба (диаметр 100мм, длина 6м) стальная"
//   result: ["диаметр 100мм", "длина 6м"]
func (ct *ContextualTokenizer) ExtractAttributesInBrackets(input string) []string {
	nestedAttrs := ct.ExtractNestedAttributes(input)

	var result []string
	// Извлекаем все атрибуты из вложенных уровней (depth > 0)
	for depth := 1; depth <= 10; depth++ { // Максимум 10 уровней вложенности
		if attrs, exists := nestedAttrs[depth]; exists {
			result = append(result, attrs...)
		}
	}

	return result
}

// ExtractMainText извлекает основной текст (depth=0), игнорируя содержимое скобок
//
// Пример:
//   input: "Панель (белая, 100x200) декоративная (матовая)"
//   result: "Панель декоративная"
func (ct *ContextualTokenizer) ExtractMainText(input string) string {
	tokens := ct.parser.ParseCharByChar(input)

	var mainText strings.Builder
	lastWasWhitespace := false

	for _, token := range tokens {
		// Берем только токены на уровне depth=0
		if token.Depth == 0 && token.Type != TokenBracketOpen && token.Type != TokenBracketClose {
			if token.Type == TokenWhitespace {
				if !lastWasWhitespace && mainText.Len() > 0 {
					mainText.WriteRune(' ')
					lastWasWhitespace = true
				}
			} else {
				mainText.WriteString(token.Value)
				lastWasWhitespace = false
			}
		}
	}

	return strings.TrimSpace(mainText.String())
}

// SplitSmartly выполняет умное разделение строки
// Возвращает основной текст и массив вложенных атрибутов
//
// Пример:
//   input: "Кабель (медный, 2x1.5мм) 100м"
//   result: mainText="Кабель 100м", attributes=["медный", "2x1.5мм"]
func (ct *ContextualTokenizer) SplitSmartly(input string) (mainText string, attributes []string) {
	mainText = ct.ExtractMainText(input)
	attributes = ct.ExtractAttributesInBrackets(input)
	return
}

// ExtractKeyValuePairs извлекает пары ключ-значение из вложенных атрибутов
// Поддерживает форматы: "ключ: значение", "ключ=значение", "ключ значение"
//
// Пример:
//   input: "Товар (цвет: белый, размер=100мм, вес 5кг)"
//   result: map["цвет"]->"белый", map["размер"]->"100мм", map["вес"]->"5кг"
func (ct *ContextualTokenizer) ExtractKeyValuePairs(input string) map[string]string {
	attributes := ct.ExtractAttributesInBrackets(input)

	result := make(map[string]string)

	for _, attr := range attributes {
		// Проверяем различные разделители
		var key, value string

		if strings.Contains(attr, ":") {
			parts := strings.SplitN(attr, ":", 2)
			if len(parts) == 2 {
				key = strings.TrimSpace(parts[0])
				value = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(attr, "=") {
			parts := strings.SplitN(attr, "=", 2)
			if len(parts) == 2 {
				key = strings.TrimSpace(parts[0])
				value = strings.TrimSpace(parts[1])
			}
		} else {
			// Пытаемся разделить по первому пробелу
			parts := strings.Fields(attr)
			if len(parts) >= 2 {
				key = parts[0]
				value = strings.Join(parts[1:], " ")
			} else if len(parts) == 1 {
				// Если только одно слово, считаем его значением с пустым ключом
				value = parts[0]
			}
		}

		if key != "" && value != "" {
			result[strings.ToLower(key)] = value
		} else if value != "" {
			// Если есть только значение без ключа, используем индекс
			result["attr_"+strings.TrimSpace(attr)] = value
		}
	}

	return result
}

// RemoveBracketedContent удаляет все содержимое в скобках
//
// Пример:
//   input: "Панель (белая) декоративная (100x200)"
//   result: "Панель декоративная"
func (ct *ContextualTokenizer) RemoveBracketedContent(input string) string {
	return ct.ExtractMainText(input)
}

// GetTokensByDepthRange возвращает токены в указанном диапазоне глубины
func (ct *ContextualTokenizer) GetTokensByDepthRange(input string, minDepth, maxDepth int) []Token {
	tokens := ct.parser.ParseCharByChar(input)

	result := make([]Token, 0)
	for _, token := range tokens {
		if token.Depth >= minDepth && token.Depth <= maxDepth {
			result = append(result, token)
		}
	}

	return result
}

// AnalyzeStructure анализирует структуру строки и возвращает статистику
type StructureInfo struct {
	TotalTokens      int
	TextTokens       int
	NumberTokens     int
	BracketPairs     int
	MaxDepth         int
	HasQuotes        bool
	DelimiterCount   int
	DepthDistribution map[int]int // Количество токенов на каждой глубине
}

func (ct *ContextualTokenizer) AnalyzeStructure(input string) *StructureInfo {
	tokens := ct.parser.ParseCharByChar(input)

	info := &StructureInfo{
		DepthDistribution: make(map[int]int),
	}

	openBrackets := 0

	for _, token := range tokens {
		info.TotalTokens++

		switch token.Type {
		case TokenText:
			info.TextTokens++
		case TokenNumber:
			info.NumberTokens++
		case TokenBracketOpen:
			openBrackets++
		case TokenBracketClose:
			info.BracketPairs++
		case TokenQuote:
			info.HasQuotes = true
		case TokenDelimiter:
			info.DelimiterCount++
		}

		info.DepthDistribution[token.Depth]++

		if token.Depth > info.MaxDepth {
			info.MaxDepth = token.Depth
		}
	}

	return info
}
