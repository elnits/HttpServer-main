package normalization

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"httpserver/database"
)

// ValidationSeverity уровень серьезности ошибки
type ValidationSeverity string

const (
	ValidationSeverityCritical ValidationSeverity = "critical" // Критическая ошибка, блокирует обработку
	ValidationSeverityHigh     ValidationSeverity = "high"     // Высокая, рекомендуется исправить
	ValidationSeverityMedium   ValidationSeverity = "medium"   // Средняя, желательно исправить
	ValidationSeverityLow      ValidationSeverity = "low"      // Низкая, можно игнорировать
)

// ValidationError ошибка валидации
type ValidationError struct {
	ItemID       int                `json:"item_id"`
	ItemName     string             `json:"item_name"`
	ItemCode     string             `json:"item_code"`
	ErrorType    string             `json:"error_type"`
	ErrorMessage string             `json:"error_message"`
	Severity     ValidationSeverity `json:"severity"`
	FieldName    string             `json:"field_name,omitempty"`
	ExpectedValue string            `json:"expected_value,omitempty"`
	ActualValue  string             `json:"actual_value,omitempty"`
	Timestamp    time.Time          `json:"timestamp"`
}

// ValidationWarning предупреждение валидации
type ValidationWarning struct {
	ItemID      int       `json:"item_id"`
	ItemName    string    `json:"item_name"`
	ItemCode    string    `json:"item_code"`
	WarningType string    `json:"warning_type"`
	Message     string    `json:"message"`
	FieldName   string    `json:"field_name,omitempty"`
	Suggestion  string    `json:"suggestion,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// ValidationRule правило валидации
type ValidationRule struct {
	Name        string
	Description string
	Severity    ValidationSeverity
	Validator   func(*database.CatalogItem) error
}

// ValidationEngine движок валидации данных
// Поддерживает:
// - Валидацию структуры данных
// - Сбор ошибок и предупреждений
// - Генерацию детальных отчетов
type ValidationEngine struct {
	errors   []ValidationError
	warnings []ValidationWarning
	rules    []ValidationRule
	mu       sync.Mutex

	// Статистика
	totalValidated    int
	totalValid        int
	totalInvalid      int
	validationStarted time.Time
}

// NewValidationEngine создает новый движок валидации
func NewValidationEngine() *ValidationEngine {
	ve := &ValidationEngine{
		errors:   make([]ValidationError, 0),
		warnings: make([]ValidationWarning, 0),
		rules:    make([]ValidationRule, 0),
	}

	// Регистрируем стандартные правила валидации
	ve.registerDefaultRules()

	return ve
}

// registerDefaultRules регистрирует стандартные правила валидации
func (ve *ValidationEngine) registerDefaultRules() {
	// Правило: Минимальная длина токенов
	ve.AddRule(ValidationRule{
		Name:        "insufficient_tokens",
		Description: "Название должно содержать минимум 2 слова",
		Severity:    ValidationSeverityHigh,
		Validator: func(item *database.CatalogItem) error {
			tokens := strings.Fields(item.Name)
			if len(tokens) < 2 {
				return fmt.Errorf("название содержит только %d слов(а), минимум 2 требуется", len(tokens))
			}
			return nil
		},
	})

	// Правило: Код не пустой
	ve.AddRule(ValidationRule{
		Name:        "missing_code",
		Description: "Код элемента обязателен",
		Severity:    ValidationSeverityCritical,
		Validator: func(item *database.CatalogItem) error {
			if strings.TrimSpace(item.Code) == "" {
				return fmt.Errorf("код элемента пустой")
			}
			return nil
		},
	})

	// Правило: Название не пустое
	ve.AddRule(ValidationRule{
		Name:        "missing_name",
		Description: "Название элемента обязательно",
		Severity:    ValidationSeverityCritical,
		Validator: func(item *database.CatalogItem) error {
			if strings.TrimSpace(item.Name) == "" {
				return fmt.Errorf("название элемента пустое")
			}
			return nil
		},
	})

	// Правило: Название не содержит "неизвестно"
	ve.AddRule(ValidationRule{
		Name:        "unknown_value",
		Description: "Название не должно содержать 'неизвестно'",
		Severity:    ValidationSeverityMedium,
		Validator: func(item *database.CatalogItem) error {
			lowerName := strings.ToLower(item.Name)
			if strings.Contains(lowerName, "неизвестно") || strings.Contains(lowerName, "unknown") {
				return fmt.Errorf("название содержит ключевое слово 'неизвестно/unknown'")
			}
			return nil
		},
	})

	// Правило: Разумная длина названия
	ve.AddRule(ValidationRule{
		Name:        "name_too_long",
		Description: "Название слишком длинное",
		Severity:    ValidationSeverityLow,
		Validator: func(item *database.CatalogItem) error {
			if len(item.Name) > 500 {
				return fmt.Errorf("название содержит %d символов, рекомендуется не более 500", len(item.Name))
			}
			return nil
		},
	})

	// Правило: Название не состоит только из чисел
	ve.AddRule(ValidationRule{
		Name:        "name_only_numbers",
		Description: "Название не должно состоять только из чисел",
		Severity:    ValidationSeverityHigh,
		Validator: func(item *database.CatalogItem) error {
			trimmed := strings.TrimSpace(item.Name)
			onlyNumbers := true
			for _, r := range trimmed {
				if !strings.ContainsRune("0123456789.,- ", r) {
					onlyNumbers = false
					break
				}
			}
			if onlyNumbers && len(trimmed) > 0 {
				return fmt.Errorf("название состоит только из чисел и символов")
			}
			return nil
		},
	})
}

// AddRule добавляет кастомное правило валидации
func (ve *ValidationEngine) AddRule(rule ValidationRule) {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	ve.rules = append(ve.rules, rule)
}

// ValidateItem валидирует один элемент по всем правилам
// Возвращает true если элемент валиден (можно обрабатывать)
func (ve *ValidationEngine) ValidateItem(item *database.CatalogItem) bool {
	ve.mu.Lock()
	ve.totalValidated++
	ve.mu.Unlock()

	valid := true

	// Применяем все правила
	for _, rule := range ve.rules {
		if err := rule.Validator(item); err != nil {
			// Критические ошибки блокируют обработку
			if rule.Severity == ValidationSeverityCritical {
				valid = false
			}

			ve.addError(item.ID, item.Name, item.Code, rule.Name, err.Error(), rule.Severity, "", "", "")
		}
	}

	// Дополнительные проверки с предупреждениями
	ve.checkForWarnings(item)

	ve.mu.Lock()
	if valid {
		ve.totalValid++
	} else {
		ve.totalInvalid++
	}
	ve.mu.Unlock()

	return valid
}

// checkForWarnings проверяет условия для предупреждений
func (ve *ValidationEngine) checkForWarnings(item *database.CatalogItem) {
	// Предупреждение: Слишком много скобок
	openBrackets := strings.Count(item.Name, "(")
	closeBrackets := strings.Count(item.Name, ")")
	if openBrackets != closeBrackets {
		ve.addWarning(item.ID, item.Name, item.Code, "unbalanced_brackets",
			fmt.Sprintf("Несбалансированные скобки: открывающих=%d, закрывающих=%d", openBrackets, closeBrackets),
			"name", "Проверьте правильность расстановки скобок")
	}

	// Предупреждение: Множественные пробелы
	if strings.Contains(item.Name, "  ") {
		ve.addWarning(item.ID, item.Name, item.Code, "multiple_spaces",
			"Название содержит множественные пробелы",
			"name", "Замените множественные пробелы на одинарные")
	}

	// Предупреждение: Пробелы в начале/конце
	trimmed := strings.TrimSpace(item.Name)
	if trimmed != item.Name {
		ve.addWarning(item.ID, item.Name, item.Code, "trailing_spaces",
			"Название содержит пробелы в начале или конце",
			"name", "Удалите лишние пробелы")
	}

	// Предупреждение: Специальные символы
	specialChars := []string{"#", "$", "%", "&", "*", "~", "`"}
	for _, char := range specialChars {
		if strings.Contains(item.Name, char) {
			ve.addWarning(item.ID, item.Name, item.Code, "special_characters",
				fmt.Sprintf("Название содержит специальный символ '%s'", char),
				"name", "Проверьте необходимость специальных символов")
			break
		}
	}
}

