package classification

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FoldingStrategyConfig определяет конфигурацию стратегии свертки уровней категорий
type FoldingStrategyConfig struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	MaxDepth    int           `json:"max_depth"` // Максимальная глубина (обычно 2)
	Priority    []string      `json:"priority"`  // Приоритетные уровни для сохранения
	Rules       []FoldingRule `json:"rules"`
}

// FoldingRule правило свертки
type FoldingRule struct {
	SourceLevels []int  `json:"source_levels"` // Какие уровни объединять [3,4,5]
	TargetLevel  int    `json:"target_level"`  // В какой уровень поместить (0 или 1)
	Separator    string `json:"separator"`     // Разделитель " / "
	Condition    string `json:"condition"`     // Условие применения
}

// AppliedRule примененное правило
type AppliedRule struct {
	RuleID    string `json:"rule_id"`
	Source    []int  `json:"source"`
	Target    int    `json:"target"`
	Operation string `json:"operation"`
}

// StrategyManager управляет стратегиями свертки
type StrategyManager struct {
	strategies map[string]FoldingStrategyConfig
}

// NewStrategyManager создает новый менеджер стратегий
func NewStrategyManager() *StrategyManager {
	sm := &StrategyManager{
		strategies: make(map[string]FoldingStrategyConfig),
	}
	sm.registerDefaultStrategies()
	return sm
}

// registerDefaultStrategies регистрирует стандартные стратегии
func (sm *StrategyManager) registerDefaultStrategies() {
	// Стратегия "top" - сохранить верхние уровни
	sm.strategies["top_priority"] = FoldingStrategyConfig{
		ID:          "top_priority",
		Name:        "Приоритет верхних уровней",
		Description: "Сохраняет верхние уровни, объединяя нижние",
		MaxDepth:    2,
		Priority:    []string{"0", "1"}, // Сохранить уровни 0 и 1
		Rules: []FoldingRule{
			{
				SourceLevels: []int{2, 3, 4, 5, 6},
				TargetLevel:  1, // Объединяем в level1
				Separator:    " / ",
			},
		},
	}

	// Стратегия "bottom" - сохранить нижние уровни
	sm.strategies["bottom_priority"] = FoldingStrategyConfig{
		ID:          "bottom_priority",
		Name:        "Приоритет нижних уровней",
		Description: "Сохраняет нижние уровни, объединяя верхние",
		MaxDepth:    2,
		Priority:    []string{"-2", "-1"}, // Сохранить последние 2 уровня
		Rules: []FoldingRule{
			{
				SourceLevels: []int{0, 1, 2, 3},
				TargetLevel:  0, // Объединяем в level0
				Separator:    " / ",
			},
		},
	}

	// Стратегия "mixed" - первый и последний
	sm.strategies["mixed_priority"] = FoldingStrategyConfig{
		ID:          "mixed_priority",
		Name:        "Смешанный приоритет",
		Description: "Сохраняет первый и последний уровни",
		MaxDepth:    2,
		Priority:    []string{"0", "-1"}, // Первый и последний
		Rules:       []FoldingRule{},
	}
}

// FoldCategoryPathSimple сворачивает путь категории до указанной глубины (простая версия)
func FoldCategoryPathSimple(fullPath []string, depth int, strategy string) []string {
	if len(fullPath) <= depth {
		// Если путь уже короче или равен нужной глубине, возвращаем как есть
		return fullPath
	}

	switch strategy {
	case "top", "top_priority":
		return foldTopPriority(fullPath, depth)
	case "bottom", "bottom_priority":
		return foldBottomPriority(fullPath, depth)
	case "mixed", "mixed_priority":
		return foldMixedPriority(fullPath, depth)
	default:
		// По умолчанию используем top
		return foldTopPriority(fullPath, depth)
	}
}

// foldTopPriority сворачивает, сохраняя верхние уровни
func foldTopPriority(levels []string, depth int) []string {
	result := make([]string, depth)
	separator := " / "

	// Берем первые depth-1 уровней как есть
	for i := 0; i < depth-1 && i < len(levels); i++ {
		result[i] = levels[i]
	}

	// Объединяем оставшиеся уровни в последний уровень
	if len(levels) > depth-1 {
		remaining := levels[depth-1:]
		result[depth-1] = strings.Join(remaining, separator)
	}

	return result
}

