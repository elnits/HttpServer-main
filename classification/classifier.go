package classification

import (
	"encoding/json"
	"fmt"
)

// CategoryNode представляет узел в дереве классификатора
type CategoryNode struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Path     string                 `json:"path"`
	Level    int                    `json:"level"`
	ParentID string                 `json:"parent_id"`
	Children []CategoryNode         `json:"children,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// FoldingStrategy интерфейс для стратегий свертки категорий
type FoldingStrategy interface {
	FoldCategory(path []string) ([]string, error)
	GetID() string
	GetName() string
	GetDescription() string
	GetMaxDepth() int
}

// ClassificationResult представляет результат классификации
type ClassificationResult struct {
	OriginalPath []string               `json:"original_path"`
	FoldedPath   []string               `json:"folded_path"`
	Strategy     string                 `json:"strategy"`
	Confidence   float64                `json:"confidence"`
	Reasoning    string                 `json:"reasoning"`
	Metadata     map[string]interface{} `json:"metadata"`
	Timestamp    string                 `json:"timestamp"`
}

// NewCategoryNode создает новый узел категории
func NewCategoryNode(id, name, path string, level int) *CategoryNode {
	return &CategoryNode{
		ID:       id,
		Name:     name,
		Path:     path,
		Level:    level,
		Children: make([]CategoryNode, 0),
		Metadata: make(map[string]interface{}),
	}
}

// AddChild добавляет дочерний узел
func (n *CategoryNode) AddChild(child *CategoryNode) {
	child.ParentID = n.ID
	child.Level = n.Level + 1
	n.Children = append(n.Children, *child)
}

// FindChild находит дочерний узел по имени
func (n *CategoryNode) FindChild(name string) *CategoryNode {
	for i := range n.Children {
		if n.Children[i].Name == name {
			return &n.Children[i]
		}
	}
	return nil
}

// ToJSON сериализует узел в JSON
func (n *CategoryNode) ToJSON() (string, error) {
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal CategoryNode: %w", err)
	}
	return string(data), nil
}

// FromJSON десериализует узел из JSON
func (n *CategoryNode) FromJSON(data string) error {
	if err := json.Unmarshal([]byte(data), n); err != nil {
		return fmt.Errorf("failed to unmarshal CategoryNode: %w", err)
	}
	return nil
}

// GetFullPath возвращает полный путь от корня до текущего узла
func (n *CategoryNode) GetFullPath() []string {
	if n.ParentID == "" {
		return []string{n.Name}
	}

	// Рекурсивно получаем путь от родителя
	// Это упрощенная версия - в реальном приложении нужен обратный обход
	return []string{n.Name}
}

// Clone создает копию узла
func (n *CategoryNode) Clone() *CategoryNode {
	clone := &CategoryNode{
		ID:       n.ID,
		Name:     n.Name,
		Path:     n.Path,
		Level:    n.Level,
		ParentID: n.ParentID,
		Children: make([]CategoryNode, len(n.Children)),
		Metadata: make(map[string]interface{}),
	}

	// Клонируем метаданные
	for k, v := range n.Metadata {
		clone.Metadata[k] = v
	}

	// Рекурсивно клонируем дочерние узлы
	for i, child := range n.Children {
		clone.Children[i] = *child.Clone()
	}

	return clone
}

// BaseFoldingStrategy базовая реализация стратегии свертки
type BaseFoldingStrategy struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	MaxDepth    int           `json:"max_depth"`
	Rules       []FoldingRule `json:"rules"`
}

// NewBaseFoldingStrategy создает базовую стратегию
func NewBaseFoldingStrategy(id, name, description string, maxDepth int) *BaseFoldingStrategy {
	return &BaseFoldingStrategy{
		ID:          id,
		Name:        name,
		Description: description,
		MaxDepth:    maxDepth,
		Rules:       make([]FoldingRule, 0),
	}
}

// FoldCategory сворачивает путь категории
func (s *BaseFoldingStrategy) FoldCategory(path []string) ([]string, error) {
	if len(path) <= s.MaxDepth {
		return path, nil
	}

	// Базовая реализация: берем первые MaxDepth элементов
	result := make([]string, s.MaxDepth)
	for i := 0; i < s.MaxDepth; i++ {
		if i < len(path) {
			result[i] = path[i]
		}
	}

	return result, nil
}

// GetID возвращает ID стратегии
func (s *BaseFoldingStrategy) GetID() string {
	return s.ID
}

// GetName возвращает имя стратегии
func (s *BaseFoldingStrategy) GetName() string {
	return s.Name
}

// GetDescription возвращает описание стратегии
func (s *BaseFoldingStrategy) GetDescription() string {
	return s.Description
}

// GetMaxDepth возвращает максимальную глубину
func (s *BaseFoldingStrategy) GetMaxDepth() int {
	return s.MaxDepth
}

// FoldCategoryPath сворачивает путь категории с использованием указанной стратегии
func FoldCategoryPath(fullPath []string, depth int, strategy FoldingStrategy) ([]string, error) {
	if strategy == nil {
		return nil, fmt.Errorf("folding strategy is nil")
	}

	// Проверяем, что depth соответствует стратегии
	if depth > strategy.GetMaxDepth() {
		depth = strategy.GetMaxDepth()
	}

	return strategy.FoldCategory(fullPath)
}

// NewClassificationResult создает новый результат классификации
func NewClassificationResult(originalPath, foldedPath []string, strategy string, confidence float64) *ClassificationResult {
	return &ClassificationResult{
		OriginalPath: originalPath,
		FoldedPath:   foldedPath,
		Strategy:     strategy,
		Confidence:   confidence,
		Metadata:     make(map[string]interface{}),
	}
}

// SetReasoning устанавливает обоснование классификации
func (r *ClassificationResult) SetReasoning(reasoning string) {
	r.Reasoning = reasoning
}

// AddMetadata добавляет метаданные к результату
func (r *ClassificationResult) AddMetadata(key string, value interface{}) {
	r.Metadata[key] = value
}

// GetMetadata получает метаданные по ключу
func (r *ClassificationResult) GetMetadata(key string) (interface{}, bool) {
	value, exists := r.Metadata[key]
	return value, exists
}

// ToJSON сериализует результат в JSON
func (r *ClassificationResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal ClassificationResult: %w", err)
	}
	return string(data), nil
}

// FromJSON десериализует результат из JSON
func (r *ClassificationResult) FromJSON(data string) error {
	if err := json.Unmarshal([]byte(data), r); err != nil {
		return fmt.Errorf("failed to unmarshal ClassificationResult: %w", err)
	}
	return nil
}

// Validate проверяет корректность результата классификации
func (r *ClassificationResult) Validate() error {
	if len(r.OriginalPath) == 0 {
		return fmt.Errorf("original path cannot be empty")
	}

	if len(r.FoldedPath) == 0 {
		return fmt.Errorf("folded path cannot be empty")
	}

	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1, got: %f", r.Confidence)
	}

	if r.Strategy == "" {
		return fmt.Errorf("strategy cannot be empty")
	}

	return nil
}
