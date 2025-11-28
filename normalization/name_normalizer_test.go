package normalization

import (
	"testing"
)

func TestNormalizeName(t *testing.T) {
	normalizer := NewNameNormalizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Простое название",
			input:    "Молоток",
			expected: "молоток",
		},
		{
			name:     "С техническим кодом",
			input:    "Молоток ER-00013004",
			expected: "молоток er", // Технический код удаляется, но "er" остается как часть слова
		},
		{
			name:     "С размерами",
			input:    "Панель 100x100",
			expected: "панель",
		},
		{
			name:     "С единицами измерения",
			input:    "Кабель 50м",
			expected: "кабель",
		},
		{
			name:     "С лишними пробелами",
			input:    "Молоток    большой",
			expected: "молоток большой",
		},
		{
			name:     "Смешанный регистр",
			input:    "МоЛоТоК",
			expected: "молоток",
		},
		{
			name:     "С числами",
			input:    "Товар 123",
			expected: "товар",
		},
		{
			name:     "Пустая строка",
			input:    "",
			expected: "",
		},
		{
			name:     "Только спецсимволы",
			input:    "!!!",
			expected: "!!!", // Спецсимволы не удаляются NameNormalizer (это делает PatternDetector)
		},
		{
			name:     "Комплексный пример",
			input:    "Молоток ER-00013004 100x100 50кг",
			expected: "молоток er", // После удаления паттернов остается "er"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRemoveExtraSpaces(t *testing.T) {
	normalizer := NewNameNormalizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Множественные пробелы",
			input:    "Молоток    большой",
			expected: "молоток большой",
		},
		{
			name:     "Пробелы в начале и конце",
			input:    "   Молоток   ",
			expected: "молоток",
		},
		{
			name:     "Табуляции и переносы",
			input:    "Молоток\t\tбольшой\n\n",
			expected: "молоток большой",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCaseHandling(t *testing.T) {
	normalizer := NewNameNormalizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Верхний регистр",
			input:    "МОЛОТОК",
			expected: "молоток",
		},
		{
			name:     "Нижний регистр",
			input:    "молоток",
			expected: "молоток",
		},
		{
			name:     "Смешанный регистр",
			input:    "МоЛоТоК",
			expected: "молоток",
		},
		{
			name:     "С заглавной буквы",
			input:    "Молоток",
			expected: "молоток",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSpecialCharacters(t *testing.T) {
	normalizer := NewNameNormalizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "С дефисом",
			input:    "Молоток-большой",
			expected: "молоток-большой",
		},
		{
			name:     "С точкой",
			input:    "Молоток.Большой",
			expected: "молоток.большой",
		},
		{
			name:     "С запятой",
			input:    "Молоток, большой",
			expected: "молоток, большой", // Запятая не удаляется NameNormalizer (удаляется только при Trim в конце)
		},
		{
			name:     "С восклицательным знаком",
			input:    "Молоток!",
			expected: "молоток!", // Восклицательный знак не удаляется NameNormalizer
		},
		{
			name:     "С русской буквой х в размерах",
			input:    "Панель 100х100",
			expected: "панель",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

