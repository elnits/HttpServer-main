package normalization

import (
	"time"
)

// MockAINormalizer мок для AINormalizer для использования в тестах
type MockAINormalizer struct {
	responses map[string]*AIResult
	errors    map[string]error
	callCount int
	latency   time.Duration
}

// NewMockAINormalizer создает новый мок AI нормализатора
func NewMockAINormalizer() *MockAINormalizer {
	return &MockAINormalizer{
		responses: make(map[string]*AIResult),
		errors:    make(map[string]error),
		callCount: 0,
		latency:   10 * time.Millisecond,
	}
}

// SetResponse устанавливает ответ для конкретного названия
func (m *MockAINormalizer) SetResponse(name string, result *AIResult) {
	m.responses[name] = result
}

// SetError устанавливает ошибку для конкретного названия
func (m *MockAINormalizer) SetError(name string, err error) {
	m.errors[name] = err
}

// SetLatency устанавливает задержку для имитации сетевой латентности
func (m *MockAINormalizer) SetLatency(latency time.Duration) {
	m.latency = latency
}

// GetCallCount возвращает количество вызовов
func (m *MockAINormalizer) GetCallCount() int {
	return m.callCount
}

// Reset сбрасывает счетчик вызовов
func (m *MockAINormalizer) Reset() {
	m.callCount = 0
	m.responses = make(map[string]*AIResult)
	m.errors = make(map[string]error)
}

// NormalizeWithAI реализует интерфейс AINormalizer
func (m *MockAINormalizer) NormalizeWithAI(name string) (*AIResult, error) {
	m.callCount++

	// Имитируем задержку
	time.Sleep(m.latency)

	// Проверяем, есть ли установленная ошибка
	if err, ok := m.errors[name]; ok {
		return nil, err
	}

	// Проверяем, есть ли установленный ответ
	if result, ok := m.responses[name]; ok {
		return result, nil
	}

	// Возвращаем дефолтный ответ
	return &AIResult{
		Category:      "другое",
		NormalizedName: name,
		Confidence:    0.5,
		Reasoning:     "Mock response",
	}, nil
}

// RequiresAI реализует интерфейс AINormalizer
func (m *MockAINormalizer) RequiresAI(name string, category string) bool {
	// В моке всегда возвращаем true для тестирования
	return true
}

// GetStats реализует интерфейс AINormalizer
func (m *MockAINormalizer) GetStats() *AIStats {
	return &AIStats{
		TotalCalls: int64(m.callCount),
		Errors:     0,
		CacheHits:  0,
	}
}

// MockPatternAIIntegrator мок для PatternAIIntegrator
type MockPatternAIIntegrator struct {
	patternDetector *PatternDetector
	aiNormalizer    *MockAINormalizer
	responses       map[string]*PatternCorrectionResult
	errors          map[string]error
}

// NewMockPatternAIIntegrator создает новый мок интегратора паттернов с AI
func NewMockPatternAIIntegrator(patternDetector *PatternDetector) *MockPatternAIIntegrator {
	return &MockPatternAIIntegrator{
		patternDetector: patternDetector,
		aiNormalizer:    NewMockAINormalizer(),
		responses:       make(map[string]*PatternCorrectionResult),
		errors:          make(map[string]error),
	}
}

// SetResponse устанавливает ответ для конкретного названия
func (m *MockPatternAIIntegrator) SetResponse(name string, result *PatternCorrectionResult) {
	m.responses[name] = result
}

// SetError устанавливает ошибку для конкретного названия
func (m *MockPatternAIIntegrator) SetError(name string, err error) {
	m.errors[name] = err
}

// SuggestCorrectionWithAI реализует интерфейс PatternAIIntegrator
func (m *MockPatternAIIntegrator) SuggestCorrectionWithAI(originalName string) (*PatternCorrectionResult, error) {
	// Проверяем, есть ли установленная ошибка
	if err, ok := m.errors[originalName]; ok {
		return nil, err
	}

	// Проверяем, есть ли установленный ответ
	if result, ok := m.responses[originalName]; ok {
		return result, nil
	}

	// Используем реальный patternDetector для обнаружения паттернов
	matches := m.patternDetector.DetectPatterns(originalName)
	algorithmicFix := m.patternDetector.ApplyFixes(originalName, matches)

	// Получаем результат от AI мока
	aiResult, err := m.aiNormalizer.NormalizeWithAI(originalName)
	if err != nil {
		return nil, err
	}

	// Формируем результат
	return &PatternCorrectionResult{
		OriginalName:     originalName,
		DetectedPatterns: matches,
		AlgorithmicFix:   algorithmicFix,
		AISuggestedFix:   aiResult.NormalizedName,
		FinalSuggestion:  aiResult.NormalizedName,
		Confidence:       aiResult.Confidence,
		Reasoning:        aiResult.Reasoning,
		RequiresReview:   false,
	}, nil
}

// GetAINormalizer возвращает мок AI нормализатора
func (m *MockPatternAIIntegrator) GetAINormalizer() *MockAINormalizer {
	return m.aiNormalizer
}

