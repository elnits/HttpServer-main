package normalization

import (
	"fmt"
	"strings"
	"unicode"
)

// Severity уровень серьезности нарушения
type Severity string

const (
	SeverityInfo     Severity = "info"     // Информационное
	SeverityWarning  Severity = "warning"  // Предупреждение
	SeverityError    Severity = "error"    // Ошибка
	SeverityCritical Severity = "critical" // Критическое
)

// ViolationCategory категория нарушения
type ViolationCategory string

const (
	CategoryCompleteness ViolationCategory = "completeness" // Полнота данных
	CategoryAccuracy     ViolationCategory = "accuracy"     // Точность
	CategoryConsistency  ViolationCategory = "consistency"  // Согласованность
	CategoryUniqueness   ViolationCategory = "uniqueness"   // Уникальность
	CategoryFormat       ViolationCategory = "format"       // Формат данных
)

// Violation нарушение правила качества
type Violation struct {
	RuleName    string            `json:"rule_name"`    // Название правила
	Category    ViolationCategory `json:"category"`     // Категория нарушения
	Severity    Severity          `json:"severity"`     // Серьезность
	Description string            `json:"description"`  // Описание проблемы
	Field       string            `json:"field"`        // Поле с проблемой
	CurrentValue string           `json:"current_value"` // Текущее значение
	Recommendation string         `json:"recommendation"` // Рекомендация по исправлению
}

// QualityRule правило проверки качества
type QualityRule struct {
	Name        string                        // Название правила
	Category    ViolationCategory             // Категория
	Severity    Severity                      // Серьезность нарушения
	Description string                        // Описание правила
	Check       func(ItemData) *Violation    // Функция проверки
}

// ItemData данные записи для проверки правилами
type ItemData struct {
	ID               int
	Code             string
	NormalizedName   string
	Category         string
	KpvedCode        string
	KpvedConfidence  float64
	ProcessingLevel  string
	AIConfidence     float64
	AIReasoning      string
	MergedCount      int
}

// QualityRulesEngine движок правил качества
type QualityRulesEngine struct {
	rules []QualityRule
}

// NewQualityRulesEngine создает новый движок правил
func NewQualityRulesEngine() *QualityRulesEngine {
	engine := &QualityRulesEngine{
		rules: make([]QualityRule, 0),
	}
	engine.registerDefaultRules()
	return engine
}

// registerDefaultRules регистрирует стандартные правила качества
func (qre *QualityRulesEngine) registerDefaultRules() {
	// Правила полноты данных
	qre.AddRule(ruleRequireNormalizedName())
	qre.AddRule(ruleRequireCategory())
	qre.AddRule(ruleRequireKpvedCode())
	qre.AddRule(ruleRequireCode())

	// Правила формата
	qre.AddRule(ruleValidKpvedFormat())
	qre.AddRule(ruleNameLength())
	qre.AddRule(ruleNameFormat())

	// Правила согласованности
	qre.AddRule(ruleKpvedConfidenceThreshold())
	qre.AddRule(ruleAIConfidenceThreshold())
	qre.AddRule(ruleCategoryOther())

	// Правила точности
	qre.AddRule(ruleProcessingLevel())
	qre.AddRule(ruleAIReasoning())
}

// AddRule добавляет новое правило
func (qre *QualityRulesEngine) AddRule(rule QualityRule) {
	qre.rules = append(qre.rules, rule)
}

// CheckAll проверяет все правила для записи
func (qre *QualityRulesEngine) CheckAll(data ItemData) []Violation {
	var violations []Violation

	for _, rule := range qre.rules {
		if violation := rule.Check(data); violation != nil {
			violations = append(violations, *violation)
		}
	}

	return violations
}

// CheckBySeverity проверяет правила определенной серьезности
func (qre *QualityRulesEngine) CheckBySeverity(data ItemData, severity Severity) []Violation {
	var violations []Violation

	for _, rule := range qre.rules {
		if rule.Severity == severity {
			if violation := rule.Check(data); violation != nil {
				violations = append(violations, *violation)
			}
		}
	}

	return violations
}

