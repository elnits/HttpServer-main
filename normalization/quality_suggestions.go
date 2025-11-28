package normalization

import (
	"fmt"
	"strings"
)

// SuggestionType тип предложения по улучшению
type SuggestionType string

const (
	SuggestionTypeSetValue      SuggestionType = "set_value"       // Установить значение
	SuggestionTypeCorrectFormat SuggestionType = "correct_format"  // Исправить формат
	SuggestionTypeReprocess     SuggestionType = "reprocess"       // Повторно обработать
	SuggestionTypeMerge         SuggestionType = "merge"           // Объединить с другой записью
	SuggestionTypeReview        SuggestionType = "review"          // Требует ручной проверки
)

// SuggestionPriority приоритет предложения
type SuggestionPriority string

const (
	PriorityLow      SuggestionPriority = "low"      // Низкий
	PriorityMedium   SuggestionPriority = "medium"   // Средний
	PriorityHigh     SuggestionPriority = "high"     // Высокий
	PriorityCritical SuggestionPriority = "critical" // Критический
)

// Suggestion предложение по улучшению качества
type Suggestion struct {
	ID             int                `json:"id"`              // ID предложения (генерируется при сохранении)
	ItemID         int                `json:"item_id"`         // ID записи
	Type           SuggestionType     `json:"type"`            // Тип предложения
	Priority       SuggestionPriority `json:"priority"`        // Приоритет
	Field          string             `json:"field"`           // Поле для изменения
	CurrentValue   string             `json:"current_value"`   // Текущее значение
	SuggestedValue string             `json:"suggested_value"` // Предлагаемое значение
	Confidence     float64            `json:"confidence"`      // Уверенность (0-1)
	Reasoning      string             `json:"reasoning"`       // Обоснование предложения
	AutoApplyable  bool               `json:"auto_applyable"`  // Можно ли применить автоматически
	Applied        bool               `json:"applied"`         // Применено ли
}

// SuggestionEngine движок генерации предложений
type SuggestionEngine struct {
	validator *QualityValidator
}

// NewSuggestionEngine создает новый движок предложений
func NewSuggestionEngine() *SuggestionEngine {
	return &SuggestionEngine{
		validator: NewQualityValidator(),
	}
}

// GenerateSuggestions генерирует предложения на основе violations
func (se *SuggestionEngine) GenerateSuggestions(data ItemData, violations []Violation) []Suggestion {
	var suggestions []Suggestion

	for _, violation := range violations {
		suggestion := se.createSuggestionFromViolation(data, violation)
		if suggestion != nil {
			suggestions = append(suggestions, *suggestion)
		}
	}

	// Генерируем дополнительные проактивные предложения
	proactiveSuggestions := se.generateProactiveSuggestions(data)
	suggestions = append(suggestions, proactiveSuggestions...)

	return suggestions
}