// ValidateBatch валидирует батч элементов
// Возвращает список валидных элементов
func (ve *ValidationEngine) ValidateBatch(items []*database.CatalogItem) []*database.CatalogItem {
	ve.mu.Lock()
	ve.validationStarted = time.Now()
	ve.mu.Unlock()

	validItems := make([]*database.CatalogItem, 0)

	for _, item := range items {
		if ve.ValidateItem(item) {
			validItems = append(validItems, item)
		}
	}

	return validItems
}

// addError добавляет ошибку валидации
func (ve *ValidationEngine) addError(itemID int, itemName, itemCode, errorType, message string,
	severity ValidationSeverity, fieldName, expectedValue, actualValue string) {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	ve.errors = append(ve.errors, ValidationError{
		ItemID:        itemID,
		ItemName:      itemName,
		ItemCode:      itemCode,
		ErrorType:     errorType,
		ErrorMessage:  message,
		Severity:      severity,
		FieldName:     fieldName,
		ExpectedValue: expectedValue,
		ActualValue:   actualValue,
		Timestamp:     time.Now(),
	})
}

// addWarning добавляет предупреждение валидации
func (ve *ValidationEngine) addWarning(itemID int, itemName, itemCode, warningType, message, fieldName, suggestion string) {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	ve.warnings = append(ve.warnings, ValidationWarning{
		ItemID:      itemID,
		ItemName:    itemName,
		ItemCode:    itemCode,
		WarningType: warningType,
		Message:     message,
		FieldName:   fieldName,
		Suggestion:  suggestion,
		Timestamp:   time.Now(),
	})
}

