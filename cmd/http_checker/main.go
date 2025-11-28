package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HTTPCheckResult результат проверки URL
type HTTPCheckResult struct {
	URL              string            `json:"url"`
	Status           int               `json:"status"`
	StatusText      string            `json:"status_text"`
	ResponseTime     time.Duration     `json:"response_time_ms"`
	Headers          map[string]string `json:"headers"`
	Error            string            `json:"error,omitempty"`
	Attempts         int               `json:"attempts"`
	Timestamp        time.Time         `json:"timestamp"`
	Category         string            `json:"category"`
	ExpectedStatus   int               `json:"expected_status,omitempty"`
	IsValid          bool              `json:"is_valid"`
	ValidationErrors []string          `json:"validation_errors,omitempty"`
}

// CheckConfig конфигурация проверки
type CheckConfig struct {
	URLs             []URLCheck       `json:"urls"`
	Timeout          time.Duration    `json:"timeout_seconds"`
	MaxRetries       int               `json:"max_retries"`
	RetryDelay       time.Duration     `json:"retry_delay_seconds"`
	ConcurrentChecks int               `json:"concurrent_checks"`
	UserAgent        string            `json:"user_agent"`
	Headers          map[string]string `json:"headers"`
	FollowRedirects  bool              `json:"follow_redirects"`
	MaxRedirects     int               `json:"max_redirects"`
}

// URLCheck конфигурация проверки конкретного URL
type URLCheck struct {
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	ExpectedStatus int               `json:"expected_status"`
	Category       string            `json:"category"`
	RequiredHeaders map[string]string `json:"required_headers,omitempty"`
	Timeout        *time.Duration    `json:"timeout_seconds,omitempty"`
}

// Report отчет о проверках
type Report struct {
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Duration     time.Duration     `json:"duration_seconds"`
	TotalChecks  int               `json:"total_checks"`
	Results      []HTTPCheckResult `json:"results"`
	Summary      ReportSummary     `json:"summary"`
}

// ReportSummary сводка отчета
type ReportSummary struct {
	Success      int `json:"success"`
	ClientErrors int `json:"client_errors"`
	ServerErrors int `json:"server_errors"`
	Redirects    int `json:"redirects"`
	Timeouts     int `json:"timeouts"`
	Invalid      int `json:"invalid"`
	TotalErrors  int `json:"total_errors"`
}

var (
	logFile *os.File
	logger  *log.Logger
)

func main() {
	configFile := flag.String("config", "http_check_config.json", "Путь к файлу конфигурации")
	urlsFile := flag.String("urls", "", "Файл со списком URL (по одному на строку)")
	outputFile := flag.String("output", "", "Файл для сохранения отчета (JSON)")
	logPath := flag.String("log", "", "Путь к файлу лога")
	timeout := flag.Duration("timeout", 7*time.Second, "Таймаут для запросов")
	maxRetries := flag.Int("retries", 3, "Максимальное количество повторов")
	concurrent := flag.Int("concurrent", 5, "Количество одновременных проверок")
	flag.Parse()

	// Настройка логирования
	setupLogging(*logPath)

	// Загрузка конфигурации
	config, err := loadConfig(*configFile, *urlsFile, *timeout, *maxRetries, *concurrent)
	if err != nil {
		logger.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	logger.Printf("🚀 Начало проверки HTTP статусов")
	logger.Printf("📊 Всего URL для проверки: %d", len(config.URLs))
	logger.Printf("⚙️  Параметры: timeout=%v, retries=%d, concurrent=%d", 
		config.Timeout, config.MaxRetries, config.ConcurrentChecks)

	startTime := time.Now()

	// Выполнение проверок
	results := performChecks(config)

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Генерация отчета
	report := generateReport(results, startTime, endTime, duration)

	// Вывод результатов
	printReport(report)

	// Сохранение отчета
	if *outputFile != "" {
		if err := saveReport(report, *outputFile); err != nil {
			logger.Printf("⚠️  Ошибка сохранения отчета: %v", err)
		} else {
			logger.Printf("✅ Отчет сохранен: %s", *outputFile)
		}
	}

	// Проверка критических ошибок
	criticalErrors := checkCriticalErrors(report)
	if len(criticalErrors) > 0 {
		logger.Printf("🔴 Обнаружены критические ошибки:")
		for _, err := range criticalErrors {
			logger.Printf("   - %s", err)
		}
	}

	// Отправка уведомлений
	notifierConfig := loadNotifierConfig()
	if notifierConfig.Enabled {
		if err := sendNotification(report, notifierConfig); err != nil {
			logger.Printf("⚠️  Ошибка отправки уведомления: %v", err)
		}
	}

	if len(criticalErrors) > 0 {
		os.Exit(1)
	}

	logger.Printf("✅ Проверка завершена успешно")
}

func setupLogging(logPath string) {
	if logPath == "" {
		logPath = filepath.Join("logs", fmt.Sprintf("http_check_%s.log", time.Now().Format("20060102_150405")))
	}

	// Создаем директорию для логов
	logDir := filepath.Dir(logPath)
	if logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Fatalf("Не удалось создать директорию логов: %v", err)
		}
	}

	var err error
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Не удалось открыть файл лога: %v", err)
	}

	// Логирование в файл и консоль
	logger = log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags|log.Lmicroseconds)
	logger.Printf("📝 Логирование в файл: %s", logPath)
}