// foldBottomPriority сворачивает, сохраняя нижние уровни
func foldBottomPriority(levels []string, depth int) []string {
	result := make([]string, depth)
	separator := " / "

	// Первый уровень - объединение всех уровней кроме последних depth-1
	if len(levels) > depth-1 {
		upperLevels := levels[:len(levels)-depth+1]
		result[0] = strings.Join(upperLevels, separator)
	} else {
		result[0] = levels[0]
	}

	// Затем последние depth-1 уровней
	for i := 1; i < depth && len(levels)-depth+i >= 0; i++ {
		result[i] = levels[len(levels)-depth+i]
	}

	return result
}

// foldMixedPriority сворачивает, сохраняя первый и последний уровни
func foldMixedPriority(levels []string, depth int) []string {
	result := make([]string, depth)
	separator := " / "

	if len(levels) == 0 {
		return result
	}

	// Первый уровень - первый элемент
	result[0] = levels[0]

	// Последний уровень - объединение всех остальных
	if len(levels) > 1 {
		remaining := levels[1:]
		if depth > 1 {
			result[depth-1] = strings.Join(remaining, separator)
		} else {
			result[0] = strings.Join(levels, separator)
		}
	}

	return result
}

// FoldCategory сворачивает категорию используя стратегию
func (sm *StrategyManager) FoldCategory(fullPath []string, strategyID string) ([]string, error) {
	strategy, exists := sm.strategies[strategyID]
	if !exists {
		// Если стратегия не найдена, используем простую свертку
		return FoldCategoryPathSimple(fullPath, 2, "top"), nil
	}

	// Если путь уже короче или равен нужной глубине
	if len(fullPath) <= strategy.MaxDepth {
		return fullPath, nil
	}

	// Применяем правила стратегии
	result := make([]string, strategy.MaxDepth)

	// Применяем приоритетные уровни
	for i, priority := range strategy.Priority {
		if i >= strategy.MaxDepth {
			break
		}

		levelIndex := parsePriorityLevel(priority, len(fullPath))
		if levelIndex >= 0 && levelIndex < len(fullPath) {
			result[i] = fullPath[levelIndex]
		}
	}

	// Применяем правила свертки
	for _, rule := range strategy.Rules {
		if sm.evaluateCondition(rule.Condition, fullPath) {
			folded := sm.applyFoldingRule(fullPath, rule)
			if rule.TargetLevel >= 0 && rule.TargetLevel < len(result) {
				result[rule.TargetLevel] = folded
			}
		}
	}

	// Заполняем пустые уровни
	for i := range result {
		if result[i] == "" {
			if i < len(fullPath) {
				result[i] = fullPath[i]
			}
		}
	}

	return result, nil
}

// parsePriorityLevel парсит приоритетный уровень
func parsePriorityLevel(priority string, totalLevels int) int {
	if priority == "0" {
		return 0
	}
	if priority == "-1" {
		if totalLevels > 0 {
			return totalLevels - 1
		}
		return 0
	}
	if priority == "-2" {
		if totalLevels > 1 {
			return totalLevels - 2
		}
		return 0
	}
	// Можно добавить парсинг конкретных индексов
	return -1
}

// applyFoldingRule применяет правило свертки
func (sm *StrategyManager) applyFoldingRule(levels []string, rule FoldingRule) string {
	var parts []string
	for _, levelIndex := range rule.SourceLevels {
		if levelIndex >= 0 && levelIndex < len(levels) {
			parts = append(parts, levels[levelIndex])
		}
	}
	separator := rule.Separator
	if separator == "" {
		separator = " / "
	}
	return strings.Join(parts, separator)
}

// evaluateCondition оценивает условие применения правила
func (sm *StrategyManager) evaluateCondition(condition string, levels []string) bool {
	if condition == "" || condition == "always" {
		return true
	}
	// Можно добавить более сложную логику условий
	return true
}

// GetStrategy возвращает стратегию по ID
func (sm *StrategyManager) GetStrategy(strategyID string) (*FoldingStrategyConfig, error) {
	strategy, exists := sm.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy not found: %s", strategyID)
	}
	return &strategy, nil
}

// AddStrategy добавляет новую стратегию
func (sm *StrategyManager) AddStrategy(strategy FoldingStrategyConfig) {
	sm.strategies[strategy.ID] = strategy
}

// GetAllStrategies возвращает все стратегии
func (sm *StrategyManager) GetAllStrategies() map[string]FoldingStrategyConfig {
	return sm.strategies
}

// LoadStrategyFromJSON загружает стратегию из JSON
func (sm *StrategyManager) LoadStrategyFromJSON(jsonData string) error {
	var strategy FoldingStrategyConfig
	if err := json.Unmarshal([]byte(jsonData), &strategy); err != nil {
		return fmt.Errorf("failed to parse strategy JSON: %w", err)
	}
	sm.AddStrategy(strategy)
	return nil
}