// GenerateReport генерирует детальный отчет по валидации
func (ve *ValidationEngine) GenerateReport() map[string]interface{} {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	// Группируем ошибки по типам
	errorsByType := make(map[string]int)
	errorsBySeverity := make(map[ValidationSeverity]int)

	for _, err := range ve.errors {
		errorsByType[err.ErrorType]++
		errorsBySeverity[err.Severity]++
	}

	// Группируем предупреждения по типам
	warningsByType := make(map[string]int)
	for _, warn := range ve.warnings {
		warningsByType[warn.WarningType]++
	}

	validationDuration := time.Since(ve.validationStarted)

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"total_validated": ve.totalValidated,
			"total_valid":     ve.totalValid,
			"total_invalid":   ve.totalInvalid,
			"total_errors":    len(ve.errors),
			"total_warnings":  len(ve.warnings),
			"validation_rate": fmt.Sprintf("%.1f%%", float64(ve.totalValid)/float64(ve.totalValidated)*100.0),
			"duration":        validationDuration.String(),
		},
		"errors_by_type":     errorsByType,
		"errors_by_severity": errorsBySeverity,
		"warnings_by_type":   warningsByType,
		"detailed_errors":    ve.errors,
		"detailed_warnings":  ve.warnings,
		"timestamp":          time.Now(),
	}
}

// GetErrors возвращает все ошибки валидации
func (ve *ValidationEngine) GetErrors() []ValidationError {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	return ve.errors
}

// GetWarnings возвращает все предупреждения валидации
func (ve *ValidationEngine) GetWarnings() []ValidationWarning {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	return ve.warnings
}

// GetErrorsBySeverity возвращает ошибки определенной серьезности
func (ve *ValidationEngine) GetErrorsBySeverity(severity ValidationSeverity) []ValidationError {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	result := make([]ValidationError, 0)
	for _, err := range ve.errors {
		if err.Severity == severity {
			result = append(result, err)
		}
	}
	return result
}

// GetErrorsByType возвращает ошибки определенного типа
func (ve *ValidationEngine) GetErrorsByType(errorType string) []ValidationError {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	result := make([]ValidationError, 0)
	for _, err := range ve.errors {
		if err.ErrorType == errorType {
			result = append(result, err)
		}
	}
	return result
}

// Clear очищает все ошибки и предупреждения
func (ve *ValidationEngine) Clear() {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	ve.errors = make([]ValidationError, 0)
	ve.warnings = make([]ValidationWarning, 0)
	ve.totalValidated = 0
	ve.totalValid = 0
	ve.totalInvalid = 0
	ve.validationStarted = time.Time{}
}

// FormatTextReport генерирует текстовый отчет
func (ve *ValidationEngine) FormatTextReport() string {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	var report strings.Builder

	report.WriteString("=== ОТЧЕТ ПО ВАЛИДАЦИИ ===\n\n")

	report.WriteString(fmt.Sprintf("Проверено элементов: %d\n", ve.totalValidated))
	report.WriteString(fmt.Sprintf("Валидных: %d (%.1f%%)\n",
		ve.totalValid, float64(ve.totalValid)/float64(ve.totalValidated)*100.0))
	report.WriteString(fmt.Sprintf("Невалидных: %d (%.1f%%)\n",
		ve.totalInvalid, float64(ve.totalInvalid)/float64(ve.totalValidated)*100.0))
	report.WriteString(fmt.Sprintf("Всего ошибок: %d\n", len(ve.errors)))
	report.WriteString(fmt.Sprintf("Всего предупреждений: %d\n\n", len(ve.warnings)))

	// Ошибки по серьезности
	report.WriteString("Ошибки по уровню серьезности:\n")
	severityCounts := make(map[ValidationSeverity]int)
	for _, err := range ve.errors {
		severityCounts[err.Severity]++
	}
	for severity, count := range severityCounts {
		report.WriteString(fmt.Sprintf("  %s: %d\n", severity, count))
	}
	report.WriteString("\n")

	// Топ-10 ошибок
	if len(ve.errors) > 0 {
		report.WriteString("Первые 10 ошибок:\n")
		limit := 10
		if len(ve.errors) < limit {
			limit = len(ve.errors)
		}
		for i := 0; i < limit; i++ {
			err := ve.errors[i]
			report.WriteString(fmt.Sprintf("  %d. [%s] %s: %s (элемент: %s)\n",
				i+1, err.Severity, err.ErrorType, err.ErrorMessage, err.ItemName))
		}
		report.WriteString("\n")
	}

	// Предупреждения
	if len(ve.warnings) > 0 {
		report.WriteString("Первые 10 предупреждений:\n")
		limit := 10
		if len(ve.warnings) < limit {
			limit = len(ve.warnings)
		}
		for i := 0; i < limit; i++ {
			warn := ve.warnings[i]
			report.WriteString(fmt.Sprintf("  %d. %s: %s (элемент: %s)\n",
				i+1, warn.WarningType, warn.Message, warn.ItemName))
		}
	}

	return report.String()
}
