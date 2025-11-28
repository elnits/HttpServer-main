package normalization

import (
	"strings"
	"unicode"
)

// TokenType определяет тип токена
type TokenType string

const (
	TokenText         TokenType = "text"
	TokenNumber       TokenType = "number"
	TokenBracketOpen  TokenType = "bracket_open"
	TokenBracketClose TokenType = "bracket_close"
	TokenDelimiter    TokenType = "delimiter"
	TokenQuote        TokenType = "quote"
	TokenWhitespace   TokenType = "whitespace"
)

// Token представляет отдельный токен в разобранном тексте
type Token struct {
	Type     TokenType // Тип токена
	Value    string    // Значение токена
	Depth    int       // Уровень вложенности (глубина скобок)
	Position int       // Позиция в исходной строке
	Length   int       // Длина токена
}

// StatefulParser парсер с отслеживанием состояния (аналог Python PFFParser)
// Основные возможности:
// - Отслеживание глубины вложенности скобок (depth)
// - Отслеживание нахождения внутри строки (inString)
// - Обработка escape-последовательностей (escapeNext)
// - Посимвольная обработка с учетом контекста
type StatefulParser struct {
	depth        int     // Текущая глубина вложенности скобок
	inString     bool    // Находимся ли внутри строки в кавычках
	escapeNext   bool    // Следующий символ экранирован
	position     int     // Текущая позиция в строке
	tokens       []Token // Собранные токены
	currentToken strings.Builder
	tokenStart   int
	tokenDepth   int
	tokenType    TokenType
}

// NewStatefulParser создает новый stateful парсер
func NewStatefulParser() *StatefulParser {
	return &StatefulParser{
		tokens: make([]Token, 0),
	}
}

// Reset сбрасывает состояние парсера
func (sp *StatefulParser) Reset() {
	sp.depth = 0
	sp.inString = false
	sp.escapeNext = false
	sp.position = 0
	sp.tokens = sp.tokens[:0]
	sp.currentToken.Reset()
	sp.tokenStart = 0
	sp.tokenDepth = 0
	sp.tokenType = TokenText
}

// ParseCharByChar выполняет посимвольный парсинг строки
// Возвращает массив токенов с метаданными (тип, позиция, глубина)
func (sp *StatefulParser) ParseCharByChar(input string) []Token {
	sp.Reset()

	for i, char := range input {
		sp.position = i
		sp.processChar(char)
	}

	// Финализируем последний токен
	sp.finalizeCurrentToken()

	return sp.tokens
}

// processChar обрабатывает один символ с учетом текущего состояния
func (sp *StatefulParser) processChar(char rune) {
	// Обработка escape-последовательностей
	if sp.escapeNext {
		sp.addToCurrentToken(char)
		sp.escapeNext = false
		return
	}

	// Обработка escape-символа
	if char == '\\' && sp.inString {
		sp.escapeNext = true
		sp.addToCurrentToken(char)
		return
	}

	// Обработка кавычек
	if char == '"' {
		sp.finalizeCurrentToken()
		sp.inString = !sp.inString
		sp.createToken(TokenQuote, string(char))
		return
	}

	// Внутри строки все символы идут в текущий токен
	if sp.inString {
		sp.addToCurrentToken(char)
		return
	}

	// Обработка открывающих скобок (только вне строк)
	if char == '(' || char == '[' || char == '{' {
		sp.finalizeCurrentToken()
		sp.depth++
		sp.createToken(TokenBracketOpen, string(char))
		return
	}

	// Обработка закрывающих скобок (только вне строк)
	if char == ')' || char == ']' || char == '}' {
		sp.finalizeCurrentToken()
		sp.depth--
		sp.createToken(TokenBracketClose, string(char))
		return
	}

	// Обработка разделителей (только вне строк)
	if char == ',' || char == ';' {
		sp.finalizeCurrentToken()
		sp.createToken(TokenDelimiter, string(char))
		return
	}

	// Обработка пробельных символов (только вне строк)
	if unicode.IsSpace(char) {
		sp.finalizeCurrentToken()
		sp.createToken(TokenWhitespace, string(char))
		return
	}

	// Все остальные символы добавляем к текущему токену
	sp.addToCurrentToken(char)
}

// addToCurrentToken добавляет символ к текущему токену
func (sp *StatefulParser) addToCurrentToken(char rune) {
	if sp.currentToken.Len() == 0 {
		sp.tokenStart = sp.position
		sp.tokenDepth = sp.depth

		// Определяем тип токена
		if unicode.IsDigit(char) {
			sp.tokenType = TokenNumber
		} else {
			sp.tokenType = TokenText
		}
	}

	// Уточняем тип токена
	if sp.tokenType == TokenNumber && !unicode.IsDigit(char) && char != '.' && char != ',' {
		sp.tokenType = TokenText
	}

	sp.currentToken.WriteRune(char)
}

// finalizeCurrentToken финализирует текущий токен и добавляет в массив
func (sp *StatefulParser) finalizeCurrentToken() {
	if sp.currentToken.Len() > 0 {
		value := sp.currentToken.String()
		sp.tokens = append(sp.tokens, Token{
			Type:     sp.tokenType,
			Value:    value,
			Depth:    sp.tokenDepth,
			Position: sp.tokenStart,
			Length:   len(value),
		})
		sp.currentToken.Reset()
	}
}

// createToken создает и добавляет токен (для разделителей, скобок и т.д.)
func (sp *StatefulParser) createToken(tokenType TokenType, value string) {
	sp.tokens = append(sp.tokens, Token{
		Type:     tokenType,
		Value:    value,
		Depth:    sp.depth,
		Position: sp.position,
		Length:   len(value),
	})
}

// GetTokensByDepth возвращает токены определенной глубины
func (sp *StatefulParser) GetTokensByDepth(depth int) []Token {
	result := make([]Token, 0)
	for _, token := range sp.tokens {
		if token.Depth == depth {
			result = append(result, token)
		}
	}
	return result
}

// GetTokensByType возвращает токены определенного типа
func (sp *StatefulParser) GetTokensByType(tokenType TokenType) []Token {
	result := make([]Token, 0)
	for _, token := range sp.tokens {
		if token.Type == tokenType {
			result = append(result, token)
		}
	}
	return result
}

// GetTextTokensAtDepth возвращает текстовые токены на определенной глубине
func (sp *StatefulParser) GetTextTokensAtDepth(depth int) []string {
	result := make([]string, 0)
	for _, token := range sp.tokens {
		if token.Depth == depth && (token.Type == TokenText || token.Type == TokenNumber) {
			result = append(result, token.Value)
		}
	}
	return result
}

// GetMaxDepth возвращает максимальную глубину вложенности
func (sp *StatefulParser) GetMaxDepth() int {
	maxDepth := 0
	for _, token := range sp.tokens {
		if token.Depth > maxDepth {
			maxDepth = token.Depth
		}
	}
	return maxDepth
}

// ReconstructText восстанавливает текст из токенов
func (sp *StatefulParser) ReconstructText() string {
	var builder strings.Builder
	for _, token := range sp.tokens {
		builder.WriteString(token.Value)
	}
	return builder.String()
}

// FilterTokens возвращает токены, удовлетворяющие условию
func (sp *StatefulParser) FilterTokens(predicate func(Token) bool) []Token {
	result := make([]Token, 0)
	for _, token := range sp.tokens {
		if predicate(token) {
			result = append(result, token)
		}
	}
	return result
}
