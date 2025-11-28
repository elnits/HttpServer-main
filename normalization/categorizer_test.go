package normalization

import (
	"testing"
)

func TestCategorizeItem(t *testing.T) {
	categorizer := NewCategorizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Инструмент",
			input:    "Молоток большой",
			expected: "инструмент",
		},
		{
			name:     "Медикаменты",
			input:    "Антибиотик энрофлоксацин",
			expected: "медикаменты",
		},
		{
			name:     "Стройматериалы",
			input:    "Панель металлическая",
			expected: "стройматериалы", // "Панель" находится только в "стройматериалы"
		},
		{
			name:     "Электроника",
			input:    "Компьютер",
			expected: "электроника", // "Компьютер" находится в категории "электроника"
		},
		{
			name:     "Оборудование",
			input:    "Насос водяной",
			expected: "оборудование",
		},
		{
			name:     "Расходники",
			input:    "Скотч двусторонний",
			expected: "расходники",
		},
		{
			name:     "Канцелярия",
			input:    "Ручка шариковая",
			expected: "канцелярия",
		},
		{
			name:     "Продукты",
			input:    "Колбаса копченая",
			expected: "продукты",
		},
		{
			name:     "Неизвестная категория",
			input:    "Неизвестный товар",
			expected: "другое",
		},
		{
			name:     "Пустая строка",
			input:    "",
			expected: "другое",
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

func TestCategoryMatching(t *testing.T) {
	categorizer := NewCategorizer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Множественные ключевые слова",
			input:    "Молоток и отвертка",
			expected: "инструмент",
		},
		{
			name:     "Ключевое слово в середине",
			input:    "Большой молоток стальной",
			expected: "инструмент",
		},
		{
			name:     "Регистр не важен",
			input:    "МОЛОТОК",
			expected: "инструмент",
		},
		{
			name:     "Часть слова",
			input:    "Молотковый",
			expected: "другое", // "Молотковый" не содержит точное совпадение "молоток"
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

func TestEmptyCategory(t *testing.T) {
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
			name:     "Неизвестный товар",
			input:    "Абсолютно неизвестный товар",
			expected: "другое",
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

