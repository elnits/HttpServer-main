package nomenclature

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// CircuitBreakerState состояние Circuit Breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota // Нормальная работа
	StateOpen                               // Breaker открыт - запросы блокируются
	StateHalfOpen                           // Пробуем восстановить соединение
)

// CircuitBreaker защита от каскадных сбоев при проблемах с API
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitBreakerState
	failureCount    int           // Счетчик неудачных запросов
	successCount    int           // Счетчик успешных запросов в half-open состоянии
	failureThreshold int          // Порог ошибок для открытия breaker
	successThreshold int          // Порог успехов для закрытия breaker
	timeout         time.Duration // Время ожидания перед переходом в half-open
	lastFailureTime time.Time     // Время последней ошибки
}

// AIClient клиент для работы с Arliai API
type AIClient struct {
	apiKey         string
	baseURL        string
	model          string
	httpClient     *http.Client
	rateLimiter    *rate.Limiter     // Rate limiter для защиты от превышения квот API
	circuitBreaker *CircuitBreaker   // Circuit breaker для защиты от каскадных сбоев
}

// AIRequest структура запроса к API
type AIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

// Message сообщение в запросе
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIResponse структура ответа от API
type AIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// AIProcessingResult результат обработки ИИ
type AIProcessingResult struct {
	NormalizedName string  `json:"normalized_name"`
	KpvedCode      string  `json:"kpved_code"`
	KpvedName      string  `json:"kpved_name"`
	Confidence     float64 `json:"confidence"`
	Reasoning      string  `json:"reasoning,omitempty"`
}

// NewAIClient создает новый клиент для работы с Arliai API
func NewAIClient(apiKey, model string) *AIClient {
	// Rate limiter: 60 запросов в минуту (1 запрос/сек) с burst=5
	// Это защищает от превышения квот API и неконтролируемых расходов
	limiter := rate.NewLimiter(rate.Every(time.Second), 5)

	// Circuit Breaker: защита от каскадных сбоев
	// - 5 ошибок подряд -> открываем breaker (блокируем запросы)
	// - Ждем 30 секунд перед попыткой восстановления
	// - 2 успешных запроса -> закрываем breaker (нормальная работа)
	breaker := &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: 5,
		successThreshold: 2,
		timeout:          30 * time.Second,
	}

	return &AIClient{
		apiKey:  apiKey,
		baseURL: "https://api.arliai.com/v1/chat/completions",
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Увеличиваем таймаут для больших классификаторов
		},
		rateLimiter:    limiter,
		circuitBreaker: breaker,
	}
}

// ProcessProduct отправляет запрос к API для обработки товара
func (c *AIClient) ProcessProduct(productName, systemPrompt string) (*AIProcessingResult, error) {
	// Проверяем Circuit Breaker перед запросом
	if !c.circuitBreaker.canProceed() {
		return nil, fmt.Errorf("circuit breaker is open (state: %s), API calls are temporarily blocked", c.circuitBreaker.getState())
	}

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("НАИМЕНОВАНИЕ ТОВАРА ДЛЯ ОБРАБОТКИ: \"%s\"", productName),
		},
	}

	request := AIRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   1024,
		Stream:      false,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Применяем rate limiting перед запросом
	ctx := context.Background()
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %v", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Записываем ошибку в Circuit Breaker
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("API request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var aiResp AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	if aiResp.Error != nil {
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("API error: %s (type: %s)", aiResp.Error.Message, aiResp.Error.Type)
	}

	if len(aiResp.Choices) == 0 {
		c.circuitBreaker.recordFailure()
		return nil, fmt.Errorf("no choices in response")
	}

	result, err := c.parseAIResponse(aiResp.Choices[0].Message.Content, productName)
	if err != nil {
		c.circuitBreaker.recordFailure()
		return nil, err
	}

	// Успешный запрос - записываем в Circuit Breaker
	c.circuitBreaker.recordSuccess()
	return result, nil
}

// parseAIResponse парсит ответ от ИИ
func (c *AIClient) parseAIResponse(content, originalName string) (*AIProcessingResult, error) {
	// Очищаем ответ от возможных markdown обрамлений
	cleaned := c.cleanJSONResponse(content)

	var result AIProcessingResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %v, content: %s", err, cleaned)
	}

	// Валидация обязательных полей
	if result.NormalizedName == "" {
		return nil, fmt.Errorf("missing normalized_name in response")
	}
	
	// Если код пустой, считаем его как "unknown"
	if result.KpvedCode == "" {
		result.KpvedCode = "unknown"
	}
	
	if result.KpvedName == "" {
		return nil, fmt.Errorf("missing kpved_name in response")
	}

	return &result, nil
}

