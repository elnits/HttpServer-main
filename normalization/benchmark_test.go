package normalization

import (
	"strings"
	"testing"
)

// BenchmarkNormalizeName тестирует производительность нормализации имен
func BenchmarkNormalizeName(b *testing.B) {
	normalizer := NewNameNormalizer()
	testName := "Молоток ER-00013004 100x100 50кг большой стальной"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizer.NormalizeName(testName)
	}
}

// BenchmarkNormalizeNameLong тестирует производительность на длинных строках
func BenchmarkNormalizeNameLong(b *testing.B) {
	normalizer := NewNameNormalizer()
	testName := strings.Repeat("Молоток ER-00013004 100x100 ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizer.NormalizeName(testName)
	}
}

// BenchmarkCategorize тестирует производительность категоризации
func BenchmarkCategorize(b *testing.B) {
	categorizer := NewCategorizer()
	testName := "Молоток большой стальной"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		categorizer.Categorize(testName)
	}
}

// BenchmarkCategorizeLong тестирует производительность на длинных строках
func BenchmarkCategorizeLong(b *testing.B) {
	categorizer := NewCategorizer()
	testName := strings.Repeat("Молоток большой стальной ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		categorizer.Categorize(testName)
	}
}

// BenchmarkDetectPatterns тестирует производительность обнаружения паттернов
func BenchmarkDetectPatterns(b *testing.B) {
	detector := NewPatternDetector()
	testName := "Товар ER-00013004 100x100 арт.123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectPatterns(testName)
	}
}

// BenchmarkDetectPatternsLong тестирует производительность на длинных строках
func BenchmarkDetectPatternsLong(b *testing.B) {
	detector := NewPatternDetector()
	testName := strings.Repeat("Товар ER-00013004 100x100 ", 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectPatterns(testName)
	}
}

// BenchmarkApplyPatterns тестирует производительность применения паттернов
func BenchmarkApplyPatterns(b *testing.B) {
	detector := NewPatternDetector()
	testName := "Товар ER-00013004 100x100 арт.123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches := detector.DetectPatterns(testName)
		detector.ApplyFixes(testName, matches)
	}
}

// BenchmarkFullNormalization тестирует производительность полной нормализации
func BenchmarkFullNormalization(b *testing.B) {
	categorizer := NewCategorizer()
	nameNormalizer := NewNameNormalizer()
	patternDetector := NewPatternDetector()
	testName := "Молоток ER-00013004 100x100 50кг большой стальной"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		category := categorizer.Categorize(testName)
		normalizedName := nameNormalizer.NormalizeName(testName)
		matches := patternDetector.DetectPatterns(testName)
		fixed := patternDetector.ApplyFixes(testName, matches)
		_ = category
		_ = normalizedName
		_ = fixed
	}
}

