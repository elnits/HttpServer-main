package nomenclature

import (
	"os"
	"time"
)

// Config конфигурация обработчика номенклатуры
type Config struct {
	DatabasePath    string
	KpvedFilePath   string
	ArliaiAPIKey    string
	AIModel         string
	MaxWorkers      int
	BatchSize       int
	MaxRetries      int
	RequestTimeout  time.Duration
	RateLimitDelay  time.Duration
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() Config {
	// Получаем модель из переменной окружения, если она установлена
	model := os.Getenv("ARLIAI_MODEL")
	if model == "" {
		model = "GLM-4.5-Air" // Дефолтная модель
	}
	
	return Config{
		DatabasePath:   "./normalized_data.db",
		KpvedFilePath:  "./КПВЭД.txt",
		AIModel:        model,
		MaxWorkers:     2, // Строго 2 потока согласно лимиту API
		BatchSize:      50,
		MaxRetries:      3,
		RequestTimeout:  30 * time.Second,
		RateLimitDelay:  500 * time.Millisecond,
	}
}

