package normalization

import (
	"fmt"
	"strings"
	"sync"

	"httpserver/database"
)

// Benchmark представляет эталонную запись
type Benchmark struct {
	ID               int
	ClientProjectID  int
	OriginalName     string
	NormalizedName   string
	Category         string
	Subcategory      string
	Attributes       string
	QualityScore     float64
	IsApproved       bool
	UsageCount       int
}

// ClientBenchmarkStore хранилище эталонных записей клиента
type ClientBenchmarkStore struct {
	db        *database.ServiceDB
	projectID int
	cache     map[string]*Benchmark
	mutex     sync.RWMutex
}

// NewClientBenchmarkStore создает новое хранилище эталонов
func NewClientBenchmarkStore(serviceDB *database.ServiceDB, projectID int) *ClientBenchmarkStore {
	return &ClientBenchmarkStore{
		db:        serviceDB,
		projectID: projectID,
		cache:     make(map[string]*Benchmark),
	}
}

// FindBenchmark ищет эталон по названию
func (s *ClientBenchmarkStore) FindBenchmark(name string) (*Benchmark, bool) {
	// Нормализуем имя для поиска
	searchKey := strings.ToLower(strings.TrimSpace(name))
	
	// Проверяем кэш
	s.mutex.RLock()
	if benchmark, found := s.cache[searchKey]; found {
		s.mutex.RUnlock()
		return benchmark, true
	}
	s.mutex.RUnlock()

	// Поиск в базе данных
	benchmark, err := s.db.FindClientBenchmark(s.projectID, name)
	if err != nil || benchmark == nil {
		return nil, false
	}

	// Преобразуем в Benchmark
	result := &Benchmark{
		ID:              benchmark.ID,
		ClientProjectID: benchmark.ClientProjectID,
		OriginalName:    benchmark.OriginalName,
		NormalizedName:  benchmark.NormalizedName,
		Category:        benchmark.Category,
		Subcategory:     benchmark.Subcategory,
		Attributes:      benchmark.Attributes,
		QualityScore:    benchmark.QualityScore,
		IsApproved:      benchmark.IsApproved,
		UsageCount:      benchmark.UsageCount,
	}

	// Сохраняем в кэш
	s.mutex.Lock()
	s.cache[searchKey] = result
	s.mutex.Unlock()

	return result, true
}

// SavePotentialBenchmark сохраняет потенциальный эталон
func (s *ClientBenchmarkStore) SavePotentialBenchmark(originalName, normalizedName, category, subcategory string, qualityScore float64) error {
	// Сохраняем только если качество достаточно высокое
	if qualityScore < 0.9 {
		return nil
	}

	_, err := s.db.CreateClientBenchmark(
		s.projectID,
		originalName,
		normalizedName,
		category,
		subcategory,
		"", // attributes
		"", // source_database
		qualityScore,
	)

	return err
}

// GetBenchmarksByCategory получает эталоны по категории
func (s *ClientBenchmarkStore) GetBenchmarksByCategory(category string) ([]*Benchmark, error) {
	benchmarks, err := s.db.GetClientBenchmarks(s.projectID, category, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get benchmarks: %w", err)
	}

	result := make([]*Benchmark, len(benchmarks))
	for i, b := range benchmarks {
		result[i] = &Benchmark{
			ID:              b.ID,
			ClientProjectID: b.ClientProjectID,
			OriginalName:    b.OriginalName,
			NormalizedName:  b.NormalizedName,
			Category:        b.Category,
			Subcategory:     b.Subcategory,
			Attributes:      b.Attributes,
			QualityScore:    b.QualityScore,
			IsApproved:      b.IsApproved,
			UsageCount:      b.UsageCount,
		}
	}

	return result, nil
}

// UpdateUsage увеличивает счетчик использования эталона
func (s *ClientBenchmarkStore) UpdateUsage(benchmarkID int) error {
	return s.db.UpdateBenchmarkUsage(benchmarkID)
}