func loadConfig(configFile, urlsFile string, timeout time.Duration, maxRetries, concurrent int) (*CheckConfig, error) {
	config := &CheckConfig{
		Timeout:          timeout,
		MaxRetries:       maxRetries,
		RetryDelay:       1 * time.Second,
		ConcurrentChecks: concurrent,
		UserAgent:        "HTTP-Checker/1.0",
		FollowRedirects:  true,
		MaxRedirects:     5,
		Headers:          make(map[string]string),
	}

	// Загрузка из JSON конфигурации
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения конфигурации: %w", err)
		}

		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("ошибка парсинга конфигурации: %w", err)
		}

		logger.Printf("✅ Конфигурация загружена из: %s", configFile)
	}

	// Загрузка URL из файла
	if urlsFile != "" {
		urls, err := loadURLsFromFile(urlsFile)
		if err != nil {
			return nil, fmt.Errorf("ошибка загрузки URL: %w", err)
		}

		// Добавляем URL из файла к существующим
		for _, url := range urls {
			config.URLs = append(config.URLs, URLCheck{
				URL:      url,
				Method:   "GET",
				Category: "general",
			})
		}

		logger.Printf("✅ Загружено %d URL из файла: %s", len(urls), urlsFile)
	}

	// Если нет URL в конфигурации, используем аргументы командной строки
	if len(config.URLs) == 0 {
		args := flag.Args()
		if len(args) > 0 {
			for _, url := range args {
				config.URLs = append(config.URLs, URLCheck{
					URL:      url,
					Method:   "GET",
					Category: "general",
				})
			}
		}
	}

	if len(config.URLs) == 0 {
		return nil, fmt.Errorf("не указаны URL для проверки")
	}

	return config, nil
}

func loadURLsFromFile(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var urls []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}

	return urls, nil
}

