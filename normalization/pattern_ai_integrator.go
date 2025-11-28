package normalization

import (
	"fmt"
	"strings"
)

// PatternAIIntegrator интегратор паттернов с AI для предложения исправлений
type PatternAIIntegrator struct {
	patternDetector *PatternDetector
	aiNormalizer    *AINormalizer
}

// NewPatternAIIntegrator создает новый интегратор паттернов с AI
func NewPatternAIIntegrator(patternDetector *PatternDetector, aiNormalizer *AINormalizer) *PatternAIIntegrator {
	return &PatternAIIntegrator{
		patternDetector: patternDetector,
		aiNormalizer:    aiNormalizer,
	}
}

// PatternCorrectionResult результат предложения исправления
type PatternCorrectionResult struct {
	OriginalName    string        `json:"original_name"`
	DetectedPatterns []PatternMatch `json:"detected_patterns"`
	AlgorithmicFix  string        `json:"algorithmic_fix"`  // Исправление алгоритмическими правилами
	AISuggestedFix  string        `json:"ai_suggested_fix"`  // Предложение от AI
	FinalSuggestion string        `json:"final_suggestion"`  // Финальное предложение
	Confidence      float64       `json:"confidence"`
	Reasoning       string        `json:"reasoning"`
	RequiresReview  bool          `json:"requires_review"`   // Требует ли ручной проверки
}

// SuggestCorrectionWithAI предлагает исправление с использованием паттернов и AI
func (pai *PatternAIIntegrator) SuggestCorrectionWithAI(originalName string) (*PatternCorrectionResult, error) {
	result := &PatternCorrectionResult{
		OriginalName: originalName,
	}

	// 1. Обнаруживаем паттерны алгоритмически
	result.DetectedPatterns = pai.patternDetector.DetectPatterns(originalName)

	// 2. Применяем алгоритмические исправления
	result.AlgorithmicFix = pai.patternDetector.ApplyFixes(originalName, result.DetectedPatterns)

	// 3. Если есть паттерны, требующие AI обработки, или алгоритмическое исправление неполное
	needsAI := pai.shouldUseAI(result.DetectedPatterns, result.AlgorithmicFix, originalName)

	if needsAI && pai.aiNormalizer != nil {
		// Формируем промпт для AI с информацией о найденных паттернах
		aiPrompt := pai.buildAIPrompt(originalName, result.DetectedPatterns, result.AlgorithmicFix)
		
		// Получаем предложение от AI
		aiResult, err := pai.aiNormalizer.NormalizeWithAI(aiPrompt)
		if err == nil && aiResult != nil {
			result.AISuggestedFix = aiResult.NormalizedName
			result.Confidence = aiResult.Confidence
			result.Reasoning = fmt.Sprintf("AI предложение: %s. Найдено паттернов: %d", 
				aiResult.Reasoning, len(result.DetectedPatterns))
		} else {
			// Если AI не сработал, используем алгоритмическое исправление
			result.AISuggestedFix = result.AlgorithmicFix
			result.Confidence = 0.7
			result.Reasoning = fmt.Sprintf("AI недоступен, использованы алгоритмические правила. Найдено паттернов: %d", 
				len(result.DetectedPatterns))
		}
	} else {
		// Используем только алгоритмическое исправление
		result.AISuggestedFix = result.AlgorithmicFix
		result.Confidence = pai.calculateAlgorithmicConfidence(result.DetectedPatterns)
		result.Reasoning = fmt.Sprintf("Исправлено алгоритмически. Найдено паттернов: %d", 
			len(result.DetectedPatterns))
	}

	// 4. Определяем финальное предложение
	result.FinalSuggestion = pai.determineFinalSuggestion(result)

	// 5. Определяем, требуется ли ручная проверка
	result.RequiresReview = pai.requiresReview(result.DetectedPatterns, result.Confidence)

	return result, nil
}

// shouldUseAI определяет, нужно ли использовать AI
func (pai *PatternAIIntegrator) shouldUseAI(matches []PatternMatch, algorithmicFix string, original string) bool {
	// Используем AI если:
	// 1. Есть паттерны, которые не могут быть исправлены автоматически
	hasNonAutoFixable := false
	for _, match := range matches {
		if !match.AutoFixable {
			hasNonAutoFixable = true
			break
		}
	}

	// 2. Алгоритмическое исправление не сильно изменило строку (возможно, нужен более глубокий анализ)
	algorithmicChanged := strings.ToLower(strings.TrimSpace(algorithmicFix)) != strings.ToLower(strings.TrimSpace(original))

	// 3. Есть паттерны с низкой уверенностью
	hasLowConfidence := false
	for _, match := range matches {
		if match.Confidence < 0.7 {
			hasLowConfidence = true
			break
		}
	}

	return hasNonAutoFixable || !algorithmicChanged || hasLowConfidence || len(matches) > 3
}

