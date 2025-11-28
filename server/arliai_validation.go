package server

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateArliaiConfig проверяет конфигурацию Arliai клиента
func ValidateArliaiConfig(baseURL, apiKey string) error {
	if baseURL == "" {
		return fmt.Errorf("ARLIAI_BASE_URL is required")
	}

	// Проверяем, что URL валидный
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid ARLIAI_BASE_URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("ARLIAI_BASE_URL must use http or https scheme")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("ARLIAI_BASE_URL must have a valid host")
	}

	// API ключ не обязателен, но если указан, должен быть не пустым
	if apiKey != "" && strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("ARLIAI_API_KEY cannot be empty if provided")
	}

	return nil
}

// ValidateModelName проверяет валидность имени модели
func ValidateModelName(modelName string) error {
	if modelName == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	if len(modelName) > 100 {
		return fmt.Errorf("model name too long (max 100 characters)")
	}

	// Проверяем на недопустимые символы
	if strings.ContainsAny(modelName, "\n\r\t") {
		return fmt.Errorf("model name contains invalid characters")
	}

	return nil
}

// SanitizeModelName очищает имя модели от недопустимых символов
func SanitizeModelName(modelName string) string {
	// Удаляем пробелы в начале и конце
	modelName = strings.TrimSpace(modelName)
	
	// Удаляем недопустимые символы
	modelName = strings.ReplaceAll(modelName, "\n", "")
	modelName = strings.ReplaceAll(modelName, "\r", "")
	modelName = strings.ReplaceAll(modelName, "\t", "")
	
	// Ограничиваем длину
	if len(modelName) > 100 {
		modelName = modelName[:100]
	}
	
	return modelName
}

