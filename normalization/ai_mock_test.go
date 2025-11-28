package normalization

import (
	"fmt"
	"testing"
	"time"
)

// TestMockAINormalizer тестирует мок AI нормализатора
func TestMockAINormalizer(t *testing.T) {
	mock := NewMockAINormalizer()

	// Устанавливаем ответ
	result := &AIResult{
		Category:      "инструмент",
		NormalizedName: "молоток",
		Confidence:    0.95,
		Reasoning:     "Test reasoning",
	}
	mock.SetResponse("Молоток ER-00013004", result)

	// Тестируем нормализацию
	normalized, err := mock.NormalizeWithAI("Молоток ER-00013004")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if normalized.Category != "инструмент" {
		t.Errorf("Expected category 'инструмент', got '%s'", normalized.Category)
	}
	if normalized.NormalizedName != "молоток" {
		t.Errorf("Expected normalized name 'молоток', got '%s'", normalized.NormalizedName)
	}
	if normalized.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %.2f", normalized.Confidence)
	}

	// Проверяем счетчик вызовов
	if mock.GetCallCount() != 1 {
		t.Errorf("Expected call count 1, got %d", mock.GetCallCount())
	}
}

// TestMockAINormalizerError тестирует обработку ошибок в моке
func TestMockAINormalizerError(t *testing.T) {
	mock := NewMockAINormalizer()

	// Устанавливаем ошибку
	mock.SetError("Invalid Name", fmt.Errorf("test error"))

	// Тестируем нормализацию с ошибкой
	_, err := mock.NormalizeWithAI("Invalid Name")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// TestMockAINormalizerDefaultResponse тестирует дефолтный ответ
func TestMockAINormalizerDefaultResponse(t *testing.T) {
	mock := NewMockAINormalizer()

	// Тестируем нормализацию без установленного ответа
	result, err := mock.NormalizeWithAI("Unknown Item")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Error("Expected result, got nil")
	}
	if result.Category == "" {
		t.Error("Expected category, got empty string")
	}
}

// TestMockAINormalizerLatency тестирует задержку
func TestMockAINormalizerLatency(t *testing.T) {
	mock := NewMockAINormalizer()
	mock.SetLatency(50 * time.Millisecond)

	start := time.Now()
	_, err := mock.NormalizeWithAI("Test Item")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("Expected latency >= 50ms, got %v", elapsed)
	}
}

// TestMockPatternAIIntegrator тестирует мок интегратора паттернов с AI
func TestMockPatternAIIntegrator(t *testing.T) {
	patternDetector := NewPatternDetector()
	mock := NewMockPatternAIIntegrator(patternDetector)

	// Устанавливаем ответ AI
	aiResult := &AIResult{
		Category:      "инструмент",
		NormalizedName: "молоток",
		Confidence:    0.9,
		Reasoning:     "Test reasoning",
	}
	mock.GetAINormalizer().SetResponse("Молоток ER-00013004", aiResult)

	// Тестируем предложение исправления
	result, err := mock.SuggestCorrectionWithAI("Молоток ER-00013004")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.OriginalName != "Молоток ER-00013004" {
		t.Errorf("Expected original name 'Молоток ER-00013004', got '%s'", result.OriginalName)
	}
	if len(result.DetectedPatterns) == 0 {
		t.Error("Expected detected patterns, got empty")
	}
	if result.FinalSuggestion == "" {
		t.Error("Expected final suggestion, got empty string")
	}
}

// TestMockPatternAIIntegratorError тестирует обработку ошибок
func TestMockPatternAIIntegratorError(t *testing.T) {
	patternDetector := NewPatternDetector()
	mock := NewMockPatternAIIntegrator(patternDetector)

	// Устанавливаем ошибку
	mock.SetError("Invalid Name", fmt.Errorf("test error"))

	// Тестируем предложение исправления с ошибкой
	_, err := mock.SuggestCorrectionWithAI("Invalid Name")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