// GetRulesBySeverity возвращает правила определенной серьезности
func (qre *QualityRulesEngine) GetRulesBySeverity(severity Severity) []QualityRule {
	var filtered []QualityRule

	for _, rule := range qre.rules {
		if rule.Severity == severity {
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

// --- Реализация стандартных правил ---

// ruleRequireNormalizedName: нормализованное имя должно быть заполнено
func ruleRequireNormalizedName() QualityRule {
	return QualityRule{
		Name:        "require_normalized_name",
		Category:    CategoryCompleteness,
		Severity:    SeverityCritical,
		Description: "Нормализованное имя должно быть заполнено",
		Check: func(data ItemData) *Violation {
			if data.NormalizedName == "" {
				return &Violation{
					RuleName:       "require_normalized_name",
					Category:       CategoryCompleteness,
					Severity:       SeverityCritical,
					Description:    "Отсутствует нормализованное имя",
					Field:          "normalized_name",
					CurrentValue:   "",
					Recommendation: "Выполните нормализацию записи",
				}
			}
			return nil
		},
	}
}

// ruleRequireCategory: категория должна быть заполнена
func ruleRequireCategory() QualityRule {
	return QualityRule{
		Name:        "require_category",
		Category:    CategoryCompleteness,
		Severity:    SeverityCritical,
		Description: "Категория должна быть заполнена",
		Check: func(data ItemData) *Violation {
			if data.Category == "" {
				return &Violation{
					RuleName:       "require_category",
					Category:       CategoryCompleteness,
					Severity:       SeverityCritical,
					Description:    "Отсутствует категория",
					Field:          "category",
					CurrentValue:   "",
					Recommendation: "Присвойте категорию записи",
				}
			}
			return nil
		},
	}
}

// ruleRequireKpvedCode: код КПВЭД должен быть заполнен
func ruleRequireKpvedCode() QualityRule {
	return QualityRule{
		Name:        "require_kpved_code",
		Category:    CategoryCompleteness,
		Severity:    SeverityWarning,
		Description: "Код КПВЭД должен быть заполнен",
		Check: func(data ItemData) *Violation {
			if data.KpvedCode == "" {
				return &Violation{
					RuleName:       "require_kpved_code",
					Category:       CategoryCompleteness,
					Severity:       SeverityWarning,
					Description:    "Отсутствует код КПВЭД",
					Field:          "kpved_code",
					CurrentValue:   "",
					Recommendation: "Выполните классификацию КПВЭД",
				}
			}
			return nil
		},
	}
}

// ruleRequireCode: код для поиска должен быть заполнен
func ruleRequireCode() QualityRule {
	return QualityRule{
		Name:        "require_code",
		Category:    CategoryCompleteness,
		Severity:    SeverityError,
		Description: "Код для поиска должен быть заполнен",
		Check: func(data ItemData) *Violation {
			if data.Code == "" {
				return &Violation{
					RuleName:       "require_code",
					Category:       CategoryCompleteness,
					Severity:       SeverityError,
					Description:    "Отсутствует код для поиска",
					Field:          "code",
					CurrentValue:   "",
					Recommendation: "Добавьте уникальный код записи",
				}
			}
			return nil
		},
	}
}

// ruleValidKpvedFormat: КПВЭД код должен иметь корректный формат
func ruleValidKpvedFormat() QualityRule {
	return QualityRule{
		Name:        "valid_kpved_format",
		Category:    CategoryFormat,
		Severity:    SeverityError,
		Description: "КПВЭД код должен иметь формат XX.XX или XX.XX.XX",
		Check: func(data ItemData) *Violation {
			if data.KpvedCode == "" {
				return nil // Проверяется другим правилом
			}

			if !isValidKpvedFormat(data.KpvedCode) {
				return &Violation{
					RuleName:       "valid_kpved_format",
					Category:       CategoryFormat,
					Severity:       SeverityError,
					Description:    "Некорректный формат кода КПВЭД",
					Field:          "kpved_code",
					CurrentValue:   data.KpvedCode,
					Recommendation: "Код КПВЭД должен быть в формате XX.XX или XX.XX.XX (например, 46.90 или 46.90.10)",
				}
			}
			return nil
		},
	}
}

// ruleNameLength: длина нормализованного имени должна быть в разумных пределах
func ruleNameLength() QualityRule {
	return QualityRule{
		Name:        "name_length",
		Category:    CategoryFormat,
		Severity:    SeverityWarning,
		Description: "Длина нормализованного имени должна быть от 3 до 100 символов",
		Check: func(data ItemData) *Violation {
			nameLen := len([]rune(data.NormalizedName))

			if nameLen < 3 {
				return &Violation{
					RuleName:       "name_length",
					Category:       CategoryFormat,
					Severity:       SeverityWarning,
					Description:    "Слишком короткое нормализованное имя",
					Field:          "normalized_name",
					CurrentValue:   data.NormalizedName,
					Recommendation: "Нормализованное имя должно содержать как минимум 3 символа",
				}
			}

			if nameLen > 100 {
				return &Violation{
					RuleName:       "name_length",
					Category:       CategoryFormat,
					Severity:       SeverityWarning,
					Description:    "Слишком длинное нормализованное имя",
					Field:          "normalized_name",
					CurrentValue:   fmt.Sprintf("%s... (%d символов)", data.NormalizedName[:50], nameLen),
					Recommendation: "Сократите нормализованное имя до 100 символов",
				}
			}

			return nil
		},
	}
}

// ruleNameFormat: нормализованное имя должно содержать буквы
func ruleNameFormat() QualityRule {
	return QualityRule{
		Name:        "name_format",
		Category:    CategoryFormat,
		Severity:    SeverityError,
		Description: "Нормализованное имя должно содержать буквы",
		Check: func(data ItemData) *Violation {
			if data.NormalizedName == "" {
				return nil // Проверяется другим правилом
			}

			hasLetters := false
			for _, r := range data.NormalizedName {
				if unicode.IsLetter(r) {
					hasLetters = true
					break
				}
			}

			if !hasLetters {
				return &Violation{
					RuleName:       "name_format",
					Category:       CategoryFormat,
					Severity:       SeverityError,
					Description:    "Нормализованное имя не содержит букв",
					Field:          "normalized_name",
					CurrentValue:   data.NormalizedName,
					Recommendation: "Нормализованное имя должно содержать текст, а не только цифры и символы",
				}
			}

			return nil
		},
	}
}

// ruleKpvedConfidenceThreshold: confidence КПВЭД должен быть достаточно высоким
func ruleKpvedConfidenceThreshold() QualityRule {
	return QualityRule{
		Name:        "kpved_confidence_threshold",
		Category:    CategoryAccuracy,
		Severity:    SeverityWarning,
		Description: "Уверенность классификации КПВЭД должна быть >= 70%",
		Check: func(data ItemData) *Violation {
			if data.KpvedCode == "" {
				return nil // Нет кода - нет проверки
			}

			if data.KpvedConfidence < 0.7 {
				return &Violation{
					RuleName:       "kpved_confidence_threshold",
					Category:       CategoryAccuracy,
					Severity:       SeverityWarning,
					Description:    "Низкая уверенность классификации КПВЭД",
					Field:          "kpved_confidence",
					CurrentValue:   fmt.Sprintf("%.1f%%", data.KpvedConfidence*100),
					Recommendation: "Проверьте корректность классификации КПВЭД вручную",
				}
			}

			return nil
		},
	}
}

// ruleAIConfidenceThreshold: AI confidence должен быть достаточно высоким для AI-enhanced записей
func ruleAIConfidenceThreshold() QualityRule {
	return QualityRule{
		Name:        "ai_confidence_threshold",
		Category:    CategoryAccuracy,
		Severity:    SeverityInfo,
		Description: "AI уверенность должна быть >= 80% для AI-enhanced записей",
		Check: func(data ItemData) *Violation {
			if data.ProcessingLevel != "ai_enhanced" {
				return nil // Только для AI-enhanced
			}

			if data.AIConfidence < 0.8 {
				return &Violation{
					RuleName:       "ai_confidence_threshold",
					Category:       CategoryAccuracy,
					Severity:       SeverityInfo,
					Description:    "Относительно низкая AI уверенность",
					Field:          "ai_confidence",
					CurrentValue:   fmt.Sprintf("%.1f%%", data.AIConfidence*100),
					Recommendation: "Рассмотрите возможность ручной проверки категории и имени",
				}
			}

			return nil
		},
	}
}

// ruleCategoryOther: категория не должна быть "другое"
func ruleCategoryOther() QualityRule {
	return QualityRule{
		Name:        "category_other",
		Category:    CategoryConsistency,
		Severity:    SeverityWarning,
		Description: "Категория не должна быть 'другое'",
		Check: func(data ItemData) *Violation {
			if strings.ToLower(data.Category) == "другое" || strings.ToLower(data.Category) == "other" {
				return &Violation{
					RuleName:       "category_other",
					Category:       CategoryConsistency,
					Severity:       SeverityWarning,
					Description:    "Категория определена как 'другое'",
					Field:          "category",
					CurrentValue:   data.Category,
					Recommendation: "Уточните категорию записи",
				}
			}
			return nil
		},
	}
}

// ruleProcessingLevel: уровень обработки должен быть установлен
func ruleProcessingLevel() QualityRule {
	return QualityRule{
		Name:        "processing_level",
		Category:    CategoryCompleteness,
		Severity:    SeverityInfo,
		Description: "Уровень обработки должен быть установлен",
		Check: func(data ItemData) *Violation {
			validLevels := map[string]bool{
				"basic":        true,
				"ai_enhanced":  true,
				"benchmark":    true,
			}

			if !validLevels[data.ProcessingLevel] {
				return &Violation{
					RuleName:       "processing_level",
					Category:       CategoryCompleteness,
					Severity:       SeverityInfo,
					Description:    "Неизвестный уровень обработки",
					Field:          "processing_level",
					CurrentValue:   data.ProcessingLevel,
					Recommendation: "Установите корректный уровень обработки (basic, ai_enhanced, benchmark)",
				}
			}

			return nil
		},
	}
}

// ruleAIReasoning: AI reasoning должен быть заполнен для AI-enhanced записей
func ruleAIReasoning() QualityRule {
	return QualityRule{
		Name:        "ai_reasoning",
		Category:    CategoryCompleteness,
		Severity:    SeverityInfo,
		Description: "AI reasoning должен быть заполнен для AI-enhanced записей",
		Check: func(data ItemData) *Violation {
			if data.ProcessingLevel != "ai_enhanced" && data.ProcessingLevel != "benchmark" {
				return nil // Только для AI-обработанных
			}

			if data.AIReasoning == "" || len(data.AIReasoning) < 10 {
				return &Violation{
					RuleName:       "ai_reasoning",
					Category:       CategoryCompleteness,
					Severity:       SeverityInfo,
					Description:    "Отсутствует AI обоснование",
					Field:          "ai_reasoning",
					CurrentValue:   data.AIReasoning,
					Recommendation: "AI обоснование помогает понять логику классификации",
				}
			}

			return nil
		},
	}
}