// createSuggestionFromViolation создает предложение на основе нарушения
func (se *SuggestionEngine) createSuggestionFromViolation(data ItemData, violation Violation) *Suggestion {
	switch violation.RuleName {
	case "require_kpved_code":
		return &Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReprocess,
			Priority:       PriorityHigh,
			Field:          "kpved_code",
			CurrentValue:   "",
			SuggestedValue: "Запустить классификацию КПВЭД",
			Confidence:     0.8,
			Reasoning:      "Отсутствует код КПВЭД. Рекомендуется выполнить классификацию",
			AutoApplyable:  true,
		}

	case "valid_kpved_format":
		corrected := suggestKpvedFormat(data.KpvedCode)
		return &Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeCorrectFormat,
			Priority:       PriorityMedium,
			Field:          "kpved_code",
			CurrentValue:   data.KpvedCode,
			SuggestedValue: corrected,
			Confidence:     0.6,
			Reasoning:      "Некорректный формат кода КПВЭД. Предлагается исправленный формат",
			AutoApplyable:  false, // Требует проверки
		}

	case "category_other":
		return &Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReprocess,
			Priority:       PriorityMedium,
			Field:          "category",
			CurrentValue:   data.Category,
			SuggestedValue: "Повторно классифицировать с помощью AI",
			Confidence:     0.7,
			Reasoning:      "Категория 'другое' означает низкую уверенность. Рекомендуется AI-классификация",
			AutoApplyable:  true,
		}

	case "name_length":
		if len([]rune(data.NormalizedName)) > 100 {
			trimmed := trimName(data.NormalizedName, 100)
			return &Suggestion{
				ItemID:         data.ID,
				Type:           SuggestionTypeCorrectFormat,
				Priority:       PriorityLow,
				Field:          "normalized_name",
				CurrentValue:   data.NormalizedName,
				SuggestedValue: trimmed,
				Confidence:     0.5,
				Reasoning:      "Слишком длинное имя. Предлагается сокращенная версия",
				AutoApplyable:  false,
			}
		}

	case "kpved_confidence_threshold":
		return &Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReview,
			Priority:       PriorityMedium,
			Field:          "kpved_code",
			CurrentValue:   data.KpvedCode,
			SuggestedValue: "Требуется ручная проверка",
			Confidence:     0.9,
			Reasoning:      fmt.Sprintf("Низкая уверенность КПВЭД (%.1f%%). Рекомендуется ручная проверка", data.KpvedConfidence*100),
			AutoApplyable:  false,
		}

	case "ai_confidence_threshold":
		return &Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReprocess,
			Priority:       PriorityLow,
			Field:          "category",
			CurrentValue:   data.Category,
			SuggestedValue: "Повторно обработать с AI",
			Confidence:     0.6,
			Reasoning:      "Низкая AI уверенность. Можно попробовать повторную обработку",
			AutoApplyable:  true,
		}
	}

	return nil
}

// generateProactiveSuggestions генерирует проактивные предложения
func (se *SuggestionEngine) generateProactiveSuggestions(data ItemData) []Suggestion {
	var suggestions []Suggestion

	// Если basic уровень - предложить AI enhancement
	if data.ProcessingLevel == "basic" {
		suggestions = append(suggestions, Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReprocess,
			Priority:       PriorityMedium,
			Field:          "processing_level",
			CurrentValue:   "basic",
			SuggestedValue: "ai_enhanced",
			Confidence:     0.8,
			Reasoning:      "Запись обработана на базовом уровне. AI может улучшить качество",
			AutoApplyable:  true,
		})
	}

	// Если нет КПВЭД кода
	if data.KpvedCode == "" && data.ProcessingLevel != "basic" {
		suggestions = append(suggestions, Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeReprocess,
			Priority:       PriorityHigh,
			Field:          "kpved_code",
			CurrentValue:   "",
			SuggestedValue: "Выполнить классификацию КПВЭД",
			Confidence:     0.9,
			Reasoning:      "Отсутствует код КПВЭД. Настоятельно рекомендуется классификация",
			AutoApplyable:  true,
		})
	}

	// Если имя содержит подозрительные паттерны
	if containsSuspiciousPatterns(data.NormalizedName) {
		cleaned := cleanSuspiciousPatterns(data.NormalizedName)
		suggestions = append(suggestions, Suggestion{
			ItemID:         data.ID,
			Type:           SuggestionTypeCorrectFormat,
			Priority:       PriorityMedium,
			Field:          "normalized_name",
			CurrentValue:   data.NormalizedName,
			SuggestedValue: cleaned,
			Confidence:     0.7,
			Reasoning:      "Имя содержит артикулы или лишние коды. Предлагается очищенная версия",
			AutoApplyable:  false,
		})
	}

	return suggestions
}

// PrioritizeSuggestions сортирует предложения по приоритету и confidence
func (se *SuggestionEngine) PrioritizeSuggestions(suggestions []Suggestion) []Suggestion {
	prioritized := make([]Suggestion, len(suggestions))
	copy(prioritized, suggestions)

	// Простая сортировка по приоритету (можно улучшить)
	priorityOrder := map[SuggestionPriority]int{
		PriorityCritical: 4,
		PriorityHigh:     3,
		PriorityMedium:   2,
		PriorityLow:      1,
	}

	// Bubble sort для простоты (в продакшене использовать sort.Slice)
	n := len(prioritized)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			score1 := float64(priorityOrder[prioritized[j].Priority]) + prioritized[j].Confidence
			score2 := float64(priorityOrder[prioritized[j+1].Priority]) + prioritized[j+1].Confidence

			if score2 > score1 {
				prioritized[j], prioritized[j+1] = prioritized[j+1], prioritized[j]
			}
		}
	}

	return prioritized
}

