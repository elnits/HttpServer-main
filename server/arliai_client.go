package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// ArliaiClient клиент для работы с Arliai API
type ArliaiClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	retryConfig RetryConfig
}

// RetryConfig конфигурация повторных попыток
type RetryConfig struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffMultiplier float64
}

// ArliaiStatusResponse ответ от Arliai API о статусе
type ArliaiStatusResponse struct {
	Status    string    `json:"status"`
	Model     string    `json:"model,omitempty"`
	Version   string    `json:"version,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ArliaiModel модель Arliai
type ArliaiModel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Speed       string   `json:"speed"`
	Quality     string   `json:"quality"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"` // active, deprecated, beta
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ArliaiModelsResponse ответ со списком моделей
type ArliaiModelsResponse struct {
	Models []ArliaiModel `json:"models"`
}

// APIError стандартизированная ошибка API
type APIError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// APIResponse стандартизированный ответ API
type APIResponse struct {
	Success   bool                   `json:"success"`
	Data      interface{}            `json:"data,omitempty"`
	Error     *APIError              `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration_ms,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewArliaiClient создает новый клиент Arliai
func NewArliaiClient() *ArliaiClient {
	baseURL := os.Getenv("ARLIAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.arliai.com/v1"
	}

	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		log.Println("Warning: ARLIAI_API_KEY not set")
	}

	// Валидируем конфигурацию (логируем предупреждения, но не прерываем работу)
	if err := ValidateArliaiConfig(baseURL, apiKey); err != nil {
		log.Printf("Warning: Arliai configuration validation failed: %v", err)
	}

	return &ArliaiClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		retryConfig: RetryConfig{
			MaxRetries:       3,
			InitialDelay:     500 * time.Millisecond,
			MaxDelay:         10 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}
}

// CheckConnection проверяет подключение к Arliai API с повторными попытками
func (c *ArliaiClient) CheckConnection(ctx context.Context, requestID string) (*ArliaiStatusResponse, error) {
	startTime := time.Now()
	
	url := fmt.Sprintf("%s/health", c.baseURL)
	
	var lastErr error
	delay := c.retryConfig.InitialDelay
	
	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[%s] Retry attempt %d/%d after %v", requestID, attempt, c.retryConfig.MaxRetries, delay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * c.retryConfig.BackoffMultiplier)
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if c.apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
		}
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set("Content-Type", "application/json")

		log.Printf("[%s] Checking Arliai connection (attempt %d/%d)", requestID, attempt+1, c.retryConfig.MaxRetries+1)
		
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			log.Printf("[%s] Connection check failed: %v", requestID, lastErr)
			continue
		}

		duration := time.Since(startTime)
		
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			
			var statusResp ArliaiStatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				lastErr = fmt.Errorf("failed to decode response: %w", err)
				log.Printf("[%s] Failed to decode response: %v", requestID, lastErr)
				resp.Body.Close()
				continue
			}

			statusResp.Timestamp = time.Now()
			log.Printf("[%s] Connection check successful (duration: %v, status: %s)", requestID, duration, statusResp.Status)
			return &statusResp, nil
		}

		resp.Body.Close()
		
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			log.Printf("[%s] Server error %d, will retry", requestID, resp.StatusCode)
			continue
		}

		// 4xx ошибки не повторяем
		lastErr = fmt.Errorf("client error: %d", resp.StatusCode)
		log.Printf("[%s] Client error %d, not retrying", requestID, resp.StatusCode)
		break
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// GetModels получает список моделей с повторными попытками
func (c *ArliaiClient) GetModels(ctx context.Context, requestID string) ([]ArliaiModel, error) {
	startTime := time.Now()
	
	url := fmt.Sprintf("%s/models", c.baseURL)
	
	var lastErr error
	delay := c.retryConfig.InitialDelay
	
	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[%s] Retry attempt %d/%d for models after %v", requestID, attempt, c.retryConfig.MaxRetries, delay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * c.retryConfig.BackoffMultiplier)
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if c.apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
		}
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set("Content-Type", "application/json")

		log.Printf("[%s] Fetching models (attempt %d/%d)", requestID, attempt+1, c.retryConfig.MaxRetries+1)
		
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			log.Printf("[%s] Models fetch failed: %v", requestID, lastErr)
			continue
		}

		duration := time.Since(startTime)
		
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				lastErr = fmt.Errorf("failed to read response: %w", err)
				log.Printf("[%s] Failed to read response: %v", requestID, lastErr)
				continue
			}

			var modelsResp ArliaiModelsResponse
			if err := json.Unmarshal(body, &modelsResp); err != nil {
				// Пробуем альтернативный формат (прямой массив)
				var models []ArliaiModel
				if err2 := json.Unmarshal(body, &models); err2 != nil {
					lastErr = fmt.Errorf("failed to decode response: %w (also tried array format: %v)", err, err2)
					log.Printf("[%s] Failed to decode models response: %v", requestID, lastErr)
					continue
				}
				modelsResp.Models = models
			}

			log.Printf("[%s] Models fetched successfully (duration: %v, count: %d)", requestID, duration, len(modelsResp.Models))
			return modelsResp.Models, nil
		}

		resp.Body.Close()
		
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			log.Printf("[%s] Server error %d, will retry", requestID, resp.StatusCode)
			continue
		}

		lastErr = fmt.Errorf("client error: %d", resp.StatusCode)
		log.Printf("[%s] Client error %d, not retrying", requestID, resp.StatusCode)
		break
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// GenerateTraceID генерирует уникальный trace ID
func GenerateTraceID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix())
}