func performChecks(config *CheckConfig) []HTTPCheckResult {
	results := make([]HTTPCheckResult, 0, len(config.URLs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Семафор для ограничения количества одновременных запросов
	semaphore := make(chan struct{}, config.ConcurrentChecks)

	for _, urlCheck := range config.URLs {
		wg.Add(1)
		go func(check URLCheck) {
			defer wg.Done()

			// Получаем слот для выполнения
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := checkURL(check, config)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(urlCheck)
	}

	wg.Wait()
	return results
}

func checkURL(urlCheck URLCheck, config *CheckConfig) HTTPCheckResult {
	result := HTTPCheckResult{
		URL:            urlCheck.URL,
		Timestamp:      time.Now(),
		Category:       urlCheck.Category,
		ExpectedStatus: urlCheck.ExpectedStatus,
		Headers:        make(map[string]string),
		Attempts:       0,
	}

	timeout := config.Timeout
	if urlCheck.Timeout != nil {
		timeout = *urlCheck.Timeout
	}

	method := urlCheck.Method
	if method == "" {
		method = "GET"
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !config.FollowRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("превышено максимальное количество редиректов: %d", config.MaxRedirects)
			}
			return nil
		},
	}

	var lastErr error
	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		result.Attempts = attempt
		startTime := time.Now()

		req, err := http.NewRequest(method, urlCheck.URL, nil)
		if err != nil {
			lastErr = err
			time.Sleep(config.RetryDelay)
			continue
		}

		// Устанавливаем заголовки
		req.Header.Set("User-Agent", config.UserAgent)
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}

		resp, err := client.Do(req)
		responseTime := time.Since(startTime)
		result.ResponseTime = responseTime

		if err != nil {
			lastErr = err
			if attempt < config.MaxRetries {
				logger.Printf("⚠️  [%s] Попытка %d/%d: %v", urlCheck.URL, attempt, config.MaxRetries, err)
				time.Sleep(config.RetryDelay)
				continue
			}
			result.Error = err.Error()
			result.Status = 0
			result.StatusText = "TIMEOUT/ERROR"
			result.IsValid = false
			result.ValidationErrors = []string{err.Error()}
			return result
		}

		defer resp.Body.Close()

		// Читаем заголовки
		for key, values := range resp.Header {
			if len(values) > 0 {
				result.Headers[key] = values[0]
			}
		}

		result.Status = resp.StatusCode
		result.StatusText = resp.Status

		// Валидация статуса
		result.IsValid = validateStatus(result, urlCheck)

		// Валидация заголовков
		if len(urlCheck.RequiredHeaders) > 0 {
			headerErrors := validateHeaders(result, urlCheck.RequiredHeaders)
			if len(headerErrors) > 0 {
				result.ValidationErrors = append(result.ValidationErrors, headerErrors...)
				result.IsValid = false
			}
		}

		// Успешная проверка
		if result.IsValid {
			logger.Printf("✅ [%s] %d %s (%.2fms)", urlCheck.URL, result.Status, result.StatusText, 
				float64(result.ResponseTime.Nanoseconds())/1e6)
			return result
		}

		// Если статус не соответствует ожидаемому, но это не критическая ошибка, продолжаем
		if attempt < config.MaxRetries && shouldRetry(result.Status) {
			logger.Printf("⚠️  [%s] Попытка %d/%d: %d %s", urlCheck.URL, attempt, config.MaxRetries, 
				result.Status, result.StatusText)
			time.Sleep(config.RetryDelay)
			continue
		}

		// Последняя попытка или не нужно повторять
		break
	}

	if lastErr != nil {
		result.Error = lastErr.Error()
	}

	logger.Printf("❌ [%s] %d %s (%.2fms, попыток: %d)", urlCheck.URL, result.Status, result.StatusText,
		float64(result.ResponseTime.Nanoseconds())/1e6, result.Attempts)

	return result
}

func validateStatus(result HTTPCheckResult, urlCheck URLCheck) bool {
	// Если указан ожидаемый статус, проверяем его
	if urlCheck.ExpectedStatus > 0 {
		return result.Status == urlCheck.ExpectedStatus
	}

	// Иначе проверяем по категориям
	status := result.Status

	// Успешные ответы (200-299)
	if status >= 200 && status < 300 {
		return true
	}

	// Редиректы (300-399) - валидны, если follow redirects включен
	if status >= 300 && status < 400 {
		return true
	}

	// Клиентские ошибки (400-499) - не валидны
	if status >= 400 && status < 500 {
		return false
	}

	// Серверные ошибки (500-599) - не валидны
	if status >= 500 && status < 600 {
		return false
	}

	return false
}

func validateHeaders(result HTTPCheckResult, requiredHeaders map[string]string) []string {
	var errors []string
	for key, expectedValue := range requiredHeaders {
		actualValue, exists := result.Headers[key]
		if !exists {
			errors = append(errors, fmt.Sprintf("отсутствует заголовок: %s", key))
		} else if expectedValue != "" && actualValue != expectedValue {
			errors = append(errors, fmt.Sprintf("неверное значение заголовка %s: ожидалось '%s', получено '%s'", 
				key, expectedValue, actualValue))
		}
	}
	return errors
}