// GetAutoApplyableSuggestions возвращает только автоприменяемые предложения
func (se *SuggestionEngine) GetAutoApplyableSuggestions(suggestions []Suggestion) []Suggestion {
	var autoApplyable []Suggestion

	for _, suggestion := range suggestions {
		if suggestion.AutoApplyable && suggestion.Confidence >= 0.8 {
			autoApplyable = append(autoApplyable, suggestion)
		}
	}

	return autoApplyable
}

// EstimateImpact оценивает потенциальное улучшение качества
func (se *SuggestionEngine) EstimateImpact(suggestion Suggestion, currentQuality float64) float64 {
	// Оценка потенциального улучшения в зависимости от типа
	baseImpact := map[SuggestionType]float64{
		SuggestionTypeSetValue:      0.05,  // +5%
		SuggestionTypeCorrectFormat: 0.03,  // +3%
		SuggestionTypeReprocess:     0.15,  // +15%
		SuggestionTypeMerge:         0.10,  // +10%
		SuggestionTypeReview:        0.08,  // +8%
	}

	impact := baseImpact[suggestion.Type]

	// Модификатор от приоритета
	priorityModifier := map[SuggestionPriority]float64{
		PriorityCritical: 1.5,
		PriorityHigh:     1.3,
		PriorityMedium:   1.0,
		PriorityLow:      0.7,
	}

	impact *= priorityModifier[suggestion.Priority]

	// Модификатор от confidence
	impact *= suggestion.Confidence

	// Не может улучшить выше 1.0
	if currentQuality+impact > 1.0 {
		impact = 1.0 - currentQuality
	}

	return impact
}

// --- Вспомогательные функции ---

// suggestKpvedFormat пытается исправить формат КПВЭД кода
func suggestKpvedFormat(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, ",", ".")
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", ".")

	// Удаляем все кроме цифр и точек
	cleaned := ""
	for _, r := range code {
		if r >= '0' && r <= '9' || r == '.' {
			cleaned += string(r)
		}
	}

	// Пытаемся разбить на части
	parts := strings.Split(cleaned, ".")
	var formatted []string

	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		// Добавляем ведущий ноль если нужно
		if len(part) == 1 {
			formatted = append(formatted, "0"+part)
		} else if len(part) >= 2 {
			formatted = append(formatted, part[:2])
		}
	}

	if len(formatted) >= 2 {
		return strings.Join(formatted, ".")
	}

	return code // Возвращаем как есть, если не удалось исправить
}

// trimName сокращает имя до указанной длины
func trimName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}

	// Обрезаем по словам
	words := strings.Fields(name)
	result := ""
	for _, word := range words {
		if len([]rune(result+" "+word)) > maxLen {
			break
		}
		if result != "" {
			result += " "
		}
		result += word
	}

	if result == "" {
		// Если не удалось по словам, просто обрезаем
		return string(runes[:maxLen])
	}

	return result
}

// containsSuspiciousPatterns проверяет наличие подозрительных паттернов в имени
func containsSuspiciousPatterns(name string) bool {
	nameLower := strings.ToLower(name)

	suspiciousPatterns := []string{
		"арт.",
		"арт:",
		"артикул",
		"код:",
		"код ",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}

	return false
}

// cleanSuspiciousPatterns удаляет подозрительные паттерны из имени
func cleanSuspiciousPatterns(name string) string {
	patterns := map[string]string{
		"арт.":    "",
		"арт:":    "",
		"артикул": "",
		"код:":    "",
	}

	cleaned := name
	for pattern, replacement := range patterns {
		cleaned = strings.ReplaceAll(strings.ToLower(cleaned), pattern, replacement)
	}

	// Удаляем лишние пробелы
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}