// buildAIPrompt строит промпт для AI с информацией о паттернах
func (pai *PatternAIIntegrator) buildAIPrompt(originalName string, matches []PatternMatch, algorithmicFix string) string {
	var prompt strings.Builder
	
	prompt.WriteString(fmt.Sprintf("НАИМЕНОВАНИЕ ТОВАРА: \"%s\"\n\n", originalName))
	
	if len(matches) > 0 {
		prompt.WriteString("ОБНАРУЖЕННЫЕ ПРОБЛЕМЫ:\n")
		for i, match := range matches {
			prompt.WriteString(fmt.Sprintf("%d. [%s] %s: '%s' (уверенность: %.0f%%)\n",
				i+1, match.Severity, match.Description, match.MatchedText, match.Confidence*100))
		}
		prompt.WriteString("\n")
	}

	if algorithmicFix != originalName {
		prompt.WriteString(fmt.Sprintf("АЛГОРИТМИЧЕСКОЕ ИСПРАВЛЕНИЕ: \"%s\"\n\n", algorithmicFix))
	}

	prompt.WriteString("ЗАДАЧА: Предложи улучшенное нормализованное наименование, учитывая найденные проблемы.\n")
	prompt.WriteString("Если алгоритмическое исправление корректно, можешь его использовать или улучшить.\n")
	prompt.WriteString("Убедись, что результат:\n")
	prompt.WriteString("- Не содержит технических кодов, артикулов, размеров\n")
	prompt.WriteString("- Имеет правильный регистр\n")
	prompt.WriteString("- Не содержит лишних пробелов и специальных символов\n")
	prompt.WriteString("- Понятен и стандартизирован\n")

	return prompt.String()
}

// calculateAlgorithmicConfidence вычисляет уверенность алгоритмического исправления
func (pai *PatternAIIntegrator) calculateAlgorithmicConfidence(matches []PatternMatch) float64 {
	if len(matches) == 0 {
		return 1.0
	}

	totalConfidence := 0.0
	for _, match := range matches {
		if match.AutoFixable {
			totalConfidence += match.Confidence
		}
	}

	avgConfidence := totalConfidence / float64(len(matches))
	
	// Если все паттерны автоприменяемые, уверенность выше
	allAutoFixable := true
	for _, match := range matches {
		if !match.AutoFixable {
			allAutoFixable = false
			break
		}
	}

	if allAutoFixable {
		return avgConfidence * 0.95 // Немного снижаем, так как алгоритмические правила не идеальны
	}

	return avgConfidence * 0.8 // Еще больше снижаем, если есть неавтоприменяемые
}

// determineFinalSuggestion определяет финальное предложение
func (pai *PatternAIIntegrator) determineFinalSuggestion(result *PatternCorrectionResult) string {
	// Если AI предложение есть и уверенность высокая, используем его
	if result.AISuggestedFix != "" && result.Confidence >= 0.8 {
		return result.AISuggestedFix
	}

	// Если AI предложение есть, но уверенность средняя, сравниваем с алгоритмическим
	if result.AISuggestedFix != "" && result.Confidence >= 0.6 {
		// Если AI предложение сильно отличается от алгоритмического, предпочитаем AI
		if strings.ToLower(strings.TrimSpace(result.AISuggestedFix)) != 
		   strings.ToLower(strings.TrimSpace(result.AlgorithmicFix)) {
			return result.AISuggestedFix
		}
	}

	// В остальных случаях используем алгоритмическое исправление
	return result.AlgorithmicFix
}

// requiresReview определяет, требуется ли ручная проверка
func (pai *PatternAIIntegrator) requiresReview(matches []PatternMatch, confidence float64) bool {
	// Требуется проверка если:
	// 1. Низкая уверенность
	if confidence < 0.7 {
		return true
	}

	// 2. Есть критические паттерны
	for _, match := range matches {
		if match.Severity == "critical" || match.Severity == "high" {
			return true
		}
		if !match.AutoFixable {
			return true
		}
	}

	// 3. Много паттернов
	if len(matches) > 5 {
		return true
	}

	return false
}

// BatchSuggestCorrections обрабатывает несколько названий пакетно
func (pai *PatternAIIntegrator) BatchSuggestCorrections(names []string) ([]*PatternCorrectionResult, error) {
	results := make([]*PatternCorrectionResult, 0, len(names))

	for _, name := range names {
		result, err := pai.SuggestCorrectionWithAI(name)
		if err != nil {
			// Продолжаем обработку даже при ошибках
			result = &PatternCorrectionResult{
				OriginalName:    name,
				AlgorithmicFix:  name,
				FinalSuggestion: name,
				Confidence:      0.0,
				Reasoning:       fmt.Sprintf("Ошибка обработки: %v", err),
				RequiresReview:  true,
			}
		}
		results = append(results, result)
	}

	return results, nil
}