// cleanJSONResponse очищает JSON ответ от markdown обрамлений
func (c *AIClient) cleanJSONResponse(content string) string {
	cleaned := strings.TrimSpace(content)

	// Удаляем markdown code blocks
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
	}
	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
	}
	if strings.HasSuffix(cleaned, "```") {
		cleaned = strings.TrimSuffix(cleaned, "```")
	}

	return strings.TrimSpace(cleaned)
}

// GetCompletion универсальный метод для получения ответа от AI
// Возвращает очищенный JSON ответ для дальнейшей обработки
func (c *AIClient) GetCompletion(systemPrompt, userPrompt string) (string, error) {
	// Проверяем Circuit Breaker перед запросом
	if !c.circuitBreaker.canProceed() {
		return "", fmt.Errorf("circuit breaker is open (state: %s), API calls are temporarily blocked", c.circuitBreaker.getState())
	}

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	request := AIRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   1024,
		Stream:      false,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Применяем rate limiting перед запросом
	ctx := context.Background()
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limiter error: %v", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("API request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var aiResp AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if aiResp.Error != nil {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("API error: %s (type: %s)", aiResp.Error.Message, aiResp.Error.Type)
	}

	if len(aiResp.Choices) == 0 {
		c.circuitBreaker.recordFailure()
		return "", fmt.Errorf("no choices in response")
	}

	// Успешный запрос - записываем в Circuit Breaker
	c.circuitBreaker.recordSuccess()

	// Возвращаем очищенный ответ
	return c.cleanJSONResponse(aiResp.Choices[0].Message.Content), nil
}

// --- Circuit Breaker методы ---

// canProceed проверяет, можно ли выполнить запрос к API
// Возвращает false если Circuit Breaker открыт (слишком много ошибок)
func (cb *CircuitBreaker) canProceed() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		// Нормальная работа - пропускаем запрос
		return true

	case StateOpen:
		// Проверяем, прошло ли время для попытки восстановления
		if time.Since(cb.lastFailureTime) > cb.timeout {
			// Переходим в half-open для пробного запроса
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		// Breaker все еще открыт - блокируем запрос
		return false

	case StateHalfOpen:
		// В half-open состоянии пропускаем ограниченное количество запросов
		return true

	default:
		return false
	}
}

// recordSuccess записывает успешный запрос
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Сбрасываем счетчик ошибок при успешном запросе
		cb.failureCount = 0

	case StateHalfOpen:
		// Увеличиваем счетчик успехов в half-open
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			// Достаточно успехов - закрываем breaker
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
	}
}

// recordFailure записывает неудачный запрос
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			// Слишком много ошибок - открываем breaker
			cb.state = StateOpen
		}

	case StateHalfOpen:
		// Ошибка в half-open - возвращаемся в open
		cb.state = StateOpen
		cb.failureCount = cb.failureThreshold // Устанавливаем максимальное значение
		cb.successCount = 0
	}
}

// getState возвращает текущее состояние Circuit Breaker (для логирования)
func (cb *CircuitBreaker) getState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// GetCircuitBreakerState возвращает детальное состояние Circuit Breaker для мониторинга
func (c *AIClient) GetCircuitBreakerState() map[string]interface{} {
	if c.circuitBreaker == nil {
		return map[string]interface{}{
			"enabled":        false,
			"state":          "unknown",
			"can_proceed":    false,
			"failure_count":  0,
			"success_count":  0,
			"last_failure_time": nil,
		}
	}

	c.circuitBreaker.mu.RLock()
	defer c.circuitBreaker.mu.RUnlock()

	stateStr := "unknown"
	switch c.circuitBreaker.state {
	case StateClosed:
		stateStr = "closed"
	case StateOpen:
		stateStr = "open"
	case StateHalfOpen:
		stateStr = "half-open"
	}

	canProceed := false
	switch c.circuitBreaker.state {
	case StateClosed:
		canProceed = true
	case StateOpen:
		canProceed = time.Since(c.circuitBreaker.lastFailureTime) > c.circuitBreaker.timeout
	case StateHalfOpen:
		canProceed = true
	}

	var lastFailureTime *string
	if !c.circuitBreaker.lastFailureTime.IsZero() {
		timeStr := c.circuitBreaker.lastFailureTime.Format(time.RFC3339)
		lastFailureTime = &timeStr
	}

	return map[string]interface{}{
		"enabled":          true,
		"state":            stateStr,
		"can_proceed":      canProceed,
		"failure_count":    c.circuitBreaker.failureCount,
		"success_count":    c.circuitBreaker.successCount,
		"last_failure_time": lastFailureTime,
	}
}