func shouldRetry(status int) bool {
	// Повторяем при серверных ошибках и некоторых клиентских
	return status >= 500 || status == 429 || status == 408
}

func generateReport(results []HTTPCheckResult, startTime, endTime time.Time, duration time.Duration) *Report {
	report := &Report{
		StartTime:   startTime,
		EndTime:     endTime,
		Duration:    duration,
		TotalChecks: len(results),
		Results:     results,
		Summary:     ReportSummary{},
	}

	for _, result := range results {
		status := result.Status

		if result.Error != "" || status == 0 {
			report.Summary.Timeouts++
			report.Summary.TotalErrors++
		} else if status >= 200 && status < 300 {
			if result.IsValid {
				report.Summary.Success++
			} else {
				report.Summary.Invalid++
				report.Summary.TotalErrors++
			}
		} else if status >= 300 && status < 400 {
			report.Summary.Redirects++
		} else if status >= 400 && status < 500 {
			report.Summary.ClientErrors++
			report.Summary.TotalErrors++
		} else if status >= 500 && status < 600 {
			report.Summary.ServerErrors++
			report.Summary.TotalErrors++
		}

		if !result.IsValid && result.Error == "" {
			report.Summary.Invalid++
		}
	}

	return report
}

func printReport(report *Report) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("📊 ОТЧЕТ О ПРОВЕРКЕ HTTP СТАТУСОВ")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("⏱️  Время выполнения: %v\n", report.Duration.Round(time.Second))
	fmt.Printf("📈 Всего проверок: %d\n", report.TotalChecks)
	fmt.Println()
	fmt.Println("📊 Сводка:")
	fmt.Printf("   ✅ Успешные (200-299): %d\n", report.Summary.Success)
	fmt.Printf("   🔄 Редиректы (300-399): %d\n", report.Summary.Redirects)
	fmt.Printf("   ⚠️  Клиентские ошибки (400-499): %d\n", report.Summary.ClientErrors)
	fmt.Printf("   🔴 Серверные ошибки (500-599): %d\n", report.Summary.ServerErrors)
	fmt.Printf("   ⏱️  Таймауты/Ошибки: %d\n", report.Summary.Timeouts)
	fmt.Printf("   ❌ Невалидные: %d\n", report.Summary.Invalid)
	fmt.Printf("   📉 Всего ошибок: %d\n", report.Summary.TotalErrors)
	fmt.Println()

	// Показываем проблемные URL
	if report.Summary.TotalErrors > 0 {
		fmt.Println("🔴 Проблемные URL:")
		for _, result := range report.Results {
			if !result.IsValid || result.Error != "" {
				statusInfo := fmt.Sprintf("%d %s", result.Status, result.StatusText)
				if result.Error != "" {
					statusInfo = result.Error
				}
				fmt.Printf("   ❌ %s - %s (%.2fms, попыток: %d)\n", 
					result.URL, statusInfo, 
					float64(result.ResponseTime.Nanoseconds())/1e6, result.Attempts)
				if len(result.ValidationErrors) > 0 {
					for _, err := range result.ValidationErrors {
						fmt.Printf("      ⚠️  %s\n", err)
					}
				}
			}
		}
		fmt.Println()
	}
}

func saveReport(report *Report, filename string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

func checkCriticalErrors(report *Report) []string {
	var errors []string

	if report.Summary.ServerErrors > 0 {
		errors = append(errors, fmt.Sprintf("Обнаружено %d серверных ошибок (500-599)", report.Summary.ServerErrors))
	}

	if report.Summary.Timeouts > 0 {
		errors = append(errors, fmt.Sprintf("Обнаружено %d таймаутов", report.Summary.Timeouts))
	}

	// Проверяем критичные URL
	for _, result := range report.Results {
		if result.Category == "critical" && !result.IsValid {
			errors = append(errors, fmt.Sprintf("Критичный URL недоступен: %s", result.URL))
		}
	}

	return errors
}

