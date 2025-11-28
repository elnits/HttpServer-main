package server

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config конфигурация сервера
type Config struct {
	// Сервер
	Port string

	// Базы данных
	DatabasePath          string
	NormalizedDatabasePath string
	ServiceDatabasePath   string

	// AI конфигурация
	ArliaiAPIKey string
	ArliaiModel  string

	// Connection pooling
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration

	// Логирование
	LogBufferSize int

	// Нормализация
	NormalizerEventsBufferSize int
}

// LoadConfig загружает конфигурацию из переменных окружения
func LoadConfig() (*Config, error) {
	config := &Config{
		// Сервер
		Port: getEnv("SERVER_PORT", "9999"),

		// Базы данных
		DatabasePath:           getEnv("DATABASE_PATH", "data.db"),
		NormalizedDatabasePath: getEnv("NORMALIZED_DATABASE_PATH", "normalized_data.db"),
		ServiceDatabasePath:    getEnv("SERVICE_DATABASE_PATH", "service.db"),

		// AI конфигурация
		ArliaiAPIKey: os.Getenv("ARLIAI_API_KEY"),
		ArliaiModel:  getEnv("ARLIAI_MODEL", "GLM-4.5-Air"),

		// Connection pooling
		MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),

		// Логирование
		LogBufferSize: getEnvInt("LOG_BUFFER_SIZE", 100),

		// Нормализация
		NormalizerEventsBufferSize: getEnvInt("NORMALIZER_EVENTS_BUFFER_SIZE", 100),
	}

	// Валидация
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate валидирует конфигурацию
func (c *Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}

	if c.DatabasePath == "" {
		return fmt.Errorf("database path is required")
	}

	if c.NormalizedDatabasePath == "" {
		return fmt.Errorf("normalized database path is required")
	}

	if c.ServiceDatabasePath == "" {
		return fmt.Errorf("service database path is required")
	}

	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("max open connections must be greater than 0")
	}

	if c.MaxIdleConns <= 0 {
		return fmt.Errorf("max idle connections must be greater than 0")
	}

	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max idle connections cannot be greater than max open connections")
	}

	return nil
}

// getEnv получает переменную окружения или возвращает значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt получает переменную окружения как int или возвращает значение по умолчанию
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvDuration получает переменную окружения как Duration или возвращает значение по умолчанию
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

