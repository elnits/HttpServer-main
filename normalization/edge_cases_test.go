package normalization

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestEdgeCases тестирует граничные случаи для всех модулей нормализации

func TestNameNormalizerEdgeCases(t *testing.T) {
	normalizer := NewNameNormalizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Пустая строка",
			input:    "",
			expected: "",
		},
		{
			name:     "Только пробелы",
			input:    "   ",
			expected: "",
		},
		{
			name:     "Очень длинная строка",
			input:    strings.Repeat("Товар ", 1000),
			expected: strings.ToLower(strings.TrimSpace(strings.Repeat("Товар ", 1000))),
		},
		{
			name:     "Unicode символы",
			input:    "Товар 🛠️ ⚙️",
			expected: "товар 🛠️ ⚙️",
		},
		{
			name:     "Смешанные языки",
			input:    "Товар Product Item",
			expected: "товар product item",
		},
		{
			name:     "Только спецсимволы",
			input:    "!@#$%^&*()",
			expected: "!@#$%^&*()",
		},
		{
			name:     "С нулевыми байтами",
			input:    "Товар\x00тест",
			expected: "товар\x00тест",
		},
		{
			name:     "С табуляциями",
			input:    "Товар\tбольшой",
			expected: "товар большой",
		},
		{
			name:     "С переносами строк",
			input:    "Товар\nбольшой",
			expected: "товар большой",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			// Проверяем, что результат валидный UTF-8
			if !utf8.ValidString(result) {
				t.Errorf("Result is not valid UTF-8: %q", result)
			}
			// Для очень длинных строк проверяем только, что результат не пустой
			if tt.name == "Очень длинная строка" {
				if len(result) == 0 {
					t.Error("Result should not be empty for long string")
				}
			} else if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCategorizerEdgeCases(t *testing.T) {
	categorizer := NewCategorizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Пустая строка",
			input:    "",
			expected: "другое",
		},
		{
			name:     "Только пробелы",
			input:    "   ",
			expected: "другое",
		},
		{
			name:     "Очень длинная строка",
			input:    strings.Repeat("молоток ", 1000),
			expected: "инструмент",
		},
		{
			name:     "Unicode символы",
			input:    "🛠️ инструмент",
			expected: "другое",
		},
		{
			name:     "Смешанные языки",
			input:    "Tool молоток",
			expected: "инструмент",
		},
		{
			name:     "Только спецсимволы",
			input:    "!@#$%",
			expected: "другое",
		},
		{
			name:     "С нулевыми байтами",
			input:    "молоток\x00",
			expected: "инструмент",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := categorizer.Categorize(tt.input)
			if result != tt.expected {
				t.Errorf("Categorize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPatternDetectorEdgeCases(t *testing.T) {
	detector := NewPatternDetector()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Пустая строка",
			input: "",
		},
		{
			name:  "Только пробелы",
			input: "   ",
		},
		{
			name:  "Очень длинная строка",
			input: strings.Repeat("Товар ER-00013004 ", 100),
		},
		{
			name:  "Unicode символы",
			input: "Товар 🛠️ ER-00013004",
		},
		{
			name:  "Смешанные языки",
			input: "Product ER-00013004",
		},
		{
			name:  "Только спецсимволы",
			input: "!@#$%^&*()",
		},
		{
			name:  "С нулевыми байтами",
			input: "Товар\x00ER-00013004",
		},
		{
			name:  "Множественные технические коды",
			input: "Товар ER-00013004 AB-12345 CD-67890",
		},
		{
			name:  "Множественные размеры",
			input: "Панель 100x100 200x200 300x300",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.DetectPatterns(tt.input)
			// Проверяем, что функция не паникует
			_ = matches

			// Применяем исправления
			fixed := detector.ApplyFixes(tt.input, matches)
			// Проверяем, что результат валидный UTF-8
			if !utf8.ValidString(fixed) {
				t.Errorf("Fixed result is not valid UTF-8: %q", fixed)
			}
		})
	}
}

func TestNormalizerEdgeCases(t *testing.T) {
	// Тесты для граничных случаев Normalizer требуют БД
	// Здесь проверяем только базовые случаи без БД

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Пустая строка",
			input: "",
		},
		{
			name:  "Очень длинная строка",
			input: strings.Repeat("Товар ", 10000),
		},
		{
			name:  "Unicode символы",
			input: "Товар 🛠️ ⚙️ 🔧",
		},
		{
			name:  "Смешанные языки",
			input: "Product Товар Item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем, что компоненты нормализатора обрабатывают граничные случаи
			categorizer := NewCategorizer()
			nameNormalizer := NewNameNormalizer()
			patternDetector := NewPatternDetector()

			category := categorizer.Categorize(tt.input)
			normalizedName := nameNormalizer.NormalizeName(tt.input)
			matches := patternDetector.DetectPatterns(tt.input)

			// Проверяем, что функции не паникуют и возвращают валидные результаты
			if !utf8.ValidString(category) {
				t.Errorf("Category is not valid UTF-8: %q", category)
			}
			if !utf8.ValidString(normalizedName) {
				t.Errorf("NormalizedName is not valid UTF-8: %q", normalizedName)
			}
			_ = matches // Проверяем только, что не паникует
		})
	}
}

func TestNegativeNumbers(t *testing.T) {
	// Тест для проверки обработки отрицательных чисел (где недопустимы)
	normalizer := NewNameNormalizer()

	// Отрицательные числа не должны встречаться в названиях товаров
	// Но если встречаются, функция должна их обработать
	input := "Товар -100"
	result := normalizer.NormalizeName(input)

	// Проверяем, что функция не паникует
	if !utf8.ValidString(result) {
		t.Errorf("Result is not valid UTF-8: %q", result)
	}
}

func TestVeryLargeNumbers(t *testing.T) {
	// Тест для очень больших чисел
	normalizer := NewNameNormalizer()

	input := "Товар 999999999999999999"
	result := normalizer.NormalizeName(input)

	// Проверяем, что функция не паникует
	if !utf8.ValidString(result) {
		t.Errorf("Result is not valid UTF-8: %q", result)
	}
}

